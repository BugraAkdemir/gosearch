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
go get github.com/BugraAkdemir/gosearch/browser    # opt-in browser engine (+chromedp)
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
