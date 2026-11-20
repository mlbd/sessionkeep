# First use

SessionKeep 0.1.0 is installed for your Windows user. It saves nothing until you do steps 1 and 2.
`sessionkeep` below means the installed program; either add its folder to your PATH or type the full
path that `sessionkeep install` printed (`%LOCALAPPDATA%\Programs\SessionKeep\sessionkeep.exe`).

## 1. Turn it on in Claude Code

    claude plugin marketplace add "%LOCALAPPDATA%\Programs\SessionKeep\claude-plugin"
    claude plugin install sessionkeep@sessionkeep-local

In PowerShell write `$env:LOCALAPPDATA` instead of `%LOCALAPPDATA%`. These two commands are the
only change to Claude Code; SessionKeep never edits Claude Code settings itself. Restart Claude Code.

## 2. Choose what is remembered

Open the dashboard and use the Projects page:

    sessionkeep dashboard

It prints a link; open it in your browser. It runs until you press Ctrl+C in that terminal.
Or from a terminal, inside a project folder:

    sessionkeep project add . --capture conversation

Exclude private or client folders first if you plan to use automatic adding
(`sessionkeep exclude add PATH`, then `sessionkeep setup --auto-add auto --level conversation`).

## 3. Bring in earlier sessions (optional)

    sessionkeep history import          # shows what would be imported, stores nothing
    sessionkeep history import --yes    # imports it

Only projects you registered are imported. Sessions run with a non-default Claude Code
configuration folder need `--from FOLDER` unless `CLAUDE_CONFIG_DIR` is set in that terminal.

## 4. Everyday use

- Claude gets your recorded instructions and the last checkpoint at the start of each session.
- In Claude Code: the `recall`, `remember` and `resume` skills of the plugin.
- In a terminal: `sessionkeep memory search "words"`, `sessionkeep session list`, `sessionkeep remember "..."`,
  `sessionkeep checkpoint "..."`, `sessionkeep resume`.
- `sessionkeep status` shows what is stored and whether anything is waiting; `sessionkeep doctor` checks health.

## 5. Summaries of finished sessions (optional, off by default)

    sessionkeep summarize on                  # explains what it means, changes nothing
    sessionkeep summarize on --yes            # turns it on (model: sonnet, at most 20 requests a day)
    sessionkeep summarize on --model opus --yes
    sessionkeep summarize run --dry-run       # which sessions would be sent
    sessionkeep summarize run                 # write them
    sessionkeep summarize status | off | delete

A summary is written by the Claude Code program on this computer under your own sign-in, so it
uses your plan's limits. It is stored locally, marked as written by a model, shown above the
session and found by `sessionkeep memory search`. Add `--auto` to `summarize on` to have one written
in the background after sessions; that part has not been tried in a real session yet.

This has only been tested against a stand-in for the program. Run `summarize run --limit 1`
first and read the result with `sessionkeep session show ID`.

## Running next to claude-mem

Both can be enabled at once; they do not share files. That has not been tested. If you remove
claude-mem, its own stored memories are not transferred: SessionKeep imports Claude Code's session
files, not claude-mem's database.

## Updating and removing

- Update: build a new version and run `sessionkeep install` again from it.
- Remove: `claude plugin uninstall sessionkeep@sessionkeep-local`, then `sessionkeep uninstall`
  (your data is kept unless you add `--purge-data`).
