"""Read-only dump of a SessionKeep database for evidence. Usage: python -I dump_db.py <sessionkeep.db>"""
import json, sqlite3, sys

path = sys.argv[1].replace("\\", "/")
c = sqlite3.connect("file:" + path + "?mode=ro", uri=True)
print("events (kind, redaction, has text):")
for r in c.execute("select kind, redaction_status, evidence_id is not null from events order by observed_at"):
    print("  ", r)
print("text records (origin, whose words, text):")
for r in c.execute("select origin, trust_level, text from evidence order by seq"):
    print("  ", r)
print("tool event payload keys and tool_input:")
for (p,) in c.execute("select safe_payload from events where kind = 'tool_use'"):
    d = json.loads(p)
    print("  ", sorted(d.keys()), d.get("tool_input"))
print("sessions (agent, state, adapter):", list(c.execute("select agent, state, adapter_version from sessions")))
print("projects (name, level, added_by):", list(c.execute("select name, capture_policy, added_by from projects")))
