# Recipes

Task-oriented, copy-paste solutions. Each snippet is a complete program or
function; imports are elided after the first recipe. For exhaustive option
semantics see [API.md](./API.md); for a from-zero walkthrough see
[GETTING_STARTED.md](./GETTING_STARTED.md).

- [Search with an automatic fallback chain](#search-with-an-automatic-fallback-chain)
- [Handle every error class correctly](#handle-every-error-class-correctly)
- [Read a page's actual content](#read-a-pages-actual-content)
- [Get page content as Markdown](#get-page-content-as-markdown)
- [Build an LLM retrieval pipeline](#build-an-llm-retrieval-pipeline)
- [Work with result freshness dates](#work-with-result-freshness-dates)
- [Enforce your own domain policy](#enforce-your-own-domain-policy)
- [Tune retries for flaky networks](#tune-retries-for-flaky-networks)
- [Route through your own proxy](#route-through-your-own-proxy)
- [Reuse your browser's session (cookies)](#reuse-your-browsers-session-cookies)
- [Bring your own HTTP client](#bring-your-own-http-client)
- [Batch many queries politely](#batch-many-queries-politely)
- [When plain HTTP is not enough: the browser engine](#when-plain-http-is-not-enough-the-browser-engine)
- [Troubleshooting](#troubleshooting)

---

## Search with an automatic fallback chain

Goal: results even when your primary engine distrusts you today.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/BugraAkdemir/gosearch"
)

func main() {
	ctx := context.Background()

	results, err := gosearch.Search(ctx, "facebook", gosearch.Google,
		gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
		gosearch.WithMaxResults(10),
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Title, "->", r.URL)
	}
	_ = errors.Is // used in the next recipe
}
```

Semantics that matter:

- Engines are tried **in exactly the order you list them**: primary first,
  then each fallback.
- A fallback engine is tried **only** on `ErrBlocked` / `ErrChallenge`.
- `ErrNoResults` (a clean, empty page) is returned as-is — it is a valid
  answer, and another engine will not conjure results for a query that has
  none.
- The first success wins; later engines are never contacted.

## Handle every error class correctly

```go
results, err := gosearch.Search(ctx, query, gosearch.DuckDuckGo,
	gosearch.WithFallback(gosearch.Google),
)
switch {
case err == nil:
	// use results
case errors.Is(err, gosearch.ErrNoResults):
	fmt.Println("no results — valid answer, not a failure")
case errors.Is(err, gosearch.ErrBlocked):
	fmt.Println("blocked (429 / IP reputation). Try later, another network, or WithFallback")
case errors.Is(err, gosearch.ErrChallenge):
	fmt.Println("engine demanded a CAPTCHA/JS challenge")
case errors.Is(err, gosearch.ErrUnsupportedEngine):
	log.Fatal("programming error: bad Engine constant")
default:
	log.Fatal(err) // network failure, timeout, ...
}
```

Why `errors.Is` and never string matching: provider errors carry context via
`%w` wrapping, and a fully-blocked chain returns `errors.Join` of every
engine's error — `errors.Is` is the only check that survives both. Through
the join you can still ask which *kinds* of failures happened:

```go
if errors.Is(err, gosearch.ErrBlocked) && errors.Is(err, gosearch.ErrChallenge) {
	// at least one engine hard-blocked us AND at least one challenged us
}
```

## Read a page's actual content

```go
page, err := gosearch.Fetch(ctx, "https://en.wikipedia.org/wiki/Facebook",
	gosearch.WithTimeout(30*time.Second),
)
if err != nil {
	log.Fatal(err)
}
fmt.Println(page.URL)     // final URL after redirects
fmt.Println(page.Title)   // best-guess title
fmt.Println(page.Content) // extracted main text (~nav/ads/footer stripped)
```

Notes:

- `Content` is plain text with boilerplate stripped; it can legitimately be
  empty for pages without a detectable main region.
- JavaScript is not executed. Client-side-rendered pages yield empty content
  → use the browser engine (last recipe).
- Anti-bot responses surface as `ErrBlocked`/`ErrChallenge` here too, not as
  raw HTML in your `Page`.

## Get page content as Markdown

Plain text flattens structure; Markdown keeps it. For any consumer that
understands formatting — above all LLMs — opt in:

```go
page, err := gosearch.Fetch(ctx, url, gosearch.WithMarkdown())
if err != nil {
	log.Fatal(err)
}
fmt.Println(page.Content)
// # Heading
// prose with an [inline link](https://…), **bold**, *italics*, `code`
// - list item
// ```
// fenced code block
// ```
```

Without `WithMarkdown()` the output stays byte-for-byte the historical plain
text, so existing callers are unaffected. Deliberate simplifications:
nested inline formatting flattens (a link inside bold renders its inner
text), `<br>` becomes a space, blockquotes collapse to one line, and table
cells carry inline text only.

## Build an LLM retrieval pipeline

The standard agent flow: search for candidates, read the top hits, hand the
combined Markdown to the model.

```go
func retrieve(ctx context.Context, question string) (string, error) {
	results, err := gosearch.Search(ctx, question, gosearch.DuckDuckGo,
		gosearch.WithFallback(gosearch.Bing),
		gosearch.WithMaxResults(5),
		gosearch.WithDates(), // lets you prefer fresh sources
		gosearch.WithBlockedDomains("pinterest.com"),
	)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	n := 3
	if len(results) < n {
		n = len(results)
	}
	for _, r := range results[:n] {
		page, err := gosearch.Fetch(ctx, r.URL, gosearch.WithMarkdown())
		if err != nil {
			continue // one unreadable source must not sink the answer
		}
		fmt.Fprintf(&sb, "# %s\n(source: %s)\n\n%s\n\n", page.Title, r.URL, page.Content)
	}
	return sb.String(), nil // feed to your model as context
}
```

Design notes: `WithDates()` is off by default so callers doing historical or
snapshot work never see date metadata by accident; filtering everything away
with domain policy yields an empty-but-valid result set, not an error.

## Work with result freshness dates

```go
results, err := gosearch.Search(ctx, q, gosearch.Bing, gosearch.WithDates())
for _, r := range results {
	fmt.Printf("%s (%s)\n  %s\n", r.Title, r.Date, r.URL)
}
```

- `Date` carries the engine's own stamp verbatim — `"2026-08-20"` on some
  engines, human relative text like `"1 day ago"` on Bing. There is no
  cross-engine normalization: the value is exactly what the page said.
- Engines frequently omit dates on their no-JavaScript pages; empty `Date`
  is normal even with `WithDates()`, not a failure.
- Enabling it does not change what the engine is asked or returns — it only
  surfaces metadata already present.

## Enforce your own domain policy

SEO-spam and AI-slop are ranking problems the library cannot judge — but you
can filter by host after the fact:

```go
results, err := gosearch.Search(ctx, q, gosearch.DuckDuckGo,
	gosearch.WithBlockedDomains("contentfarm.example"),   // deny first…
	gosearch.WithAllowedDomains("wikipedia.org", "gov.tr"), // …then allowlist
)
```

- Matching is host-or-subdomain: blocking `spam.example.net` also kills
  `www.spam.example.net` but spares `notspam.example.net`.
- Deny is applied before allow when both lists are set.
- Filtering everything away returns an empty slice — a valid answer that
  does **not** trigger fallback (fallback exists for blocks/challenges).

## Tune retries for flaky networks

Transient problems — connection resets, timeouts, HTTP 408/500/502/503/504 —
are retried automatically with exponential backoff (default: 2 retries).
Blocks and challenges are **never** retried; retrying an engine that already
flagged your IP only digs the hole deeper. Fallback exists for that.

```go
// more patience on a flaky mobile hotspot:
results, err := gosearch.Search(ctx, q, gosearch.DuckDuckGo,
	gosearch.WithRetries(4),
)

// deterministic latency matters more than resilience (e.g. inside tests):
results, err = gosearch.Search(ctx, q, gosearch.DuckDuckGo,
	gosearch.WithRetries(0), // disabled entirely
)
```

## Route through your own proxy

```go
results, err := gosearch.Search(ctx, q, gosearch.DuckDuckGo,
	gosearch.WithProxy("socks5://127.0.0.1:9050"),
)
```

Any scheme Go's transport understands works (`http://`, `https://`,
`socks5://`). This is legitimate egress control — using *your* network exit —
not identity rotation to defeat anti-bot systems, which the library refuses
to do by design.

## Reuse your browser's session (cookies)

If an engine keeps challenging you but your normal browser browses fine, hand
the library the session cookies your browser earned:

```go
ck := &http.Cookie{Name: "SESSION_ID", Value: "...", Domain: ".duckduckgo.com"}
results, err := gosearch.Search(ctx, q, gosearch.DuckDuckGo,
	gosearch.WithCookies(ck),
)
```

Export cookies from your browser's DevTools (Application → Cookies). The jar
persists across the whole call, including every fallback engine.

## Bring your own HTTP client

Full control when you need custom TLS settings, tracing, or middleware:

```go
client := &http.Client{
	Timeout:   20 * time.Second,
	Transport: myInstrumentedTransport,
	Jar:       myCookieJar,
}

results, err := gosearch.Search(ctx, q, gosearch.DuckDuckGo,
	gosearch.WithHTTPClient(client),
)
```

Caveat: this bypasses the library's default headers, cookie seeding, and rate
limiting — `WithTimeout`/`WithProxy`/`WithHeader` are ignored alongside it.
You own the request shape entirely.

## Batch many queries politely

One client per program run; let the library's built-in per-host rate limiter
and backoff do their job:

```go
queries := []string{"go generics", "golang context", "go workspaces"}

for _, q := range queries {
	results, err := gosearch.Search(ctx, q, gosearch.DuckDuckGo,
		gosearch.WithFallback(gosearch.Google, gosearch.Yandex),
		gosearch.WithMaxResults(5),
	)
	if err != nil {
		log.Printf("%q: %v", q, err) // keep going; one bad query ≠ abort
		continue
	}
	process(q, results)
}
```

Every `Search` call reuses realistic headers and self-imposed pacing. Do not
add your own tight retry loops around `Search` — between `WithRetries` (for
transient errors) and `WithFallback` (for blocks) the library already covers
the cases where trying again is meaningful.

## When plain HTTP is not enough: the browser engine

Two situations need a real browser:

1. A search endpoint answers non-JS clients with a JS challenge
   (Google's `enablejs` wall — what `ErrChallenge` usually means there).
2. A page you want to `Fetch` renders its content only via JavaScript
   (empty `Page.Content` from the plain-HTTP path).

The browser lives in a separate module so the core stays dependency-light:

```bash
go get github.com/BugraAkdemir/gosearch/browser
```

```go
e, err := browser.New(ctx) // finds Chrome/Edge/Chromium on the system
if err != nil {
	log.Fatal(err)
}

// Search over rendered DOM — clears JS-gated walls:
results, err := e.Search(ctx, "facebook")
_ = e.Close() // close explicitly before any log.Fatal/os.Exit path
if err != nil {
	log.Fatal(err)
}

// Fetch a JS-rendered page — drop-in same return type as gosearch.Fetch:
page, err := e.Fetch(ctx, "https://example.com/")
```

What to know:

| Aspect | Detail |
|---|---|
| Executable source | System Chrome/Edge/Chromium discovery → opt-in download (`AllowDownload(true)`) → embedded archive (`-tags gosearch_embed_engine`, see the module README) |
| Memory | One long-lived process + one reused tab; ~one page's worth steady-state. Keep the `Engine` alive for your program's lifetime instead of creating one per query |
| Deployment | `browser.Install(ctx, browser.AllowDownload(true))` pre-warms during provisioning; `browser.WithCacheDir(path)` for read-only default cache locations |
| Detection honesty | The browser is unmodified: no stealth patches, no webdriver masking. It clears JS-gated pages; it does **not** beat IP-reputation blocks or CAPTCHAs |

Full details: [`browser/README.md`](../browser/README.md).

## Troubleshooting

| Symptom | Meaning | What to do |
|---|---|---|
| `errors.Is(err, ErrChallenge)` on Google, always | You hit the enablejs/JS wall — typical from datacenter IPs | Use `WithFallback(DuckDuckGo)`; or the browser engine; or search from a better-reputed network |
| `errors.Is(err, ErrChallenge)` on DuckDuckGo | Its anomaly/captcha page (arrives with HTTP 200!) | Slow down; try later; consider `WithCookies` from a real session |
| `errors.Is(err, ErrBlocked)` | Hard block (429/IP reputation) | Stop hammering. Back off, change egress legitimately (your own proxy/home network), rely on fallback |
| `errors.Is(err, ErrNoResults)` | Genuinely zero matches — success | Don't retry other engines; refine the query |
| `Fetch` returns empty `Content` | Page renders via JavaScript | Use the browser engine's `Fetch` |
| Every engine returns blocked/challenge at once | Your egress IP has bad reputation everywhere | That is the joined error telling the truth: fix the network situation; no client trick will help |

Two rules that prevent most confusion:

1. **Check errors with `errors.Is`, never by text.**
2. **A 200 status does not mean success** — engines serve challenge pages
   with status 200. The library detects this for you; treat the sentinels as
   the truth, not the absence of a transport error.
