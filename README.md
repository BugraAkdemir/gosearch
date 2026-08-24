# gosearch

[![CI](https://github.com/BugraAkdemir/gosearch/actions/workflows/ci.yml/badge.svg)](https://github.com/BugraAkdemir/gosearch/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/BugraAkdemir/gosearch.svg)](https://pkg.go.dev/github.com/BugraAkdemir/gosearch)
![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

**Web search and page-content extraction for Go — no API key, no SDK, no
account.**

`gosearch` queries real search engines (DuckDuckGo, Bing, Google, Yandex) by
fetching their public HTML result pages and parsing them directly, and
extracts the readable main content of any URL. It is built for local-first
programs — LLM agents, CLI tools, self-hosted services — that cannot or will
not depend on a hosted search API.

```go
results, _ := gosearch.Search(ctx, "golang html parser", gosearch.DuckDuckGo)

page, _ := gosearch.Fetch(ctx, results[0].URL,
    gosearch.WithMarkdown(), // headings, lists, links, code fences preserved
)
fmt.Println(page.Content) // ready to feed an LLM
```

## Contents

- [Why](#why)
- [Features](#features)
- [Quick start](#quick-start)
- [Usage patterns](#usage-patterns)
- [Choosing an engine](#choosing-an-engine)
- [Options](#options)
- [Reliability, honestly](#reliability-honestly)
- [Explicit non-goals](#explicit-non-goals)
- [Real-browser rendering (`gosearch/browser`)](#real-browser-rendering-gosearchbrowser)
- [Versioning & stability](#versioning--stability)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Why

Most "web search for my agent/tool" solutions assume you'll pay for and depend
on a hosted search API (SerpAPI, Google Custom Search, Bing Search API…).
That is a real dependency: an account, a key, a bill, a third party on your
critical path. Projects built to run fully locally need none of those things
— so `gosearch` talks to the engines' own public result pages directly, the
same way a web browser would, and parses what comes back.

The core module's entire third-party dependency surface is
[`golang.org/x/net/html`](https://pkg.go.dev/golang.org/x/net/html). That is
a deliberate architectural bet: you can drop this into almost any Go program
without dragging in an SDK.

## Features

- **Four engines, one interface** — DuckDuckGo, Bing, Google, Yandex through
  a single `Search(ctx, query, engine, ...Option)` call returning structured
  `[]Result{Title, URL, Snippet}`.
- **Readable content extraction** — `Fetch(ctx, url, ...Option)` returns a
  page's main content with navigation, ads, scripts, and boilerplate removed
  — never raw HTML.
- **Markdown output** — `WithMarkdown()` renders extracted content as
  GitHub-flavored Markdown: headings, lists, tables, fenced code, links, and
  emphasis survive. Built for LLM context, where structure carries meaning.
- **Ordered fallback chain** — `WithFallback(...)` moves to the next engine
  only when the current one reports being blocked or challenged, instead of
  failing outright.
- **Honest failure signaling** — anti-bot interventions surface as typed
  sentinel errors (`ErrBlocked`, `ErrChallenge`) via `errors.Is`, never as a
  silent empty result or a parse panic on a captcha page.
- **Near-duplicate collapsing** — engines often list one page under several
  URL spellings (percent-encoded vs literal `İ`/`I`, reordered parameters);
  these collapse into one result, original spelling preserved.
- **Opt-in freshness dates** — `WithDates()` fills `Result.Date` from each
  engine's own metadata when provided; off by default so timeless use cases
  are never polluted by accident.
- **Caller-side domain policy** — `WithBlockedDomains(...)` /
  `WithAllowedDomains(...)` enforce your spam and quality rules on the
  result list. The library judges no site on its own.
- **Resilience built in** — transient-failure retries with exponential
  backoff, realistic browser headers, persistent cookie jar, per-host rate
  limiting.

## Quick start

Requirements: **Go 1.25+**. No keys, no accounts, no config files.

```bash
go get github.com/BugraAkdemir/gosearch@latest
```

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

	// Search (DuckDuckGo is the most reliable default).
	results, err := gosearch.Search(ctx, "facebook", gosearch.DuckDuckGo,
		gosearch.WithMaxResults(5),
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Title, "->", r.URL)
	}

	// Extract any page's readable content.
	page, err := gosearch.Fetch(ctx, "https://en.wikipedia.org/wiki/Facebook")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(page.Title)
	fmt.Println(page.Content)
}
```

## Usage patterns

### Resilient searches with fallback

If the primary engine blocks or challenges the request, the same query
continues down the chain automatically. First success wins.

```go
results, err := gosearch.Search(ctx, "facebook", gosearch.Google,
	gosearch.WithFallback(gosearch.Bing, gosearch.DuckDuckGo),
)
```

### Page content as Markdown (LLM-ready)

```go
page, err := gosearch.Fetch(ctx, url, gosearch.WithMarkdown())
// page.Content now contains "# Heading", "- list items", "[links](…)", "```code```"
```

### The LLM-agent pipeline

The two calls compose into the standard retrieval flow:

```go
// 1. Find candidate sources.
results, err := gosearch.Search(ctx, question, gosearch.DuckDuckGo,
	gosearch.WithFallback(gosearch.Bing),
	gosearch.WithMaxResults(5),
	gosearch.WithBlockedDomains("pinterest.com"), // your quality policy
)

// 2. Read the top hits as Markdown context.
var context strings.Builder
for _, r := range results[:min(3, len(results))] {
	page, err := gosearch.Fetch(ctx, r.URL, gosearch.WithMarkdown())
	if err != nil {
		continue // one unreadable source ≠ abort
	}
	fmt.Fprintf(&context, "## %s\n%s\n\n", page.Title, page.Content)
}
// context.String() → model input
```

### Freshness-aware results

```go
results, _ := gosearch.Search(ctx, q, gosearch.Bing,
	gosearch.WithDates(), // Result.Date: "1 day ago", "2026-08-20", …
)
```

Full option semantics: [`docs/API.md`](./docs/API.md). Task-oriented
copy-paste recipes: [`docs/RECIPES.md`](./docs/RECIPES.md).

## Choosing an engine

Anti-bot strictness differs per engine **and per network** — IP reputation is
often the deciding factor, not client behavior. Rough expectations from a
normal residential connection, based on live testing of this library:

| Engine | Expected reliability | Notes |
|--------|----------------------|-------|
| DuckDuckGo | Highest | Official no-JS HTML endpoint; parser validated against a real captured page. |
| Bing | High | Served clean organic results even to a flagged datacenter IP; titles arrive behind a click-tracker unwrapped best-effort. |
| Google | Moderate | No official no-JS endpoint; DOM changes without notice. |
| Yandex | Lowest | Aggressive geo/IP-based captcha gating, especially outside Russia. |

"Best-effort heuristic" status: the DuckDuckGo parser is validated against a
real captured response; the other three parsers match current observed markup
and are tested against synthetic fixtures, but engines rotate their DOM
without notice — treat a parse miss as a signal to capture fresh HTML, not
necessarily a bug.

## Options

One variadic `...Option` applies to both `Search` and `Fetch`; scope notes
below. Full table with semantics: [`docs/API.md`](./docs/API.md).

| Option | Scope | Purpose |
|---|---|---|
| `WithTimeout(d)` | both | Request deadline (default 15s) |
| `WithMaxResults(n)` | Search | Cap result count |
| `WithFallback(engines...)` | Search | Ordered block/challenge fallback |
| `WithDates()` | Search | Populate `Result.Date` (default off) |
| `WithBlockedDomains(ds...)` / `WithAllowedDomains(ds...)` | Search | Your domain policy |
| `WithRetries(n)` | both | Transient-failure retries (default 2, `0` disables) |
| `WithProxy(rawURL)` | both | Route through your own egress |
| `WithCookies(cs...)` | both | Seed session cookies |
| `WithHeader(k, v)` / `WithUserAgent(ua)` | both | Override request headers |
| `WithHTTPClient(c)` | both | Bring your own `*http.Client` (escape hatch) |
| `WithMarkdown()` | Fetch | Markdown instead of plain-text content |

## Reliability, honestly

Search engines run anti-bot systems. A `200 OK` does not mean success —
engines serve captcha/challenge pages with status 200, which is why every
response passes through block detection before parsing, and why failures are
typed errors rather than empty slices. From datacenter/cloud IPs every engine
may challenge or block you regardless of politeness; residential networks
fare far better. When every engine in a fallback chain refuses you, `Search`
returns an `errors.Join` of each engine's error so you can see exactly what
happened.

## Explicit non-goals

This library behaves like an ordinary visitor — and stops there. It will
**never** solve CAPTCHAs, execute JS challenges to disguise itself, mask
automation flags, or rotate identities to defeat a security control. Those
cross from "look like a normal visitor" into "defeat a security control,"
which is out of scope on principle, not merely as a technical limitation.

## Real-browser rendering (`gosearch/browser`)

For pages whose content only exists after JavaScript runs — which defeats
plain-HTTP `Fetch()` by definition — an opt-in **separate Go module** drives
a real, unmodified Chromium-family browser:

- Discovers Chrome/Edge/Chromium on the system; optionally downloads Google's
  official `chrome-headless-shell` with explicit permission; or embeds the
  engine at compile time via `-tags gosearch_embed_engine`.
- One long-lived process and one reused tab: steady-state cost is about one
  page's RAM, not a browser per request.
- Same honest line: it executes JavaScript; it does not solve CAPTCHAs or
  mask automation.

Because it lives in its own module, `go get github.com/BugraAkdemir/gosearch`
never pulls browser dependencies into your project. See
[`browser/README.md`](./browser/README.md) for installation and trade-offs.

## Versioning & stability

Releases follow [semantic versioning](https://semver.org/) as annotated
`vX.Y.Z` tags; the module proxy distributes them automatically. During
`v0.x`, the public API may still evolve between minor versions — pin an exact
version if that matters to you. The `browser/` directory is a separate Go
module and will carry its own `browser/vX.Y.Z` tags when published.

## Documentation

| Document | Contents |
|---|---|
| [`docs/GETTING_STARTED.md`](./docs/GETTING_STARTED.md) | From zero to first search and first extraction |
| [`docs/RECIPES.md`](./docs/RECIPES.md) | Copy-paste solutions: fallback chains, error handling, LLM pipelines, batch searching, the browser engine, troubleshooting |
| [`docs/API.md`](./docs/API.md) | Human-readable reference for every function, type, option, and error (`go doc -all .` remains the source of truth) |
| [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) | Package graph, request flow, provider status, and the reasoning behind the `internal/` split |

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md). Development conventions,
verification commands, and known pitfalls live in
[`AGENTS.md`](./AGENTS.md); the phased roadmap and exit criteria live in
[`plan.md`](./plan.md).

## License

[MIT](./LICENSE)
