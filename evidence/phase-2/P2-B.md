# Phase 2, part B — optional session summaries and search guidance

Date: 2026-11-20 (session 16). Agent: Claude Code (claude-fable-5-2).
Everything below was run on Windows 10 (amd64). **No real Claude Code request was made**: the
program was replaced by a stand-in in every test. What the real program answers is unverified.

## 1. Why

The owner asked on 2026-11-20 whether summaries and meaning-based search could use the Claude
plan with a strong model. Findings:

| Question | Finding | Source, date read |
|---|---|---|
| Can `claude -p` use the plan? | Yes. "When you're signed in with your Claude plan, the Claude Agent SDK and `claude -p` still draw from your subscription limits" (update of 2026-10-07) | Support article 15036540, text pasted by the owner 2026-11-20. An earlier web search the same day found conflicting older reports (a separate credit from 2026-06-15, later paused); the owner's paste is the newest statement |
| Does Anthropic offer an embedding model? | No; its documentation points to Voyage AI, a separate paid service | `platform.claude.com/docs/en/build-with-claude/embeddings`, via search 2026-11-20 |
| Which options exist? | Read from `claude --help` of Claude Code 2.1.329 on this computer | 2026-11-20 |

So: summaries through the user's own Claude Code sign-in are built; embeddings are not (P2-04
stays open). Two things stand in for meaning-based search: summaries are indexed and searched,
and the search output, the `recall` skill and the MCP tool tell Claude to try other wordings.

## 2. What was built

| Item | Where |
|---|---|
| Table `summaries` with its own full-text index (migration 0005) | `internal/store/migrations/0005_summaries.sql`, `internal/store/summaries.go` |
| Input layout, system prompt, arguments, answer parser, runner | `internal/summarize` |
| `sessionkeep summarize status / on / off / run / delete` | `internal/cli/summarize.go` |
| Settings `summaries` (enabled, model, daily_limit, auto) | `internal/config/settings.go` |
| Summary shown above a session; matching summaries listed apart in search | `session show`, `memory search`, dashboard History and Search pages |
| "Try other words" guidance | search output, `recall` skill, MCP `memory_search` description |

## 3. Rules and how each was checked

| Rule | Check | Result |
|---|---|---|
| Off by default; `summarize run` refuses while off | `TestSummariesStayOffUntilTheOwnerConfirms` | pass |
| `summarize on` without `--yes` explains and changes nothing | same | pass |
| The background drain makes no request unless `--auto` was confirmed | same, `TestBackgroundSummaries…` | pass |
| Only finished sessions (ended, or silent for 6 hours) | open session never sent; mutation makes the test fail | pass |
| Only projects saving conversation text and not paused | paused project never sent; mutation makes the test fail | pass |
| Session text goes in on standard input, never on the command line | argument list inspected | pass |
| The program gets: print mode, the chosen model, `--safe-mode`, `--tools ""`, `--no-session-persistence`, JSON output, SessionKeep's system prompt | argument list inspected after passing through a `.cmd` file, as the real program is started on Windows; the empty argument survives | pass |
| SessionKeep's hooks do nothing inside a summary call (`SESSIONKEEP_SUMMARIZER`) | `TestHooksDoNothingInsideASummaryCall`; mutation fails | pass |
| The answer is redacted before storing | secret in the stand-in's answer is absent afterwards; mutation fails | pass |
| A summary is labelled with its model and how many pieces the model saw | `session show` output | pass |
| Summaries are found only inside the project, and not when a search is limited by speaker, date or exact match | search checks | pass |
| A session that gained text is summarised again and the old summary replaced | request count and "3 of 3" | pass |
| A summary written while the session changed is refused | `TestSummaryIsRefusedWhenTheSessionChanged…`; mutation fails | pass |
| Deleting any text of a session, its project's captured text or the project deletes the summary, also from the index | CLI and store tests | pass |
| The daily limit counts requests made, including failed ones and replacements | `TestSummarizeKeepsToTheDailyLimit` | pass |
| Two failures in a row stop the run | three sessions, two requests | pass |
| A failed or cut-short background attempt is not repeated for 30 minutes | marker file test | pass |
| Sessions with under 200 bytes of text are never sent | mutation fails | pass |
| Long sessions: the middle is left out, the input says so, pieces are cut on character boundaries | `TestBuildInputLeavesOutTheMiddleAndSaysSo` | pass |
| Failure of the program (exit code, non-JSON, reported error) stores nothing and reports the reason | three modes | pass |

Defect found by the tests while building: the daily limit first counted stored summaries, so a
replaced summary or a failed request did not count, and one background run could make two
requests. It now counts requests in a small file, written before each request.

## 4. Not verified

- **A real request.** The shape of the program's JSON answer (`result`, `is_error`,
  `modelUsage`) is taken from the agent's knowledge of Claude Code, not from an observed answer.
  If it differs, `summarize run` reports "the answer was not the expected JSON object" and
  stores nothing.
- That `--safe-mode` keeps the plan sign-in working, and that `--tools ""` is accepted together
  with it, in version 2.1.329.
- Summary quality, length and cost per session with any model.
- Whether the host ends a background (async) hook before a summary request finishes. If it does,
  that request is wasted and the 30-minute pause applies. For this reason `--auto` is separate
  and off unless asked for.
- How much the summaries improve recall. The synthetic recall check of part A was not rerun with
  summaries, because a stand-in's fixed answer would prove nothing.
- Linux and macOS (the code compiles; the shell-script form of the stand-in was not run).

The approved ten live sessions are used. These items need the owner's permission for a few more.
