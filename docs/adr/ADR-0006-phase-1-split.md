# ADR-0006 — Split Phase 1 into 1A (capture-to-recall loop) and 1B (UI, export, install)

- Date / owner / status: 2026-11-19 / project owner / **accepted** by owner 2026-11-19 (limits listed below still apply)
- Requirement IDs and affected phase: R01–R08, R16, R19, R22; D15, D20; Phase 1
- Decision and constraints: Keep every Phase 1 requirement, but gate it in two steps. **1A** proves the core promise end to end on Windows with no UI and no supervisor: consent → hook capture → spool → lease-holder ingest → instructions with versioning → cached injection in a new session → CLI/skill search, with crash and isolation tests. **1B** adds the loopback web UI, local export/import and deletion, installer/uninstall, opt-in supervisor, the optional MCP server and the full acceptance matrix. 1B starts only after the 1A gate.
- Evidence: `phases/PHASE_1.md` lists thirteen tickets whose first observable end-to-end result arrives at P1-08 while P1-10 to P1-12 each add a separate toolchain or OS surface (React/Vite build, authenticated HTTP, Task Scheduler). The project rule is "small tested vertical slices".
- Alternatives and reasons for rejecting them: Keep one gate (longer time before the owner can use and judge the memory loop; UI and installer defects would block the judgement of capture reliability). Drop the UI from Phase 1 entirely (R08 is a core-milestone requirement; it is sequenced, not removed).
- Quality, durability, resource and compatibility implications: The owner can run 1A against a synthetic project, then opt in a real project, before any HTTP surface exists — a smaller attack surface during the period when capture correctness is being established.
- Security, privacy, licensing and cost implications: No local web server in 1A. Consent and pause are CLI commands in 1A and gain UI in 1B.
- Migration / rollout / rollback / degradation plan: None; this is ordering.
- Known limitations, open questions and next review trigger: R08 (dashboard) is unmet at the 1A gate by design; `STATUS.md` must say so.
- Approval / supersedes: approved by owner 2026-11-19; amends ROADMAP phase 1 row and `phases/PHASE_1.md` ordering.
