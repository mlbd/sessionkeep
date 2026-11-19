# Project status

**Last updated:** 2026-11-19 18:40 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1, 1A-2, 1A-3 done; 1A-4 next) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

## What exists

- Specification package and Phase 0 evidence (`evidence/phase-0/README.md`, §9 runtime results).
- Product code (Go module `sessionkeep`, CGO-free):
  - `internal/cli`: `setup`, `project add|list|pause|resume|capture|which`, `exclude add|remove|list`, `doctor`, `paths`, `version`, `help`; `--json` everywhere.
  - `internal/config`: data-directory resolution; settings file (`config.json`: device id, auto-add mode and level, exclusions) with atomic save and fail-closed load.
  - `internal/project`: path normalization, registered-root matching, the capture decision (`capture` / `none` / `remind` / `auto_register`), repository-root detection, roots cache.
  - `internal/redact`: secret-shape redaction (rule set version 1).
  - `internal/store`: SQLite writer, engine gate, migrations `0001_foundation`, `0002_project_pause`; project repository functions.
  - `internal/fsx`: atomic file replace. `scripts/check.sh`: the verification gate.
- **Nothing is captured yet.** Not built: hook entry point, spool, ingester and lease, auto-registration itself, instruction commands, search, plugin, UI, export, installer.

## Latest session record (session 4, 2026-11-19)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-03 (slice 1A-3); owner decision on unregistered folders (ADR-0007).
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED** (cli, config, project, redact, store tests; build; `version` and `doctor` smoke).
  - Two tests failed during the slice and the code was fixed: URL-credential redaction was not idempotent; auto-add accepted a top-level folder.
  - Two mutation checks confirmed the exclusion and fail-closed tests detect breakage.
  - Real-binary walkthrough in a sandbox: recorded in the evidence file.
  - `go test -race` → **not run** (no C compiler).
  - A cleanup command targeting a sandbox at the drive root was blocked by the tool's safety check before running; the sandbox was moved under `.tools/` instead. Nothing was created or deleted at the drive root.
- Evidence: `evidence/phase-1/1A-3.md`. Decisions: `DECISIONS.md` session 4 section (D29–D31).

## Not yet verified

That redacted text never reaches disk end to end (nothing writes captured text until 1A-4); race detector; Linux, macOS, arm64, Windows 11; symlink/junction paths; interactive-mode hooks, compaction/resume, coexistence with other plugins; power loss, disk full; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None for Phase 1A.

## Next smallest task and acceptance checks

**1A-4** (`phases/PHASE_1_PLAN.md`): `sessionkeep hook <Event>` — parse the Claude Code payload (fixtures in `evidence/phase-0/fixtures/claude-2.1.328/`), keep only allowlisted fields, apply the capture decision and level, redact, write one spool file durably, exit 0 within the internal deadline; emit the `ask`-mode reminder; record capture gaps. Pass = fixture payloads produce the expected normalized events; the secrets fixture does not appear in any file under the data directory; unregistered/excluded/paused/off folders write nothing; garbage input, missing data directory and read-only spool all exit 0 with empty stdout; 500-run latency report from process start (target p95 ≤ 100 ms); `bash scripts/check.sh` green; raw output in `evidence/phase-1/1A-4.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
