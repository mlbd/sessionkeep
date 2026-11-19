# Project status

**Last updated:** 2026-11-20 01:40 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1 … 1A-8 done; 1A-9 next, then the 1A gate) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

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
  - `internal/instruct`, `sessionkeep remember`, `sessionkeep instruction list|show|supersede|archive`: word-for-word instructions with versions; per-project cache; the bounded session envelope returned by the SessionStart and UserPromptSubmit hooks.
  - `sessionkeep memory search` / `memory get` (`evidence get`): project-scoped full-text and literal search over stored text, pagination, provenance; read-only, no lease. The generated plugin now includes `recall` and `remember` skills.
  - `internal/fsx`: atomic file replace that works under concurrent readers on Windows. `scripts/check.sh`: the verification gate, including compile-only builds for Linux, macOS and Windows arm64.
- **Capture and instruction recall work live** in test sessions (sandbox data, plugin loaded with `--plugin-dir`); search works from the command line. **Still not installed in the owner's Claude Code**, and the real data directory has never been created. Not built: task checkpoints and `resume` (1A-9), UI, export, installer, engine/MCP.

## Latest session record (session 8, 2026-11-20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-07 CLI part (slice 1A-8).
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED**.
  - Real-binary walkthrough of search across two projects: recorded in the evidence file.
  - `claude plugin validate` on the plugin with skills → Validation passed. **No live session was run**; the skills are untested in Claude Code.
  - Four mutation checks (search scope, lookup scope, query quoting, cursor pinning) are each caught by tests.
  - Problems fixed: non-ASCII output characters unreadable in a Windows console; a misleading hint on empty results; one generated edit that corrupted two files (restored from git).
  - `go test -race` → **not run** (no C compiler).
- Evidence: `evidence/phase-1/1A-8.md`. Decisions: `DECISIONS.md` session 8 section (D44–D47).

## Not yet verified

The `recall` and `remember` skills in a live session; search speed on a large store; how reliably a long instruction set is followed; SessionStart re-injection after compaction and on resume; interactive-mode sessions; coexistence with the owner's other plugins; installation into the real Claude Code configuration; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None blocking. A live test of the two skills needs approval (uses the Claude subscription). Open when convenient: D40 (should pausing a project also stop its instructions being recalled?).

## Next smallest task and acceptance checks

**1A-9** (`phases/PHASE_1_PLAN.md`): task checkpoints and `resume`. `sessionkeep checkpoint "<what is done / what is next>"` records the current task state for a project with the Git branch, HEAD and a digest of uncommitted changes (read with `git` commands only, never modifying the repository); `sessionkeep resume [--project …]` prints a continuation pack: active instructions, the latest checkpoint, the most recent exchanges, and a warning when branch, HEAD or uncommitted state differ from the checkpoint; a `resume` skill in the plugin. Pass = T08: resuming on a changed branch or with different uncommitted work warns and never touches the working tree (tree digest identical before and after); a project without Git works and says so; checkpoints are project-scoped; `bash scripts/check.sh` green; raw output in `evidence/phase-1/1A-9.md`. After 1A-9 comes the **1A gate**: an owner demo on a synthetic project and the decision whether to install into the real Claude Code configuration.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
