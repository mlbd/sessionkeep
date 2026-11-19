#!/usr/bin/env bash
# Slice 1A-9 live check of the generated skills. Synthetic project and sandbox data only.
# Usage: live_skills.sh <sessionkeep.exe> <empty scratch dir outside any repository>
set -u
EXE="$1"; L="$2"
mkdir -p "$L/proj" "$L/out"
cp "$EXE" "$L/sessionkeep.exe"; D="$L/sessionkeep.exe"
export SESSIONKEEP_DATA_DIR="$(cygpath -w "$L/data")"
P="$(cygpath -w "$L/proj")"
"$D" plugin write "$(cygpath -w "$L/plugin/sessionkeep")" > /dev/null
claude plugin validate "$(cygpath -w "$L/plugin/sessionkeep")" 2>&1 | tail -1
"$D" project add "$P" --capture conversation > /dev/null

say() { # event session text
  python -I -c "import json,sys; e,s,c,t=sys.argv[1:5]; b={'session_id':s,'cwd':c,'hook_event_name':e}; b['last_assistant_message' if e=='Stop' else 'prompt']=t; sys.stdout.write(json.dumps(b))" "$1" "$2" "$P" "$3" | "$D" hook "$1"
}
say UserPromptSubmit old-1 "We decided the deploy key rotation happens on the first Monday of each month, and the on-call owner is the platform team."
say Stop old-1 "Recorded. Rotation: first Monday monthly. Owner: platform team."
"$D" ingest > /dev/null
"$D" checkpoint "Rotation script drafted in rotate.sh. Next: add the dry-run flag, then write the runbook." --project "$P" > /dev/null

digest() { (cd "$CLAUDE_CONFIG_DIR" && ls settings*.json plugins/installed_plugins.json 2>/dev/null | xargs sha256sum | sha256sum | cut -c1-16); }
B=$(digest)
ask() { # name prompt
  (cd "$L/proj" && claude -p "$2" --plugin-dir "$(cygpath -w "$L/plugin")" --setting-sources project --strict-mcp-config --no-session-persistence --model sonnet --allowedTools Bash Skill --output-format json > "$L/out/$1.json" 2> "$L/out/$1.err")
  echo "--- $1: claude exit=$?"
  python -I -X utf8 -c "
import json,sys
d=json.load(open(sys.argv[1],encoding='utf-8'))
print('turns', d.get('num_turns'), '| cost', d.get('total_cost_usd'), '| error', d.get('is_error'))
print('reply:'); print(d.get('result'))" "$(cygpath -w "$L/out/$1.json")"
}
ask resume_recall "I'm back on this project after a break. Where did we leave off, what is the next step, and what did we decide about deploy key rotation? Answer briefly."
ask remember "From now on, always indent with tabs in this project. Please remember that permanently."
echo "--- instructions actually recorded after the remember request:"
"$D" instruction list
echo "--- settings digest before=$B after=$(digest)"
"$D" status | head -2
