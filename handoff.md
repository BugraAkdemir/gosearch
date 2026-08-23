# handoff.md — how this file works

`handoff.md` is the running session log a coding agent reads at the **start**
of every session and writes to at the **end** of every session. It is what
lets context survive across sessions and across different models — the agent
should never have to reconstruct "what was I doing" from git log alone.

**Rules for this file:**

1. **Newest entry always goes on top.** Prepend, never append — the top of
   the file is always "what happened most recently and what's pending."
2. **One entry per session**, using the exact heading format below. If a
   session has multiple distinct legs (e.g. resumed later the same day),
   either add a new dated entry or clearly mark a sub-section — don't merge
   unrelated work into one undifferentiated blob.
3. **Every entry ends with a "Next Session" section.** This is the single
   most important part of the entry — it's the actual handoff. Be specific:
   name files, name the exact next step, name what was *deliberately left
   out of scope* and why (so it isn't silently forgotten or redone).
4. **Verification results are pasted, not summarized.** "Tests pass" is not
   verification; the actual command and its actual output (or a faithful
   excerpt) is.
5. This file is allowed to grow long. Don't prune old entries — they're the
   permanent record. If it gets unwieldy to read top-to-bottom, that's a sign
   to `grep` for a module/date, not a sign to delete history.
6. Fixed bugs get their permanent record in `AGENTS.md`'s Known Pitfalls
   section (with the `~~struck~~ → fixed` convention) and in git history —
   this file's job is session narrative and handoff state, not a bug
   database. Open bugs live in `BUG_REPORT.md`.

---

# Handoff — 2026-08-23 (Session 2) — Finish Phase 1 (example, CI, lint config)

## Summary

Picked up exactly where Session 1 left off: the three remaining Phase 1
checklist items in `plan.md` (`examples/basic/main.go`, CI workflow,
`.golangci.yml`). No architecture or API changes — this session only adds
scaffolding around the already-implemented core.

**Commit status:** `bd179a0` on `main`, not yet pushed to origin. Working
tree clean otherwise.

## What Was Done

- `examples/basic/main.go` — runnable demo: `Search` on DuckDuckGo, prints
  results, then `Fetch`es the first result's URL and prints the extracted
  title/content. Distinguishes `ErrBlocked`/`ErrChallenge` from other errors
  per the pattern documented in `errors.go`. Not run against the live
  network this session (see Next Session #1 — needs a residential IP).
- `.github/workflows/ci.yml` — runs on push to `main` and on PRs: checkout,
  `actions/setup-go@v5` (go 1.25), `go build ./...`, `go vet ./...`, a
  `gofmt -l .` check that fails the job on any output,
  `golangci/golangci-lint-action@v6` (version: latest), `go test -race ./...`.
- `.golangci.yml` — written in **v2 config format** (`version: "2"`,
  `linters.default: standard` + explicit `enable` list: errcheck, govet,
  ineffassign, staticcheck, unused, gocritic, revive; `formatters.enable`:
  gofmt, goimports). Chose v2 format because `golangci-lint-action@v6` with
  `version: latest` currently resolves to a golangci-lint v2.x binary.
  **Not verified locally** — `golangci-lint` is not installed on this
  machine (see Next Session #2).
- `plan.md` — checked off the three items above plus the Phase 1 tests
  checkbox, which was actually already satisfied by Session 1's work
  (`client_test.go`, `detect_test.go`, `duckduckgo_test.go`,
  `readability_test.go` all exist and pass) but had been left unchecked.

## Verification

```bash
$ go build ./...      # (no output = ok)
$ go vet ./...         # (no output = ok)
$ gofmt -l .           # (no output = all formatted)
$ go test -race ./...
ok  	github.com/BugraAkdemir/gosearch	1.017s
?   	github.com/BugraAkdemir/gosearch/examples/basic	[no test files]
?   	github.com/BugraAkdemir/gosearch/internal/htmlx	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	1.390s
?   	github.com/BugraAkdemir/gosearch/internal/provider	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	1.022s
ok  	github.com/BugraAkdemir/gosearch/internal/readability	1.015s
?   	github.com/BugraAkdemir/gosearch/internal/serrors	[no test files]
```

**NOT verified:**
- `golangci-lint run` was not run locally (tool not installed) — the
  `.golangci.yml` syntax is untested until either it's installed locally or
  the CI workflow actually runs on GitHub. If the v6 action resolves to a
  v1.x binary instead of v2.x for any reason, this config's `version: "2"` /
  `linters.default` / `formatters` block will fail to parse and the lint job
  will need a v1-format rewrite (`linters.disable-all` + explicit
  `enable`, no `formatters` section, `gofmt`/`goimports` listed directly
  under `linters.enable` instead).
- `examples/basic` was not actually run (`go run ./examples/basic`) — no
  live network validation this session. This is still the single most
  important outstanding gap carried over from Session 1.
- No push to `origin/main` yet — commit `bd179a0` is local only.

## Next Session

1. **Highest priority, carried over from Session 1 twice now:** run
   `go run ./examples/basic` from a real residential IP (not this sandbox)
   to confirm DuckDuckGo returns real results, then capture that real
   response HTML as a `testdata/duckduckgo/` regression fixture so
   `parse()` is validated against reality instead of only the synthetic
   fixture. This is Phase 1's actual exit criterion per `plan.md` line 82-83
   and has not been done in either session.
2. **Verify CI actually runs green on GitHub** once pushed — first real PR
   or push to `main` will reveal whether `golangci-lint-action@v6` +
   `version: latest` picks v1 or v2, and whether `.golangci.yml` needs the
   format rewrite noted above. Push `bd179a0` (and get user confirmation
   first, since pushing affects the shared remote).
3. **Then Phase 2 (Google provider)** — still blocked on having a real,
   non-datacenter-blocked Google success page to write the parser against;
   do not start this until #1 above produces (or a separate residential
   capture provides) that fixture.
4. Nothing was deliberately descoped this session beyond what Session 1
   already deferred (Google/Yandex providers, Phase 4/5) — this was a
   narrowly-scoped "finish the Phase 1 checklist" session.

---

# Handoff — 2026-07-29 (Session 1) — Project bootstrap + Phase 1 core (skeleton, httpclient, DuckDuckGo, readability, orchestration)

## Summary

First session for `gosearch`, a new zero-API-key Go library for web search
(Google/Yandex/DuckDuckGo via direct HTML scraping) and page-content
extraction, extracted as a standalone project (it will also be consumed by the
Memo project, but is designed to be general-purpose). We agreed the
architecture and phased roadmap (`plan.md`), initialized the module + public
GitHub repo, and implemented essentially all of **Phase 1**: core public
types, the shared browser-like HTTP client with anti-bot block detection, the
DuckDuckGo provider, the readability content extractor, and the root
`Search`/`Fetch` orchestration with the fallback chain. Everything is
fixture-based tested and green.

Design constraint locked in: gosearch behaves like an honest browser
(realistic headers, cookie jar, rate limiting) but will **never** solve
CAPTCHAs, run JS challenges, or spoof identity to defeat anti-bot controls.
A separate opt-in `gosearch/browser` subpackage for real-browser rendering is
planned (Phase 5) but not started.

**Commit status:** all work committed and pushed to
`https://github.com/BugraAkdemir/gosearch` (main). Commits this session:
`6e95326` (scaffolding/design docs), `2bd9593` (fill AGENTS/BUG_REPORT),
`ce17564` (core types/errors/options), `7cd8898` (httpclient + detection),
`ff9bd0c` (duckduckgo + htmlx), `ec60ea5` (readability), `37ec6bf`
(orchestration). Working tree clean except this handoff edit.

---

## What Was Done

### 1. Project bootstrap
- `git init`, `go mod init github.com/BugraAkdemir/gosearch`, MIT `LICENSE`,
  `.gitignore`, public README, `CONTRIBUTING.md`, and a detailed `plan.md`
  (5-phase roadmap incl. the browser-subpackage runtime-download and
  `embedbrowser` embed models).
- Created the public GitHub repo via `gh` and pushed; added repo topics.
- Filled `AGENTS.md` with real facts: Conventional Commits, **no AI
  attribution** in commits, zero-dependency discipline (only
  `golang.org/x/net/html`), verification commands, anti-bot Known Pitfalls.

### 2. Phase 1 implementation

| File | Change |
|------|--------|
| `result.go`, `errors.go`, `engine.go`, `options.go` | Public types: `Result`, `Page`, `Engine` enum, sentinel errors (re-exported from `internal/serrors` to dodge an import cycle), unified `Option` type + `config`. |
| `internal/httpclient/client.go` | Shared client: realistic Chrome headers, cookie jar (domain-seeded), per-host rate limiter honoring ctx, 8 MiB body cap. Deliberately does NOT set Accept-Encoding (lets Go handle gzip). |
| `internal/httpclient/detect.go` | `Detect()` → ErrChallenge/ErrBlocked/nil from status, redirect target, per-engine markers. |
| `internal/htmlx/htmlx.go` | Shared DOM helpers (Attr/HasClass/Tag/Text/Walk/Find*). |
| `internal/provider/provider.go` | Internal `Result` type providers return. |
| `internal/providers/duckduckgo/duckduckgo.go` | Parses `html.duckduckgo.com/html`, decodes `uddg` redirect links. |
| `internal/readability/readability.go` | Noise-strip + container-scoring content extractor → `Article{Title, Content}`. |
| `gosearch.go` | `Search`/`Fetch`, test-overridable `dispatch` var, fallback chain (advance only on block/challenge; join on all-blocked). |
| `testdata/` | Real captured block pages (DDG captcha, Google enablejs) + synthetic success/article fixtures. |

**Root cause note (test flake avoided):** first `Accept-Encoding` assertion
was wrong — Go's transport transparently adds `gzip`, so the server sees it;
fixed the test to assert transparent gzip decoding instead of an unset header.

**Deliberately not done / out of scope:**
- **Google & Yandex providers (Phase 2/3):** not implemented. `Search` with
  those engines returns an unexported not-implemented error. They were
  deferred on purpose because all three engines blocked this build's
  datacenter IP, so we have **no real successful HTML capture** to write/verify
  their parsers against — that must come from a residential IP first (see
  plan.md Phase 2/3 exit criteria).
- **CI workflow + golangci config** (`.github/workflows/ci.yml`,
  `.golangci.yml`) and **`examples/basic/main.go`**: remaining Phase 1 items,
  not yet written.
- Phase 4 (retry/backoff) and Phase 5 (browser subpackage): future.

---

## Verification

```bash
$ go build ./...   # (no output = ok)
$ go vet ./...     # (no output = ok)
$ gofmt -l .       # (no output = all formatted)
$ go test ./...
ok  	github.com/BugraAkdemir/gosearch	0.004s
?   	github.com/BugraAkdemir/gosearch/internal/htmlx	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	0.278s
?   	github.com/BugraAkdemir/gosearch/internal/provider	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	0.005s
ok  	github.com/BugraAkdemir/gosearch/internal/readability	0.002s
?   	github.com/BugraAkdemir/gosearch/internal/serrors	[no test files]
```

`go test -race ./...` was also run green throughout the session.

**NOT verified:** no live end-to-end run against the real DuckDuckGo endpoint
from a residential IP. All provider tests are fixture-based (by design, for
determinism). The DuckDuckGo `parse()` is validated only against our
*synthetic* success fixture, not a real DuckDuckGo success page — the real
markup should be spot-checked from a residential IP and captured as a
regression fixture. This is the single most important thing to confirm before
trusting `Search` in the wild.

---

## Next Session

1. **Finish Phase 1:** write `examples/basic/main.go` (Search + Fetch demo)
   and the CI workflow (`.github/workflows/ci.yml`: build, vet, gofmt check,
   golangci-lint, `go test -race`) + `.golangci.yml`. Then run the example
   from Bugra's own (residential) machine to confirm DuckDuckGo returns real
   results — and capture that real HTML as a `testdata/duckduckgo/` regression
   fixture to validate `parse()` against reality.
2. **Then Phase 2 (Google provider):** but only after capturing a real Google
   success page from a residential IP — do not write the parser blind against
   the datacenter-blocked response we have.
3. **Watch:** `go.mod` requires `go 1.25` (forced by `x/net v0.57.0`). If
   broader Go-version compatibility becomes a goal, that means pinning an
   older `x/net` — noted in AGENTS.md Tech Stack.
