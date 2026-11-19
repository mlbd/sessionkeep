# Project status

**Last updated:** 2026-11-20 08:00 UTC • **Project name:** SessionKeep • **Active phase:** 1B (Phase 1A slices 1A-1 … 1A-9 are built; the 1A owner demo has not happened) • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19. On 2026-11-20 the owner asked for the rest of Phase 1 to be done in one run and accepted the defaults listed in `DECISIONS.md` (session 9). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

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
- **Lifecycle:** `install [--with-mcp]`, `uninstall [--purge-data] [--yes]`, `autostart enable|disable|status` (per-user logon entry, Windows only), install checks in `doctor`.
- **Not built:** a resident engine process (by design so far), signing, updates.

## Latest session record (session 13, 2026-11-20)

- Agent, working tree: Claude Code (claude-fable-5-2). Local commits on `main`, no remote. See `git log`.
- Slices done in this run so far: 1A-9, 1B-1, 1B-2, 1B-3, 1B-4. `bash scripts/check.sh` → CHECK PASSED after each, with one exception recorded in `evidence/phase-1/1B-3.md` (an intermittent failure that exposed a real defect in the drain; fixed, suite then green ten times in a row).
- 1B-4: real install into a sandbox works from a folder with spaces and accents; Claude Code accepted the generated marketplace and plugin in an isolated profile; the owner's Claude settings digest is unchanged; a logon-triggered scheduled task could not be created without administrator rights, so auto-start uses the per-user logon list instead (D66); the entry was created, run by hand and removed.
- Four of ten pre-approved live sessions used. `go test -race` still **not run**.
- Evidence: `evidence/phase-1/1B-4.md`. Decisions: `DECISIONS.md` session 13.

## Not yet verified

A real install on the owner's machine and a real Windows logon running the auto-start entry; whether any console window appears. How the dashboard pages look and behave in a real browser (layout, contrast, keyboard flow, phone width). Interactive-mode sessions; SessionStart re-injection after compaction/resume; the confirm half of the `remember` skill; coexistence with the owner's other plugins; installation into the real Claude Code configuration; search speed on a large store; disk full and power loss; the race detector; any execution on Linux, macOS, arm64 or Windows 11; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

None blocking. For the owner: look at the dashboard pages (`sessionkeep dashboard`); confirm or reverse ADR-0008 (plain pages instead of React); the 1A gate decision to install for real, once the installer exists.

## Next smallest task and acceptance checks

**1B-5** (`phases/PHASE_1_PLAN.md`): Phase 1 acceptance run. Put a C compiler in `.tools` and run the suite with the race detector; measure on the reference machine: hook latency, search latency and database size at 100k stored records, drain throughput, idle memory of each optional process; run the required end-to-end demonstration from `phases/PHASE_1.md` as one script against the real binary (register, live session with an instruction, stop/second session, recall, supersede, 90-day age, second project isolation, export, restore, compare); walk the T01–T09, T18, T21, T23 rows and mark each pass / partial / not run with the evidence file; write `evidence/phase-1/ACCEPTANCE.md` and the Phase 1 gate summary for the owner. Nothing in it may be marked passed without a run behind it.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
