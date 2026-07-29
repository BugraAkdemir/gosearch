# gosearch

**Web search and page-content extraction for Go — no API key, no SDK, no
account to sign up for.**

`gosearch` sends direct HTTP requests to public search engine result pages
(Google, Yandex, DuckDuckGo) and parses the HTML itself, so a local-first Go
program can search the web without paying for or depending on a hosted
search API. It also ships a `Fetch()` function that pulls the clean,
readable content out of any URL — title and main body text, with
navigation/ads/footers stripped — for feeding page content into something
like an LLM agent's context.

> **Status: pre-implementation.** The architecture, public API, and roadmap
> below are the agreed design — see [`plan.md`](./plan.md) for the phased
> build-out and current progress. Nothing in the "Usage" section is
> functional yet; it documents the API this library is being built to.

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

- **`Search(ctx, query, engine, ...opts)`** — queries Google, Yandex, or
  DuckDuckGo and returns parsed `[]Result{Title, URL, Snippet}`.
- **`Fetch(ctx, url, ...opts)`** — fetches any URL and extracts its main
  readable content (a simplified readability-style algorithm), not a raw
  HTML dump.
- **Engine fallback** — `WithFallback(engine, engine, ...)` tries engines in
  the order you specify and moves to the next one if the current one reports
  it's blocked, instead of failing outright.
- **Honest failure signaling** — if an engine's anti-bot system blocks the
  request, you get a typed `ErrBlocked`/`ErrChallenge` error, not a silent
  empty result or a parse panic on a captcha page.

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
provider (`Google`, `Yandex`, `DuckDuckGo`).

## Usage (planned API)

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

	// Search, falling back to DuckDuckGo then Yandex if Google is blocked.
	results, err := gosearch.Search(ctx, "facebook", gosearch.Google,
		gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Title, r.URL, r.Snippet)
	}

	// Fetch and read a page's actual content, not its raw HTML.
	page, err := gosearch.Fetch(ctx, "https://facebook.com")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(page.Title)
	fmt.Println(page.Content)
}
```

## Reliability, honestly

Anti-bot strictness differs a lot per engine and per network. Live testing
during this project's design phase (from a datacenter/cloud IP) got a
captcha or JS-challenge from all three engines on the very first request —
most likely IP-reputation driven, not something a well-behaved client can
fully avoid. Expected reliability, roughly, from a normal residential
network:

| Engine | Expected reliability | Why |
|--------|----------------------|-----|
| DuckDuckGo | Highest | Only engine with an official no-JS HTML endpoint; least aggressive captcha threshold. |
| Google | Moderate | No official no-JS endpoint; DOM is regionally A/B tested and changes without notice. |
| Yandex | Lowest | Very aggressive geo/IP-based captcha gating, especially outside Russia. |

This isn't sold as "always works" — see [`plan.md`](./plan.md) for exactly
how each provider's block-detection and fallback behavior is meant to work,
and `AGENTS.md`'s Known Pitfalls log for what's actually been verified
against a live engine response versus what's still best-effort.

## Optional: real-browser rendering (`gosearch/browser`)

For pages that require JS to render at all (which defeats the plain-HTTP
`Fetch()`), a separate, opt-in subpackage will drive a real, unmodified
headless browser — never a dependency of the core module, so `go get
github.com/BugraAkdemir/gosearch` stays exactly as light as described above.
See Phase 5 in [`plan.md`](./plan.md) for the full design, including the
runtime-download-with-persistent-cache model and the opt-in
`embedbrowser` build tag for developers who'd rather trade binary size for
zero runtime network dependency.

## Roadmap

Full phased build-out, exit criteria, and design rationale live in
[`plan.md`](./plan.md). Short version:

1. Skeleton + DuckDuckGo provider + `Fetch()`
2. Google provider
3. Yandex provider
4. Fallback/retry hardening
5. Optional real-browser rendering subpackage

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) and [`AGENTS.md`](./AGENTS.md) —
the latter is written for both human contributors and coding agents working
across sessions, and is the source of truth for build/test/verification
commands.

## License

[MIT](./LICENSE).
