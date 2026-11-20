# First use

SessionKeep is installed for your Windows user. It saves nothing until you do steps 1 and 2.
`sessionkeep` below means the installed program; either add its folder to your PATH or type the full
path that `sessionkeep install` printed (`%LOCALAPPDATA%\Programs\SessionKeep\sessionkeep.exe`).

## 1. Turn it on in Claude Code

This computer has more than one Claude Code account, and SessionKeep is set to the one you use,
which you start with your own short command instead of `claude`. Below, ACCOUNT stands for that
command; `sessionkeep install` prints both lines with the real name filled in:

    ACCOUNT plugin marketplace add "%LOCALAPPDATA%\Programs\SessionKeep\claude-plugin"
    ACCOUNT plugin install sessionkeep@sessionkeep-local

In PowerShell write `$env:LOCALAPPDATA` instead of `%LOCALAPPDATA%`. These two commands are the
only change to Claude Code; SessionKeep never edits Claude Code settings itself. Restart Claude Code.

The plugin is turned on per account. `sessionkeep account list` shows the accounts SessionKeep found,
`sessionkeep account use NAME` changes which one it acts for (summaries, importing earlier sessions,
and the commands it prints), and `sessionkeep install` prints the two commands again for that account.

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

## 5. Summaries of finished sessions

Summaries are **on** in your installed copy (model: Sonnet, at most 20 requests a day). You do
not run anything: some seconds after a session has ended and the next one has started, a summary of
the ended one is written in the background by the Claude Code program on this computer, under
your own sign-in. It is stored locally, marked as written by a model, shown above the session
and found by search.

Change it on the dashboard (Overview, "Session summaries") or in a terminal:

    sessionkeep summarize status
    sessionkeep summarize off
    sessionkeep summarize on --model opus --yes
    sessionkeep summarize on --manual --yes   # only when you run: sessionkeep summarize run
    sessionkeep summarize delete --yes        # remove the summaries, keep the text

After importing earlier sessions, their summaries arrive over several days, at most 20 a day.

## 6. Facts that go out of date

API behaviour, versions, prices and limits change. SessionKeep can keep such a fact for a project
together with the page that says so, the exact words quoted from it and the date it was checked.
A fact is stored as verified only if SessionKeep itself fetched the page and found the words on it.
After its re-check date (one day for prices and security, fourteen for API behaviour, and so on)
it is shown as STALE until it is checked again; if the words are gone from the page it is shown
as DISPUTED.

You do not run anything: the plugin's `research` skill tells Claude to look at these facts before
relying on one, and to record what it looks up. In a terminal:

    sessionkeep research status
    sessionkeep research list
    sessionkeep research check --due          # fetch the sources of stale facts again
    sessionkeep research off                  # stop all fetching

Fetching is **on** in your installed copy. A page is fetched only when a command names it or
re-checks a recorded fact: public https pages only, nothing in the background, and nothing about
your project is sent. The page text is not kept, only its address, date, a digest and the quote.

## Running next to claude-mem

Both can be enabled at once; they do not share files. That has not been tested. If you remove
claude-mem, its own stored memories are not transferred: SessionKeep imports Claude Code's session
files, not claude-mem's database.

## Updating and removing

- Update: build a new version and run `sessionkeep install` again from it.
- Remove: `ACCOUNT plugin uninstall sessionkeep@sessionkeep-local`, then `sessionkeep uninstall`
  (your data is kept unless you add `--purge-data`).
