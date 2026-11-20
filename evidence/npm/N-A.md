# Installing through npm — part A (local package, not published)

Date: 2026-11-20 (session 18). Agent: Claude Code (claude-fable-5-2). Windows 10 (amd64), Node
24.21.0, npm 11.19.0, Claude Code 2.1.329. Decisions D105–D110.

## 1. What was built

- `npm/`: `package.json` (private), `bin/sessionkeep.js` (starts the program), `lib/locate.js`
  (finds it), `postinstall.js` (setup after a global install), `README.md`, tests.
- `scripts/npm-pack.sh VERSION [all | os/arch ...]` builds `dist/sessionkeep-VERSION.tgz`.
  For this computer only: 6.3 MB.
- `sessionkeep install --enable-plugin`: after installing, runs Claude Code's own
  `plugin marketplace add` and `plugin install` for the account SessionKeep is set to.

## 2. Tests

| What | Test | Kind |
|---|---|---|
| Without the flag, Claude Code is never started; with it, exactly the two commands run | `TestEnablePluginRunsClaudeCodesOwnTwoCommandsOnlyWhenAsked` | mock program |
| The commands run for the chosen account; a failure is reported, exits non-zero, prints the commands, and leaves the program installed | `TestEnablePluginUsesTheChosenAccountAndReportsFailure` | mock program |
| System names, install folder, which copy runs, when setup runs | `npm/test/locate.test.js` (5 tests) | unit |

Three changes made on purpose (plugin step always on; account not passed; failure not reported)
each made a test fail.

## 3. Real runs in a sandbox

Real npm, real package file, real Claude Code; but a throwaway npm folder (`--prefix`), a
throwaway SessionKeep install and data folder, and a throwaway Claude Code configuration folder.
The owner's own account and npm folder were not touched in these runs.

| Case | Result |
|---|---|
| Claude Code's two commands, each run twice | first run "Successfully added / installed"; second run "already", exit 0 |
| `npm install -g --prefix SANDBOX sessionkeep-0.7.0.tgz` | command installed; program and plugin in place; `sessionkeep doctor` all ok; `claude plugin list` shows `sessionkeep@sessionkeep-local` 0.7.0, enabled, read from the install folder |
| The same with `--ignore-scripts` | only the command installed; `sessionkeep status` says "version 0.7.0 is not set up yet" and names the command; `sessionkeep install --enable-plugin` then completes |
| The same with the Claude Code program made unfindable | npm install still succeeds; program installed; the reason and the two commands are shown at once (with `--foreground-scripts`) and again on the next `sessionkeep` command; after repair the notice is gone |
| Installing again over itself | succeeds; doctor all ok |
| `sessionkeep uninstall`, `npm uninstall -g` | install folder and command removed; data kept |

npm printed a notice in every run: install scripts are "not yet covered by allowScripts". The
script ran anyway in npm 11.19.

## 4. The real install (owner's computer)

Built from the clean commit `f088d69`, then `npm install -g --foreground-scripts dist/sessionkeep-0.7.0.tgz`:

- npm exit 0; the setup step printed "installed sessionkeep 0.7.0" and "The plugin is turned on in
  Claude Code" for the owner's active account.
- `sessionkeep` resolves on PATH; `sessionkeep version` → 0.7.0, commit f088d696f068; doctor: 6 ok.
- Kept from before: summaries on (sonnet, 20 a day), research fetching on, chosen account.
- `plugin list` for that account: `sessionkeep@sessionkeep-local` 0.7.0, enabled. The default
  account's settings file does not mention sessionkeep.
- Not yet seen: a Claude Code session with the plugin loaded.

## 5. Not built, not verified

- Nothing is published; `npx sessionkeep` from the registry does not exist.
- The macOS and Linux programs build (`all`) but have never been run, and neither has the
  wrapper on those systems.
- An update while the dashboard is running was not tried.
- A later npm version that blocks install scripts by default: the fallback message exists, the
  blocked case was simulated with `--ignore-scripts` only.
