# gosearch/browser — optional real-browser engine

Drives an **unmodified** Chromium-family browser over CDP (via
[`chromedp`](https://github.com/chromedp/chromedp)) to run searches and
extract page content where plain HTTP cannot: pages that only render behind
JavaScript, and search endpoints that answer non-JS clients with a JS
challenge.

This is a **separate Go module**. The core `gosearch` module does not import
it, and installing the core never pulls this in:

```bash
go get github.com/BugraAkdemir/gosearch            # core only (zero extra deps)
```

Installing the browser module:

```bash
go get github.com/BugraAkdemir/gosearch/browser@latest
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	browser "github.com/BugraAkdemir/gosearch/browser"
)

func main() {
	ctx := context.Background()

	e, err := browser.New(ctx) // discovers Chrome/Edge/Chromium on the system
	if err != nil {
		log.Fatal(err)
	}
	defer e.Close()

	results, err := e.Search(ctx, "facebook") // rendered DOM, post-JS
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Title, r.URL)
	}
	page, err := e.Fetch(ctx, "https://example.com/") // same shape as gosearch.Fetch
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(page.Title, len(page.Content))
}
```

One `Engine` = one long-lived browser process. By default, calls serialize on
a single reused tab so steady-state memory stays at roughly one page's worth
(~100–250 MB idle + page), not a fresh browser per request; pass
`browser.WithMaxConcurrency(n)` to run up to `n` isolated tabs concurrently
inside the same process. `Engine` is safe for concurrent use, honors caller
`ctx` cancellation promptly, and transparently restarts Chromium (one-shot
recovery) if the underlying browser process exits unexpectedly. Images are
disabled and the GPU process is off by default to keep consumption down.

`Engine.Fetch` delegates extraction to the core module's `internal/readability`
package (capped at 8 MiB on a UTF-8/tag-safe boundary), so plain-text and
`browser.WithMarkdown()` outputs match `gosearch.Fetch` identically.

### Hybrid HTTP → Browser Fallback

When most target pages are static and only a few require JavaScript or hit
anti-bot walls, use `FetchWithFallback` / `SearchWithFallback`:

```go
// Tries fast plain-HTTP gosearch.Fetch first (never launches Chromium on
// rich static pages); falls back to e.Fetch on ErrBlocked/ErrChallenge or
// thin/empty JS shells (< 100 runes by default; tune with WithFallbackMinContent).
page, err := e.FetchWithFallback(ctx, "https://example.com/spa")

// Tries plain-HTTP gosearch.Search first; falls back to e.Search ONLY on
// ErrBlocked/ErrChallenge (never on ErrNoResults).
results, err := e.SearchWithFallback(ctx, "golang", gosearch.DuckDuckGo)
```

### Options Reference

| Option | Purpose |
|---|---|
| `WithMarkdown()` | Return GitHub-flavored Markdown from `Fetch` / `FetchWithFallback` |
| `WithTimeout(d)` | Override per-call timeout (defaults: 30s Search, 45s Fetch) |
| `WithProxy(rawURL)` | Route Chromium traffic via `--proxy-server` (`http`/`https`/`socks5`) |
| `WithMaxResults(n)` | Cap `Search` results (default 30) |
| `WithAllowedDomains(...)` / `WithBlockedDomains(...)` | Post-search suffix-host domain filter (deny first, then allow) |
| `WithMaxConcurrency(n)` | Concurrent tabs per `Engine` (default 1 serialized tab) |
| `WithWaitSelector(css)` | Wait for a CSS selector in `Fetch` for async SPAs (React/Vue/Next) |
| `WithWaitDelay(d)` | Context-aware settle delay in `Fetch` after DOM/selector readiness |
| `WithFallbackMinContent(n)` | Rune threshold for `FetchWithFallback` JS-shell detection (default 100; `0` = empty only) |
| `WithHeadless(bool)` | Run headless (default `true`) or visible window (`false`) |
| `WithProfileDir(path)` | Persistent Chromium profile directory (kept on `Close`) |
| `WithUserAgent(ua)` | Override declared desktop Chrome User-Agent |
| `WithExecutable(path)` | Force a specific Chromium-family binary |
| `AllowDownload(bool)` | Opt-in one-time download (chrome-for-testing, or Playwright CDN on `linux/arm64`) |
| `WithProgress(fn)` | Byte-level progress callback during network download |
| `WithCacheDir(path)` | Override cache root for downloaded/embedded engines |

Deployment & cache helpers:
- `browser.Install(ctx, opts...)` performs discovery-or-download without
  constructing an `Engine` (ideal for Dockerfile `RUN` steps or setup scripts).
- `browser.CleanCache(opts...)` / `browser.Uninstall(opts...)` safely remove
  previously downloaded or extracted engines from the gosearch cache (never
  touching system-installed browsers).
- `browser.StableEngineAsset(ctx)` returns the download version & URL for the
  current platform (including the pinned Playwright CDN asset on `linux/arm64`).


### When Google serves its `/sorry` CAPTCHA

`Search` reports exactly where the tab landed when it fails, e.g.
`landed on: https://www.google.com/sorry/...` — that is Google's
IP-reputation CAPTCHA, which no tool here will auto-solve. The legitimate
escape hatch is **you** solving it once, in a persistent profile:

```go
// 1) once, headed — click the CAPTCHA yourself, then close:
e, _ := browser.New(ctx,
	browser.AllowDownload(true),
	browser.WithProfileDir("/home/me/.gosearch-profile"),
	browser.WithHeadless(false),
)
e.Search(ctx, "warm up") // solve by hand in the window; Ctrl+C after

// 2) from now on, headless with the same profile reuses that session:
e2, _ := browser.New(ctx,
	browser.AllowDownload(true),
	browser.WithProfileDir("/home/me/.gosearch-profile"),
)
defer e2.Close()
```

Related knobs: `WithUserAgent(ua)` overrides the declared identity string —
by default the engine declares a standard desktop Chrome UA (the same
realistic-identity policy as the core HTTP client) instead of
chrome-headless-shell's `HeadlessChrome`; webdriver flags and fingerprints
stay untouched. Searches also warm up on the homepage first and use
Google's default locale/result-count (no bot-ish `num=20`), because ordinary
flow is precisely what keeps the wall away.

## Where the executable comes from

Resolution order in `New`:

1. `browser.WithExecutable(path)` — force a specific binary.
2. Embedded archive — if this module was built with
   `-tags gosearch_embed_engine`, the binary carries chrome-headless-shell
   inside and self-extracts to the OS cache on first use. Produce the
   archive first:
   ```bash
   cd browser
   go run ./tools/fetch-engine -out engine/chrome-headless-shell.zip
   go build -tags gosearch_embed_engine ...
   ```
   The build fails without the archive present — an engine-less "embedded"
   binary can never ship by accident. Expect roughly +110 MB compressed /
   +250 MB extracted binary size.
3. System discovery — stable-channel names first (`google-chrome-stable`,
   `google-chrome`, `chromium`, `chromium-browser`, `microsoft-edge`,
   `chrome-headless-shell`) plus standard install paths per OS.
4. Prior cached download — read-only check of `<user-cache>/gosearch/browser/`
   (or `WithCacheDir`) so a previously installed binary works without network
   and without re-passing `AllowDownload(true)`.
5. Opt-in download — with `browser.AllowDownload(true)`, fetches
   `chrome-headless-shell` from Google's official chrome-for-testing CDN (or
   Microsoft's Playwright CDN on `linux/arm64`) into
   `<user-cache>/gosearch/browser/<version>/`. Never downloads without that
   explicit flag; otherwise returns `ErrNoBrowserFound` naming every path
   probed. Truly unsupported platforms (e.g. `linux/386`) return
   `ErrUnsupportedPlatform`.

## Honest limitations

The browser is driven **unmodified**: no stealth patches, no
`navigator.webdriver` masking, no fingerprint spoofing. Consequences:

- It clears **JavaScript-gated** pages (e.g. Google's enablejs wall).
- It does **not** defeat IP-reputation blocks or interactive CAPTCHAs, and
  headless/automation signals remain detectable. From a datacenter IP,
  expect challenges regardless of engine.
- Search extraction is heuristic (h3-in-anchor titles, container text as
  snippet) because Google A/B-tests its rendered DOM without notice.
- Yandex is not targeted here: its SmartCaptcha is interactive by design;
  plain-HTTP results from a trusted IP remain the supported path for it.

When a page renders but no recognizable results appear, `Search` wraps
`gosearch.ErrChallenge`; genuinely empty extraction surfaces as
`gosearch.ErrNoResults`; calls on a closed engine return
`browser.ErrEngineClosed`.

## Tests

```bash
go test -race ./...                        # offline/deterministic, never launches a browser
go test -race -tags integration ./...      # live: skips itself if no browser is installed
```
