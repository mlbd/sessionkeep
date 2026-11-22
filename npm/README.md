# SessionKeep (npm package)

SessionKeep is a program that keeps developer memory for Claude Code on your own computer. This
package carries that program and sets it up. Node is used only to install it; the program
itself does not need Node to run.

It carries programs for Windows, macOS and Linux (x64 and arm64). MIT licensed; the licences of
the software compiled into the program are in `THIRD_PARTY_NOTICES.txt`.

## Install or update

    npm install -g sessionkeep

That one command:

1. puts the `sessionkeep` command on your PATH;
2. copies the program to its per-user folder and generates the Claude Code plugin there;
3. turns the plugin on in Claude Code by running Claude Code's own commands
   (`plugin marketplace add` and `plugin install`) for the account SessionKeep is set to.

Steps 2 and 3 are done by the package's install script. Recent npm versions print a notice
that it is "not yet covered by allowScripts" and run it anyway. To allow it without the notice:

    npm install -g sessionkeep --allow-scripts=sessionkeep

If your npm is set to block install scripts (`strict-allow-scripts` or `ignore-scripts`), the
command is installed but not set up: run `sessionkeep install --enable-plugin` once.

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
