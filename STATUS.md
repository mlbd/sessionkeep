# Project status

**Last updated:** 2026-11-21 (session 18) • **Project name:** SessionKeep • **Active phase:** Phase 3 (current research), part A built; **version 0.8.0 installed on the owner's computer through the npm package, plugin turned on in the owner's active Claude Code account (D107); no session has run with it yet** • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted 2026-11-19; rest-of-Phase-1 defaults accepted 2026-11-20; **Phase 2 accepted by the owner 2026-11-20 (D94)**. Awaiting owner: D73 (look at the redesign), D74 (screenshots taken by the agent), D66 (logon list), D98/D103/D104 (agent's judgements in Phase 3 part A), publishing the npm package (D106). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## Start here

- `evidence/npm/N-A.md` — session 18: the npm package, the plugin step, sandbox and real install runs.
- `evidence/phase-3/P3-A.md` — session 17: claims with a checked source, safe page fetching, staleness; what is not built yet.
- `evidence/phase-2/P2-C.md` — session 16: import speed measured and improved; secret removal made 37 times faster with equivalence checks.
- `evidence/phase-2/P2-B.md` — session 16: automatic session summaries through the owner's Claude Code sign-in; three real requests made (section 4).
- `evidence/phase-2/P2-A.md` — what session 15 built, tested and did not do.
- `evidence/phase-1/ACCEPTANCE.md` — what Phase 1 does, measured results, partial and not-run test rows.
- `docs/FIRST_USE.md` — the owner's steps to start using SessionKeep.

## What exists (Go module `sessionkeep`, CGO-free, one program of about 20 MB)

- **Capture:** `sessionkeep hook <Event>` called by Claude Code hooks (field allowlist, capture level, redaction, durable spool, always exit 0); `sessionkeep ingest` under an OS-level lease; background drains started by the plugin's own hooks; recovery of interrupted captures.
- **Consent:** `setup` (manual / ask / auto), `project add|list|pause|resume|capture|which`, `exclude add|remove|list`.
- **Instructions and continuity:** `remember`, `instruction list|show|supersede|archive`, `checkpoint`, `resume`; instructions and the latest checkpoint are delivered at SessionStart in a bounded envelope.
- **Recall:** `memory search` (stemmed words, literal fallback, labelled partial matches), `memory get`, `session list|show`.
- **History:** `history import` brings in earlier Claude Code sessions of registered projects (preview unless `--yes`).
- **Summaries (off by default, automatic when on):** `summarize status|on|off|run|delete` and a block on the dashboard Overview; written by the user's own `claude -p` from a detached background process, stored apart from original text, searched by `memory search`.
- **Research (fetching off by default):** `research status|on|off|fetch|record|list|search|show|check|withdraw`; a claim is verified only by a quote SessionKeep found on a page it fetched, goes stale by category, and is disputed when the quote disappears.
- **Accounts:** `account list|use|auto` for computers with more than one Claude Code sign-in (D90).
- **Data control:** `export`, `import [--dry-run]`, `forget record|captured|project`.
- **Surfaces:** generated Claude Code plugin with four skills, optional read-only MCP server, local dashboard (redesigned in session 15; pages: Overview, Projects, History, Search, Instructions, Pick up work, Health).
- **Lifecycle:** `install`, `uninstall`, `autostart`, `doctor`, `status`, `paths`, `version`.
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0006.

## State of the owner's computer

- `sessionkeep 0.8.0` is installed by `npm install -g dist/sessionkeep-0.8.0.tgz` (the `sessionkeep` command is on PATH), with automatic summaries **on** (Sonnet, 20 requests a day; D88) and research fetching **on** (D103), in the per-user programs folder with its generated plugin. `sessionkeep doctor` there: all checks ok.
- The data directory exists with default settings (unregistered folders: ask). **No project is registered, so nothing is saved yet.**
- **The plugin is turned on in the owner's active (second) Claude Code account** (`plugin list` there: `sessionkeep@sessionkeep-local`, enabled). Claude Code's default account was not touched. It loads at the next start of Claude Code. No auto-start entry exists.
- claude-mem is untouched.

## What has not happened

- No real Claude Code session has run with the plugin yet (it was turned on during session 18; Claude Code must be restarted). No real import of earlier sessions has been run (only a preview that stored nothing).
- The owner has not seen the redesigned dashboard.
- Nothing has been run on Linux, macOS, arm64 or Windows 11 (they compile).
- No remote exists; all commits are local on `main`.

## Latest session record (session 18, 2026-11-20)

- Agent: Claude Code (claude-fable-5-2). Owner asked whether SessionKeep can be installed with npm and set up behind the scenes, then: "go with your recommendation, build it locally first".
- Built the npm package (not published) and `sessionkeep install --enable-plugin`. Record: `evidence/npm/N-A.md`. Decisions D105–D110; D48 and D65 changed by the owner (D107).
- `bash scripts/check.sh` → **CHECK PASSED** (now includes the wrapper's tests) before commit `f088d69`.
- Sandbox runs with real npm and real Claude Code: normal install, scripts skipped, Claude Code missing, reinstall, removal. All behaved as designed.
- Real install on the owner's computer from the clean commit: npm exit 0; `sessionkeep 0.7.0`, doctor 6 ok, summaries / research / account settings kept; plugin enabled in the active account only.
- Owner tried the dashboard: the import button gave no sign of working (the status line was off-screen). Fixed in 0.7.1: buttons show a turning ring while they work, the import shows a moving bar in place, the status line stays in view. Not looked at in a browser by the agent.
- 2026-11-21, owner's first real use: projects added from the dashboard, import preview correct (71 sessions found; 4 sessions "already saved as they happened", so live capture works); the plugin's four skills are listed in a live session.
- Owner asked for an easy way to turn SessionKeep on for his other account. Built in 0.8.0 (D111): Overview section "Claude Code accounts" and `sessionkeep account plugins | enable | disable`. Tests: `TestThePluginIsTurnedOnAndOffPerAccount`, `TestDashboardTurnsThePluginOnAndOffForAnAccount` (mock Claude Code); four rules broken on purpose, each caught. Real, read-only: `account plugins` on the owner's computer answered in 2 s (default account off, active account on). Not done by the agent: turning it on in the default account (the owner's click), and looking at the new section in a browser.
- Phase 3 part B was not started; it is still the next task.

## Session 17 record (2026-11-20)

- Agent: Claude Code (claude-fable-5-2). Owner: "accept Phase 2, lets move to next" (D94).
- Built Phase 3 part A (P3-01, P3-02, most of P3-03). Record: `evidence/phase-3/P3-A.md`. Decisions D94–D104.
- `bash scripts/check.sh` → **CHECK PASSED** before the commit. The first run failed: a data race in a new test (the test's own variable, not the program); fixed.
- Twelve rules broken on purpose; each made a test fail. `FuzzExtract` 30 s, 310,727 inputs, no failure.
- Four real requests to one public page (go.dev release history) proved the https path (D104).
- Not built: session-start notice and dashboard page for stale claims, version comparison against lock files, search providers, scheduled re-checks, contradicting sources, MCP tools.

## Earlier session record (session 16, 2026-11-20)

- Agent: Claude Code (claude-fable-5-2). Owner asked whether summaries could use the Claude plan and supplied the support-article text (D81).
- Built P2-03 (optional summaries) and search guidance in place of embeddings (D82–D85). Record: `evidence/phase-2/P2-B.md`.
- `bash scripts/check.sh` → **CHECK PASSED** before each commit. Eight mutation checks each made a test fail.
- Three real requests against synthetic text (D87). They exposed a defect: the program could not be started from a folder with a space in its name. Fixed.
- Owner asked for automatic operation: summaries now run from a detached background process (D86), verified in one real `claude -p` session. Summaries are on in the installed copy (D88). Voyage AI declined for now (D89).
- Defect found while building: the daily limit counted stored summaries instead of requests. Fixed.
- Several Claude Code accounts supported (D90); installed copy set to the owner's active account (D91).
- Import of 12,800 synthetic pieces: about 39 s before, about 8 s after (D92, D93). Four unexplained slow runs are recorded in `P2-C.md`.

## Session 15 record (2026-11-20)

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

1. Restart Claude Code so the plugin loads, then register projects (`sessionkeep project add . --capture conversation`, or automatic mode). Nothing is saved before that.
1a. npm package: decide whether and under which name and licence to publish it (D106). macOS and Linux builds have never been run.
2. Look at the redesigned dashboard and say whether it is what was wanted (D73).
3. Confirm or overrule D74 (screenshots) and D78 (Phase 1 gate read as accepted for a private pilot).
4. Summaries are on in the installed copy (Sonnet). Say if Opus is wanted, or change it on the Overview page. Judge the quality of the first real summaries.
4a. Phase 2 remainder (accepted as open, D94): fact versioning needs a design decision; a local embedding model needs a download and an ADR; claude-mem database import needs a schema and licence review.
5. D66: auto-start through the per-user logon list.
6. Phase 3 part A: confirm or overrule D98 (robots.txt not consulted), D103 (fetching on in the installed copy), D104 (one real page fetched by the agent).

## Next smallest task and acceptance checks

Phase 3 part B: (1) a line at session start when a project has stale or disputed claims, and a Research page on the dashboard (P3-06); acceptance: a stale claim is announced once per session start without any fetch, and the page shows status, date, source and quote as escaped text. (2) `--applies-to` compared with the version found in the project's lock file for the common ecosystems (P3-03); acceptance: a claim about another major version is labelled as not matching. Then P3-05 (bounded, opt-in scheduled re-checks) and P3-04 (search providers) only by owner decision. After the owner's first real sessions, capture gaps and recall misses come first.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
