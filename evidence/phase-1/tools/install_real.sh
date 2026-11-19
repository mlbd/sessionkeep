#!/usr/bin/env bash
# Slice 1B-4: real-binary install into a sandbox, the Claude Code plugin commands in an isolated
# Claude profile, and the logon task created and removed for real.
# Usage: install_real.sh <sessionkeep.exe> <empty sandbox dir>
set -u
EXE="$1"; B="$2"
mkdir -p "$B"
export SESSIONKEEP_INSTALL_DIR="$(cygpath -w "$B/Programs é/Dev Weft")"
export SESSIONKEEP_DATA_DIR="$(cygpath -w "$B/data")"
OWNER_CFG="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
digest() { (cd "$OWNER_CFG" && ls settings*.json plugins/installed_plugins.json plugins/known_marketplaces.json 2>/dev/null | xargs sha256sum | sha256sum | cut -c1-16); }
BEFORE=$(digest)
scrub() { sed -e "s#$(cygpath -w "$B" | sed 's#\\#\\\\#g')#<BOX>#g" -e "s#$(cygpath -m "$B")#<BOX>#g" -e "s/$USERNAME/<USER>/g"; }

echo "== install"
"$EXE" install | scrub
I="$B/Programs é/Dev Weft/sessionkeep.exe"
echo "== installed program runs from its own folder"
"$I" version
echo "== claude plugin validate (plugin, then marketplace)"
claude plugin validate "$(cygpath -w "$B/Programs é/Dev Weft/claude-plugin/sessionkeep")" 2>&1 | tail -1
claude plugin validate "$(cygpath -w "$B/Programs é/Dev Weft/claude-plugin")" 2>&1 | tail -1

echo "== the two commands the owner would run, in an ISOLATED Claude profile (not the owner's)"
export CLAUDE_CONFIG_DIR="$(cygpath -w "$B/claude-profile")"
mkdir -p "$B/claude-profile"
claude plugin marketplace add "$(cygpath -w "$B/Programs é/Dev Weft/claude-plugin")" 2>&1 | tail -2 | scrub
claude plugin install sessionkeep@sessionkeep-local 2>&1 | tail -2 | scrub
claude plugin list 2>&1 | grep -i -A3 sessionkeep | head -6 | scrub
echo "-- hook command recorded in the isolated profile's installed copy:"
find "$B/claude-profile" -name hooks.json -path "*sessionkeep*" 2>/dev/null | head -1 | while read -r f; do python -I -c "import json,sys; d=json.load(open(sys.argv[1],encoding='utf-8')); print(d['hooks']['Stop'][0]['hooks'][0]['command'])" "$f" | scrub; done
claude plugin uninstall sessionkeep@sessionkeep-local 2>&1 | tail -1 | scrub
claude plugin marketplace remove sessionkeep-local 2>&1 | tail -1 | scrub
unset CLAUDE_CONFIG_DIR
export CLAUDE_CONFIG_DIR="$OWNER_CFG"
echo "owner's Claude settings digest: before=$BEFORE after=$(digest)"

echo "== logon entry: create, inspect, run its command once, remove"
"$I" autostart status
"$I" autostart enable | scrub
NAME=$("$I" autostart status --json | python -I -c "import json,sys; print(json.load(sys.stdin)['data']['task_name'])")
echo "-- as Windows sees it (HKCU ...\\CurrentVersion\\Run):"
reg.exe query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Run' //v "$NAME" 2>&1 | grep -i sessionkeep | scrub | sed -E 's/drain [0-9a-f]{8}/drain <id>/'
echo "-- run that exact command once, with events waiting in the sandbox spool:"
"$I" project add "$(cygpath -w "$B")" --capture conversation > /dev/null
python -I -c "import json,sys; sys.stdout.write(json.dumps({'session_id':'s','cwd':sys.argv[1],'hook_event_name':'UserPromptSubmit','prompt':'waiting event'}))" "$(cygpath -w "$B")" | "$I" hook UserPromptSubmit
"$I" status | sed -n 2p
"$(cygpath -u "$SYSTEMROOT")/System32/conhost.exe" --headless "$(cygpath -w "$I")" ingest --quiet; echo "command exit code: $?"
"$I" status | sed -n 1,2p
"$I" autostart disable
reg.exe query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Run' //v "$NAME" > /dev/null 2>&1 && echo "entry still exists (unexpected)" || echo "entry is gone"

echo "== uninstall (run from the build binary), data kept"
"$EXE" uninstall | scrub
echo "files left in the install folder: $(find "$B/Programs é" -type f 2>/dev/null | wc -l)"
echo "database still present: $([ -f "$B/data/sessionkeep.db" ] && echo yes || echo no)"
