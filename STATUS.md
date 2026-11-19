# Project status

**Last updated:** 2026-11-19 19:30 UTC • **Project name:** SessionKeep • **Active phase:** 1A (slices 1A-1 … 1A-4 done; 1A-5 next) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed, not yet needed.

## What exists

- Specification package and Phase 0 evidence (`evidence/phase-0/README.md`, §9 runtime results).
- Product code (Go module `sessionkeep`, CGO-free):
  - `internal/cli`: `setup`, `project add|list|pause|resume|capture|which`, `exclude add|remove|list`, `doctor`, `paths`, `version`, `help`; `--json` everywhere.
  - `internal/config`: data-directory resolution; settings file (`config.json`: device id, auto-add mode and level, exclusions) with atomic save and fail-closed load.
  - `internal/project`: path normalization, registered-root matching, the capture decision (`capture` / `none` / `remind` / `auto_register`), repository-root detection, roots cache.
  - `internal/capture` and `sessionkeep hook <Event>`: Claude Code hook adapter — field allowlist, capture decision and level, redaction, one durable spool file per event, gap log, always exit 0.
  - `internal/redact`: secret-shape redaction (rule set version 1, including command-line credentials).
  - `internal/store`: SQLite writer, engine gate, migrations `0001_foundation`, `0002_project_pause`; project repository functions.
  - `internal/fsx`: atomic file replace. `scripts/check.sh`: the verification gate.
- **Events can be written to the spool, but nothing reads it and the hook is not registered with Claude Code.** Not built: ingester and lease, auto-registration itself, instruction commands, search, plugin, UI, export, installer.

## Latest session record (session 4, 2026-11-19)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Requirements addressed: P1-03 (slice 1A-3), P1-04 (slice 1A-4); owner decision on unregistered folders (ADR-0007).
- Commands and results (all real):
  - `bash scripts/check.sh` → **CHECK PASSED** after each slice (capture, cli, config, project, redact, store tests; build; smoke).
  - Real-binary hook run: 500 captures, **p95 17.4 ms** from process start (target 100 ms), one 238 ms outlier; no secret strings in any file under the data directory; unregistered and paused folders wrote nothing.
  - Tests that failed for real and led to fixes: URL-credential redaction not idempotent; auto-add accepting a top-level folder; a `curl -u user:pass` password reaching the spool.
  - Mutation checks: exclusions, fail-closed loading, redaction on the capture path and level filtering are each detected by tests. One mutation attempt did not compile and was redone before its result was recorded.
  - `go test -race` → **not run** (no C compiler). Disk-full and kill-during-write were not re-run against this binary.
  - A cleanup command aimed at a sandbox at the drive root was blocked by the tool's safety check before running; the sandbox was moved under `.tools/`. Nothing was created or deleted at the drive root.
- Evidence: `evidence/phase-1/1A-3.md`, `evidence/phase-1/1A-4.md`. Decisions: `DECISIONS.md` session 4 section (D29–D33).

## Not yet verified

The hook running under Claude Code itself (registration is slice 1A-6); whether the `systemMessage` reminder is shown at SessionStart; the 2 s deadline path; disk full, power loss, antivirus interference; race detector; Linux, macOS, arm64, Windows 11; symlink/junction paths; compaction/resume sources; coexistence with other plugins; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None for Phase 1A.

## Next smallest task and acceptance checks

**1A-5** (`phases/PHASE_1_PLAN.md`): the ingester. `sessionkeep ingest --drain` takes the OS-level ingest lease, reads spool files in order, and for each one, in a single transaction, creates the project if the event carries an auto-add request (re-checking exclusions), upserts the session, stores evidence and the event idempotently, then deletes the file; invalid files are quarantined; the gap log is imported; the roots cache is rewritten. Administrative commands take the same lease. Pass = kill before and after commit at random points over 1,000 events leaves every event exactly once; two drains at once — one gets the lease; 20 concurrent hook writers across 5 projects lose nothing; an auto-add event creates exactly one project even when 50 events arrive for it; an event for a folder excluded after it was spooled is discarded; `bash scripts/check.sh` green; raw output in `evidence/phase-1/1A-5.md`.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
