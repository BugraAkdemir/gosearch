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
