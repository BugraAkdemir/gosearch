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

## Session 3, leg 5 (same day) — post-release CI failure caught and fixed forward

Checking CI after the v0.1.0 push surfaced a red run: golangci-lint (revive)
rejected the lowercase doc comments on the newly exported `Endpoint` vars in
all three provider packages ("comment on exported var Endpoint should be of
the form 'Endpoint ...'"). Cosmetic only — no code behavior — but main was
red on top of a published release.

**Decision:** fix forward; never move/retag a published tag (proxy pins by
hash, and AGENTS.md treats tags as effectively permanent). v0.1.0 remains
valid: it builds, tests green, and the finding is doc-comment style.

- Fix: `9fbbbba fix(lint): capitalize Endpoint var doc comments` → CI run
  #8 SUCCESS (verified via `gh run list`; api.github.com was flaky from this
  sandbox mid-session — the Actions web page worked as fallback evidence).
- **golangci-lint v2.13.1 is NOW INSTALLED LOCALLY** at
  `$(go env GOPATH)/bin/golangci-lint` — future sessions must include
  `golangci-lint run` in the verification pass (AGENTS.md always said "if
  installed locally"; it is now).

## Session 3, leg 4 (same day) — v0.1.0 tagged, pushed, published

The user explicitly authorized the tag/release in this moment (satisfying the
hard rule in AGENTS.md). Steps taken:

- Pre-release doc refresh first (`552190a`): README status block → "Status:
  v0.1", AGENTS.md Release section now documents the actual process
  (annotated tag on main, `git push origin main --follow-tags`, no separate
  publish step) instead of "no release process yet".
- Full verification suite re-run green right before tagging.
- Annotated tag `v0.1.0` created on the docs commit; pushed with main.
- Go module proxy triggered + verified via escaped path
  `proxy.golang.org/github.com/!bugra!akdemir/gosearch/@v/v0.1.0.info` →
  returned Version v0.1.0, Hash 552190a9… (the exact tagged commit). Note:
  unescaped module paths in proxy URLs return "invalid escaped module path"
  because of the capitals in the owner name.
- GitHub release published with notes:
  https://github.com/BugraAkdemir/gosearch/releases/tag/v0.1.0

**State:** v0.1.0 is public and effectively permanent on the Go module proxy.
pkg.go.dev will index it automatically.

**Next Session:** nothing blocking v0.1. Remaining candidates: (a) user-supplied
Google/Yandex real captures to close those exit criteria, (b) optional Phase 5
`gosearch/browser` subpackage, (c) any post-release issues surfaced by early
users. Do not retag/re-cut releases without an explicit fresh user ask.

## Session 3, leg 3 (same day) — Phase 4 (Hardening) complete; only Phase 5 optional remains

Phase 4 items, all verified:

- **Retry/backoff** (`4ef3e52`): `httpclient.Get` refactored into a retry
  loop (`getOnce` per attempt) over transport errors and HTTP 408/5xx,
  doubling backoff capped at 5s; every attempt passes the per-host rate
  limiter; ctx cancellation never retried. A request ending on a transient
  status now returns an error instead of handing a server-error page to a
  parser. Blocks/challenges deliberately NOT retried (deterministic for IP
  reputation; fallback's job). Root option `WithRetries(n)` — default 2,
  `0`/negative disables. Tests: transient-then-success, give-up-after-N,
  no-retry-on-403, disabled-retries, backoffFor table, root option clamping.
- **WithFallback end-to-end** (`d7efa3b`): new `internal/e2e` package runs
  public API → dispatch → REAL providers against httptest servers. Requires
  each provider's endpoint var to be exported (`endpoint` → `Endpoint`,
  internal-only surface change done via ast_edit across 6 files). Confirms:
  primary blocked → next engine succeeds and later engines are never
  consulted; all engines blocked → errors.Join preserves both ErrBlocked
  and ErrChallenge through errors.Is.
- **Docs**: README "Tuning and escape hatches" (retries/proxy/cookies/
  custom client) + explicit non-goals section; `docs/API.md` options table
  gained `WithRetries`.
- **Final pass**: AGENTS.md Known Open Work refreshed (Phases 1–3 shipped,
  Phase 4 active), BUG_REPORT.md last-updated line current, plan.md Phase 4
  boxes ticked.

**Verification (pasted):**

```
$ go build ./... && go vet ./... && gofmt -l . ; go test -race -count=1 ./...
ok  	github.com/BugraAkdemir/gosearch	1.041s
ok  	github.com/BugraAkdemir/gosearch/internal/e2e	1.059s
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	1.444s
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	1.057s
ok  	github.com/BugraAkdemir/gosearch/internal/providers/google	1.064s
ok  	github.com/BugraAkdemir/gosearch/internal/providers/yandex	1.054s
ok  	github.com/BugraAkdemir/gosearch/internal/readability	1.029s
(htmlx/provider/serrors: no test files; gofmt printed nothing)
```

**Commits:** `4ef3e52 feat(httpclient)`, `d7efa3b test(e2e)`, plus this docs
commit.

**Next Session:** v0.1 core roadmap is DONE through Phase 4. Remaining work
is either (a) user-supplied real captures for Google/Yandex
(`testdata/{google,yandex}/real_success.html` — regression tests self-activate
and close those exit criteria), or (b) optional Phase 5 `gosearch/browser`
subpackage (separate module/build tag; read plan.md's full design first).
A release process decision (tagging v0.1) needs explicit user ask — see the
hard rule in AGENTS.md.

## Session 3, leg 2 (same day) — Phase 3 (Yandex) built; all engines wired

Continued straight down plan.md into Phase 3. `internal/providers/yandex/`
(`yandex.go` + `yandex_test.go`): `li.serp-item` containers scope every lookup
so adjacent results cannot leak into each other; titles from `h2` inside
`a.organic__url`; snippets from `organic__text`. `cleanURL` promotes
protocol-relative hrefs to https, unwraps `/clck/` click-trackers only when a
decodable destination rides in a query param (`url`/`u`/`www`), and rejects
Yandex-internal destinations (relative + absolute `/search/` pagination,
`/passport` path AND `passport.`/`captcha.` hosts — the host check exists
because the real failing case was `passport.yandex.ru/auth`, whose *path*
carries no passport prefix). Dedup + maxResults cap + ErrNoResults match the
other providers.

Tests: synthetic `testdata/yandex/success.html` (direct + protocol-relative
hrefs, pagination and undecodable-clck items that must be skipped), cleanURL
table, dedup, maxResults, end-to-end httptest success (asserts the query goes
in the `text` param), NoResults, and the plan-specified mocked
302-to-showcaptcha block test (handler terminates at `/showcaptcha` with a
200 body, mirroring how the live engine serves the captcha page at that URL;
Detect classifies via FinalURL). `TestParseRealSuccessFixture` follows the
Google skip-until-capture pattern against `testdata/yandex/real_success.html`.

Cleanup: `errNotImplemented` and its dispatch branch are gone — all three
engines are wired; `TestSearchNotImplementedEngines` deleted with them.
README status block → "Phase 3", `docs/API.md` bullet rewritten,
`docs/ARCHITECTURE.md` provider-status table updated (the old table still
claimed Google/Yandex "Not implemented"), Phase 3 plan items ticked.

**Commits:** `0ead2ef feat(providers): yandex`, plus this docs commit.

**Verification (pasted):**

```
$ go build ./... && go vet ./... && gofmt -l . && go test -race -count=1 ./...
ok  	github.com/BugraAkdemir/gosearch	1.040s
?  	github.com/BugraAkdemir/gosearch/examples/basic	[no test files]
?  	github.com/BugraAkdemir/gosearch/internal/htmlx	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	1.497s
?  	github.com/BugraAkdemir/gosearch/internal/provider	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	1.058s
ok  	github.com/BugraAkdemir/gosearch/internal/providers/google	1.057s
ok  	github.com/BugraAkdemir/gosearch/internal/providers/yandex	1.054s
ok  	github.com/BugraAkdemir/gosearch/internal/readability	1.035s
?  	github.com/BugraAkdemir/gosearch/internal/serrors	[no test files]
```
(gofmt printed nothing.)

**Next Session:** Phase 4 — Hardening: retry/backoff on transient failures,
end-to-end `WithFallback` confirmation, README per-engine reliability +
custom-client docs, final AGENTS.md/BUG_REPORT.md pass. Still pending from
Phases 2/3 exit criteria (user action, one-time): drop real captures from a
trusted network at `testdata/google/real_success.html` and
`testdata/yandex/real_success.html` — both regression tests activate
automatically and close those criteria.

## Session 3 (2026-08-23) — CI-vs-residential question answered; Phase 2 (Google) built and green

The user asked whether Google/Yandex real-success validation could be done on
CI instead of switching networks. Answer delivered with evidence: a throwaway
`.capture/main.go` probe (written, used, deleted — not committed) hit both
engines live from this sandbox — Google returned the `enablejs` JS-challenge
with HTTP 200, Yandex 302'd to `showcaptcha`, same as 2026-07-29. GitHub
Actions runners are datacenter IPs too (worse reputation), so a live-capture
CI job would not produce success fixtures either. The residential requirement
is one-time manual capture only (curl or DevTools response copy from any
trusted network → `testdata/<engine>/real_success.html`); all validation
afterwards is offline in CI by design.

Since the plan explicitly decouples provider code from the pending capture,
Phase 2 was implemented this session:

- `internal/providers/google/` (`google.go` + `google_test.go`):
  `/url?q=` redirect decoding, h3-inside-or-wrapping-link title heuristic,
  container-scoped snippet lookup (g/xpd/Gx5Zad/MjjYud classes, s3v9rd/st
  snippet blocks), dedup + pagination-link filtering, maxResults cap,
  ErrNoResults convention identical to DuckDuckGo's.
- `testdata/google/success.html`: synthetic modern basic-HTML fixture;
  legacy `h3.r > a` + `span.st` shape covered by an inline test case.
- `TestParseRealSuccessFixture` pre-wired for the future residential
  capture; skips while `testdata/google/real_success.html` is absent.
- Latent bug found & fixed: `TestSearchNotImplementedEngines` in
  `orchestration_test.go` called the REAL dispatch for engines without
  providers and was safe only because `errNotImplemented` short-circuited
  pre-HTTP; wiring Google made it hit the live web in unit tests (observed as
  a 1.1s test that received a real challenge page). Restricted to Yandex with
  a warning comment.
- Dispatch wired in `gosearch.go`; stale doc comments updated; README,
  `docs/API.md`, `plan.md` (Phase 2 items ticked), AGENTS.md pitfalls
  (re-probe logged) updated in the same change.

**Commit:** `cccd447 feat(providers): google` on main (plus the docs commit
carrying this entry).

**Verification (pasted):**

```
$ go build ./... && go vet ./... && gofmt -l . && go test -race ./...
ok  	github.com/BugraAkdemir/gosearch	(cached)
?  	github.com/BugraAkdemir/gosearch/examples/basic	[no test files]
?  	github.com/BugraAkdemir/gosearch/internal/htmlx	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	(cached)
?  	github.com/BugraAkdemir/gosearch/internal/provider	[no test files]
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	(cached)
ok  	github.com/BugraAkdemir/gosearch/internal/providers/google	(cached)
ok  	github.com/BugraAkdemir/gosearch/internal/readability	(cached)
?  	github.com/BugraAkdemir/gosearch/internal/serrors	[no test files]
```
(gofmt printed nothing; golangci-lint not installed locally — CI runs it.)

**Next Session:** Phase 3 — build `internal/providers/yandex` against the
`serp-item`/organic-result heuristic with a mocked `showcaptchafast` 302 +
synthetic success fixture (no real blocked fixture needed, see plan.md).
Independent of that: whenever a trusted-network capture of Google's basic
result HTML lands at `testdata/google/real_success.html`,
`TestParseRealSuccessFixture` activates and closes Phase 2's exit criterion;
adjust the heuristic if the real markup diverges (expected in some details).

## Session 2, leg 2 (same day) — Live validation closes Phase 1's real exit criterion

After the leg below, the user asked me to actually run the example myself
rather than hand it off. `go run ./examples/basic` succeeded from this
session's sandbox against the live `html.duckduckgo.com/html/` endpoint: a
clean HTTP 200 with 10 real results for "facebook", no captcha/anomaly
markup — a different outcome than the 2026-07-29 design-phase finding that
all three engines blocked this project's sandbox on first contact (see
`AGENTS.md` Known Pitfalls). Captured the real response body via a
throwaway `.capture/main.go` (written, used, then deleted — never
committed) and saved it as `testdata/duckduckgo/real_success.html`. Added
`TestParseRealSuccessFixture` in `duckduckgo_test.go` asserting structural
properties (10 results, non-empty Title/URL, uddg redirects decoded,
`results[0].URL == "https://www.facebook.com/"`) against the real capture —
deliberately not asserting exact snippet prose, since that can drift on a
future re-capture even though the markup shape stays stable.

Checked the captured HTML for anything sensitive before committing it
(grepped for `vqd=`, `token`, `session` — none found) since it's a
real server response now permanently in git history.

Updated `AGENTS.md`'s Known Pitfalls with a dated note that DuckDuckGo is
now re-validated (Google/Yandex are not — that part of the original finding
still stands) and checked off `plan.md`'s Phase 1 exit-criteria line.

**Commit:** `bf9c5a0` on `main`, local only (3 commits ahead of origin now:
`bd179a0`, `b45f413`, `bf9c5a0`). Not pushed — need to ask the user before
pushing, per the standing rule about actions visible to others.

**Verification:**
```
$ go build ./... && go vet ./... && gofmt -l . && go test -race ./...
ok  	github.com/BugraAkdemir/gosearch	(cached)
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	(cached)
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	1.019s
ok  	github.com/BugraAkdemir/gosearch/internal/readability	(cached)
```

**Next Session:**
1. Ask the user whether to push `bd179a0`/`b45f413`/`bf9c5a0` to
   `origin/main` — still not done.
2. `golangci-lint` config (`.golangci.yml`, v2 format) is still unverified
   locally — first real CI run on GitHub will tell if the format guess
   (`golangci-lint-action@v6` + `version: latest` → v2 binary) was right.
3. Google/Yandex providers (Phase 2/3) remain correctly blocked on a real,
   non-datacenter-blocked capture of *their* success pages — DuckDuckGo's
   success here doesn't transfer to them; each engine needs its own real
   capture before its parser is trusted.

---

## Session 2, leg 4 (same day) — Pushed; CI actually failed twice, now genuinely green; Google/Yandex re-confirmed blocked

User asked to proceed to the next step, then approved pushing. What
actually happened once real CI ran (this is why "should pass" claims
without running it are worthless — see AGENTS.md rule #3):

**Attempt 1** (`bd179a0`..`4885326`, 6 commits): CI failed at the Lint step.
`golangci-lint-action@v6` with `version: latest` installed **v1.64.8**
(built with go1.24), which refuses to lint a module declaring `go 1.25.0`
("the Go language version (go1.24) used to build golangci-lint is lower
than the targeted Go version"). Not a `.golangci.yml` v1/v2 format problem
as the earlier handoff speculated — a binary-version problem.

**Attempt 2** (`a70dcb8`): Pinned `version: v2.13.1` (confirmed locally: built
with go1.27, satisfies go1.25). Also fixed the 7 real lint findings that
surfaced once the linter could actually run — downloaded the v2.13.1 binary
locally and ran it before pushing again:
- `internal/httpclient/client.go:172` — unchecked `resp.Body.Close()`
  (errcheck) → wrapped in `defer func() { _ = resp.Body.Close() }()`.
- 6× `revive` unused-parameter warnings across
  `internal/httpclient/client_test.go`, `internal/providers/duckduckgo/duckduckgo_test.go`,
  and `orchestration_test.go` — unused `httptest.HandlerFunc` params (`w`
  or `r`) renamed to `_`.

CI still failed: `golangci-lint-action@v6` **hard-rejects golangci-lint v2
entirely** ("golangci-lint v2 is not supported by golangci-lint-action v6,
you must update to golangci-lint-action v7").

**Attempt 3** (`cbf31d6`): Bumped the action to `@v9` (current major, not
just the minimum `@v7`). **This one passed.**

**Attempt 4** (`5383e1f`, cosmetic): the passing run still carried a
Node.js 20 deprecation annotation from `actions/checkout@v4` and
`actions/setup-go@v5`. Bumped both to their current majors (`@v7` each).
Passed clean, no annotations.

Also used this leg to re-confirm, with fresh captures, that **Google and
Yandex are still blocked from this sandbox** — same outcome as the
2026-07-29 design-phase finding, now re-verified same-day as DuckDuckGo's
success:
- Google (`https://www.google.com/search?q=facebook`): 200 OK but body is
  the `/httpservice/retry/enablejs` JS-required redirect page — confirmed
  byte-identical in shape to the existing `testdata/google/blocked.html`
  fixture (different nonce/session id only). `httpclient.Detect` correctly
  classifies it as `ErrChallenge`.
- Yandex (`https://yandex.com/search/?text=facebook`): 200 status but
  `FinalURL` redirected to `/showcaptcha` with `X-Yandex-Captcha: captcha`
  header present — `httpclient.Detect` correctly classifies as
  `ErrChallenge`.

Both capture attempts used a throwaway `.capture/main.go` (same pattern as
leg 2's DuckDuckGo capture): written, run, deleted, never committed. Did
not save these as new fixtures since they're materially identical to what
`testdata/google/blocked.html` already covers — no new regression coverage
to gain from a second nonce of the same JS-challenge page.

User chose (via AskUserQuestion) to keep Phase 2/3 blocked rather than
pursue a residential capture or switch to unrelated work this session —
confirmed by the "push it" answer that followed.

**Commit status:** `5383e1f` on `main`, **pushed to origin** — this is the
first push of the session; all four prior handoff legs' work is now live.

**Verification (local, before each push):**
```bash
$ go build ./... && go vet ./... && gofmt -l . && go test -race ./...
ok  	github.com/BugraAkdemir/gosearch	1.015s
ok  	github.com/BugraAkdemir/gosearch/internal/httpclient	1.379s
ok  	github.com/BugraAkdemir/gosearch/internal/providers/duckduckgo	1.024s
ok  	github.com/BugraAkdemir/gosearch/internal/readability	(cached)
```
**Verification (CI, actual GitHub Actions runs, not assumed):**
- `32636532133` (attempt 1): ❌ failure — Lint step, golangci-lint binary/Go-version mismatch
- `32636739726` (attempt 2): ❌ failure — Lint step, action v6 rejects golangci-lint v2 outright
- `32636797726` (attempt 3): ✅ success — 48s, 1 Node.js-20-deprecation annotation (non-fatal)
- `32636878289` (attempt 4): ✅ success — 59s, zero annotations

Updated `AGENTS.md`'s CI section with a note that `golangci-lint-action`
major version and `.golangci.yml`'s v1/v2 format must move together (v6
cannot run a v2 config's binary at all, regardless of format correctness).

## Next Session

1. Phase 2/3 (Google/Yandex providers) remain correctly blocked — need a
   real, non-JS-challenge capture from a residential IP for each. Nothing
   in this sandbox can produce that; needs the user's own network.
2. No other open items from this session. The golangci-lint config format
   question flagged as "untested" in leg 1's handoff is now resolved: the
   v2 format itself was never the problem, only the action major version.

---

## Session 2, leg 3 (same day) — docs/ (API reference + architecture)

User asked for `docs/` + confirmed CI was already in place. Added:

- `docs/API.md` — human-readable companion to `go doc -all .`: Search/Fetch
  contracts, all Option/Engine/Result/Page/error semantics, in one page.
- `docs/ARCHITECTURE.md` — package graph, why the `internal/serrors` and
  `internal/provider` split exists (import-cycle avoidance), Search/Fetch
  request flow, block-detection order-of-checks, and a Provider status table.

Both are explicitly framed as companions to `go doc`, not replacements —
source doc comments stay authoritative. Linked both from `README.md`'s new
"Documentation" section and added a `docs/` row to `AGENTS.md`'s Module Map.

**Commit:** `edd4b4d` on `main`, local only (5 commits ahead of origin now:
`bd179a0`, `b45f413`, `bf9c5a0`, `3192d37`, `edd4b4d`). Still not pushed —
user has twice declined to push this session; ask again next session rather
than assuming.

**Verification:** `go build/vet/gofmt/test -race` all green (no code
changed, docs-only commit).

**Next Session:** same three items as leg 2's handoff (push decision,
golangci-lint format unverified until real CI run, Google/Yandex still
blocked on their own real captures) — nothing new opened this leg.

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
