# Project status

**Last updated:** 2026-11-20 03:00 UTC • **Project name:** SessionKeep • **Active phase:** 1B (Phase 1A slices 1A-1 … 1A-9 are built; the 1A owner demo has not happened) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. On 2026-11-20 the owner asked for the rest of Phase 1 to be done in one run and accepted the defaults listed in `DECISIONS.md` (session 9). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## What exists (Go module `sessionkeep`, CGO-free)

- **Capture:** `sessionkeep hook <Event>` (field allowlist, capture level, redaction, durable spool, gap log, always exit 0); `sessionkeep ingest` with an OS-level lease; background drains started by plugin hooks.
- **Consent:** `setup` (manual / ask / auto), `project add|list|pause|resume|capture|which`, `exclude add|remove|list`.
- **Instructions:** `remember`, `instruction list|show|supersede|archive`; bounded envelope injected at SessionStart (and on a prompt when the set changed).
- **Recall:** `memory search`, `memory get`, `checkpoint`, `resume`; `status`, `doctor`, `paths`.
- **Claude Code plugin:** `sessionkeep plugin write DIR` generates exec-form hooks and three skills (`recall`, `remember`, `resume`).
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0003.
- **Not installed in the owner's Claude Code.** Everything has run in sandboxes with `--plugin-dir`; the real per-user data directory has never been created.
- **Not built:** MCP server, web UI, export/import/delete, installer and auto-start, engine process.

## Latest session record (session 9, 2026-11-20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Slice 1A-9 done: checkpoints, `resume`, third skill. `bash scripts/check.sh` → CHECK PASSED. One test failed for real (untracked-file changes not detected) and was fixed. Two live sessions: `resume`/`recall` answered correctly from stored history; `remember` previewed and asked instead of saving. Evidence `evidence/phase-1/1A-9.md`.
- `go test -race` still **not run** (C compiler to be added in 1B-5).

## Not yet verified

Interactive-mode sessions; SessionStart re-injection after compaction/resume; the confirm half of the `remember` skill; coexistence with the owner's other plugins; installation into the real Claude Code configuration; search speed on a large store; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None. The 1A gate (owner demo, decision to install for real) is the owner's step; the owner chose to run the real install personally once the installer exists (1B-4).

## Next smallest task and acceptance checks

**1B-1** (`phases/PHASE_1_PLAN.md`): optional `sessionkeep mcp` stdio server exposing read-only tools (`memory_search`, `memory_get`, `instructions_get`, `checkpoint_get`, `project_status`). Pass = protocol initialise and tool calls work over real stdio; stdout carries protocol messages only; tools enforce the same project scope as the commands; per-session memory measured and reported; the generated plugin can include the server on request; `bash scripts/check.sh` green; `evidence/phase-1/1B-1.md`. Then 1B-2 (web UI), 1B-3 (export/import/delete), 1B-4 (installer, auto-start), 1B-5 (acceptance run, race detector, benchmarks).

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
