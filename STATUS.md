# Project status

**Last updated:** 2026-11-20 00:30 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1 … 1A-7 done; 1A-8 next) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

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
  - `internal/fsx`: atomic file replace that works under concurrent readers on Windows. `scripts/check.sh`: the verification gate, including compile-only builds for Linux, macOS and Windows arm64.
- **Capture and instruction recall both work live** in test sessions (sandbox data, plugin loaded with `--plugin-dir`). **Still not installed in the owner's Claude Code**, and the real data directory has never been created. Not built: search commands, checkpoints/resume, UI, export, installer, engine/MCP, a way for Claude itself to record an instruction.

## Latest session record (session 7, 2026-11-19/20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-08 (slice 1A-7).
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED** at the end. One run in between **failed** (intermittent `TestReplaceWhileReading`), which led to the second-copy fix (D43).
  - Live session: a global and a project instruction were both followed by Claude in a fresh session; reported cost USD 0.033; owner settings digest unchanged.
  - Tests that failed for real and led to fixes: instruction order random within one second; a file being replaced looking missing to readers.
  - Three mutation checks (envelope budget, project scope, archived versions) are each caught by tests.
  - `go test -race` → **not run** (no C compiler).
- Evidence: `evidence/phase-1/1A-7.md`. Decisions: `DECISIONS.md` session 7 section (D38, D40–D43).

## Not yet verified

How reliably a long or conflicting instruction set is followed; SessionStart re-injection after compaction and on resume; interactive-mode sessions; coexistence with the owner's other plugins that inject at SessionStart; installation into the real Claude Code configuration; why a replaced file can look missing on this machine; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None blocking. Open for the owner when convenient: D40 (should pausing a project also stop its instructions being recalled?).

## Next smallest task and acceptance checks

**1A-8** (`phases/PHASE_1_PLAN.md`): search and evidence retrieval. `sessionkeep memory search "<query>" [--project …] [--kind …] [--since …] [--limit N] [--cursor …]` over stored text with full-text ranking plus exact matching for error strings, paths and ids; `sessionkeep memory get <id>` and `sessionkeep evidence get <id>` returning the original text with provenance (session, time, who said it, redaction and completeness); project scope enforced before ranking, cross-project search only with an explicit flag; stable pagination; skills in the generated plugin so Claude can call these commands. Pass = T06: an adversarial two-project fixture returns zero foreign rows; exact error string, file path and id lookups hit; punctuation-heavy queries do not error; a query with no match says "not found" and returns nothing; pagination returns every row exactly once; `bash scripts/check.sh` green; raw output in `evidence/phase-1/1A-8.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
