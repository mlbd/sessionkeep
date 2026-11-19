# Phase 1 — local trustworthy memory vertical slice

Prerequisite: approved Phase 0 baseline/ADR and actual target environment. Aim for reliable local operation without mandatory external AI. The implementation should be a real useful end-to-end experience, not stubs for every roadmap item. No broad scans or global hook changes without consent; use synthetic fixture project first.

| Ticket | Deliverable and acceptance | Depends on |
|---|---|---|
| P1-01 | Initialize approved repo/module/build/lint/test structure, CLI version/help, config/data paths and sandbox test harness. Verify no private material in staging. | P0 approval |
| P1-02 | SQLite driver/version check (patched WAL), migrations for projects/sessions/evidence/events/instructions/tasks/jobs, FTS5 and repositories, transactional idempotency/foreign keys. Unit + migration tests. | 01 |
| P1-03 | Capture policy/project consent and registered roots; redaction and safe event normalization, scope check before any persistence. Test secrets and unknown fields. | 02 |
| P1-04 | Short-lived capture CLI + durable spool writer with bounded deadlines; worker-independent, no network; safe failure diagnostics and gap accounting. Crash injection. | 03 |
| P1-05 | Ingest queue, singleton engine, leases/replay/health, atomic transaction, recovery, disk full/busy handling. Actual concurrency tests. | 04 |
| P1-06 | Supported Claude plugin hooks + synthetic actual integration; capture prompts/turns and reconcile permitted transcript snippets/source IDs; no dependence on SessionEnd. Controlled installation/rollback only. | 05 |
| P1-07 | Scoped memory search/GET/evidence CLI and MCP; FTS and exact identifiers, pagination, provenance. Prove no cross-project data; local cache if daemon off. | 02,06 |
| P1-08 | Explicit permanent instructions with scope, confirmation, versioning, conflict and cached deterministic retrieval; exact recall new session and aged fixture. | 07 |
| P1-09 | Task checkpoint and `resume` context pack, versioned branch/dirty-state and evidence, safe fallback semantic continuation. | 08 |
| P1-10 | Loopback authenticated web API and minimal professional React UI (overview, register project, consent, search/detail, instructions, task checkpoint, diagnostics). Accessibility/security tests. | 05,07,09 |
| P1-11 | Local consistent backup/export/import, deletion/index purge controls, restore dry run and integrity verification. No cloud. | 02,10 |
| P1-12 | One-time installer/manual startup and opt-in supervisor for approved OS; safe doctor/uninstall, preserve data; verify repeated sessions and crash recovery. | 05,06,10 |
| P1-13 | Execute acceptance matrix core subset, resource benchmarks, security checks, owner demo and approved independent review if available. Fix findings and retest; update docs and STATUS. | all |

## Required end-to-end demonstration

Register test project with capture consent, start actual Claude session with synthetic long-lived instruction, record discussion and task, exit; stop worker, open second session, capture permitted event, recall instruction via cached path or documented degraded state; restart engine and verify both records one time. Search original evidence from UI/MCP, correct instruction, reopen session and prove supersession. Simulate a 90-day age and recover old instruction by ID. Switch to unrelated project and prove isolation. Export consistent local snapshot, restore into test profile, compare counts/digests and preserve task. Run memory and resource tests on identified hardware. Show actual failures and retries, not mock success. Primary validation on real owner's Windows platform if approved; other OS claims limited to tested targets.

## Gate

All relevant T01–09, local T18, T21/T23 tests in `docs/11-TEST_EVAL_PLAN.md` must pass or feature disabled and documented. Any unresolved data loss or cross-project leak blocks Phase 1. Optional advanced embeddings, graph parsing, external research, Codex paid jobs and cloud sync excluded from this phase except already approved spikes. Owner approves next phase. Keep product requirements intact even if implementation is deferred.
