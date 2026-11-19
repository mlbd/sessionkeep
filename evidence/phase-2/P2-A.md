# Phase 2, part A — dashboard redesign, session history, history import, recall check

Date: 2026-11-20 (session 15). Agent: Claude Code (claude-fable-5-2). Commits: `b66895f`, `f9f91b3`, `5ea683b`, `e4091fc`.
All results below are from real runs on Windows 10 (amd64) unless marked otherwise.

## 1. What was built

| Item | Where | Notes |
|---|---|---|
| Dashboard redesign | `internal/web/ui/` (`index.html`, `app.css`, `app.js`, `favicon.svg`) | Owner rejected the plain pages on 2026-11-20. Still no build step, no npm, no remote resource. |
| Session history | `internal/store/sessions.go`, `sessionkeep session list\|show`, `GET /api/sessions`, `GET /api/sessions/{id}`, History page | P2-05 (history UI), read-only. |
| Fixed-width event times | `capture.TimeFormat` | Defect found by the new session test under the race detector. |
| History import | `internal/history`, `sessionkeep history import`, `POST /api/history/import`, block on the Projects page | P2-01. Parser version 1. |
| Recall check | `internal/cli/recall_eval_test.go` | P2-06, synthetic gold set. |
| Fuzz tests | `internal/history` (`FuzzParse`), `internal/redact` (`FuzzTextIsIdempotent`) | Short runs only. |
| Sample data script | `evidence/phase-1/tools/sample_data.py` | Synthetic content for looking at the dashboard. |

## 2. Design of the dashboard

- Subject: a loom. The left rail ends in a woven band (warp threads crossed by a saffron weft). Projects, sessions, instructions and health checks are rows with a "strand" at the left whose weave shows state: solid = saving, dashed = paused, outline = off, red = failed. State is always also written in words.
- Type: system sans for the interface, Bahnschrift / DIN-style for headings, a reading serif (Sitka Text, Charter, Georgia) for anything a person or Claude actually said, so recorded words never look like interface text. System fonts only: the Content-Security-Policy forbids remote resources.
- Colours: linen `#eef1f5`, sheet `#ffffff`, ink `#14213a`, warp `#24345a`, saffron `#c98a00`, teal `#0b6b5e`, madder `#a8261c`; a dark set is applied with `prefers-color-scheme`.
- Every choice (capture level, unregistered-folder mode, instruction scope) is a group of radio tiles showing each option with its meaning, inside a `fieldset` with a `legend`.
- Kept from before: `textContent` only, labelled controls, visible focus, reduced motion, small-screen layout, preview before every save of an instruction and before every deletion.

### How it was looked at

The agent took five screenshots with a headless browser against **synthetic** data (`sample_data.py`): Overview, Projects, Instructions (light, 1440 px), a session in dark mode, Projects at 390 px. Findings fixed from them: a stray focus outline on the content area, uneven tile text alignment, a scrollbar under the navigation at phone width. The screenshots were not kept.

This is **not** an owner review and not an accessibility audit. Not checked: a screen reader, keyboard-only use end to end, contrast measurement, Firefox or Safari, zoom at 200 %. The owner's standing rule says no automatic browser automation; see D74 for why it was used here and that it needs the owner's confirmation.

## 3. Defect found: event order within one second

`TestSessionListAndShowStayInsideTheProject` failed in `go test -race`: the answer was listed before the question. Cause: event times were written with `time.RFC3339Nano`, which drops trailing zeros, so `…09.5Z` sorts after `…09.51Z` as text. Fix: events and gaps use the nine-digit form (`capture.TimeFormat`) that instructions and checkpoints already used; the session view also sorts by parsed instant. `TestStoredTimesSortAsTextInTimeOrder` covers it. Search's literal fallback orders by the same column and benefits from the fix.

Process slip: commit `b66895f` was made while the gate was failing (steps chained with `;`). The fix followed in `f9f91b3`. Same slip as recorded once in Phase 1; the commit command now tests the gate's last line first.

## 4. History import

Rules (all covered by `TestHistoryImportRespectsEveryConsentRule` unless noted):

| Rule | Result |
|---|---|
| Without `--yes` nothing is stored; exit code "needs consent"; report printed | pass |
| Only registered projects at level conversation or extended | pass (metadata project left out) |
| Paused projects left out | pass |
| Excluded folders left out | pass |
| Folders that are not projects are never registered, even with auto-add on | pass |
| Sessions already captured live are skipped | pass; mutation (rule disabled) makes the test fail |
| Secrets removed with the same rules as live capture | pass (`password=…` not searchable afterwards) |
| Second run stores nothing new | pass (0 new, 2 already there) |
| `--project` limits the import to one project | pass (`TestHistoryImportOneProjectAndBadSource`) |
| Dashboard: preview is 409, confirm is 200 | pass (`TestDashboardHistoryImportNeedsConfirmation`) |
| Parser emits only typed and answered text; reasoning, tool input/output, images, subagent lines, inserted text never | pass (`TestParseKeepsOnlyWhatWasSaid`, synthetic lines) |
| Oversize line skipped, parsing continues | pass |

Real-data check (preview only, nothing stored, empty sandbox data directory): 588 session files, 295,301 lines, 0 unreadable lines, 0 oversize, 0 with missing fields, 12,788 turns recognised, 909 inserted texts skipped. Only counts were printed; no content was read by the agent.

Limits, stated plainly:
- The session-file layout is not a published interface. It was learned from key names of files written by the Claude Code version in use on 2026-11-20. A future version may change it; unknown shapes are counted and skipped.
- The list of "inserted text" prefixes is a fixed list; an inserted text with another prefix would be imported as if typed.
- A session that was partly captured live is skipped whole, so turns from before SessionKeep was installed in a session that continued afterwards are not imported.
- A full import was **not** run on the owner's data (no project is registered there yet). Speed at 12,788 turns is therefore estimated, not measured: each turn is one durable transaction.
- Claude Code deletes old session files on its own schedule; history older than that cannot be imported.

## 5. Recall check (synthetic)

12 facts among 24 unrelated texts, one question per fact using the same key words: 12/12 found in the first five results. Three questions asked in different words: 2 found their fact through the labelled partial-match fallback, 1 did not. This is a small synthetic set written by the same agent that wrote the search; it shows no regression, not real-world recall quality.

## 6. Fuzzing

| Target | Duration | Executions | Result |
|---|---|---|---|
| `history.Parse` | 40 s | 13,061 | no failure |
| `redact.Text` idempotence | 45 s | 177,069 | no failure |

Short runs on one machine. The hook payload parser and archive extraction are still not fuzzed.

## 7. Gate

`bash scripts/check.sh` → `CHECK PASSED` before commits `f9f91b3`, `5ea683b`, `e4091fc` (unit tests, race detector, three cross-compiles, build, smoke).

## 8. Installation on the owner's computer

`sessionkeep 0.1.0` (commit `e4091fc`) built with the version flag and installed with `sessionkeep install`: program and generated plugin under the per-user programs folder; data directory created by `sessionkeep setup` (mode: ask, no exclusions, no projects). `sessionkeep doctor`: all six checks ok, 10 hook commands point at an existing program. No Claude Code setting was changed, no auto-start entry was created, and the plugin is **not** enabled in Claude Code: the two `claude plugin` commands remain the owner's step (D48).

## 9. Not done in Phase 2

| Item | Reason |
|---|---|
| P2-02 fact versioning and candidate extraction | Needs a design decision on what a "fact" is and who confirms it; not guessed overnight. |
| P2-03 batched summaries | Needs a model; would be paid inference or a model download. Not allowed without the owner. |
| P2-04 local embeddings / hybrid ranking | Needs a model download and an ADR on the runtime (the core must stay free of Python/Node). |
| claude-mem database import | Needs schema and licence review, opt-in and a backup first (`phases/LATER_PHASES.md`). |
| Corrections UI | History is readable and single pieces can be deleted; editing stored text is forbidden by design (immutable evidence). |
