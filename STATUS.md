# Project status

**Last updated:** 2026-11-20 06:30 UTC • **Project name:** SessionKeep • **Active phase:** 1B (Phase 1A slices 1A-1 … 1A-9 are built; the 1A owner demo has not happened) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. On 2026-11-20 the owner asked for the rest of Phase 1 to be done in one run and accepted the defaults listed in `DECISIONS.md` (session 9). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

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
- **Data control:** `export FILE` (unencrypted zip with manifest), `import FILE [--dry-run]` (empty data directory only), `forget record|captured|project` with preview; deletion buttons in the dashboard.
- **Not built:** installer and auto-start, engine process.

## Latest session record (session 12, 2026-11-20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Slices done in this run so far: 1A-9, 1B-1 (MCP), 1B-2 (dashboard), 1B-3 (export/import/delete). `bash scripts/check.sh` → CHECK PASSED after each.
- 1B-3: export under concurrent capture lost nothing; restore into a fresh profile works end to end; eleven damaged or hostile archives refused; deleted text verified absent from search, every file in the data directory and a later export. A real defect was found and fixed on the way (deleted words surviving in the search index; one path skipping compaction).
- Four of ten pre-approved live sessions used. `go test -race` still **not run**.
- Evidence: `evidence/phase-1/1B-3.md`. Decisions: `DECISIONS.md` session 12.

## Not yet verified

How the dashboard pages look and behave in a real browser (layout, contrast, keyboard flow, phone width). Interactive-mode sessions; SessionStart re-injection after compaction/resume; the confirm half of the `remember` skill; coexistence with the owner's other plugins; installation into the real Claude Code configuration; search speed on a large store; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None blocking. For the owner: look at the dashboard pages (`sessionkeep dashboard`); confirm or reverse ADR-0008 (plain pages instead of React); the 1A gate decision to install for real, once the installer exists.

## Next smallest task and acceptance checks

**1B-4** (`phases/PHASE_1_PLAN.md`): install, doctor, uninstall, auto-start. `sessionkeep install` copies the binary to a stable per-user location, generates the plugin beside it, and prints the exact Claude Code command to enable it (the owner runs that; SessionKeep edits no Claude setting); `sessionkeep uninstall` removes what install created and keeps the data unless `--purge-data` is confirmed; `sessionkeep autostart enable|disable|status` manages a per-user logon task for the background drain; `doctor` checks the installed binary path against the plugin's hooks. Pass = T01/T02 subset on the reference machine in a sandbox profile: install twice is idempotent; paths with spaces and non-ASCII work; uninstall leaves no owned file and no task, and the data intact; the scheduled task is created and removed for real and left removed; no console window flashes are caused by hooks (checked by process flags, not by eye); `bash scripts/check.sh` green; `evidence/phase-1/1B-4.md`. Then 1B-5 (acceptance run, race detector, benchmarks).

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
