# Contributing / Agent Working Files

This repository keeps three files that a coding agent (Claude Code or
similar) — or a human contributor — uses to stay reliable across long,
multi-session work:

| File | Purpose |
|------|---------|
| `AGENTS.md` | Project map, working rules, verification commands, and a running "Known Pitfalls" log. Read at the start of every session. |
| `handoff.md` | Chronological session log — newest entry on top. Written at the end of every session so context survives to the next one. |
| `BUG_REPORT.md` | The list of bugs that are open *right now*. Fixed bugs are deleted from it, not archived here — their permanent record is git history and `AGENTS.md`. |
| `plan.md` | The current step-by-step implementation roadmap. Follow it in order; update it rather than silently diverging if it turns out to be wrong. |

## Process rules to keep close to verbatim

These aren't project facts; they're what keeps the other files trustworthy
after months of sessions instead of rotting into stale claims nobody
re-verifies:

- The **Agent Working Rules** and **Verification Commands** sections in
  `AGENTS.md`.
- The `~~struck~~ → fixed` convention in `AGENTS.md`'s Known Pitfalls log.
- The "prepend, one entry per session, always end with Next Session" rule in
  `handoff.md`.
- **Documentation is not optional** (`AGENTS.md` Agent Working Rules #9):
  every exported symbol gets a real doc comment, and public API changes are
  reflected in `README.md`'s examples in the same commit.

## Before opening a PR

1. `go build ./...`, `go vet ./...`, `gofmt -l .` (must print nothing), and
   `go test -race ./...` must all be clean — see `AGENTS.md`'s Verification
   Commands section for the exact invocations.
2. If you touched provider parsing logic, note in `AGENTS.md`'s Known
   Pitfalls whether you validated it against a live engine response (and
   from what kind of network — residential vs. datacenter IPs behave very
   differently against these providers, see `plan.md`'s Context section).
