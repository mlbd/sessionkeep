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

## Approval rules

Changes to scope, licensing, public distribution, cloud trust, provider data sharing, secret policy or security defaults require explicit owner approval. Reversible internal implementation choices may be approved in a phase plan; record reasons and tests. No claims of API availability, supported platform or measurements based solely on discussion.
