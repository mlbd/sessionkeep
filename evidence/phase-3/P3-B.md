# Phase 3, part B — stale facts announced, shown and compared with the project's version

Date: 2026-11-21 (session 19). Agent: Claude Code (claude-fable-5-2). Windows 10 (amd64), one
machine. Work packages touched: P3-06 (notices and dashboard page), P3-03 (version freshness).
Decisions D112–D114.

## 1. What was built

- **Session-start note** (`internal/instruct`, `internal/capture`): the per-project cache file now
  also lists up to 200 sourced claims (id, subject, state, re-check date). At SessionStart the
  hook adds a note when any of them is past its date or disputed: the two counts, at most five
  claims by id and subject, and the commands to read and re-check them.
- **Version comparison** (`internal/research/versions.go`): `--applies-to` written as
  `PACKAGE@VERSION` or `PACKAGE VERSION` is compared with the version found in the lock files of
  the project's top folder. The result is part of every claim shown (`version_check` in JSON, a
  line in text).
- **Research page** (`internal/web`): `GET /api/research`, `POST /api/research/{id}/check`,
  `POST /api/research/{id}/withdraw`, and a page that lists a project's facts.
- **Skill text**: the research skill now says to write `package@version` and what the label
  NOT ABOUT THIS PROJECT'S VERSION means.
- `sessionkeep research list --limit N` (the option was accepted before but had no effect).

## 2. The rules, and the test that holds each

| Rule | Test |
|---|---|
| A stale claim is named at session start; a current one produces no note | `TestSessionStartNamesStaleAndDisputedClaimsWithoutFetching`, `TestResearchNoteIsBoundedAndOnlyAboutClaimsThatNeedALook` |
| Nothing is fetched for the note (request counter unchanged) | first test |
| The note is given at SessionStart only: not with a later prompt, also not when a prompt re-sends changed instructions | first test |
| Another project never receives it; a withdrawn claim is no longer named | first test |
| A disputed claim is named as disputed | first test |
| A claim becomes stale by the clock without the cache being rewritten | second test |
| At most five claims are named, the rest counted; the note stays under 2,500 units and the whole message under the 9,000 budget next to more instructions than fit | second test |
| The note names the claim and does not repeat its statement | first test |
| Another major version is labelled; the same one is not; the label follows the lock file without re-recording | `TestAClaimAboutAnotherMajorVersionIsLabelled` |
| Free text in `--applies-to` is never compared | same, `TestAppliesToIsComparedOnlyWhenItNamesAPackageAndAVersion` |
| Comparison uses only the numbers the claim states | `TestVersionsAreComparedByTheNumbersTheClaimStates` |
| Each of the eight lock-file kinds is read; a nested npm copy and a transitive pnpm entry are not taken for the project's own | `TestInstalledVersionIsReadFromEachKindOfLockFile` |
| A damaged, missing or directory-named lock file is skipped, not guessed from | `TestUnreadableLockFilesAreSkipped` |
| The page receives status, date, source, quote and version label; markup in a subject arrives as plain data | `TestDashboardShowsAndChecksResearchedFacts` |
| Listing fetches nothing; the check button fetches exactly once; refused while fetching is off | same |
| Only `check` and `withdraw` can be run from the page; an id that looks like an option is treated as an id | same |
| The page builds no markup from data and shows no state by colour alone | `TestPagesAreSelfContainedAndInjectionSafe`, `TestAccessibilityBasics` (existing) |

All tests above run against a local stand-in site and temporary folders (mock, not real).

## 3. Rules broken on purpose

`.tools/mut34.py` changes one line, runs the named tests and restores the file. Result of the
final run: nine of nine caught.

    CAUGHT     the note is given with later prompts too
    CAUGHT     every verified claim counts as stale
    CAUGHT     the cache is not rewritten when claims change
    CAUGHT     withdrawn claims stay in the note
    CAUGHT     another major version is called a match
    CAUGHT     free text is compared
    CAUGHT     a nested copy is read from package-lock.json
    CAUGHT     the id is not separated from options
    CAUGHT     any research command can be run from the page

The first run reported the second line as not caught because the changed line did not compile;
the change was rewritten and the run repeated.

## 4. Failures on the way

- `TestAccessibilityBasics` failed once: the version warning on the new page used a state colour
  on a line the test could not see words on. The line was rewritten.

## 5. Not done, not verified

- Nothing here was run in a real Claude Code session: the note and the page have been seen by
  tests only. The agent has not looked at the Research page in a browser.
- No real lock file of the owner's projects was read; the eight formats were tested on small
  hand-written samples. pnpm and yarn files are read line by line, not with a full parser.
- Lock files in sub-folders (monorepo packages), `composer.lock`, `Gemfile.lock`, Maven, Gradle
  and NuGet files, and version ranges are not read. The answer in those cases is "not found".
- A claim beyond the 200 cached per project is not looked at for the note; the note says how many.
- Not built: scheduled re-checks (P3-05), search providers (P3-04), a second source that
  contradicts the first, research tools in the MCP server, comparing two snapshots side by side.
