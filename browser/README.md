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
# Until the first prefixed tag is published, pull the tip of main:
go get github.com/BugraAkdemir/gosearch/browser@main
# After a browser/vX.Y.Z tag exists (multi-module repos require prefixed tags):
go get github.com/BugraAkdemir/gosearch/browser
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

One `Engine` = one long-lived browser process with one reused tab. Keep it
alive for the lifetime of your program; steady-state memory stays at roughly
one page's worth (~100–250 MB idle + page), not a fresh browser per request.
Images are disabled and the GPU process is off by default to keep consumption
down.

Deployment helpers: `browser.Install(ctx, opts...)` performs the same
discovery-or-download without building an Engine — call it in a Dockerfile
`RUN` step or setup script so the one-time download never lands on a live
user request. `browser.WithCacheDir(path)` points storage somewhere writable
when the default user-cache location is read-only (locked-down containers).


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
4. Opt-in download — with `browser.AllowDownload(true)`, fetches
   chrome-headless-shell from Google's official chrome-for-testing CDN into
   `<user-cache>/gosearch/browser/<version>/`. Never downloads without that
   explicit flag; otherwise returns `ErrNoBrowserFound` naming every path
   probed.

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
`gosearch.ErrNoResults` — same sentinel vocabulary as the core module.

## Tests

```bash
go test -race ./...                        # offline/deterministic, never launches a browser
go test -race -tags integration ./...      # live: skips itself if no browser is installed
```
