#!/usr/bin/env bash
# Phase 1 required end-to-end demonstration (phases/PHASE_1.md), against the real binary and real
# Claude Code sessions. Synthetic projects and sandbox data only; the owner's Claude settings and
# the real SessionKeep data directory are not touched.
# Usage: demo.sh <sessionkeep.exe> <empty scratch dir outside any repository>
set -u
EXE="$1"; L="$2"
mkdir -p "$L/proj one" "$L/proj two" "$L/out"
export SESSIONKEEP_INSTALL_DIR="$(cygpath -w "$L/Programs/SessionKeep")"
export SESSIONKEEP_DATA_DIR="$(cygpath -w "$L/data")"
OWNER_CFG="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
digest() { (cd "$OWNER_CFG" && ls settings*.json plugins/installed_plugins.json 2>/dev/null | xargs sha256sum | sha256sum | cut -c1-16); }
BEFORE=$(digest)
scrub() { sed -E -e "s/$USERNAME/<USER>/Ig" -e 's/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/<UUID>/g' -e 's/\(([0-9a-f]{8})\)/(<id>)/g' -e 's/^[0-9a-f]{8}  /<id>      /'; }
step() { printf '\n### %s\n' "$1"; }
P1="$(cygpath -w "$L/proj one")"; P2="$(cygpath -w "$L/proj two")"

step "1. Install into a sandbox and register a project with explicit consent"
"$EXE" install | sed -n 1p
D="$L/Programs/SessionKeep/sessionkeep.exe"
PLUG="$(cygpath -w "$L/Programs/SessionKeep/claude-plugin/sessionkeep")"
"$D" project add "$P1" --capture conversation | scrub
"$D" project add "$P2" --capture conversation | scrub | sed -n 1p

step "2. Record a long-lived instruction (preview, then confirm)"
"$D" remember "In this project every reply must end with the line: [checked: kestrel-42]" --scope project --project "$P1" | scrub | sed -n 1,5p
"$D" remember "In this project every reply must end with the line: [checked: kestrel-42]" --scope project --project "$P1" --yes | scrub | sed -n 1p

claude_run() { # name cwd prompt
  (cd "$2" && claude -p "$3" --plugin-dir "$PLUG" --setting-sources project --strict-mcp-config --no-session-persistence --model sonnet --allowedTools Bash Skill --output-format json > "$L/out/$1.json" 2> "$L/out/$1.err")
  python -I -X utf8 -c "
import json,sys
d=json.load(open(sys.argv[1],encoding='utf-8'))
print('reply   :', repr(d.get('result')))
print('turns', d.get('num_turns'), '| cost', d.get('total_cost_usd'), '| error', d.get('is_error'))" "$(cygpath -w "$L/out/$1.json")" | scrub
}

step "3. First real Claude Code session: a discussion and a task"
claude_run s1 "$L/proj one" "We are designing the nightly export. Decision for the record: the export job runs at 02:00 UTC and writes ISO 8601 dates. Acknowledge in one sentence."
sleep 2
"$D" checkpoint "Export schedule decided (02:00 UTC, ISO dates). Next: write the retry logic." --project "$P1" | scrub | sed -n 1p
"$D" status | sed -n 1,2p

step "4. Second session with nothing running in between: capture continues, instruction and history are recalled"
claude_run s2 "$L/proj one" "I am back after a break. What time does the export job run, and what is my next step? Use what is recorded for this project."
sleep 2
"$D" ingest
"$D" status | sed -n 1,2p

step "5. Every record once: sessions, events and text records"
python -I "$(dirname "$0")/dump_db.py" "$(cygpath -w "$L/data/sessionkeep.db")" | scrub | cut -c1-200 | sed -n '1,22p'

step "6. Search the original evidence from the command line"
"$D" memory search "export job 02:00 UTC" --project "$P1" --who user | scrub | cut -c1-200

step "7. Correct the instruction; a new session must follow the new one only"
ID=$("$D" instruction list --json | python -I -c "import json,sys; print(json.load(sys.stdin)['data'][0]['id'])")
"$D" instruction supersede "$ID" "In this project every reply must end with the line: [checked: osprey-77]" --reason "new marker" --yes | scrub
claude_run s3 "$L/proj one" "Reply with the single word READY."
"$D" instruction show "$ID" | scrub | grep -E "revision|checked"

step "8. Ninety days later (all SessionKeep files aged), the instruction is recovered by id and still injected"
python -I -c "
import os,sys,time
old=time.time()-90*86400
for r,_,fs in os.walk(sys.argv[1]):
    for f in fs: os.utime(os.path.join(r,f),(old,old))" "$(cygpath -w "$L/data")"
"$D" instruction show "$ID" | scrub | sed -n 1,3p
python -I -c "import json,sys; sys.stdout.write(json.dumps({'session_id':'day-90','cwd':sys.argv[1],'hook_event_name':'SessionStart','source':'startup'}))" "$P1" | "$D" hook SessionStart | python -I -X utf8 -c "
import json,sys
t=json.load(sys.stdin)['hookSpecificOutput']['additionalContext']
print('injected at day 90:', 'osprey-77' in t, '| old text injected:', 'kestrel-42' in t)
print(t.splitlines()[1])"

step "9. An unrelated project sees none of it"
"$D" memory search "export job" --project "$P2" | scrub
python -I -c "import json,sys; sys.stdout.write(json.dumps({'session_id':'p2','cwd':sys.argv[1],'hook_event_name':'SessionStart','source':'startup'}))" "$P2" | "$D" hook SessionStart | wc -c | sed 's/^/bytes injected into project two at session start: /'
"$D" resume --project "$P2" | scrub | sed -n '1,8p'

step "10. Export, restore into a fresh profile, compare, and the task is preserved"
"$D" export "$(cygpath -w "$L/backup.sessionkeep")" | scrub | sed -n 2p
SESSIONKEEP_DATA_DIR="$(cygpath -w "$L/restored")" "$D" import "$(cygpath -w "$L/backup.sessionkeep")" | scrub | grep -E "contains|RESTORED|integrity"
echo "source  : $("$D" status | sed -n 1p)"
echo "restored: $(SESSIONKEEP_DATA_DIR="$(cygpath -w "$L/restored")" "$D" status | sed -n 1p)"
SESSIONKEEP_DATA_DIR="$(cygpath -w "$L/restored")" "$D" resume --project "$P1" | scrub | sed -n '/Latest checkpoint/,/at that time/p'

step "11. Owner's Claude Code settings"
echo "digest before=$BEFORE after=$(digest)"
