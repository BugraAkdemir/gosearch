# API Reference

This is a human-readable companion to `go doc` — the doc comments in the
source are the source of truth (`go doc github.com/BugraAkdemir/gosearch` or
`go doc -all .` from the repo root always reflects the current code). This
file exists for readers who'd rather browse one page than run a command.

## `Search`

```go
func Search(ctx context.Context, query string, engine Engine, opts ...Option) ([]Result, error)
```

Queries `engine` for `query` and returns parsed results.

- **As of this version, `DuckDuckGo` and `Google` are implemented.** `Yandex`
  is a defined `Engine` constant but calling `Search` with it currently
  returns an error — see [`plan.md`](../plan.md) for the phased rollout and
  why (short version: a parser is not trusted until it's been validated
  against a real, non-blocked capture of that engine's success page — see
  `AGENTS.md`'s Known Pitfalls). The Google parser is best-effort by design:
  Google serves its basic-HTML result page only to clients it trusts, and its
  DOM changes without notice, so a parse miss is not necessarily a bug —
  capture the actual HTML first.
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
  permanent limitation of plain-HTTP fetching, not a bug to work around here
  (see [Phase 5 / the planned `gosearch/browser` subpackage](ARCHITECTURE.md#planned-gosearchbrowser-phase-5)
  for the opt-in real-browser answer to this).
- Returns `ErrBlocked`/`ErrChallenge` if the server responds with an
  anti-bot page instead of real content.
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
