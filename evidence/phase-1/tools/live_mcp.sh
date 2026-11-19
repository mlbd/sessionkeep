#!/usr/bin/env bash
# Slice 1B-1 live check: Claude Code using the sessionkeep MCP server. Synthetic project, sandbox data.
# Usage: live_mcp.sh <sessionkeep.exe> <empty scratch dir outside any repository>
set -u
EXE="$1"; L="$2"
mkdir -p "$L/proj" "$L/out"
cp "$EXE" "$L/sessionkeep.exe"; D="$L/sessionkeep.exe"
export SESSIONKEEP_DATA_DIR="$(cygpath -w "$L/data")"
P="$(cygpath -w "$L/proj")"
"$D" project add "$P" --capture conversation > /dev/null
python -I -c "import json,sys; sys.stdout.write(json.dumps({'session_id':'old','cwd':sys.argv[1],'hook_event_name':'UserPromptSubmit','prompt':'We decided the staging database is refreshed every Tuesday night, and the owner is the data team.'}))" "$P" | "$D" hook UserPromptSubmit
"$D" ingest > /dev/null
python -I -c "import json,sys; json.dump({'mcpServers':{'sessionkeep':{'command':sys.argv[1],'args':['mcp']}}}, open(sys.argv[2],'w'))" "$(cygpath -w "$D")" "$(cygpath -w "$L/mcp.json")"
cd "$L/proj" && claude -p "Using only the sessionkeep memory tools, tell me when the staging database is refreshed and who owns it. If the tools do not say, answer that you do not know." \
  --mcp-config "$(cygpath -w "$L/mcp.json")" --strict-mcp-config --setting-sources project --no-session-persistence --model sonnet \
  --allowedTools mcp__sessionkeep__memory_search mcp__sessionkeep__memory_get mcp__sessionkeep__resume_pack mcp__sessionkeep__project_status \
  --output-format stream-json --verbose > "$L/out/run.jsonl" 2> "$L/out/run.err"
echo "claude exit=$?"
python -I -X utf8 -c "
import json,sys
for l in open(sys.argv[1],encoding='utf-8',errors='replace'):
    try: e=json.loads(l)
    except Exception: continue
    if e.get('type')=='system' and e.get('subtype')=='init':
        print('mcp servers:', [(m.get('name'), m.get('status')) for m in e.get('mcp_servers',[])])
        print('sessionkeep tools offered:', sorted(t for t in e.get('tools',[]) if 'sessionkeep' in t))
    if e.get('type')=='assistant':
        for c in e.get('message',{}).get('content',[]):
            if c.get('type')=='tool_use': print('tool call:', c.get('name'), json.dumps(c.get('input')))
    if e.get('type')=='result':
        print('turns', e.get('num_turns'), '| cost', e.get('total_cost_usd')); print('reply:', e.get('result'))
" "$(cygpath -w "$L/out/run.jsonl")"
