# AGENTS.md — gosearch

`gosearch` is a zero-API-key Go library that does web search (Google, Yandex,
DuckDuckGo, Bing) by fetching and parsing each engine's public HTML result page
directly, plus a `Fetch()` that extracts the readable content of any URL. It
exists for local-first / zero-dependency Go programs (e.g. an LLM agent's
web-search tool) that can't or won't depend on a hosted search API. Core
architectural bet: **stay as close to zero-dependency as possible** — the
only third-party import is `golang.org/x/net/html`.

---

## Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Language | Go | `go 1.25` directive (required by x/net v0.57.0); developed on toolchain 1.26 |
| HTML parsing | `golang.org/x/net/html` | v0.57.0 |
| HTTP client | Go stdlib `net/http` (custom-configured) | — |
| Everything else | Go stdlib only | — |

No frontend, no database, no cache/queue — it's a library.

---

## Architecture

Single Go module, one public package (`gosearch`) that dispatches to
unexported provider packages. The public surface is intentionally tiny:
`Search()`, `Fetch()`, the `Engine` enum, the functional-option types, and
the sentinel errors. Everything else lives under `internal/` so it can be
refactored freely without breaking anyone's import.

Providers implement a common internal interface (each engine = one package
under `internal/providers/`). A shared `internal/httpclient` gives every
provider the same realistic-browser HTTP behavior (headers, cookie jar, rate
limiting) and a single block-detection helper, so anti-bot handling isn't
reimplemented per provider. `Fetch()` is engine-independent and uses
`internal/readability` to pull main content out of arbitrary pages.

### Module Map

| Directory | Responsibility | Key Files |
|-----------|---------------|-----------|
| `.` (package `gosearch`) | Public API: `Search`/`Fetch`, `Engine`, options, sentinel errors, provider dispatch + fallback chain | `gosearch.go`, `result.go`, `errors.go`, `options.go` |
| `internal/httpclient/` | Shared HTTP client: realistic headers, cookie jar, per-host rate limiter, block-detection helper | `client.go` |
| `internal/providers/duckduckgo/` | Parse `html.duckduckgo.com/html/` results | `duckduckgo.go` |
| `internal/providers/google/` | Parse Google result page (best-effort, see Pitfalls) | `google.go` |
| `internal/providers/yandex/` | Parse Yandex result page (most fragile, see Pitfalls) | `yandex.go` |
| `internal/readability/` | Noise-stripping + container-scoring content extractor for `Fetch()` | `readability.go` |
| `examples/basic/` | Runnable usage example | `main.go` |
| `testdata/` | Captured real block/success pages + synthetic fixtures per provider | `*/blocked.html`, `*/success.html`, `*/real_success.html` |
| `docs/` | Human-readable API reference + architecture doc (companions to `go doc`, not a replacement) | `API.md`, `ARCHITECTURE.md` |
| `browser/` | SEPARATE Go module: optional real-browser engine (chromedp/CDP) for JS-gated pages; system discovery, opt-in download, `-tags gosearch_embed_engine` embedding. See its README + the Phase 5 dependency decision above | `engine.go`, `search.go`, `fetch.go`, `cfd.go`, `discover.go` |

### Entry Points

It's a library — no `main`. The public entry points are `gosearch.Search()`
and `gosearch.Fetch()` in `gosearch.go`. `examples/basic/main.go` is the only
runnable binary and exists purely to demonstrate/verify the API by hand.

---

## Development

### Quick Start

```bash
go build ./...
go run ./examples/basic          # hits the live web; may be blocked from datacenter IPs
```

### Testing

```bash
go test -race ./...              # unit tests: fixture-based, no network required
go vet ./...
gofmt -l .                       # must print nothing
```

Tests are **fixture-based and offline by default** — they parse captured HTML
under `testdata/`, they do NOT hit the live engines (that would be flaky,
rate-limited, and IP-dependent). Any future live/integration test must be
gated behind a build tag (e.g. `//go:build integration`) so `go test ./...`
stays deterministic and CI-safe.

CI: GitHub Actions (`.github/workflows/ci.yml`) runs `go build`, `go vet`,
`gofmt` check, `golangci-lint`, and `go test -race` on every push and PR.

### Release

Releases are annotated tags `vX.Y.Z` on `main`, pushed together with the
branch (`git push origin main --follow-tags`). There is no separate publish
step: once the tag is public, `go get` + `proxy.golang.org` handle
distribution automatically. First release: v0.1.0, cut 2026-08-23 after
Phases 1–4 (Phase 5 remains optional/unbuilt).

**Hard rule:** never push a git tag / cut a release without the user
explicitly asking for it in that specific moment — a tag is public and
effectively permanent on the Go module proxy once fetched.

---

## Known Pitfalls & Technical Debt

> Living log. When you find a bug, add a bullet (symptom + root cause). When
> fixed, `~~strike~~` it and append `→ fixed <date> (<commit>): ...`. If a
> claim turns out stale, mark `→ stale: ...` rather than deleting it.

### Anti-bot / providers (2026-07-29, design-phase findings)
- All three engines returned an anti-bot response on the **first** request
  from this project's build sandbox (a datacenter IP): DuckDuckGo served an
  image-captcha (`anomaly-modal` markup), Google returned a JS-challenge page
  (`/httpservice/retry/enablejs` redirect, no real results), Yandex 302'd to
  `showcaptchafast` with an `x-yandex-captcha: captcha` header. This is
  believed IP-reputation driven; residential IPs should fare much better.
  **Consequence for parsers:** Google/Yandex parsers are written against
  documented/known markup, not a real successful capture — they must be
  re-validated against a real success page from a residential IP before being
  trusted (Phase 2/3 exit criteria in `plan.md`).
- Google's result DOM is regionally A/B tested and changes without notice —
  any parser here is inherently best-effort. Don't treat a parse miss as
  necessarily a bug in our code; capture the actual HTML first.
- **DuckDuckGo re-validated 2026-08-23:** `go run ./examples/basic` against
  the live endpoint from this session's environment returned a clean 200
  with 10 real results for "facebook" — no captcha/anomaly markup this time
  (contrast with the 2026-07-29 finding above). The real response HTML is
  captured as `testdata/duckduckgo/real_success.html` and parsed by
  `TestParseRealSuccessFixture` in `duckduckgo_test.go`, so `parse()` is now
  validated against real markup, not only the synthetic fixture. This
  satisfies the Phase 1 exit criterion in `plan.md`. Google/Yandex are still
  unvalidated against a real success page — that finding stands.
- **Google/Yandex re-probed 2026-08-23 (Session 3):** still blocked from this
  sandbox — Google again served the `enablejs` JS-challenge page with HTTP
  200, Yandex again 302'd to `showcaptcha`. CI won't help either: GitHub
  Actions runners are datacenter IPs with worse reputation. The real-success
  capture remains a one-time manual step from a trusted network; drop the
  file at `testdata/google/real_success.html` and the pre-wired
  `TestParseRealSuccessFixture` in `google_test.go` activates automatically
  (it skips itself while the file is absent).

- **Bing probed 2026-08-24 (Session 4):** unlike Google/Yandex, Bing served a
  clean HTTP 200 with ten organic results to a plain GET from this flagged
  datacenter sandbox — no captcha, no JS requirement — making it the most
  automation-tolerant engine after DuckDuckGo. `internal/providers/bing`
  parses `li.b_algo` containers; title links ride Bing's `/ck/a`
  click-tracker, and destinations are recovered from the `u=a1<base64url>`
  query param when present, else rebuilt **best-effort** from the visible
  `<cite>` display URL (Bing truncates long paths with "…", so tail segments
  can be lost). Real-capture regression test pre-wired at
  `testdata/bing/real_success.html` (skips until the file lands).

### Security
- **FOUND & FIXED 2026-08-24 (live probe, en.wikipedia.org):** `readability`'s
  noise markers ("header", "menu", "nav"…) substring-matched against
  `<html>`/`<body>` feature-flag classes
  (`vector-feature-language-in-header-enabled`) → the ROOT was removed and
  EVERY page returned empty content. Fix: root elements are exempt from
  noise removal (`TestExtractSurvivesNoiseMarkerClassesOnRoot` pins it).
  Lesson: the synthetic article fixture could never catch this — only a real
  page could; keep live probes in the loop after extractor changes.
- Block-detection markers (header names, redirect substrings, marker CSS
  classes) live in `internal/httpclient`. Treat them as untrusted external
  input — never `eval`/execute anything from a fetched page; we only ever
  string-match against it.

---

## Agent Working Rules (READ FIRST, EVERY SESSION)

1. **Start of session:** read this file, then `handoff.md` (top entry = last session's state and pending work).
2. **End of session:** prepend a new entry to `handoff.md` (what was done, commit status, verification results, what's next). This is the primary cross-session/cross-model handoff mechanism, not an afterthought.
3. **Never claim "done" without running the verification commands below and pasting the actual results.** A claim unbacked by a real command's output is not verification.
4. Plan lives in `plan.md` — follow it in order, tick items off, don't improvise a different architecture mid-plan. If the plan is wrong, say so and edit `plan.md` rather than silently diverging.
5. Work in small units: a bounded number of plan items per session, each with its own tests, verified green before moving to the next.
6. **Commit automatically, without asking, once a fix/feature is verified green.** Use **Conventional Commits** (`feat(scope): ...`, `fix(scope): ...`, `docs: ...`, `test(scope): ...`, `ci: ...`, `refactor(scope): ...`), with a body explaining the *why*. **Never include an AI-attribution / `Co-Authored-By` line, under any circumstance.** Commit frequently at natural checkpoints (not every file save, not only once per whole task) — especially right before a risky step, so history can be bisected/reverted cleanly.
7. **Code exploration:** the codebase-memory MCP tools (`search_graph`, `trace_path`, `get_code_snippet`) are available and preferred over blind grepping for "who calls X / what does X call" questions once the project has enough code to index. For a package this small, plain Read/Grep is fine too — use judgment.
8. **Zero-dependency discipline is a hard rule:** do not add any third-party dependency beyond `golang.org/x/net/html` without recording an explicit decision in this file first. The entire value proposition is being lightweight enough to drop into a local-first project — a stray dependency silently breaks that promise for every downstream user.
   **Decision 2026-08-23 (Phase 5):** the `browser/` subdirectory is a SEPARATE Go module (own `go.mod`, `replace` to the parent) precisely so it may carry third-party deps — `github.com/chromedp/chromedp` for CDP driving — without touching the core module's promise. `go get github.com/BugraAkdemir/gosearch` still resolves to the core only; the browser engine is opt-in via `go get github.com/BugraAkdemir/gosearch/browser`. Core rule unchanged: nothing beyond x/net enters the root module.
9. **Documentation is not optional.** Every exported type, function, and package gets a real Go doc comment (behavior, not a name restatement). When a change adds or changes public API, exported errors, or provider/fallback behavior, update `README.md`'s examples in the **same commit**. Package-level docs must state real limitations (per-provider anti-bot fragility), not just the happy path.

### Verification Commands (mandatory before any "done" claim)

```bash
go build ./...
go vet ./...
gofmt -l .            # must print NOTHING; any output = unformatted file
go test -race ./...
```

If `golangci-lint` is installed locally, also run `golangci-lint run`; CI
runs it regardless. CI pins `golangci-lint` to `v2.13.1` via
`golangci-lint-action@v9` (`.github/workflows/ci.yml`) — `golangci-lint-action@v6`
hard-rejects any v2.x `golangci-lint` version, so don't downgrade the action
without also downgrading `.golangci.yml` back to v1 format, or vice versa;
they must move together.

Acceptable pre-existing noise: none currently. Any new vet/lint/test failure
must be addressed before claiming done.

---

## Gotchas (project-specific traps — violating these causes real, shipped bugs)

**HTTP / anti-bot**
- A `200 OK` does NOT mean success — an engine can serve a captcha/challenge
  page with status 200 (DuckDuckGo does exactly this). Always run the
  response through the block-detector before parsing; a parser fed a captcha
  page will return zero results and look like a "no results" bug.
- Block detection must run on the *final* response after redirects (Yandex
  signals via a 302 to `showcaptchafast`). Don't disable redirect following
  without preserving the ability to see that the redirect happened.

**Parsing**
- Providers `errors.Is`-match sentinel errors; callers must never string-match
  error text. If you wrap an error, wrap with `%w` so `errors.Is` still works
  through the fallback chain (which uses `errors.Join`).
- `testdata/*/blocked.html` are real captured captcha/challenge pages — they
  are the regression guard that block-detection keeps working. Don't
  "clean them up" or regenerate them casually.

**Fallback**
- `WithFallback` only advances to the next engine on `ErrBlocked`/
  `ErrChallenge` — NOT on a successful-but-empty result (0 results is a valid
  answer, not a failure). Preserve this distinction; conflating them will
  hammer every fallback engine on every empty query.

---

## Known Open Work

Open bugs and technical debt are tracked in **`BUG_REPORT.md`** — don't
duplicate that list here. As of 2026-08-24: 0 open items; Phases 1–4 are
shipped, Phase 5 browser module built, plus a post-v0.1 quality package
(canonical URL dedup, opt-in `WithMarkdown`/`WithDates`, domain policy).
Still open from the exit criteria: real success-page
captures from a trusted network for Google (`testdata/google/
real_success.html`) and Yandex (`testdata/yandex/real_success.html`) — and
from ANY network for Bing (`testdata/bing/real_success.html`) — the
pre-wired regression tests activate on their own once the files land.

---

## Code Style

- Errors are values, never panics across package boundaries. Providers return typed sentinel errors (`ErrBlocked`, `ErrChallenge`, `ErrNoResults`) wrapped with context via `fmt.Errorf("%w: ...")` — callers `errors.Is` against the sentinel, never string-match an error message.
- No third-party dependency beyond `golang.org/x/net/html` without an explicit decision recorded here — the whole point of this library is staying close to zero-dependency for people who can't/won't pull in an API SDK.
- Every exported symbol has a doc comment (see Agent Working Rules #9). `go doc ./...` should read as a complete usage guide on its own, not just a name restatement.
- Provider packages under `internal/providers/` are intentionally unexported — the public surface is the root package's `Search`/`Fetch` plus the `Engine` enum. Don't promote a provider package to public API without a deliberate decision recorded here first.
