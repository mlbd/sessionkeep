# Project status

**Last updated:** 2026-11-20 05:30 UTC • **Project name:** SessionKeep • **Active phase:** 1B (Phase 1A slices 1A-1 … 1A-9 are built; the 1A owner demo has not happened) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. On 2026-11-20 the owner asked for the rest of Phase 1 to be done in one run and accepted the defaults listed in `DECISIONS.md` (session 9). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## What exists (Go module `sessionkeep`, CGO-free)

- **Capture:** `sessionkeep hook <Event>` (field allowlist, capture level, redaction, durable spool, gap log, always exit 0); `sessionkeep ingest` with an OS-level lease; background drains started by plugin hooks.
- **Consent:** `setup` (manual / ask / auto), `project add|list|pause|resume|capture|which`, `exclude add|remove|list`.
- **Instructions:** `remember`, `instruction list|show|supersede|archive`; bounded envelope injected at SessionStart (and on a prompt when the set changed).
- **Recall:** `memory search`, `memory get`, `checkpoint`, `resume`; `status`, `doctor`, `paths`.
- **Claude Code plugin:** `sessionkeep plugin write DIR` generates exec-form hooks and three skills (`recall`, `remember`, `resume`).
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0003.
- **Not installed in the owner's Claude Code.** Everything has run in sandboxes with `--plugin-dir`; the real per-user data directory has never been created.
- **MCP (optional):** `sessionkeep mcp` with four read-only tools; `plugin write --with-mcp`.
- **Dashboard:** `sessionkeep dashboard` — loopback-only server, one-time sign-in link, six plain pages (overview, projects/onboarding, search, instructions, resume, diagnostics). **Not yet looked at in a browser by anyone.**
- **Not built:** export/import/delete, installer and auto-start, engine process.

## Latest session record (session 11, 2026-11-20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Slices done in this run so far: 1A-9, 1B-1 (MCP), 1B-2 (dashboard). `bash scripts/check.sh` → CHECK PASSED after each.
- 1B-2: attack tests against a real listening server all refused; owner journey through the API passes; real-binary HTTP smoke run clean; pages are self-contained. **The UI was built as plain pages instead of React (ADR-0008, awaiting owner) and has not been viewed in a browser.**
- Four of ten pre-approved live sessions used. `go test -race` still **not run**.
- Evidence: `evidence/phase-1/1B-2.md`. Decisions: `DECISIONS.md` session 11.

## Not yet verified

How the dashboard pages look and behave in a real browser (layout, contrast, keyboard flow, phone width). Interactive-mode sessions; SessionStart re-injection after compaction/resume; the confirm half of the `remember` skill; coexistence with the owner's other plugins; installation into the real Claude Code configuration; search speed on a large store; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None blocking. For the owner: look at the dashboard pages (`sessionkeep dashboard`); confirm or reverse ADR-0008 (plain pages instead of React); the 1A gate decision to install for real, once the installer exists.

## Next smallest task and acceptance checks

**1B-3** (`phases/PHASE_1_PLAN.md`): local export, import, deletion. `sessionkeep export FILE` writes a consistent copy taken with SQLite's online backup while the system is in use, plus settings, with a manifest of counts and digests; `sessionkeep import FILE --dry-run` reports what a restore would do, and `sessionkeep import FILE` restores into an empty data directory only; `sessionkeep forget` deletes one record, one project's captured text, or a whole project, with a preview of what will go and removal from the search index. Pass = local T18: export while hooks and a drain are running, import into a fresh profile, counts and digests match, search and instructions work there; a deleted record is absent from search, from the database file after compaction, and from a later export; nothing is deleted without confirmation; `bash scripts/check.sh` green; `evidence/phase-1/1B-3.md`. Then 1B-4 (installer, auto-start), 1B-5 (acceptance run, race detector, benchmarks).

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
