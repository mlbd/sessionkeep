# Project status

**Last updated:** 2026-11-20 (session 15) • **Project name:** SessionKeep • **Active phase:** Phase 2, part A built; **version 0.1.0 installed on the owner's computer, plugin not yet enabled in Claude Code** • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted 2026-11-19; rest-of-Phase-1 defaults accepted 2026-11-20; owner instruction of 2026-11-20: redesign the dashboard, move to Phase 2, finish for use. Awaiting owner: D72/D78 (Phase 1 gate, read by the agent as accepted for a private pilot), D73 (look at the redesign), D74 (screenshots taken by the agent), D66 (logon list). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## Start here

- `evidence/phase-2/P2-A.md` — what session 15 built, tested and did not do.
- `evidence/phase-1/ACCEPTANCE.md` — what Phase 1 does, measured results, partial and not-run test rows.
- `docs/FIRST_USE.md` — the owner's steps to start using SessionKeep.

## What exists (Go module `sessionkeep`, CGO-free, one program of about 20 MB)

- **Capture:** `sessionkeep hook <Event>` called by Claude Code hooks (field allowlist, capture level, redaction, durable spool, always exit 0); `sessionkeep ingest` under an OS-level lease; background drains started by the plugin's own hooks; recovery of interrupted captures.
- **Consent:** `setup` (manual / ask / auto), `project add|list|pause|resume|capture|which`, `exclude add|remove|list`.
- **Instructions and continuity:** `remember`, `instruction list|show|supersede|archive`, `checkpoint`, `resume`; instructions and the latest checkpoint are delivered at SessionStart in a bounded envelope.
- **Recall:** `memory search` (stemmed words, literal fallback, labelled partial matches), `memory get`, `session list|show`.
- **History:** `history import` brings in earlier Claude Code sessions of registered projects (preview unless `--yes`).
- **Data control:** `export`, `import [--dry-run]`, `forget record|captured|project`.
- **Surfaces:** generated Claude Code plugin with three skills, optional read-only MCP server, local dashboard (redesigned in session 15; pages: Overview, Projects, History, Search, Instructions, Pick up work, Health).
- **Lifecycle:** `install`, `uninstall`, `autostart`, `doctor`, `status`, `paths`, `version`.
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0004.

## State of the owner's computer

- `sessionkeep 0.1.0` (commit `e4091fc`) is installed in the per-user programs folder with its generated plugin. `sessionkeep doctor` there: all checks ok.
- The data directory exists with default settings (unregistered folders: ask). **No project is registered, so nothing is saved yet.**
- **The plugin is not enabled in Claude Code.** The two `claude plugin` commands printed by `sessionkeep install` are the owner's step (D48). No auto-start entry exists.
- claude-mem is untouched.

## What has not happened

- No real Claude Code session has run with the installed plugin. No real import of earlier sessions has been run (only a preview that stored nothing).
- The owner has not seen the redesigned dashboard.
- Nothing has been run on Linux, macOS, arm64 or Windows 11 (they compile).
- No remote exists; all commits are local on `main`.

## Latest session record (session 15, 2026-11-20)

- Agent: Claude Code (claude-fable-5-2). Commits `b66895f`, `f9f91b3`, `5ea683b`, `e4091fc`, then the documentation commit.
- `bash scripts/check.sh` → **CHECK PASSED** (unit tests, race detector, cross-compiles, build, smoke) before each commit except `b66895f`, which was made with a failing gate by mistake and corrected by `f9f91b3`.
- Defect found and fixed: events within one second could be ordered wrongly (times stored with trailing zeros dropped).
- History import: 12 rule checks pass; parser read 588 real session files (295,301 lines) with 0 unreadable lines in a preview that stored nothing.
- Recall check on a synthetic set: 12/12 with matching words; 2 of 3 with different words.
- Fuzzing: 40 s on the session-file parser, 45 s on redaction, no failure.
- Five headless-browser screenshots of the redesign were taken by the agent against synthetic data (D74).
- No live Claude Code session was run (the approved ten are used).

## Not yet verified

Real daily use with the installed plugin; a full history import on real data and its speed; the redesign as judged by the owner, with a screen reader, by keyboard only, or in Firefox/Safari; a real Windows logon running the auto-start entry; interactive-mode sessions; SessionStart after compaction or resume; running next to claude-mem and the owner's other plugins; disk full and power loss; fuzzing of the hook payload parser and archive extraction; 20 real concurrent sessions; import of an older data version; any other operating system; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

1. Enable the plugin: run the two `claude plugin` commands (`docs/FIRST_USE.md`), then register projects.
2. Look at the redesigned dashboard and say whether it is what was wanted (D73).
3. Confirm or overrule D74 (screenshots) and D78 (Phase 1 gate read as accepted for a private pilot).
4. Phase 2 remainder (D79): summaries and meaning-based search need a model (download or paid); claude-mem database import needs a schema and licence review. Say which, if any, to pursue.
5. D66: auto-start through the per-user logon list.

## Next smallest task and acceptance checks

After the owner's first real sessions: collect capture gaps (`sessionkeep status`), recall misses and anything confusing in the dashboard; fix those first. Then, by owner choice: (a) open rows of the Phase 1 acceptance file (disk-full test, literal-search speed, remaining fuzz targets); (b) a batch transaction for history import if a real import proves slow; (c) an ADR for local embeddings.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
