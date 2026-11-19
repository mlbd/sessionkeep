# Decision register

Status: **required** = directly established user requirement; **provisional** = proposed baseline awaiting owner/Phase 0; **open** = unresolved. Use `templates/ADR.md` for changes. Do not overwrite prior decision history.

| ID | Decision | Status / reason |
|---|---|---|
| D01 | Quality, accurate research and long-term recall above token savings | Required |
| D02 | Local development, optional cloud storage only; public open source | Required |
| D03 | One-time setup; lightweight; no routine manual worker restart | Required |
| D04 | Include Graphify-style structure and dependency knowledge | Required |
| D05 | Independent Codex review inside Claude; professional UI | Required |
| D06 | Portable workspaces including memory, source and unfinished work | Required |
| D07 | Go core + SQLite/FTS5, browser UI, no always-loaded AI | Provisional; verify perf and packaging |
| D08 | Durable local spool, optional supervised daemon, fail-open hooks | Provisional; failure-mode spike |
| D09 | Preserve original permitted evidence, derivations reversible, no summaries-only | Provisional implementation of D01; privacy/retention consent mandatory |
| D10 | Default manual read-only review via official Codex CLI, App Server later | Provisional; validate installed CLI, sandbox and auth |
| D11 | Native deterministic graph for initial TS/JS support, parser adapters for more | Provisional; compare Graphify reuse vs own parser, license and cost |
| D12 | S3-compatible encrypted immutable checkpoint first, no live SQLite replication | Provisional; verify provider and backup/restore contract |
| D13 | Apache-2.0 preferred project license, pending owner selection and dependency audit | Provisional; no LICENSE generated yet |
| D14 | Opt-in external inference/sync, capture consent, no raw auth tokens | Provisional security baseline; mandatory for release |
| D15 | Windows-first alpha; test macOS/Linux before claiming support | Provisional release sequencing |
| D16 | Product name DevIntel only working name | Open; package/ trademark search needed |
| D17 | Exact transcript portability vs semantic continuation | Open; test official resume formats; fallback required |
| D18 | Optional AI summarization and embeddings provider selection | Open; default no provider; benchmark recall and privacy |
| D19 | Cloud encryption format, key recovery, multi-device concurrency model | Open; prefer vetted backup tool; security review before release |
| D20 | Public alpha features, target hardware and performance budgets | Open; approve after Phase 0 |

## Phase 0 session 1 additions (2026-11-19) — all awaiting owner approval

Earlier rows are unchanged. Evidence: `evidence/phase-0/README.md`.

| ID | Decision | Status / reason |
|---|---|---|
| D07 | Refined by [ADR-0001](docs/adr/ADR-0001-core-stack.md): Go 1.27 + modernc.org/sqlite v1.62.0, version/FTS5 asserted at startup | Proposed; doc-level only, Go spike not run |
| D08 | Refined by [ADR-0002](docs/adr/ADR-0002-claude-hook-integration.md) and [ADR-0003](docs/adr/ADR-0003-process-model.md): exec-form hooks, documented hook fields as primary capture, 9,000-char envelope with coverage manifest, lease-based ingester | Proposed; live hook fixture not run |
| D10 | Refined by [ADR-0004](docs/adr/ADR-0004-codex-review-adapter.md): `codex exec` on an isolated snapshot; sandbox not trusted as the boundary | Proposed; flags verified on 0.167.1, no review run |
| D11 | Narrowed by [ADR-0005](docs/adr/ADR-0005-graph-parser-deferred.md): parser chosen by a Phase 4 spike; Graphify not embedded (Python) | Proposed |
| D16 | Name: 94 GitHub repos match "devintel"; folder is named "SessionKeep" (1 unrelated repo, npm name free) | Open; owner to choose, trademark search still needed |
| D21 | [ADR-0006](docs/adr/ADR-0006-phase-1-split.md): Phase 1 gated as 1A (capture→recall, CLI only) then 1B (UI, export, installer, MCP) | Proposed; scope sequencing needs owner approval |
| D22 | MCP server is a per-session resident process, optional, counted in resource reports; ARCHITECTURE §1 wording "short-lived" to be corrected on approval | Proposed (ADR-0003) |
| D23 | Transcript parsing is secondary reconciliation, never the primary capture path | Proposed (ADR-0002) |

## Phase 0 session 2 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D16 | Project name is **SessionKeep** (CLI `sessionkeep`, plugin namespace `sessionkeep`) | **Decided by owner 2026-11-19.** Trademark search still not done. Existing documents still say "DevIntel"; renaming them is the first task of Phase 1A-1 |
| D24 | Local Git repository initialised (`main`), no remote, no commits yet | Approved by owner 2026-11-19 |
| D25 | Go 1.27.4 toolchain kept project-local in `.tools/` (git-ignored), system PATH unchanged | Approved by owner 2026-11-19 |
| D07 | ADR-0001 runtime evidence obtained on windows/amd64 (SQLite 3.54.1, FTS5, WAL, backup, cross-builds) | Proposed; ready for owner approval |
| D08 | ADR-0002/0003 runtime evidence obtained (live hooks in print mode, 14 ms p95 hook, exactly-once replay) | Proposed; ready for owner approval with listed limits |
| D10 | ADR-0004: real Codex run could not read the target repository in its Windows sandbox | Proposed, **blocked**; needs a second approved test |
| D26 | Backups run on the writer connection or with ingest paused, with a deadline (finding B1) | Proposed (ADR-0001 update) |

## Session 3 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D07 | ADR-0001 core stack | **Accepted by owner 2026-11-19** |
| D08, D22, D23 | ADR-0002 hook contract and ADR-0003 process model, including manual-mode ingest only by explicit drain | **Accepted by owner 2026-11-19** |
| D21 | ADR-0006 Phase 1A / 1B split; plan is `phases/PHASE_1_PLAN.md` | **Accepted by owner 2026-11-19** |
| D10 | ADR-0004 Codex adapter: second real run also produced no working review | Proposed, **blocked**; further tests need approval |
| D27 | Go module path is `sessionkeep` (no hosting location chosen yet) | Provisional; change to the real repository path before any public release |
| D28 | First local commit of documents, evidence and the 1A-1 scaffold | Approved by owner 2026-11-19 |

## Session 4 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D29 | [ADR-0007](docs/adr/ADR-0007-unregistered-folder-modes.md): unregistered folders are handled by one setting — `manual`, `ask` (shipped default) or `auto` (owner-enabled, explicit level, exclusion list). `project add` has no default capture level | **Accepted by owner 2026-11-19.** Narrows "never auto-enable global capture" to "never by SessionKeep itself" |
| D30 | Onboarding is CLI-only in Phase 1A (`sessionkeep setup`); the web onboarding screen is part of Phase 1B and writes the same settings | Recorded; follows ADR-0006 |
| D31 | Hook-payload field allowlist moves from slice 1A-3 to 1A-4, where payloads are first parsed | Internal sequencing change, recorded |
| D32 | Hook deadline 2 s instead of 60 ms; spool events keyed by generated id; strict field allowlist; `systemMessage` reminder in ask mode (ADR-0002 update) | Internal, reversible; recorded with measurements in `evidence/phase-1/1A-4.md` |
| D33 | Redaction rule set gained command-line credential rules after a test caught `curl -u user:pass` reaching the spool | Done in slice 1A-4 |

## Session 5 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D34 | Migration `0001_foundation` was edited in place: `sessions` is unique per (project, agent, device, external session) so one host session can span projects | Done before any database existed outside test sandboxes. **From slice 1A-6 on (first run against real data) applied migrations are never edited; changes go in new numbered files** |
| D35 | Atomic file replacement on Windows uses a POSIX-semantics rename (needs Windows 10 1709+), classic rename as fallback; files replaced this way are read through `fsx.ReadFile` | Internal; forced by a failing test (`evidence/phase-1/1A-5.md`) |
| D36 | Only exclusions discard already-spooled events. Pausing a project or setting it to `off` stops new capture, but events spooled before that are still ingested | Provisional; the owner may prefer pause to discard as well |
| D37 | `sessionkeep status` (read-only counts and spool backlog) and `sessionkeep ingest` (one drain) added | Internal |

## Session 6 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D36 | Pause and `off` keep already-spooled events; only exclusions discard them | **Confirmed by owner 2026-11-19** ("keep pause as is") |
| D38 | Without an engine, drains are started by asynchronous plugin hook entries after `SessionStart` and `Stop` (plus `sessionkeep ingest` by hand). Extends the manual-mode answer the owner accepted for ADR-0003 | Implemented and observed live; **owner to confirm**, since the earlier approval said drains happen only when the user or a skill runs one |
| D39 | The plugin directory is generated per installation (`sessionkeep plugin write DIR`) and embeds the binary's absolute path; SessionKeep does not edit any Claude Code setting | Internal; follows ADR-0002 |

## Session 7 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D38 | Background drains started by plugin hooks | Owner did not object when asked and said to continue; treated as accepted, can be reverted on request |
| D40 | Recall is independent of capture: a paused or `off` project still receives its recorded instructions; excluded and unregistered folders receive nothing | Provisional; owner may prefer pause to stop recall too |
| D41 | Instruction scopes built now: global and project. Branch/task/session scopes and contradiction detection are deferred; identical active text in one scope is refused | Scope reduction inside slice 1A-7, recorded so it is not mistaken for done |
| D42 | `remember` has no default scope and saves nothing without `--yes`; text that looks like a secret is refused, not redacted; one instruction is at most 4,000 characters | Internal; follows the no-default rule of ADR-0007 |
| D43 | Every atomically written file has a second copy (`.alt`) that readers fall back to, because a file being replaced was measured to look missing for up to 65 ms on the reference machine | Internal; forced by an intermittent test failure (`evidence/phase-1/1A-7.md`) |

## Session 8 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D44 | Search always needs exactly one scope: the current (or named) project, or the explicit `--all-projects` flag. Reading a record by id is subject to the same check | Implements R05 / spec "cross-project ID requires scope check" |
| D45 | Queries are "all these words", each word quoted so punctuation and operator words are never interpreted; if nothing matches, the query is retried as one literal string | Internal; chosen for exact recall of errors, paths and ids |
| D46 | Command output uses plain ASCII for its own text (match markers `**`, `...`, `-`) | Internal; Windows console showed garbage for typographic characters |
| D47 | The generated plugin carries two skills, `recall` and `remember`; `remember` instructs Claude to preview, ask the user, and only then save | Not yet exercised live |

## Session 9 additions (2026-11-20)

Owner instruction: "ok lets finish rest phase 1", given in reply to a list of seven defaults. Treated as acceptance of those defaults:

| ID | Decision | Status / reason |
|---|---|---|
| D48 | Real installation into the owner's Claude Code is done by the owner; the agent builds the installer and tests it in a sandbox profile only | Owner default 1 |
| D49 | Up to ten short live Claude Code sessions (sandbox data) are pre-approved for the rest of Phase 1 | Owner default 2; two used in 1A-9 |
| D50 | npm packages for the web UI (React, Vite, Tailwind) may be downloaded at build time, pinned and recorded with licences | Owner default 3 |
| D51 | The dashboard is verified with API and accessibility tests without a browser; the owner looks at the pages. No browser automation | Owner default 4 |
| D52 | Auto-start (per-user scheduled task) is built and tested by creating and removing the task on this machine, leaving it removed | Owner default 5 |
| D53 | A C compiler may be placed in the git-ignored `.tools` folder so the race detector can run | Owner default 6 |
| D40 | A paused project still receives its instructions | Confirmed (owner default 7) |
| D54 | If a slice meets something outside these answers, record it, take the safer option, and continue with the rest | Agreed way of working for this run |
| D55 | Checkpoints are a note plus Git state (branch, commit, fingerprint of uncommitted and untracked content); no task or plan objects yet | Scope of slice 1A-9 |

## Session 10 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D56 | MCP server uses the official Go SDK v1.8.0, is read-only (four tools), optional (`plugin write --with-mcp`), and answers only for the session's project unless the owner starts it with `--all-projects` | Implements ADR-0003 item 4; a model must not be able to widen its own scope |
| D57 | `--` ends option parsing on the command line; all model-supplied text is passed after it | Closes an option-injection path found while designing the MCP tools |
| D58 | Full-text index rebuilt with word stemming (migration 0004); when no record has all the words, a labelled partial-match fallback is returned | Forced by a live session in which a correctly stored fact was not found (`evidence/phase-1/1B-1.md`) |
| D59 | Plan tools `instructions_get` and `checkpoint_get` are merged into `resume_pack` | Internal simplification |

## Approval rules

Changes to scope, licensing, public distribution, cloud trust, provider data sharing, secret policy or security defaults require explicit owner approval. Reversible internal implementation choices may be approved in a phase plan; record reasons and tests. No claims of API availability, supported platform or measurements based solely on discussion.
