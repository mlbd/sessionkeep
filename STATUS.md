# Project status

**Last updated:** 2026-11-23 (session 21; **renamed from DevWeft to SessionKeep in the source, D126; the owner's computer was switched to `sessionkeep 0.15.0` the same day**) • **Project name:** SessionKeep • **Active phase:** Phase 3 (current research), parts A and B built; **version 0.14.0 installed on the owner's computer through the npm package, plugin on in the owner's active Claude Code account (D107); live capture seen working there** • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted 2026-11-19; rest-of-Phase-1 defaults accepted 2026-11-20; **Phase 2 accepted by the owner 2026-11-20 (D94)**. Awaiting owner: D73 (look at the redesign), D74 (screenshots taken by the agent), D66 (logon list), D98/D103/D104 (agent's judgements in Phase 3 part A), publishing the npm package (D106). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## Start here

- `evidence/phase-3/P3-B.md` — session 19: session-start note for stale facts, Research page, version comparison with lock files.
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
- **Research (fetching off by default):** `research status|on|off|fetch|record|list|search|show|check|withdraw`; a claim is verified only by a quote SessionKeep found on a page it fetched, goes stale by category, and is disputed when the quote disappears. Stale and disputed claims are named at session start; `--applies-to PACKAGE@VERSION` is compared with the project's lock file.
- **Accounts:** `account list|use|auto|plugins|enable|disable` for computers with more than one Claude Code sign-in (D90, D111).
- **Data control:** `export`, `import [--dry-run]`, `forget record|captured|project`.
- **Surfaces:** generated Claude Code plugin with four skills, optional read-only MCP server, local dashboard (redesigned in session 15; pages: Overview, Pick up work, History, Search, Instructions, Research, Projects, Settings, Health; design "Margin", D115).
- **Lifecycle:** `install`, `uninstall`, `autostart`, `doctor`, `status`, `paths`, `version`.
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0006.

## State of the owner's computer

- `sessionkeep 0.15.0` is installed by `npm install -g dist/sessionkeep-0.15.0.tgz` (the `sessionkeep` command is on PATH), with automatic summaries **on** (Sonnet, 20 requests a day; D88) and research fetching **on** (D103), in the per-user programs folder with its generated plugin. `sessionkeep doctor` there: all checks ok.
- The data directory exists with default settings (unregistered folders: ask). The owner registered projects from the dashboard on 2026-11-21; an import preview there reported 4 sessions "already saved as they happened".
- **The plugin is turned on in the owner's active (second) Claude Code account** (`plugin list` there: `sessionkeep@sessionkeep-local`, enabled). Claude Code's default account was not touched. It loads at the next start of Claude Code. No auto-start entry exists.
- claude-mem is untouched.

## What has not happened

- Whether the owner ran the real import of earlier sessions is not known to the agent (a preview was seen: 71 sessions).
- The owner uses the dashboard but has not said whether the redesign is what was wanted (D73). The agent has not looked at the pages added since session 15 (accounts section, Research page) in a browser.
- No researched fact has been recorded in a real session yet, so the session-start note and the Research page have only been seen by tests.
- Nothing has been run on Linux, macOS, arm64 or Windows 11 (they compile).
- No remote exists; all commits are local on `main`.

## Latest session record (session 21, 2026-11-21 to 22)

The owner asked for research into what would make SessionKeep more useful day to day and for teams, then approved the whole recommended list. Built, each as its own commit with the full check passing (D117 to D122), released together as 0.11.0:

- **After a compaction** the next session start also carries the session's last exchanges word for word (D117). Instructions and the checkpoint were already re-sent.
- **An error met before**: a shell command that fails with an error an earlier session of the project met is answered with what the assistant said then (D118). Database change 0007 (an index).
- **Recap**: `sessionkeep recap [today|yesterday|week|DATE]` and a Recap page (D119).
- **Handoff file**: `sessionkeep handoff make` / `receive` gives one project's instructions, checkpoints and optionally sessions to a teammate (D120). Command line only; no dashboard button.
- **`sessionkeep file PATH`** and **`sessionkeep why COMMIT|FILE:LINE`** (D121). Command line only.
- **`sessionkeep suggest`** and a "Things you repeat" button on Instructions (D122).
- Evidence: unit and command tests for each (mock sessions through the real hook entry point). `.tools/smoke_ui.js`: 49 page states. **Not verified: none of this has run in a real Claude Code session or been seen in a browser.** In particular the `compact` session start and the note after a failed command rest on the hook documentation (read 2026-11-21, partly through a summary), not on an observed run.
- 0.12.0 (owner asked, D123): a recall mode per project, `asked` (default) or `automatic`. Automatic adds a note of the latest sessions to every session start and, to every request, the stored pieces that share its words. The hook reads the database for that, read-only and time-limited. Database change 0008. No project was switched to automatic by the agent.
- 0.13.0 (owner asked, D124, ADR-0009): team sync through a bucket in the owner's account (Cloudflare R2 first), encrypted with a team passphrase before upload; `sessionkeep team ...` and a "Team sync" button. Tested against a stand-in bucket only (mock). **Not verified: no request has been made to a real R2 bucket; no project is connected.** New dependency `filippo.io/age` v1.3.2.
- 0.14.0 (owner asked, D125, ADR-0010): sharing is set up and run from the dashboard. Settings has "Cloud storage for shared projects" (saved once); each project has "Share through the cloud" (shared name and its own passphrase), then "Sync now" (receive and send, shown first) and "Stop sharing". New commands `sessionkeep team storage | list | sync | storage-forget`. Tested through the dashboard's API against a stand-in bucket (mock), and the stand-in page check presses the dialogs. **Real service: the owner connected the SessionKeep project to a real R2 bucket on 2026-11-22 (keys accepted, listing answered, nothing uploaded). Not verified: upload and download against R2; the new screens in a browser.** A helper `.tools/team-connect.ps1` (not in Git) reads a `.env` in the project folder; the owner was told to delete that `.env` after connecting.
- 0.15.0 (owner chose the name, D126): renamed to SessionKeep everywhere, including earlier records and evidence (see D126 for what that means when reading them). Data saved under the old name is moved on first use; old handoff files and exports are still read; bundles in a bucket move to a new place, so the next sync sends everything again. Tested: unit tests for the move and for reading an old handoff file, and the full check. **Switched on the owner's computer on 2026-11-23 with their go-ahead:** the old plugin was turned off and the old program and npm package removed, then 0.15.0 installed; the data folder was moved by the first run, and the counts before and after agree (6 projects, 88 sessions; 11,003 events before, 11,006 after, the difference being events captured during the switch); `doctor` reports all checks ok, the plugin is on for the owner's active account, and the shared project is still listed. A copy of the data folder from before the move is kept beside it. Git history was rewritten the same day at the owner's request: the old name and the co-author lines are gone from all commit messages, and the old name from the files of every commit but the newest (D126). **Not verified: a sync after the rename (bundles now go to a new place in the bucket), and a new Claude Code session with the renamed plugin.** The owner reported on 2026-11-23 that the real sync to R2 works ("its working now"); which of the tests they ran was not said.
- Not built from the approved list: decision drafts (needs a model and a place for approved decisions to live; owner to choose), and shared project memory through Git (ROADMAP phase 9; needs its own threat model).
- Research sources are listed in the session's report to the owner; none of the tools found were copied from.

## Session 20 record (2026-11-21)

- Agent: Claude Code (claude-fable-5-2). Owner asked for a redesign, rejected the agent's five variants ("just changing color scheme"), asked for a prompt, designed five directions in Claude Design and chose "Margin".
- Rebuilt `internal/web/ui` (index.html, app.css, app.js) to that design. Decision D115. `design/DESIGN_PROMPT.md` is the prompt the owner used.
- Checks: `bash scripts/check.sh` -> CHECK PASSED. `.tools/smoke_ui.js` runs the real script against a stand-in page and stand-in answers (mock, not a browser): 43 page states visited and every button pressed without an exception. It found one fault on the way (the word "null" would have been printed under a session); fixed.
- **Not verified: nobody has looked at the new dashboard in a browser.** Layout, spacing, dark theme and the narrow window are unseen. No server route changed.
- Later the same session (0.10.1, D116): History lists every saved session, in pages of 200; a test holds that saved sessions outlive a cleared conversation and the loss of Claude Code's own files. CHECK PASSED.
- 0.10.2: a "Sync with Claude Code" button on History (when a project is chosen) and in the project pane. It stores waiting events, then runs the existing per-project import preview and asks before importing. No server change. Seen by the stand-in page check only.
- 0.10.3 (owner asked): after a project is added from the dashboard at a level that saves text, the dashboard looks for its earlier Claude Code sessions and offers to import them (numbers first, then a confirm button). Nothing is imported without that click. Seen by the stand-in page check only.
- Not built from the design: turns around a search result, a repair button on Health, settings that save without a button, a remembered theme.

## Session 19 record (2026-11-21)

- Agent: Claude Code (claude-fable-5-2). Owner: "Let's move next."
- Built Phase 3 part B (P3-06, rest of P3-03). Record: `evidence/phase-3/P3-B.md`. Decisions D112–D114.
- `bash scripts/check.sh` → **CHECK PASSED** before the commit. One failure on the way: the page showed a version warning by colour without the word `text:` the accessibility test looks for; fixed.
- Nine rules broken on purpose; each made a test fail.
- No page was fetched from the internet and no `claude -p` request was made.
- Not built: scheduled re-checks (P3-05), search providers (P3-04), contradicting sources, MCP tools for research, lock files in sub-folders.

## Session 18 record (2026-11-20)

- Agent: Claude Code (claude-fable-5-2). Owner asked whether SessionKeep can be installed with npm and set up behind the scenes, then: "go with your recommendation, build it locally first".
- Built the npm package (not published) and `sessionkeep install --enable-plugin`. Record: `evidence/npm/N-A.md`. Decisions D105–D110; D48 and D65 changed by the owner (D107).
- `bash scripts/check.sh` → **CHECK PASSED** (now includes the wrapper's tests) before commit `f088d69`.
- Sandbox runs with real npm and real Claude Code: normal install, scripts skipped, Claude Code missing, reinstall, removal. All behaved as designed.
- Real install on the owner's computer from the clean commit: npm exit 0; `sessionkeep 0.7.0`, doctor 6 ok, summaries / research / account settings kept; plugin enabled in the active account only.
- Owner tried the dashboard: the import button gave no sign of working (the status line was off-screen). Fixed in 0.7.1: buttons show a turning ring while they work, the import shows a moving bar in place, the status line stays in view. Not looked at in a browser by the agent.
- 2026-11-21, owner's first real use: projects added from the dashboard, import preview correct (71 sessions found; 4 sessions "already saved as they happened", so live capture works); the plugin's four skills are listed in a live session.
- Owner asked for an easy way to turn SessionKeep on for his other account. Built in 0.8.0 (D111): Overview section "Claude Code accounts" and `sessionkeep account plugins | enable | disable`. Tests: `TestThePluginIsTurnedOnAndOffPerAccount`, `TestDashboardTurnsThePluginOnAndOffForAnAccount` (mock Claude Code); four rules broken on purpose, each caught. Real, read-only: `account plugins` on the owner's computer answered in 2 s (default account off, active account on). Not done by the agent: turning it on in the default account (the owner's click), and looking at the new section in a browser.

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

1. Use it in real sessions and report capture gaps and recall misses. Restart the dashboard to see the pages of 0.9.0.
1a. npm package: decide whether and under which name and licence to publish it (D106). macOS and Linux builds have never been run.
2. Look at the redesigned dashboard and say whether it is what was wanted (D73).
3. Confirm or overrule D74 (screenshots) and D78 (Phase 1 gate read as accepted for a private pilot).
4. Summaries are on in the installed copy (Sonnet). Say if Opus is wanted, or change it on the Overview page. Judge the quality of the first real summaries.
4a. Phase 2 remainder (accepted as open, D94): fact versioning needs a design decision; a local embedding model needs a download and an ADR; claude-mem database import needs a schema and licence review.
5. D66: auto-start through the per-user logon list.
6. Phase 3: confirm or overrule D98 (robots.txt not consulted), D103 (fetching on in the installed copy), D104 (one real page fetched by the agent), D113 (which lock files are read and how `--applies-to` must be written).

## Next smallest task and acceptance checks

Owner decision needed before more of Phase 3: P3-05 (re-checks on a schedule; today nothing is fetched unless a command or a click asks) and P3-04 (search providers; needs a provider, a key and consent). Without that decision the next task is what real use turns up: capture gaps and recall misses first. Smallest open item inside Phase 3 that needs no decision: read-only research tools in the MCP server (list, search, show); acceptance: another project's claims are never returned, and no tool fetches.

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
