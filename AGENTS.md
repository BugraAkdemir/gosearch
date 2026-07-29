# AGENTS.md — <Project Name>

> **How to use this template:** copy this file to `AGENTS.md` in a new project and fill in every `<...>` placeholder. Delete sections that don't apply (e.g. "Mobile" if there's no mobile client) rather than leaving them empty. The **Agent Working Rules** and **Verification Commands** sections are the part worth keeping closest to verbatim — they encode a working process, not project facts, and are what actually keeps a coding agent reliable across long-running, multi-session work.

<One or two sentences: what the product is, who it's for, and the core architectural bet (e.g. "local-first," "offline-capable," "multi-tenant SaaS").>

---

## Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Backend | `<e.g. Go, Node, Python>` | `<version>` |
| Frontend | `<e.g. React, Flutter, SwiftUI>` | `<version>` |
| State management | `<e.g. Redux, Riverpod, Zustand>` | `<version>` |
| HTTP client | `<library>` | `<version>` |
| Database | `<e.g. Postgres, SQLite>` | `<version>` |
| Cache / queue | `<e.g. Redis, none>` | `<version>` |

---

## Architecture

<One paragraph: process/service topology (monolith? two-process client-server? microservices?), how components talk to each other (REST/gRPC/SSE/WebSocket), and any deliberate decoupling pattern worth naming (e.g. a bridge/adapter interface between layers).>

### Module Map

| Directory | Responsibility | Key Files |
|-----------|---------------|-----------|
| `<path/>` | `<what it owns>` | `<file.ext>`, `<file.ext>` |
| `<path/>` | `<what it owns>` | `<file.ext>` |

### Entry Points

`<how the app boots — main.go / index.ts / App.swift — and what it wires together before handing off to the framework's own runtime.>`

---

## Data & Config Layout

| Path | Purpose |
|------|---------|
| `<config path>` | All runtime settings |
| `<data dir>/<subdir>` | `<what's stored there>` |
| `.env` | Secrets / environment overrides |

---

## Development

### Quick Start

```bash
<install deps>
<run dev server / dev client>
```

### Build

```bash
<production build command(s), one per deployable artifact>
```

### Testing

```bash
<unit test command>
<lint/analyze command>
<e2e/integration test command, if any>
```

CI: `<what runs on push/PR and where — e.g. "GitHub Actions runs lint + unit tests on every push">`.

### Release

`<Pointer to a dedicated release process/checklist if one exists (versioned files to bump, changelog format, artifact naming, publish targets) — don't inline the whole checklist here if it's long; link to a skill/script/doc instead, and reference it, the way this section does.>`

**Hard rule:** `<any release/deploy action that is real, visible, and hard to reverse — e.g. "never push a version tag / trigger a deploy without the user explicitly asking for it in that specific moment.">`

---

## Known Pitfalls & Technical Debt

> This section is a **living log**, not a wiki page. Follow the convention below religiously — it's what makes this file trustworthy after months of sessions instead of just accumulating stale claims:
>
> - When you find a bug, add a bullet describing the **symptom and root cause**, in plain language, as if explaining it to the next agent who has zero context.
> - When it's fixed, don't delete the bullet — wrap the original description in `~~strikethrough~~` and append `→ fixed <date> (<commit-ish>): <what changed and why, plus any residual risk or what's *not* covered>`.
> - If a claim turns out to be **stale or was never true** (re-verified and the described code no longer exists, or never behaved that way), mark it `→ stale: <what you actually found>` rather than silently deleting it — the correction itself is information for the next session.
> - Group entries under a `### <Module/Area>` heading per subsystem, with a date if the finding is time-sensitive.
> - Never re-describe a fix that's already fully captured by `git log`/commit messages — link to the commit hash instead of duplicating the diff in prose.

### `<Module A>` (`<path/>`)
- <Open or fixed issue, following the convention above.>

### `<Module B>` (`<path/>`)
- <Open or fixed issue, following the convention above.>

### Security
- <Any hardening work worth remembering — auth gaps closed, secrets handling, rate limiting, path traversal, etc.>

---

## Agent Working Rules (READ FIRST, EVERY SESSION)

1. **Start of session:** read this file, then `handoff.md` (top entry = last session's state and pending work).
2. **End of session:** prepend a new entry to `handoff.md` (what was done, commit status, verification results, what's next). This is how context survives between sessions and models — treat it as the primary handoff mechanism, not an afterthought.
3. **Never claim "done" without running the verification commands below and pasting the actual results.** A claim unbacked by a real command's output is not verification.
4. Plan files (`plan.md`, `PLAN_*.md`) contain step-by-step implementation plans — follow them in order, tick items off as completed, don't improvise a different architecture mid-plan. If the plan turns out to be wrong, say so and update the plan file rather than silently diverging from it.
5. Work in small units: a bounded number of plan items per session, each with its own tests, verified green before moving to the next.
6. **Commit automatically, without asking for confirmation, once a fix/feature is verified green.** Don't ask "should I commit?" — just commit, using this project's commit convention (`<e.g. Conventional Commits: fix(scope): ..., feat(scope): ...>`), with a body that explains the *why* (root cause, what changed, what it fixes) — and `<state the project's actual attribution policy here explicitly, e.g. "never include an AI-attribution / Co-Authored-By line, under any circumstance" or the opposite if the project wants one — this must be an explicit, stated choice, not left implicit>`. **Commit frequently, not just once per finished task** — break a multi-step request into checkpoints and commit after each one once it's verified, especially right before a risky step (a refactor touching a hot/shared code path, a change you're not fully certain about). Finer-grained history means a bad step can be bisected/reverted without losing unrelated good work from the same session. Don't over-fragment either — this is about natural checkpoints, not every file save.
7. **Code exploration:** `<if the project has a code-graph/indexing tool available, name it here and state when to prefer it over blind grepping — e.g. "use search_graph/trace_path for 'who calls X' questions before grepping". Delete this rule if no such tooling exists.>`
8. `<Any zero-tolerance project convention worth stating as a hard rule — e.g. "all user-facing strings go through the i18n layer, no exceptions" or "no direct SQL writes outside the serialized write path.">` State *why* it's a hard rule (what shipped bug it prevents) so a future agent understands it's not just style preference.
9. **Documentation is not optional.** Every exported type, function, and package gets a real Go doc comment (what it does and any non-obvious behavior — not a restatement of the name). When a change adds or changes public API, exported error types, or provider/fallback behavior, update `README.md`'s usage examples in the same commit — a doc that describes a signature the code no longer has is worse than no doc. Package-level docs (`doc.go` or a top-of-file comment) must state what the package is for and its actual limitations (e.g. anti-bot fragility per provider), not just what it does when things go well.

### Verification Commands (mandatory before any "done" claim)

```bash
# Backend
<build command>
<lint/vet command>
<test command, with -race or equivalent if the language supports it>

# Frontend
<build/analyze command>
<test command>

# Project-specific convention checks (delete if not applicable)
<e.g. a grep-based check for hardcoded UI strings, or a schema-drift check>
```

Acceptable pre-existing noise: `<list any known, accepted lint warnings that aren't worth chasing, so the agent doesn't waste a session "fixing" something already triaged as fine>`. Anything else new must be addressed before claiming done.

---

## Gotchas (project-specific traps — violating these causes real, shipped bugs)

> Keep this section brutally concrete. Each bullet should name the exact file/pattern and the exact failure mode it caused — "be careful with concurrency" is useless; "X reads Y without holding Z's mutex, causing a data race confirmed under `-race` on <date>" is useful.

**Paths & data**
- <e.g. "All data paths go through a single accessor function — never hardcode a data directory; it differs per OS/deployment target.">

**Concurrency & architecture**
- <e.g. "All writes to the shared store go through a single serialized write path — calling the raw driver directly bypasses it and has corrupted data before.">
- <e.g. "This resource is swapped at runtime under a specific mutex — always take that mutex before touching it.">

**Streaming / real-time**
- <e.g. "Frontend stream timeout must match the backend's generation budget exactly — a mismatch here has silently aborted valid slow responses before.">
- <e.g. "Any 'is this operation still in-progress' flag must be cleared by *every* exit path of the function that sets it — a branch that returns early without clearing it leaves the UI stuck. Grep every branch, not just the happy path, before declaring a fix complete.">

**Framework-specific**
- <e.g. React/Riverpod/Vue-specific footguns this project has actually hit — instance reuse across rebuilds, stale closures, etc.>

**Types & misc**
- <e.g. "Two same-named types in different packages/modules are NOT interchangeable — they don't cross-assign.">
- <Any intentional, non-obvious product decision that looks like a bug but isn't (e.g. mixed-language UI is intentional for the target audience) — state it here so it isn't "fixed" by mistake.>

---

## Known Open Work

Open bugs and technical debt are tracked in **`BUG_REPORT.md`** — don't duplicate that list here. As of `<date>` it has `<N>` open items.

---

## Code Style

- Errors are values, never panics across package boundaries. Providers return typed sentinel errors (`ErrBlocked`, `ErrChallenge`, `ErrNoResults`) wrapped with context via `fmt.Errorf("%w: ...")` — callers `errors.Is` against the sentinel, never string-match an error message.
- No third-party dependency beyond `golang.org/x/net/html` without an explicit decision recorded here — the whole point of this library is staying close to zero-dependency for people who can't/won't pull in an API SDK.
- Every exported symbol has a doc comment (see Agent Working Rules #9). `godoc`/`go doc ./...` should read as a complete usage guide on its own, not just a name restatement.
- Provider packages under `internal/providers/` are intentionally unexported — the public surface is the root package's `Search`/`Fetch` plus the `Engine` enum. Don't promote a provider package to public API without a deliberate decision recorded here first.
