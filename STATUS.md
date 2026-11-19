# Project status

**Last updated:** 2026-11-19 20:40 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1 … 1A-5 done; 1A-6 next) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

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
  - `internal/fsx`: atomic file replace that works under concurrent readers on Windows. `scripts/check.sh`: the verification gate, including compile-only builds for Linux, macOS and Windows arm64.
- **The capture pipeline works end to end when fed by hand (hook → spool → `sessionkeep ingest` → database), but the hook is not registered with Claude Code, so nothing is captured from real sessions yet.** Not built: the plugin, instruction commands and injection, search commands, checkpoints, UI, export, installer, engine/MCP.

## Latest session record (session 5, 2026-11-19)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-05 (slice 1A-5).
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED** (all packages; compile-only builds for linux/amd64, darwin/arm64, windows/arm64).
  - Real binary: 20 concurrent hook processes wrote 1,000 events with 0 failures; a second `ingest` was refused while the first ran; 20 hard kills during drains; final count exactly 1,000 events, 1,000 text records, 20 sessions, 5 projects, empty spool.
  - Tests that failed for real and led to fixes: hook captures lost while a drain rewrote the roots cache; atomic replace failing on Windows while readers held the file (fixed with a POSIX-semantics rename); a session moving between projects could not be stored.
  - Three mutation checks (replay protection, exclusion re-check, auto-add lookup) are each caught by tests.
  - `go test -race` → **not run** (no C compiler).
- Evidence: `evidence/phase-1/1A-5.md`. Decisions: `DECISIONS.md` session 5 section (D34–D37).

## Not yet verified

The hook running under Claude Code itself; whether the `systemMessage` reminder is shown; the 2 s hook deadline path; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11 (they compile only); Windows older than 10 1709 for the rename path; symlink/junction paths; coexistence with other plugins; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

Slice 1A-6 needs approval for one or two short real Claude Code sessions with the plugin loaded through `--plugin-dir` (uses the Claude subscription; no settings file is changed).

## Next smallest task and acceptance checks

**1A-6** (`phases/PHASE_1_PLAN.md`): the Claude Code plugin. A plugin directory with exec-form hooks (`SessionStart`, `UserPromptSubmit`, `PostToolUse`, `PostToolUseFailure`, `Stop`, `PostCompact`, `SessionEnd`) pointing at the absolute path of the built binary; `claude plugin validate` clean; loaded only through `--plugin-dir` in this slice. Decide and implement when a drain is triggered without an engine. Pass = the Phase 0 fixtures replay green through the plugin hook definitions; one real Claude Code session against a synthetic project is captured end to end and visible in `sessionkeep status`; the `ask`-mode reminder is observed, or recorded as not shown; Claude settings files are byte-identical before and after; raw output in `evidence/phase-1/1A-6.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
