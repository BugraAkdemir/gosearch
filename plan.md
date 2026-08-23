# plan.md — gosearch v0.1 Roadmap

> Follow this in order. Tick items off as completed. If a phase turns out to
> be wrong, say so and edit this file rather than silently diverging from it.

## Context

`gosearch` is a Go library for people building local-first / zero-dependency
tools who need web search and page-content extraction without paying for or
depending on a search API. It does this by making direct HTTP requests to
Google, Yandex, and DuckDuckGo's public result pages and parsing the HTML,
plus a general-purpose `Fetch()` that pulls the readable content out of any
URL (title + main text, ads/nav/footer stripped).

Live testing from this sandbox showed all three engines apply anti-bot
measures (DuckDuckGo: image captcha; Google: JS-challenge redirect; Yandex:
302 to `showcaptchafast`) even on a single, first-time request — most likely
IP-reputation driven (datacenter IP), not something triggered by our request
pattern. From a residential IP (e.g. the user's home network) success odds
are expected to be meaningfully better, especially for DuckDuckGo. The
library will use legitimate, non-evasive mitigations (realistic headers,
cookie jar, self-imposed rate limiting, lowest-friction endpoints) and will
**never** attempt to auto-solve CAPTCHAs, execute JS challenges, or rotate
proxies to hide identity — those cross from "look like a normal visitor"
into "defeat a security control," which is out of scope by design.

Because Google/Yandex returned only blocked responses in this environment,
their parsers are written against known/documented result markup and unit
tested against synthetic fixtures; a real "successful" HTML capture from a
residential IP is needed before their parsers can be trusted end-to-end —
see Phase 2/3 exit criteria below.

---

## Phase 1 — Skeleton + most reliable provider (DuckDuckGo) + Fetch

- [x] `result.go` — `Result{Title, URL, Snippet string}` (+ `Page` for Fetch)
- [x] `errors.go` — `ErrBlocked`, `ErrChallenge`, `ErrNoResults`,
      `ErrUnsupportedEngine` sentinels (values in `internal/serrors`,
      re-exported to avoid an import cycle with internal packages)
- [x] `options.go` — unified `Option` type (not separate Search/Fetch types):
      timeout, user-agent, proxy, header, cookies, custom client,
      `WithFallback`, `WithMaxResults`
- [x] `internal/httpclient` — shared `http.Client` with:
  - full realistic Chrome-like header set (not just User-Agent): `Accept`,
    `Accept-Language`, `Accept-Encoding`, `Sec-Fetch-*`, `sec-ch-ua*`
  - persistent `http.CookieJar` across requests within a session/client
  - a simple self-imposed rate limiter (min interval between requests to the
    same host)
  - a block-detector helper: given a response, decide `ErrBlocked` /
    `ErrChallenge` / nil, based on status code, redirect target, and known
    marker strings/headers (e.g. `x-yandex-captcha`, DDG's
    `anomaly-modal`, Google's `enablejs` redirect) — documented per provider
  - **[x] done** (`internal/httpclient`: `client.go` + `detect.go`), with
    fixture-based detection tests against the real captured DDG/Google block
    pages and a synthesized Yandex captcha redirect
- [x] `internal/providers/duckduckgo` — parses `html.duckduckgo.com/html/`
      result markup (`result__a`, `result__snippet`), decodes the `uddg`
      redirect param to the real destination URL. Shares `internal/htmlx` DOM
      helpers and returns `internal/provider.Result`. Tested against the
      synthetic success fixture + real captured captcha + empty-page cases.
- [x] `internal/readability` — noise-stripping + container-scoring content
      extractor used by `Fetch()`. Returns `Article{Title, Content}`; tested
      against a synthetic noisy article fixture (asserts prose kept,
      nav/ads/footer stripped), title fallback chain, empty/JS-only page, and
      malformed HTML.
- [x] `gosearch.go` — root `Search(ctx, query, Engine, ...Option)` and
      `Fetch(ctx, url, ...Option)`, dispatch table (var, test-overridable),
      fallback-chain logic (advances only on ErrBlocked/ErrChallenge; joins
      errors when all blocked; ErrNoResults/other errors are final). Google/
      Yandex dispatch returns an unexported not-implemented error until their
      phases. Orchestration + Fetch tested via httptest + fake dispatch.
- [x] Tests: DuckDuckGo block-detection against the real captured captcha
      fixture (`testdata/duckduckgo/blocked.html`, captured 2026-07-29);
      parser against a synthetic success fixture; readability extractor
      against a local `httptest` fixture page
- [x] `examples/basic/main.go`
- [x] CI (`golangci-lint` + `go vet` + `go test -race`), `.gitignore`,
      `LICENSE` (MIT), public `README.md` with an honest per-engine
      reliability note
- [x] **Exit criteria:** `go build ./...`, `go vet ./...`, `gofmt -l .` clean,
  `go test -race ./...` green. `go run ./examples/basic` confirmed
  DuckDuckGo returns real results (2026-08-23); the real response is
  captured as `testdata/duckduckgo/real_success.html` and regression-tested.

## Phase 2 — Google provider

- [x] `internal/providers/google` — parses the classic `/url?q=...&` link +
      adjacent `<h3>` heuristic (documented as best-effort; Google's DOM is
      regionally A/B tested and changes without notice)
- [x] Unit tests against the real captured JS-challenge fixture
      (`testdata/google/blocked.html`, captured 2026-07-29) + a synthetic
      success fixture matching the heuristic
- **Exit criteria:** same as Phase 1, plus — once the user has run this from
  a residential IP and captured one real successful result page, add it as
  a regression fixture and confirm the parser extracts correct results from
  it. Update `AGENTS.md` Known Pitfalls with whatever the real markup turned
  out to be (it will very likely differ in some detail from the heuristic
  written here).

## Phase 3 — Yandex provider

- [ ] `internal/providers/yandex` — parses `serp-item` / organic-result
      heuristic (documented as the most fragile of the three, per the
      anti-bot findings above)
- [ ] Unit tests against a mocked 302-to-`showcaptchafast` response (no real
      fixture needed — the redirect + header signature is enough to test
      block-detection) + a synthetic success fixture
- **Exit criteria:** same shape as Phase 2.

## Phase 4 — Hardening

- [ ] Retry/backoff policy on transient (non-block) failures
- [ ] Confirm `WithFallback` end-to-end: primary engine returns
      `ErrBlocked`/`ErrChallenge` → next engine in the caller-supplied list is
      tried in order → first success wins → if all fail, return
      `errors.Join` of every engine's error so the caller can see exactly
      what happened per engine
- [ ] Document in README: expected reliability per engine, how to supply a
      custom cookie jar/proxy, and the explicit non-goal (no captcha
      solving/JS execution/proxy rotation — ever)
- [ ] Final pass on `AGENTS.md`/`BUG_REPORT.md` to reflect actual shipped
      state instead of Phase-1-era assumptions

## Phase 5 — Optional `gosearch/browser` subpackage (real-browser rendering)

Separate Go module or build-tag-gated subpackage — **never a dependency of
the core `gosearch` module**, so `go get github.com/BugraAkdemir/gosearch`
stays exactly as light as it is in Phase 1-4. This exists for callers who
explicitly opt in and accept the extra weight, e.g. when Google's JS-challenge
blocks the plain-HTTP provider too often for their use case.

Rendering approach: run a real, **unmodified** headless Chromium/Edge/Firefox
— no stealth patches, no `navigator.webdriver` masking, no fingerprint
spoofing. We render the page like a genuine browser would; we do not
disguise the fact that it's automated. That distinction is the line this
project won't cross (see Context in Phase 1-4 above).

- [ ] Browser discovery, in order, first match wins:
  1. **Windows:** Edge (ships preinstalled on every modern Windows box —
     check via `chromedp`'s default discovery / known install paths), then
     Chrome if Edge isn't found.
  2. **macOS/Linux:** Chrome or Chromium via `PATH` / standard install
     locations (this is what `chromedp` already does out of the box).
  3. **Linux fallback:** Firefox/Gecko, since most desktop Linux distros
     ship it preinstalled by default where Chrome/Chromium may not be —
     needs a Gecko-side automation path (e.g. `go-rod`'s Firefox support or
     driving Firefox via the WebDriver/BiDi protocol; Chromium's CDP
     protocol chromedp uses does not speak to Firefox).
- [ ] If no supported browser is found on the system: **do not silently
      download anything.** Return a clear error naming what's missing, and
      only proceed to a one-time headless Chromium download into a local
      cache directory (`~/.cache/gosearch/browser` or OS equivalent) if the
      caller has explicitly opted in (e.g. `browser.New(ctx,
      browser.AllowDownload(true))`) — never implicit, never on first import.
- [ ] `browser.Fetch(ctx, url) (*gosearch.Page, error)` mirrors the core
      `Fetch()` signature/return type so it's a drop-in swap for callers who
      hit JS-walls with the plain-HTTP path.
- [ ] Document real trade-offs in this subpackage's own README: adds a
      ~100-300MB dependency (existing browser or downloaded Chromium),
      slower per-request (real browser startup/render cost vs. a raw HTTP
      GET), and is still not a guarantee against detection — some anti-bot
      systems flag headless-mode signals even on an unmodified real browser.
- **Exit criteria:** builds and runs independently of the core module (a
  `go build ./...` from the core module's root must not require this
  subpackage's dependencies at all); manually verified against at least one
  page that the plain-HTTP `Fetch()` fails on due to required JS rendering.

### Runtime-download model — what actually ships in the binary vs. what doesn't

This is the part callers of `browser.AllowDownload(true)` most need to
understand correctly, so it gets its own explicit spec instead of being left
implicit:

- **`go build` only embeds what exists in the module's source tree at
  compile time** (via `go:embed` or static linking of Go code). A Chromium
  binary fetched over the network at runtime is never part of that build
  step — it cannot be, since it doesn't exist yet when `go build` runs. The
  compiled artifact a developer ships (a binary, a Docker image layer,
  whatever) is therefore **just the small Go binary**, on the order of a few
  MB, identical in size whether or not `browser.AllowDownload` is ever
  exercised.
- **The download happens on the machine that *executes* the program, the
  first time `browser.New(ctx, browser.AllowDownload(true))` runs and no
  supported browser is found locally.** This means: if the developer builds
  on their laptop and ships the resulting binary to a server/container/end
  user's machine, the download happens there, on first run, on that
  target — not once at build time, not baked into the artifact the
  developer distributes.
- **Cache location and reuse — this is a persistent install, not a
  temp-file:** downloaded once per machine into
  `~/.cache/gosearch/browser/<version>/` (or the OS-appropriate equivalent —
  `os.UserCacheDir()`). This directory survives reboots and every
  subsequent run of the Go binary — it is not cleared automatically by the
  OS or by gosearch itself. So in the common case (a normal machine with a
  normal persistent filesystem), the download cost is paid **exactly once,
  ever**, not once per run: run 1 downloads and caches, run 2 through run
  N just detect the cached copy and launch it directly, with no network
  call and no meaningfully different startup latency than launching any
  other installed headless browser. Re-download only happens if the cache
  directory is manually deleted, or the environment itself has no
  persistent storage (see the container caveat below), or a gosearch
  release bumps the pinned Chromium version (the version segment in the
  path prevents silently launching a stale/incompatible cached binary
  against new automation code).
- [ ] `browser.Install(ctx, opts...)` — an explicit, standalone pre-warm
      function that performs the same discovery-or-download logic as the
      first `browser.New(ctx, browser.AllowDownload(true))` call, but as its
      own callable step. Lets a developer pay the one-time download cost
      during deployment/provisioning (e.g. a `RUN` step in a Dockerfile, a
      setup script, a CI image-build stage) instead of having their
      application's *first real user request* be the one that blocks on a
      ~150MB download. `browser.New` still works standalone without ever
      calling `Install` first — `Install` is a convenience for pushing the
      cost earlier, not a required step.
- **Cross-platform correctness for free:** because the download happens on
  the actual target machine rather than being cross-compiled and embedded by
  the developer, it always fetches the binary matching *that* machine's real
  OS/arch — a developer can build once on Linux/amd64 and ship the same Go
  binary to a Linux/arm64 or Windows box without needing to cross-embed
  multiple Chromium variants themselves.
- **Failure modes that must produce clear, typed errors (never a silent
  hang or a generic panic):**
  - No internet access on the target machine at first run → explicit error
    naming the download URL that failed, not a bare network timeout.
  - No write permission / read-only filesystem at the cache path (common in
    locked-down containers) → explicit error naming the path that couldn't
    be written, suggesting `browser.WithCacheDir(...)` to point at a writable
    location if the default isn't usable.
  - Ephemeral/non-persistent storage (e.g. a container with no volume
    mounted at the cache path) → not something the library can detect
    directly, so this is a **documented operational caveat**: every fresh
    container start with no persistent cache volume re-downloads Chromium
    from scratch, which is slow and bandwidth-heavy. README must say this
    plainly so a developer doesn't discover it as a surprise in production.
- **Default non-goal:** by default, this subpackage will not produce a
  single self-contained binary with Chromium baked in — that's the
  runtime-download model described above. But it's the developer's call,
  not a hard restriction; see the opt-in embedded mode immediately below for
  the case where a developer explicitly decides binary size doesn't matter
  to them and instant startup does.

### Opt-in alternative: fully embedded binary (`embedbrowser` build tag)

Some developers will legitimately prefer the opposite trade-off: they don't
care that the compiled binary grows by ~150-300MB, but they do care that the
*end user* never waits on a first-run download and never needs network
access just to launch the browser-rendering path. This is the developer's
explicit choice to make, not the library's to make for them — so it's an
opt-in build tag, off by default, never silently enabled.

- [ ] `go build -tags embedbrowser` compiles a variant of `gosearch/browser`
      that uses `go:embed` to bundle a specific, pinned Chromium build
      directly into the resulting binary — no discovery, no runtime
      download, no network call ever, for the browser-rendering path
      specifically. The instant the binary starts, the browser is already
      on disk (extracted from the embedded blob into a temp/cache path on
      first use within that run, or run directly from an embedded
      filesystem if the automation library supports it).
- [ ] Because `go:embed` bundles whatever is present in the source tree at
      build time, and a single build target is single-OS/single-arch, this
      mode embeds **one platform's Chromium build per compiled binary** —
      it does not magically make one binary portable across OS/arch (no
      approach can do that; a Windows binary needs the Windows Chromium
      build embedded, a linux/arm64 binary needs that build embedded, etc.).
      A developer cross-compiling for multiple targets needs the matching
      per-target Chromium binary present at each build, which the build
      tooling/Makefile for this mode must fetch and place before invoking
      `go build` — documented as a build-time recipe (e.g. a `make
      embed-browser GOOS=... GOARCH=...` step), not something `go build`
      does on its own.
- [ ] This mode ships as its own clearly separate package/subdirectory
      (e.g. `gosearch/browser/embedded`) so importing it is a deliberate,
      visible decision in a developer's `go.mod` — never something pulled in
      by accident through the plain `gosearch/browser` import path.
- [ ] README for this mode must state the trade-off plainly and not oversell
      it: instant startup and zero runtime network dependency, in exchange
      for a binary that is ~150-300MB larger and a build pipeline that must
      manage per-platform Chromium binaries at compile time instead of
      letting the runtime handle it. Same non-goal as the rest of Phase 5
      applies here too: no stealth patches, no fingerprint spoofing — the
      embedded browser runs unmodified, same as the downloaded one.

---

## Fallback design (reference)

```go
results, err := gosearch.Search(ctx, "facebook", gosearch.Google,
    gosearch.WithFallback(gosearch.DuckDuckGo, gosearch.Yandex),
)
```

- Tries `Google` first (the explicit primary argument).
- If it returns `ErrBlocked` or `ErrChallenge`, tries `DuckDuckGo`, then
  `Yandex`, in the exact order given to `WithFallback` — order is always the
  caller's choice, the library does not hardcode a "best" default order
  since reliability depends on the caller's network/region.
- Returns the first successful `[]Result`.
- If every engine in the chain fails, returns `errors.Join(engine1err,
  engine2err, ...)` so the caller can inspect exactly which engines failed
  and why (`errors.Is(err, gosearch.ErrBlocked)` still works against the
  joined error).
- No fallback engine is tried on a *successful-but-empty* result (`0`
  results is not a failure) unless the caller opts into that via a separate
  option — an empty result set from the primary engine is returned as-is by
  default.
