# Architecture

This expands on `AGENTS.md`'s Module Map with the actual data flow and the
reasoning behind the package boundaries. `AGENTS.md` stays the terse
per-session reference; this file is where the "why" lives.

## Design goal

Stay as close to zero-dependency as possible (`golang.org/x/net/html` is the
only third-party import) while still behaving like an honest browser well
enough that public search-engine result pages serve it real content instead
of an anti-bot challenge. The corollary: gosearch will never solve a
CAPTCHA, execute a JS challenge, or spoof identity to defeat a security
control — see the "Reliability" section of the package doc comment in
[`result.go`](../result.go).

## Package graph

```
                    ┌─────────────────────┐
                    │   gosearch (root)   │   Search() / Fetch()
                    │  public API surface │   Engine enum, Option, errors
                    └──────────┬──────────┘
                               │ dispatches to
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
   internal/providers/  internal/providers/  internal/providers/
      duckduckgo             google*             yandex*
              │                                      
              │ all providers share
              ▼
    ┌──────────────────┐      ┌──────────────────┐
    │ internal/httpclient│     │  internal/htmlx  │
    │ (Get + Detect)     │     │ (DOM helpers)     │
    └──────────────────┘      └──────────────────┘
              │
              ▼ same client, independent of provider
    internal/readability   (used only by Fetch, not Search)

    internal/serrors  — sentinel errors, imported by root + providers + httpclient
    internal/provider — the Result{Title,URL,Snippet} shape providers return
```

## Why the internal split

- **`internal/serrors`** exists solely to break an import cycle: the root
  package imports provider packages, so provider packages cannot import the
  root package back to get at its error values. Sentinels live in
  `internal/serrors`; both sides import it; the root package re-exports the
  same values under `ErrBlocked` etc. so `errors.Is` sees one identity
  throughout, and callers never see or import `serrors` directly.
- **`internal/provider`** is the same trick for the result shape: providers
  return `provider.Result`, and `gosearch.go`'s `toResults` copies each into
  the public `gosearch.Result` at the boundary — so the public type's doc
  comment and identity live in exactly one place (the root package) even
  though four packages produce/consume the shape.
- **`internal/httpclient`** is shared so anti-bot handling (realistic
  headers, cookie jar, per-host rate limiting, and the block-detection
  helper in `detect.go`) is written and tested once, not reimplemented with
  subtly different bugs in each of three provider packages.
- **`internal/htmlx`** is shared DOM-walking helpers (`Attr`, `HasClass`,
  `Tag`, `Text`, `Find*`) over `x/net/html`'s tree, used by
  `internal/providers/duckduckgo` and (once written) the other providers, so
  each parser reads as result-shape logic rather than re-deriving tree
  traversal.
- **Provider packages are unexported on purpose.** The public surface is
  intentionally just `Search`/`Fetch` + the `Engine` enum — nobody imports
  `internal/providers/duckduckgo` directly. This is a deliberate API
  minimalism choice recorded in `AGENTS.md`'s Code Style section, not an
  oversight; promoting one would need an explicit decision recorded there
  first.

## Request flow

### `Search(ctx, query, engine, opts...)`

1. Validate `engine` against the defined constants → `ErrUnsupportedEngine`
   early if not.
2. `apply(opts)` resolves a `config` from defaults + options
   ([`options.go`](../options.go)).
3. Build one `httpclient.Client` for the whole call (shared cookie jar +
   rate limiter across the fallback chain).
4. Walk `[engine] + cfg.fallback` in order via the `dispatch` var (a
   package-level function var — swapped out in tests to inject fake
   providers without hitting the network; see `gosearch_test.go` /
   `orchestration_test.go`).
5. For each engine: call its `Search`, which internally does
   `httpclient.Client.Get` → `httpclient.Detect` → parse. A provider's
   `Search` returns `serrors.ErrNoResults` if parsing succeeded but found
   zero results.
6. In the fallback loop: `err == nil` → done, convert and return. Otherwise,
   only `errors.Is(err, ErrBlocked)` or `errors.Is(err, ErrChallenge)`
   advances to the next engine; anything else (including `ErrNoResults`) is
   returned immediately. If the loop runs out of engines, the accumulated
   errors are joined with `errors.Join` so `errors.Is` still matches through
   it.

This is the one behavioral contract most likely to regress silently if
touched: **fallback must never trigger on a successful empty result.**
Conflating "blocked" with "no results" would hammer every fallback engine on
every query that legitimately has no matches. See the Gotchas section in
`AGENTS.md`.

### `Fetch(ctx, url, opts...)`

Independent of the `Search`/provider/dispatch machinery — it builds its own
client from the same `config`/`newHTTPClient` path, does one `Get` +
`Detect`, then hands the body to `internal/readability.Extract`, which
returns an `Article{Title, Content}` that `Fetch` copies into the public
`Page`. `WithFallback`/`WithMaxResults` are meaningless here and silently
ignored (they're Search-only options; nothing in `Fetch`'s path reads
`cfg.fallback` or `cfg.maxResults`).

## Block detection

`httpclient.Detect` runs on the *final* response (after redirects — this
matters because Yandex signals a block via a 302 to `/showcaptcha`, not a
marker in the initial response) and checks, in order: status code (429/403
apply to any engine), then per-engine markers (header names, redirect-target
substrings, body substrings) documented inline in `detect.go`. A `200 OK`
is not sufficient evidence of success — DuckDuckGo serves its captcha page
with status 200, which is why detection must inspect the body even on a
"successful" status. The real captured pages under `testdata/*/blocked.html`
are the regression fixtures that keep these markers honest; they should
never be hand-edited or "cleaned up."

## Provider status

| Engine | Status | Why |
|---|---|---|
| DuckDuckGo | Implemented, tested against a real capture | Only engine with an official no-JS HTML endpoint; validated 2026-08-23 (`testdata/duckduckgo/real_success.html`) |
| Google | Implemented, heuristic only — pending real-capture validation | Written against the documented basic-HTML markup (`/url?q=` redirects + h3 titles) and tested on synthetic + real-blocked fixtures; a real success page from a trusted network must still land at `testdata/google/real_success.html`, where `TestParseRealSuccessFixture` activates automatically |
| Yandex | Implemented, heuristic only — pending real-capture validation | Most aggressive anti-bot gating of the three (sandbox is 302'd to showcaptcha); same synthetic-fixture + skip-until-capture pattern as Google, targeting `testdata/yandex/real_success.html` |

## Planned: `gosearch/browser` (Phase 5)

Not started. A separate, opt-in subpackage to drive a real, unmodified
headless browser for pages `Fetch` cannot handle because their content is
rendered entirely client-side. Deliberately kept out of the core module so
`go get github.com/BugraAkdemir/gosearch` never pulls in a browser
dependency — see `plan.md` Phase 5 for the runtime-download-with-cache
design and the opt-in `embedbrowser` build tag.
