# Project status

**Last updated:** 2026-11-19 21:20 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1 … 1A-6 done; 1A-7 next) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

## What exists

- Specification package and Phase 0 evidence (`evidence/phase-0/README.md`, §9 runtime results).
- Product code (Go module `sessionkeep`, CGO-free):
  - `internal/cli`: `setup`, `project add|list|pause|resume|capture|which`, `exclude add|remove|list`, `doctor`, `paths`, `version`, `help`; `--json` everywhere.
  - `internal/config`: data-directory resolution; settings file (`config.json`: device id, auto-add mode and level, exclusions) with atomic save and fail-closed load.
  - `internal/project`: path normalization, registered-root matching, the capture decision (`capture` / `none` / `remind` / `auto_register`), repository-root detection, roots cache.
  - `internal/capture` and `sessionkeep hook <Event>`: Claude Code hook adapter — field allowlist, capture decision and level, redaction, one durable spool file per event, gap log, always exit 0.
  - `internal/redact`: secret-shape redaction (rule set version 1, including command-line credentials).
  - `internal/store`: SQLite writer, engine gate, migrations `0001_foundation`, `0002_project_pause`; project repository functions.
  - `internal/lease`: OS-level single-writer lock. `internal/ingest` and `sessionkeep ingest`: drain the spool into the database (auto-add projects, sessions, evidence, events; quarantine; gap import; roots cache). `sessionkeep status`: stored counts and spool backlog.
  - `internal/plugin` and `sessionkeep plugin write DIR`: generates the Claude Code plugin (exec-form capture hooks for eight events, asynchronous drain hooks after SessionStart and Stop). `sessionkeep ingest --quiet` is the silent form those hooks call.
  - `internal/fsx`: atomic file replace that works under concurrent readers on Windows. `scripts/check.sh`: the verification gate, including compile-only builds for Linux, macOS and Windows arm64.
- **Capture works live**: a real Claude Code session with the generated plugin was captured into a sandbox database. **It is not installed in the owner's Claude Code** — the plugin has only been loaded with `--plugin-dir` for test sessions, and the real data directory has never been created. Not built: instruction commands and injection (so nothing is recalled into a session yet), search commands, checkpoints, UI, export, installer, engine/MCP.

## Latest session record (session 6, 2026-11-19)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-06 (slice 1A-6). Owner approved the live sessions and confirmed the pause behaviour.
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED**.
  - `claude plugin validate` on the generated plugin → Validation passed.
  - Live run 1 (registered synthetic project): 5 events captured; 4 stored by the background drain before the session ended, the `session_end` event stored by the next drain; prompt and reply stored word for word; tool event kept the file path, not file content.
  - Live run 2 (unregistered folder, ask mode): nothing stored; the reminder reached Claude Code as a hook response. Interactive display **not observed**.
  - Owner's Claude settings digest identical before and after both runs. Reported cost USD 0.035 + 0.031.
  - `go test -race` → **not run** (no C compiler).
- Evidence: `evidence/phase-1/1A-6.md`. Decisions: `DECISIONS.md` session 6 section (D36, D38, D39).

## Not yet verified

Interactive-mode sessions (including whether the reminder is displayed), `/compact`, resume, tool failures and API errors live; coexistence with the owner's other plugins; installation into the real Claude Code configuration; the 2 s hook deadline path; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; symlink/junction paths; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None blocking. To confirm when convenient: D38 (background drains started by plugin hooks after SessionStart and Stop).

## Next smallest task and acceptance checks

**1A-7** (`phases/PHASE_1_PLAN.md`): durable instructions. `sessionkeep remember "<text>" [--scope project|global|branch]` with explicit confirmation of scope and exact text; versioning and supersession (`sessionkeep instruction list|show|supersede|archive`); conflict state; the lease holder writes a per-project instruction cache; the SessionStart and UserPromptSubmit hooks return the envelope from ADR-0002 (manifest of ids and digests, verbatim text up to 9,000 characters, explicit notice when not everything fits). Pass = T05: a 20-instruction fixture is recovered verbatim in a new session at simulated day 7/30/90; superseded text is never injected; an over-budget set produces the coverage notice; with the database unavailable the cached envelope is used and labelled with its timestamp; global instructions apply in every registered project and never in excluded or unregistered folders; one live session shows Claude following an injected instruction (needs approval, uses the subscription); `bash scripts/check.sh` green; raw output in `evidence/phase-1/1A-7.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
