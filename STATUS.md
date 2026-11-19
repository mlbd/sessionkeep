# Project status

**Last updated:** 2026-11-19 17:45 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1 and 1A-2 done; 1A-3 next) • **Approval:** ADR-0001, 0002, 0003, 0006 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

## What exists

- Specification package (renamed DevIntel → SessionKeep; decision history and name findings keep the old name on purpose).
- Phase 0 evidence: `evidence/phase-0/README.md` (§9 runtime results), spikes, sanitized fixtures.
- Product code (Go module `sessionkeep`, CGO-free):
  - `cmd/sessionkeep`, `internal/cli`: `version`, `paths`, `doctor`, `help` (`--json` where it applies).
  - `internal/config`: data-directory resolution (Windows/macOS/Linux rules, `SESSIONKEEP_DATA_DIR` override), `Ensure`.
  - `internal/store`: SQLite open for the writer, engine gate (refuses SQLite < 3.51.3 or without FTS5 before enabling WAL), checksummed transactional migrations, migration `0001_foundation` (projects, roots, sessions, blobs, evidence, events, capture gaps, instructions, instruction versions, jobs, `evidence_fts`).
  - `scripts/check.sh`: the verification gate.
- **Not built:** consent/redaction, capture hooks, spool, ingester and lease, instruction commands, search commands, plugin, UI, export, installer.

## Latest session record (session 3, 2026-11-19)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-01 (1A-1), P1-02 (1A-2), P0-06 second test.
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED** after 1A-2: git-reachability and personal-path checks, driver pin (sqlite v1.62.0 / libc v1.80.1), gofmt, `go vet`, `go test ./...` (cli, config, store), CGO-free build, `version` and `doctor` smoke.
  - `internal/store`: 16 tests pass against the real engine (SQLite 3.54.1). Two mutation checks confirmed the cross-project and FTS-delete tests fail when the schema rule is removed.
  - One intermediate `check.sh` run **failed** (a generated edit broke two string literals in `cli.go`); fixed and rerun green.
  - `go test -race` → **not run** (no C compiler).
  - Second Codex read-only review → ran, no working review (evidence §9.6).
- Evidence: `evidence/phase-1/1A-1.md`, `evidence/phase-1/1A-2.md`, `evidence/phase-0/README.md` §9.6.
- Decisions and approvals: `DECISIONS.md` session 3 section.

## Not yet verified

Race detector; any execution on Linux, macOS, arm64 or Windows 11; process-kill or power-loss during a migration; interactive-mode hooks, compaction/resume, coexistence with other installed plugins; disk full; a Codex review that can read its target; parser candidates; research providers; snapshot/restore; trademark search for SessionKeep; Windows ACLs on a custom data directory.

## Blockers / awaiting owner

None for Phase 1A. Optional: installing a C compiler would enable the race detector. Codex sandbox diagnosis (Phase 5 only) needs further approved runs — options in evidence §9.6.

## Next smallest task and acceptance checks

**1A-3** (`phases/PHASE_1_PLAN.md`): project registration and consent (`sessionkeep project add / list / pause`, capture level), device identity, redaction and field allowlist applied before any write. Pass = secrets fixture (multiline PEM, bearer header, URL credentials, base64-wrapped token) never appears in anything the code writes; an unregistered directory resolves to "no project, capture nothing"; nested and case-variant Windows paths map to the right root; `bash scripts/check.sh` green; raw output in `evidence/phase-1/1A-3.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
