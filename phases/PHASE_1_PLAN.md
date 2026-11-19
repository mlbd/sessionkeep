# Phase 1 implementation plan — approved 2026-11-19 (1A/1B split)

**Status:** approved by the owner on 2026-11-19 together with ADR-0001, 0002, 0003 and 0006. Step 0 items 0.1–0.4 are done (evidence §9); 0.5 ran once and failed, second run recorded in evidence §9.6. `phases/PHASE_1.md` remains the requirement source.

## Original requirements

R01–R08, R16 (local part), R19, R20 (Windows only), R22 from `PRODUCT_SPEC.md`; acceptance outcomes 1, 2 and 7; tickets P1-01…P1-13; tests T01–T09, local T18, relevant T21/T23. Nothing is removed. R08 (dashboard) and R16 are delivered in 1B.

## Current state and evidence

No repository, no code. Reference machine: Windows 10 Pro 22H2 x64, i5-6500, 16 GB, SATA SSD. Claude Code 2.1.328, Codex CLI 0.167.1. Go not installed. Evidence and limits: `evidence/phase-0/README.md`.

## Step 0 — finish Phase 0 (each needs an owner yes/no)

| # | Action | Needs | Pass condition |
|---|---|---|---|
| 0.1 | Install Go 1.27.x (official zip into a project-local tools directory, PATH untouched) | approval to install | `go version` prints 1.27.x |
| 0.2 | Go spike in a throwaway directory: open modernc SQLite, print `sqlite_version()` and compile options, create FTS5 table, WAL + `synchronous=FULL`, one writer with four readers, online backup while writing, `integrity_check` on the copy; cross-build windows/amd64, linux/amd64, darwin/arm64; record binary sizes | 0.1 | version ≥ 3.51.3, FTS5 present, backup copy passes integrity check, all three builds succeed |
| 0.3 | Go spool spike: temp file + `Sync` + rename; consumer killed before and after commit at random points over 1,000 events; hook-shaped exe timed from spawn to exit over 500 runs | 0.1 | every acknowledged event present exactly once after replay; p95 reported against the 100 ms target |
| 0.4 | Isolated live hook fixture: temp plugin directory with a recorder exe, `claude -p` with restricted setting sources and a synthetic prompt in a temp project; capture real stdin payloads for SessionStart, UserPromptSubmit, PostToolUse, Stop, SessionEnd; return a marker in `additionalContext` and check the model can repeat it | approval (uses the owner's Claude subscription for one or two short turns; owner config untouched) | payload fixtures saved (synthetic only); marker round-trip observed; 9,000-character envelope round-trip observed |
| 0.5 | Synthetic Codex read-only review in a temp repository with a seeded defect; digest the directory before and after; try a write and a network call from the prompt and record what the sandbox does | approval (uses the owner's Codex subscription) | structured result parsed; no file changed; sandbox behaviour recorded as observed, whatever it is |

Any failed item goes back to the ADR concerned before Phase 1 starts.

## Phase 1A — capture-to-recall loop (no UI, no supervisor, no MCP)

Ordered; each slice ends with tests that run on the reference machine and with raw output kept under `evidence/phase-1/`.

| Slice | Deliverable | Acceptance tests (all real unless marked) |
|---|---|---|
| 1A-1 (P1-01) | Git repository initialised with owner approval; Go module; `sessionkeep version/help`; data-dir resolution with a test-only override; lint, `go vet`, race tests; staging check that `_private/` is never added | CI-equivalent script passes locally; data dir resolves correctly for a profile path containing spaces and non-ASCII characters |
| 1A-2 (P1-02) | Driver gate from ADR-0001; migrations for projects, roots, sessions, events, evidence, blobs, instructions, instruction versions, capture gaps, jobs; FTS5 over permitted message text; composite project scoping | migrate from empty; re-run is a no-op; foreign-key and cross-project-join tests fail closed; startup refuses a fake "old SQLite" version |
| 1A-3 (P1-03) | Project registration and consent (`project add`, `pause`, capture level); redaction and field allowlist applied before any write | secrets fixture (multiline PEM, bearer header, URL credentials, base64-wrapped token) never appears in spool, DB, cache or logs; unregistered directory captures nothing |
| 1A-4 (P1-04) | `sessionkeep hook <Event>` exec-form entry: parse, scope, redact, durable spool write, exit 0 within the internal deadline; gap accounting | 500-run latency report (p50/p95/p99 from spawn); kill during write leaves no partial event visible; disk-full and read-only-directory simulations exit 0 and record a gap where possible |
| 1A-5 (P1-05) | Lease-based ingester per ADR-0003 (`sessionkeep ingest --drain` and `sessionkeep start` foreground engine); idempotent insert; quarantine for invalid records | T03/T04 subset: kill before and after commit, 20 concurrent hook writers across 5 projects, two processes competing for the lease; every acknowledged event exactly once |
| 1A-6 (P1-06) | Plugin directory with exec-form hooks for SessionStart, UserPromptSubmit, PostToolUse(+Failure), Stop, PostCompact; `claude plugin validate` clean; install only via `--plugin-dir` in this slice | payload fixtures from step 0.4 replay green; one real session against a synthetic project captured end to end; uninstall leaves the owner's settings byte-identical (digest before/after) |
| 1A-7 (P1-08) | `remember`, scope confirmation, versioning, supersession, conflict state; instruction cache; SessionStart/UserPromptSubmit envelope per ADR-0002 | T05: 20-instruction fixture recovered verbatim at simulated day 7/30/90 in a new session; superseded text never injected; over-budget set produces the explicit coverage notice; engine stopped → cached envelope labelled with its timestamp |
| 1A-8 (P1-07, CLI part) | `memory search/get`, `evidence get` with pagination and provenance; skills that call the CLI | T06: adversarial two-project fixture returns zero foreign rows; exact error string, path and ID lookups hit; failed lookup says "not found", never guesses |
| 1A-9 (P1-09) | Task checkpoint and `resume` pack with branch and dirty-state digest | T08: resume on a changed branch warns and does not touch the working tree |
| **1A gate** | Owner demo on a synthetic project, then optional opt-in of one real project | all above green; no unresolved data-loss or cross-project defect; measured numbers published next to targets |

## Phase 1B — control surface and lifecycle

| Slice | Deliverable | Acceptance tests |
|---|---|---|
| 1B-1 (P1-07, MCP part) | Optional `sessionkeep mcp` on the official Go SDK; typed read tools only | stdout purity, initialise/cancel, per-session RSS measured and reported |
| 1B-2 (P1-10) | Loopback API with one-time bootstrap auth, Host/Origin checks; minimal React UI: overview, projects/consent, search/detail, instructions, checkpoint, diagnostics | T09; cross-origin and DNS-rebinding requests rejected; keyboard and axe checks; browser automation only when the owner asks for it |
| 1B-3 (P1-11) | Online-backup export, import into a test profile, deletion with index purge, restore dry run | local T18: counts and digests match; deleted item absent after restore |
| 1B-4 (P1-12) | Installer script, `doctor`, uninstall preserving data, opt-in per-user Task Scheduler entry | T01/T02 on the reference machine: 100 start/stop cycles, one engine, no console windows; honours a non-default Claude config directory |
| 1B-5 (P1-13) | Acceptance matrix run, resource benchmarks, security checks; independent review if authorised | Phase 1 gate in `phases/PHASE_1.md` |

## Verification matrix (extract)

| Requirement | Expected behaviour | Test | Fixture |
|---|---|---|---|
| R02 | Coding continues with engine stopped; capture still durable | T03, 1A-4, 1A-5 | synthetic event stream, kill points seeded |
| R03 | Acknowledged events survive restart once | T04 | 1,000-event replay |
| R04 | Verbatim instruction recall, correct supersession | T05 | 20 instructions, fake clock |
| R05 | No cross-project rows | T06 | two projects, shared strings |
| R19 | Secrets absent everywhere | T21 subset | redaction corpus |
| R22 | Hook p95 ≤ 100 ms from spawn | T23 subset | 500 runs, cold and warm |

## Risk register

| Risk | Likelihood / impact | Response |
|---|---|---|
| modernc build lacks FTS5 or is too slow | low / high | step 0.2 decides; fallback driver behind `store` |
| Model does not reliably follow a long factual envelope | medium / high | step 0.4 measures; smaller envelope plus mandatory tool retrieval |
| Another plugin's SessionStart output crowds or contradicts ours | medium / medium | coexistence test on this machine, which already has several |
| Volatile SSD write cache makes fsync weaker than assumed | medium / medium | document; power-loss claim limited to "process crash and OS-level flush" until tested on other hardware |
| Windows lock/rename semantics differ under antivirus scanning | medium / medium | retry-with-deadline on sharing violations; measured in 0.3 |
| Codex CLI changes flags between releases | high / low for Phase 1 | version-keyed probe (ADR-0004); not on the Phase 1 path |
| Windows 10 only on the reference machine | certain / medium | public claims limited to tested OS builds; Windows 11 test before any alpha |

## Completion

Nothing is complete. Next smallest task after approvals: step 0.1 then 0.2.
