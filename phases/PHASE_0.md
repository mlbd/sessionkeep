# Phase 0 — research and architecture qualification

**Objective:** verify the interfaces and assumptions before committing to production architecture. Read `START_HERE.md`. Use local synthetic fixtures, do not modify real Claude configs, install software, submit real code to external agents or upload cloud data without approval. If a prototype needs a dependency or paid provider, report approval needed. Output `evidence/phase-0/` records and ADRs, not a fabricated pass.

| Ticket | Work and acceptance evidence | Dependencies |
|---|---|---|
| P0-01 | Inventory actual OS/arch, hardware, Go/Node/Git/Claude/Codex versions and available shells; commands read-only, do not dump env/credentials. Record missing tools, supported versions and target machine. | none |
| P0-02 | Inspect official current Claude hooks/manifests/plugin loading. Pin version and capture synthetic actual payload fixtures; prove context injection for SessionStart/UserPromptSubmit, bounded capture, transcript reconciliation and Windows quoting. If live hook test requires config change use isolated temp setup with consent. | P0-01 |
| P0-03 | Compare native Claude memory and Claude-Mem architectural public source at specific commits; review failure reports and license boundaries. Identify what needs independent implementation; no claim of reproducing original install failure. | P0-01 |
| P0-04 | Spike Go executable with FTS5, patched SQLite, WAL, backup and concurrent reads/one writer, cross-build. Query SQLite version; inspect driver license, critical fix status, backup interface and binary size. | P0-01 |
| P0-05 | Prototype atomic durable spool, kill consumer and replay idempotently; hook latency including launch, fsync and busy timeout; compare daemon-free approach vs service; OS supervisor design and no terminal windows. | P0-02,04 |
| P0-06 | Verify Codex CLI local version/help/auth readiness, supported structured output and read-only sandbox; inspect official plugin license and App Server/MCP alternatives. Mock first; approved synthetic actual review tests permission and cancellation, no credential access. | P0-01 |
| P0-07 | Evaluate Graphify public graph architecture and licensing, TS/JS parser choices (WASM/native/CGO/optional packs), sample gold edges, distribution and CPU/memory. Decide first supported language and coverage. | P0-01,04 |
| P0-08 | Research provider selection and freshness policies; design local fixture with changing docs; verify SSRF controls, source rights and external sharing. Choose direct fetch + optional search adapter; no paid keys required. | P0-01 |
| P0-09 | Review local backup, Restic and S3 CAS options, encryption/key recovery and portability constraints. Prototype only local synthetic snapshot/restore, cloud requires consent. Exact transcript resume spike or explicitly defer to semantic continuation. | P0-02,04 |
| P0-10 | Design threat model, capture consent and redaction boundaries, data retention, local web auth, sandbox policies; review all proposed deps/license/IP and working name availability. Resolve owner-facing security choices. | prior spikes |
| P0-11 | Finalize proposed core scope, reference hardware budgets and test harness; decide stack and phase gating ADRs; if authorized request independent Codex critique of plan. | P0-01..10 |

## Completion package

1. `evidence/phase-0/README.md`: exact commit/tool versions, source register with checked dates and proof, commands/test results real or mocked, failed/unavailable checks and reproducible setup.
2. ADRs using template; update `DECISIONS.md` with approved/provisional/open statuses and explicit alternatives. No copied private upstream source. Pin baseline dependencies only if compatible and license reviewed.
3. Phase 1 ordered backlog tailored to actual environment, acceptance targets, scope (local no cloud/paid dependencies), UI entry point, security controls, risk register and expected test fixtures.
4. Show unresolved blockers and material owner decisions (initial capture mode, first supported OS, license/name, optional provider permissions). Do not re-ask already established priorities. Owner approves Phase 1 before production code.

## Stop conditions

If hooks or SQLite contracts unsupported, propose alternative and do not fake compatibility. If requested review cannot run without auth/consent, record not tested. If license unsure, no code reuse. If performance target missed, preserve durability and measure alternatives rather than removing evidence. Source information older than applicable policy must be revalidated. Actual project code remains untouched.
