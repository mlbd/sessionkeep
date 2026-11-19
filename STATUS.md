# Project status

**Last updated:** 2026-11-20 04:10 UTC • **Project name:** SessionKeep • **Active phase:** 1B (Phase 1A slices 1A-1 … 1A-9 are built; the 1A owner demo has not happened) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. On 2026-11-20 the owner asked for the rest of Phase 1 to be done in one run and accepted the defaults listed in `DECISIONS.md` (session 9). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## What exists (Go module `sessionkeep`, CGO-free)

- **Capture:** `sessionkeep hook <Event>` (field allowlist, capture level, redaction, durable spool, gap log, always exit 0); `sessionkeep ingest` with an OS-level lease; background drains started by plugin hooks.
- **Consent:** `setup` (manual / ask / auto), `project add|list|pause|resume|capture|which`, `exclude add|remove|list`.
- **Instructions:** `remember`, `instruction list|show|supersede|archive`; bounded envelope injected at SessionStart (and on a prompt when the set changed).
- **Recall:** `memory search`, `memory get`, `checkpoint`, `resume`; `status`, `doctor`, `paths`.
- **Claude Code plugin:** `sessionkeep plugin write DIR` generates exec-form hooks and three skills (`recall`, `remember`, `resume`).
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0003.
- **Not installed in the owner's Claude Code.** Everything has run in sandboxes with `--plugin-dir`; the real per-user data directory has never been created.
- **MCP (optional):** `sessionkeep mcp` with four read-only tools; `plugin write --with-mcp`.
- **Not built:** web UI, export/import/delete, installer and auto-start, engine process.

## Latest session record (session 10, 2026-11-20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Slices done this run so far: 1A-9 (checkpoints, resume) and 1B-1 (MCP server). `bash scripts/check.sh` → CHECK PASSED after each.
- 1B-1: real-binary stdio test clean (no non-protocol output); memory about 10 MB idle, 18 MB after use; a live session exposed a search miss ("refresh" vs "refreshed"), fixed with stemming and a labelled partial-match fallback, and the repeated live session answered correctly. Four of ten pre-approved live sessions used.
- Evidence: `evidence/phase-1/1A-9.md`, `evidence/phase-1/1B-1.md`. Decisions: `DECISIONS.md` sessions 9–10.
- `go test -race` still **not run** (C compiler to be added in 1B-5).

## Not yet verified

Interactive-mode sessions; SessionStart re-injection after compaction/resume; the confirm half of the `remember` skill; coexistence with the owner's other plugins; installation into the real Claude Code configuration; search speed on a large store; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None. The 1A gate (owner demo, decision to install for real) is the owner's step; the owner chose to run the real install personally once the installer exists (1B-4).

## Next smallest task and acceptance checks

**1B-2** (`phases/PHASE_1_PLAN.md`): local web dashboard. `sessionkeep dashboard` serves a loopback-only API with one-time bootstrap authentication and Host/Origin checks, and an embedded React UI: overview, projects and consent (the onboarding the owner asked about), search and record detail, instructions, checkpoints, diagnostics. Pass = unauthenticated, cross-origin and wrong-Host requests are refused; the API never binds a non-loopback address; every mutating call needs the session and a confirmation where the commands need one; UI assets are embedded and load no remote resource; API and accessibility checks run without a browser (owner looks at the pages); `bash scripts/check.sh` green; `evidence/phase-1/1B-2.md`. Then 1B-3 (export/import/delete), 1B-4 (installer, auto-start), 1B-5 (acceptance run, race detector, benchmarks).

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
