# SessionKeep (npm package)

SessionKeep is a program that keeps developer memory for Claude Code on your own computer. This
package carries that program and sets it up. Node is used only to install it; the program
itself does not need Node to run.

This package is not published. It is built as a local file by `scripts/npm-pack.sh`.

## Install or update

    npm install -g ./sessionkeep-VERSION.tgz

That one command:

1. puts the `sessionkeep` command on your PATH;
2. copies the program to its per-user folder and generates the Claude Code plugin there;
3. turns the plugin on in Claude Code by running Claude Code's own commands
   (`plugin marketplace add` and `plugin install`) for the account SessionKeep is set to.

Restart Claude Code afterwards. npm hides what step 2 and 3 print; add `--foreground-scripts`
to see it, or run `sessionkeep doctor` afterwards. If setup could not finish, the next `sessionkeep`
command says why.

Nothing is remembered until you register a project (`sessionkeep project add . --capture
conversation`) or choose automatic mode (`sessionkeep setup --auto-add auto --level conversation`).

To install the command without any setup: set `SESSIONKEEP_SKIP_SETUP=1`, or use
`npm install -g --ignore-scripts`, and run `sessionkeep install` yourself later.

## Remove

    sessionkeep uninstall          (your data is kept unless you add --purge-data)
    npm uninstall -g sessionkeep

`sessionkeep uninstall` prints the Claude Code command that removes the plugin there.
