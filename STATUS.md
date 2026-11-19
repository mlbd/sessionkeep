# Project status

**Last updated:** 2026-11-19 17:10 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slice 1A-1 done; 1A-2 next) • **Approval:** ADR-0001, 0002, 0003, 0006 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

## What exists

- Specification package, renamed from DevIntel to SessionKeep (decision history in `DECISIONS.md` and the name findings in the evidence keep the old name on purpose).
- Phase 0 evidence: `evidence/phase-0/README.md` (§9 has the runtime results), spikes and sanitized fixtures.
- Product code: Go module `sessionkeep`; `cmd/sessionkeep`; `internal/cli` (`version`, `paths`, `help`, each with `--json` where it applies); `internal/config` (data-directory resolution for Windows/macOS/Linux, `SESSIONKEEP_DATA_DIR` override, `Ensure`); `internal/buildinfo`; `scripts/check.sh`.
- **Not built:** database, migrations, capture hooks, spool, ingester, instructions, search, plugin, UI, installer — everything from 1A-2 onward.

## Session 3 record

- Date, agent, phase, working tree: 2026-11-19, Claude Code (claude-fable-5-2), Phase 0 close-out and Phase 1A-1. First local commit on `main` (see `git log`); no remote.
- Requirements addressed: P1-01 (slice 1A-1); P0-06 second test.
- Changed files: rename across spec documents and `.gitignore`; ADR status lines; `phases/PHASE_1_PLAN_PROPOSED.md` → `phases/PHASE_1_PLAN.md`; new `go.mod`, `cmd/`, `internal/`, `scripts/check.sh`, `evidence/phase-1/1A-1.md`, `evidence/phase-0/spikes/codexreview/run.sh`, `fixtures/codex-0.167.1/result-run2.json`; evidence §9.6; `DECISIONS.md` session 3 section.
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED**: private/tool folders not reachable by git, no personal paths in files git would add, gofmt clean, `go vet` clean, `go test ./...` ok (cli, config), CGO-free build and `sessionkeep version` smoke. Raw output in `evidence/phase-1/1A-1.md`.
  - `go test -race` → **not run** (no C compiler on this machine).
  - Second Codex read-only review → **ran, no working review**: every shell command "blocked by policy"; tree unchanged; 25 s, ~57k input tokens.
- Decisions and approvals: see `DECISIONS.md` session 3 section.

## Not yet verified

Race detector; any execution on Linux, macOS, arm64 or Windows 11; interactive-mode hooks, compaction/resume, coexistence with other installed plugins; power loss and disk full; a Codex review that can read its target; parser candidates; research providers; snapshot/restore; trademark search for SessionKeep; Windows ACLs on a custom data directory.

## Blockers / awaiting owner

None for Phase 1A. For Codex (Phase 5 only): choose whether to spend further runs on diagnosing the Windows sandbox (options listed in evidence §9.6).

## Next smallest task and acceptance checks

**1A-2** (`phases/PHASE_1_PLAN.md`): `internal/store` with the modernc driver pinned as in ADR-0001; startup gate that reads `sqlite_version()` and compile options and refuses WAL below 3.51.3 or without FTS5; numbered migrations with checksums for projects, roots, sessions, events, evidence, blobs, instructions, instruction versions, capture gaps and jobs; FTS5 over permitted message text. Pass = migrate from empty; second run is a no-op; foreign-key and cross-project-join tests fail closed; the gate rejects a simulated old SQLite version; `bash scripts/check.sh` green; raw output saved to `evidence/phase-1/1A-2.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
