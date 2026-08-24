# gosearch

**Web search and page-content extraction for Go — no API key, no SDK, no
account to sign up for.**

`gosearch` sends direct HTTP requests to public search engine result pages
(Google, Yandex, Bing, DuckDuckGo) and parses the HTML itself, so a
local-first Go program can search the web without paying for or depending on
a hosted search API. It also ships a `Fetch()` function that pulls the clean,
readable content out of any URL — title and main body text, with
navigation/ads/footers stripped — for feeding page content into something
like an LLM agent's context.

> **Status: v0.1.** All four providers (`DuckDuckGo`, `Google`, `Yandex`,
> `Bing`) plus `Fetch()` are implemented and hardened (transient-failure
> retries, ordered fallback chain). DuckDuckGo is validated against a real
> captured page; the Google, Yandex, and Bing parsers are documented
> best-effort heuristics pending real-capture validation — see
> [`plan.md`](./plan.md) and `AGENTS.md`'s Known Pitfalls.

---

## Why

Most "web search for my agent/tool" solutions assume you'll pay for and
depend on a hosted search API (SerpAPI, Google Custom Search, Bing Search
API, etc.). That's a real dependency — an account, a key, a bill, a network
call to a third party — which doesn't fit projects built to run fully
locally with zero external services. `gosearch` exists for that case: it
talks to the search engines' own public result pages directly, the same way
a web browser would.

## What it does

- **`Search(ctx, query, engine, ...Option)`** — queries Google, Yandex,
  Bing, or DuckDuckGo and returns parsed `[]Result{Title, URL, Snippet}`.
- **`Fetch(ctx, url, ...Option)`** — fetches any URL and extracts its main
  readable content (a simplified readability-style algorithm) into a
  `Page{URL, Title, Content}`, not a raw HTML dump. Pass `WithMarkdown()` to
  get the content as Markdown instead of plain text — headings, lists, code
  fences, links, and emphasis preserved (ideal for LLM context).
- **Engine fallback** — `WithFallback(engine, engine, ...)` tries engines in
  the order you specify and moves to the next one if the current one reports
  it's blocked, instead of failing outright.
- **One `Option` type** — `WithTimeout`, `WithProxy`, `WithHeader`,
  `WithCookies`, `WithUserAgent`, `WithHTTPClient`, and `WithMarkdown` work on
  both `Search` and `Fetch` where meaningful (`WithMarkdown` is Fetch-only);
  `WithFallback` and `WithMaxResults` are search-only and are ignored by
  `Fetch`.
- **Honest failure signaling** — if an engine's anti-bot system blocks the
  request, you get a typed `ErrBlocked`/`ErrChallenge` error, not a silent
  empty result or a parse panic on a captcha page.
- **Near-duplicate collapsing** — engines often list one page under several
  URL spellings (percent-encoded vs literal `İ`/`I`, reordered parameters);
  those collapse to a single result with the original spelling preserved.

## What it deliberately does not do

Search engines run anti-bot systems (image captchas, JS-execution
challenges, IP-reputation blocks). `gosearch` behaves like a normal,
honest visitor — realistic browser headers, a persistent cookie jar,
self-imposed rate limiting, and the lowest-friction endpoint each engine
offers — but it will **never**:

- automatically solve a CAPTCHA,
- execute a JS anti-bot challenge to disguise itself as something it isn't,
- or rotate proxies/spoof fingerprints to hide that a request is automated.

Those cross from "look like a normal visitor" into "defeat a security
control," which is out of scope for this project on principle, not just as
a technical limitation.

## Install

```bash
go get github.com/BugraAkdemir/gosearch
```

No API key, no config file, no signup required to use the plain-HTTP
provider (`Google`, `Yandex`, `Bing`, `DuckDuckGo`).

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/BugraAkdemir/gosearch"
)

func main() {
	ctx := context.Background()

	// Search DuckDuckGo (most reliable) or Google (best-effort — see below).
	results, err := gosearch.Search(ctx, "facebook", gosearch.DuckDuckGo)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Title, r.URL, r.Snippet)
	}

	// Fetch and read a page's actual content, not its raw HTML.
	page, err := gosearch.Fetch(ctx, "https://en.wikipedia.org/wiki/Facebook")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(page.Title)
	fmt.Println(page.Content)
}
```

With `WithFallback` a block/challenge on the primary engine automatically
tries the next one:

```go
// Try Google first; fall back to DuckDuckGo, then Yandex, on a block/challenge.
results, err := gosearch.Search(ctx, "facebook", gosearch.Google,
	gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
)
```

## Reliability, honestly

Anti-bot strictness differs a lot per engine and per network. Live testing
during this project's design phase (from a datacenter/cloud IP) got a
captcha or JS-challenge from all three engines probed at the time on the very
first request — most likely IP-reputation driven, not something a
well-behaved client can fully avoid. Expected reliability, roughly, from a
normal residential network:

| Engine | Expected reliability | Why |
|--------|----------------------|-----|
| DuckDuckGo | Highest | Only engine with an official no-JS HTML endpoint; least aggressive captcha threshold. |
| Bing | High | Served clean organic results even to a flagged datacenter IP; titles arrive behind a click-tracker that is unwrapped best-effort. |
| Google | Moderate | No official no-JS endpoint; DOM is regionally A/B tested and changes without notice. |
| Yandex | Lowest | Very aggressive geo/IP-based captcha gating, especially outside Russia. |

### Tuning and escape hatches

- **Retries:** transient failures (network errors, HTTP 408/5xx) are retried
  with exponential backoff — default 2 retries, `gosearch.WithRetries(n)` to
  change it, `WithRetries(0)` to disable. Blocks and challenges are never
  retried; that is what fallback is for.
- **Proxy / egress:** `gosearch.WithProxy("socks5://host:port")` routes every
  request through your own proxy.
- **Session reuse:** `WithCookies(...)` seeds cookies (for example exported
  from your own browser) so a request looks like a returning visitor.
- **Full control:** `WithHTTPClient(client)` uses your own `*http.Client`
  as-is; you then own headers, jar, transport, and rate limiting.

### Explicit non-goals

This library will **never** solve CAPTCHAs, execute JS challenges, mask
`navigator.webdriver`, rotate proxies to hide identity, or otherwise defeat
a security control. It behaves like an ordinary visitor and reports honestly
when an engine blocks it — anything beyond that is out of scope by design.

## Optional: real-browser rendering (`gosearch/browser`)

For pages that require JS to render at all (which defeats the plain-HTTP
`Fetch()`), an opt-in **separate module** drives a real, unmodified headless
Chromium-family browser — never a dependency of the core, so `go get
github.com/BugraAkdemir/gosearch` stays exactly as light as described above.

```bash
go get github.com/BugraAkdemir/gosearch/browser
```

- Discovers Chrome/Edge/Chromium on the system; can optionally download
  Google's official `chrome-headless-shell` with explicit permission, or be
  compiled with the engine embedded via `-tags gosearch_embed_engine`.
- One long-lived instance + one reused tab: steady-state cost ≈ one page's
  RAM, not a browser per request.
- Same honest line as the rest of the project: it executes JavaScript but
  never solves CAPTCHAs or masks automation. See
  [`browser/README.md`](./browser/README.md) for trade-offs and limits.

This isn't sold as "always works" — see [`plan.md`](./plan.md) for exactly
how each provider's block-detection and fallback behavior is meant to work,
and `AGENTS.md`'s Known Pitfalls log for what's actually been verified
against a live engine response versus what's still best-effort.


## Roadmap

Full phased build-out, exit criteria, and design rationale live in
[`plan.md`](./plan.md). Short version:

1. Skeleton + DuckDuckGo provider + `Fetch()`
2. Google provider
3. Yandex provider
4. Fallback/retry hardening
5. Optional real-browser rendering subpackage
6. Bing provider (post-v0.1 addition)

## Documentation

- [`docs/GETTING_STARTED.md`](./docs/GETTING_STARTED.md) — from zero to first
  search and first extracted page: install, quickstart, engine primer.
- [`docs/RECIPES.md`](./docs/RECIPES.md) — task-oriented, copy-paste recipes:
  fallback chains, error-handling patterns, retries/proxy/cookies/custom
  client, batch searching, the browser engine, troubleshooting.
- [`docs/API.md`](./docs/API.md) — human-readable API reference (`go doc -all .` is the source of truth; this is a companion, not a replacement).
- [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) — package graph, request flow, and the reasoning behind the `internal/` split.

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) and [`AGENTS.md`](./AGENTS.md) —
the latter is written for both human contributors and coding agents working
across sessions, and is the source of truth for build/test/verification
commands.

## License

[MIT](./LICENSE).
