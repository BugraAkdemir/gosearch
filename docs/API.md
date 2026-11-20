# API Reference

This is a human-readable companion to `go doc` — the doc comments in the
source are the source of truth (`go doc github.com/BugraAkdemir/gosearch` or
`go doc -all .` from the repo root always reflects the current code). This
file exists for readers who'd rather browse one page than run a command.

New here? Start with [GETTING_STARTED.md](./GETTING_STARTED.md) for a
from-zero walkthrough, or [RECIPES.md](./RECIPES.md) for task-oriented
copy-paste solutions. Internal design lives in
[ARCHITECTURE.md](./ARCHITECTURE.md).

## `Search`

```go
func Search(ctx context.Context, query string, engine Engine, opts ...Option) ([]Result, error)
```

Queries `engine` for `query` and returns parsed results.

- **All four engines (`DuckDuckGo`, `Google`, `Yandex`, `Bing`) are
  implemented.**
  Reliability differs sharply per engine and per network: DuckDuckGo's parser
  is validated against a real captured success page; the Google, Yandex, and
  Bing parsers are best-effort heuristics — those engines change their DOM
  without notice (Google/Yandex also gate results behind anti-bot checks), so
  a parse miss is not necessarily a bug; capture
  the actual HTML first (see [`plan.md`](../plan.md) and `AGENTS.md`'s Known
  Pitfalls).
- `WithFallback` engines are tried, in order, **only** when the current
  engine returns `ErrBlocked` or `ErrChallenge`. A successful-but-empty
  result (`ErrNoResults`) does **not** trigger fallback — an empty result is
  a valid answer, not a failure, and a different engine is no more likely to
  have results for a query that genuinely has none.
- If every engine in the chain is blocked/challenged, `Search` returns
  `errors.Join` of each engine's error, so `errors.Is(err, ErrBlocked)` /
  `errors.Is(err, ErrChallenge)` still report `true` through the join.
- The same underlying HTTP client (cookie jar + rate limiter) is reused
  across the whole fallback chain for one call.
- Results whose URLs differ only in spelling collapse into one result:
  percent-encoded vs literal non-ASCII query values, dotted vs undotted
  capital İ/I (observed live on Bing), parameter order, host case, fragments,
  and default ports are treated as the same page; the first-seen original
  spelling is what you get in `Result.URL`.

```go
results, err := gosearch.Search(ctx, "facebook", gosearch.DuckDuckGo,
    gosearch.WithMaxResults(5),
)
```

## `Fetch`

```go
func Fetch(ctx context.Context, url string, opts ...Option) (*Page, error)
```

Retrieves `url` and extracts its main readable content (title + body text,
navigation/ads/boilerplate stripped) into a `Page`.

- Does **not** run JavaScript. A page whose content is rendered entirely
  client-side will yield an empty `Page.Content` — this is a known,
  permanent limitation of plain-HTTP fetching, not a bug to work around here.
  The opt-in real-browser answer is the separate
  [`gosearch/browser`](../browser/README.md) module — see
  [the browser recipe](./RECIPES.md#when-plain-http-is-not-enough-the-browser-engine).
- Returns `ErrBlocked`/`ErrChallenge` if the server responds with an
  anti-bot page instead of real content.
- Pass `WithMarkdown()` to receive `Content` as Markdown (headings, lists,
  fenced code, links, emphasis) instead of plain text — the natural format
  for feeding page content to an LLM.
- `WithFallback` and `WithMaxResults` are Search-only options; `Fetch`
  ignores them.

```go
page, err := gosearch.Fetch(ctx, "https://en.wikipedia.org/wiki/Facebook")
```

## Types

### `Result`

One search result. `Snippet` may be empty — not every engine/result type
provides one.

| Field | Meaning |
|---|---|
| `Title` | Clickable heading text |
| `URL` | Destination link |
| `Snippet` | Short excerpt below the title (may be `""`) |
| `Date` | Engine's own freshness stamp, verbatim (`"2026-08-20"`, `"1 day ago"`). Always `""` without `WithDates`; often `""` even with it — engines frequently omit dates on no-JS pages |

### `Page`

The extracted content of a `Fetch`ed URL — never raw HTML.

| Field | Meaning |
|---|---|
| `URL` | Final URL after following redirects |
| `Title` | Best-guess title (`<title>`, an `<h1>`, or article metadata) |
| `Content` | Extracted main text; `""` if no main content region was found |

### `Engine`

```go
const (
    DuckDuckGo Engine = iota
    Google
    Yandex
    Bing
)
```

Pass one as `Search`'s third argument, and any number more via
`WithFallback`. `Engine.String()` gives the lowercase name for logs.

## Options

All options are `func(*config)` values passed as `Search`/`Fetch`'s trailing
variadic args, applied in the order given.

| Option | Applies to | Effect |
|---|---|---|
| `WithTimeout(d time.Duration)` | Both | Bounds the request. Non-positive resets to the 15s default. |
| `WithUserAgent(ua string)` | Both | Overrides the default browser User-Agent. Override only with a specific reason — a realistic UA is part of how this library avoids looking like a bot. |
| `WithProxy(rawURL string)` | Both | Routes requests through a proxy (`http://`, `socks5://`, etc.) — your own network egress, not identity rotation to defeat anti-bot controls. |
| `WithHeader(key, value string)` | Both | Adds/overrides one request header. Call multiple times for multiple headers. |
| `WithCookies(cookies ...*http.Cookie)` | Both | Seeds the cookie jar before the first request (e.g. reuse a session exported from your own browser). |
| `WithHTTPClient(client *http.Client)` | Both | Escape hatch: uses your `*http.Client` as-is, bypassing this library's default headers/cookie jar/rate limiting entirely. `WithTimeout`/`WithProxy`/`WithHeader` are **not** layered on top when this is set. |
| `WithFallback(engines ...Engine)` | Search only | Ordered fallback chain, engaged only on `ErrBlocked`/`ErrChallenge`. Ignored by `Fetch`. |
| `WithMaxResults(n int)` | Search only | Caps returned results. `0` (default) = no cap. Ignored by `Fetch`. |
| `WithRetries(n int)` | Both | Transient failures (transport errors, HTTP 408/5xx) are retried with exponential backoff. Default `2`; `0`/negative disables. Blocks/challenges (`ErrBlocked`/`ErrChallenge`) are never retried — use `WithFallback` for those. |
| `WithMarkdown()` | Fetch only | Renders `Page.Content` as GitHub-flavored Markdown (headings, lists, fenced code, links, emphasis) instead of plain text. Ideal when feeding page content to an LLM. Default off — output is unchanged without it. Ignored by `Search`. |
| `WithDates()` | Search only | Fills `Result.Date` from each engine's freshness metadata (best-effort, often absent). Does not change the query or results — only surfaces metadata already on the page. Default off. Ignored by `Fetch`. |
| `WithBlockedDomains(domains ...string)` | Search only | Drops results whose host is the domain or a subdomain of one (`spam.example.net` kills `www.spam.example.net`, spares `notspam.example.net`). Applied after a successful search; filtering everything away is a valid empty answer and does **not** trigger fallback. |
| `WithAllowedDomains(domains ...string)` | Search only | Inverse: keeps only hosts matching the list (host-or-subdomain); everything else, including unparseable-host results, drops. Deny is applied before allow when both are set. |

## Errors

All four are sentinel errors — always check with `errors.Is`, never by
matching error text (providers wrap them with `fmt.Errorf("%w: ...")`, and
the fallback chain further wraps with `errors.Join`, so `errors.Is` is the
only comparison that survives both).

| Error | Meaning | Triggers `WithFallback`? |
|---|---|---|
| `ErrBlocked` | Anti-bot system flagged the request (429, IP-reputation block, "you look like a bot" redirect) | Yes |
| `ErrChallenge` | Engine served an interactive challenge (CAPTCHA / JS challenge) instead of results | Yes |
| `ErrNoResults` | Request succeeded, page parsed cleanly, genuinely zero results | **No** — a valid answer |
| `ErrUnsupportedEngine` | `Search` called with an `Engine` value that isn't one of the defined constants | N/A (returned before any request is made) |

```go
results, err := gosearch.Search(ctx, query, gosearch.DuckDuckGo)
switch {
case errors.Is(err, gosearch.ErrNoResults):
    // valid: nothing matched
case errors.Is(err, gosearch.ErrBlocked), errors.Is(err, gosearch.ErrChallenge):
    // anti-bot system intervened; consider WithFallback next time
case err != nil:
    // network error, etc.
}
```

---

## `gosearch/browser` Module

Optional separate Go module (`github.com/BugraAkdemir/gosearch/browser`) for
JavaScript-rendered pages and JS-gated search walls. Install separately:

```bash
go get github.com/BugraAkdemir/gosearch/browser@latest
```

### Functions & `Engine` Methods

| Symbol | Effect |
|---|---|
| `browser.New(ctx, ...Option) (*Engine, error)` | Resolves executable (explicit → embedded → system → cached → opt-in download) and returns a lazily started, concurrency-safe `Engine`. |
| `(*Engine).Search(ctx, query) ([]gosearch.Result, error)` | Renders Google post-JS, deduplicates via canonical URLs, enforces domain policy, and caps results. |
| `(*Engine).Fetch(ctx, rawURL) (*gosearch.Page, error)` | Renders `rawURL`, captures post-JS HTML (8 MiB UTF-8/tag-safe cap), and runs core `internal/readability` (plain text or Markdown). |
| `(*Engine).FetchWithFallback(ctx, rawURL, ...gosearch.Option) (*gosearch.Page, error)` | Tries fast plain-HTTP `gosearch.Fetch` first (Chromium stays unstarted on static pages); falls back to `e.Fetch` on `ErrBlocked`/`ErrChallenge` or thin/empty JS shells. |
| `(*Engine).SearchWithFallback(ctx, query, primary, ...gosearch.Option) ([]gosearch.Result, error)` | Tries plain-HTTP `gosearch.Search` first; falls back to `e.Search` only on `ErrBlocked`/`ErrChallenge` (never on `ErrNoResults`). |
| `(*Engine).Executable() string` | Reports the resolved Chromium-family binary path. |
| `(*Engine).Close() error` | Gracefully shuts down Chromium (`chromedp.Cancel`), waits for exit, and removes throwaway profiles (keeps `WithProfileDir`). Idempotent. |
| `browser.Install(ctx, ...Option) error` | Runs executable discovery/download without creating an `Engine` (useful in Dockerfiles / setup steps). |
| `browser.CleanCache(...Option) error` / `browser.Uninstall(...Option) error` | Safely removes downloaded/extracted cache entries (honors `WithCacheDir`; rejects unsafe roots; never touches system browsers). |
| `browser.StableEngineAsset(ctx) (version, url string, err error)` | Returns the stable headless-shell version and download URL for the host platform (chrome-for-testing, or pinned Playwright CDN on `linux/arm64`). |
| `browser.CurrentPlatform() string` | Returns the chrome-for-testing slug (`linux64`, `mac-arm64`, `mac-x64`, `win64`, or `""`). |
| `browser.DownloadFile(ctx, url, dstFile) error` | Streams `url` to `dstFile` atomically (`.part` then rename). |

### `browser.Option`

| Option | Effect |
|---|---|
| `WithMarkdown()` | Return GitHub-flavored Markdown from `Fetch` / `FetchWithFallback`. Default off. |
| `WithTimeout(d time.Duration)` | Override per-call timeout (defaults: 30s `Search`, 45s `Fetch`). Non-positive resets to defaults. |
| `WithProxy(rawURL string)` | Route Chromium traffic through `--proxy-server` (`http`/`https`/`socks4`/`socks5`/`socks5h`). Validated in `New`. |
| `WithMaxResults(n int)` | Cap `Search` results after domain filtering (default 30). |
| `WithAllowedDomains(ds ...string)` / `WithBlockedDomains(ds ...string)` | Suffix-host domain policy for `Search` (deny first, then allow). |
| `WithMaxConcurrency(n int)` | Max concurrent calls per `Engine`. `<= 1` (default) serializes on one shared tab; `> 1` opens isolated tabs on the same browser process. |
| `WithWaitSelector(selector string)` | Wait for a CSS selector in `Fetch` after `body` is ready (for async SPAs). |
| `WithWaitDelay(d time.Duration)` | Context-aware settle delay in `Fetch` after DOM/selector readiness. |
| `WithFallbackMinContent(chars int)` | Minimum trimmed rune count `FetchWithFallback` accepts from HTTP before falling back (default `100`; `0` = empty only). |
| `WithHeadless(v bool)` | Run headless (`true`, default) or visible window (`false`). |
| `WithProfileDir(path string)` | Persistent Chromium profile kept across runs and after `Close`. |
| `WithUserAgent(ua string)` | Override the desktop Chrome User-Agent. |
| `WithExecutable(path string)` | Force a specific Chromium-family binary, bypassing discovery. |
| `AllowDownload(v bool)` | Permit one-time headless-shell download into cache when no local browser exists. |
| `WithProgress(fn func(downloaded, total int64))` | Progress callback invoked per chunk during network download (`AllowDownload`). |
| `WithCacheDir(path string)` | Override cache directory for downloaded/embedded engines. |

### `browser` Errors

| Error | Meaning |
|---|---|
| `browser.ErrNoBrowserFound` | No local/cached browser found and `AllowDownload` was not enabled. |
| `browser.ErrDownloadDisabled` | Downloads explicitly disabled or embedded archive absent. |
| `browser.ErrUnsupportedPlatform` | Host `GOOS/GOARCH` has no automatic download source (e.g. `linux/386`). |
| `browser.ErrEngineClosed` | `Search`, `Fetch`, or fallback helpers called after `Engine.Close()`. |
