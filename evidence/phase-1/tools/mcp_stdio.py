"""Slice 1B-1: talk to the real `sessionkeep mcp` binary over stdio and measure its memory.
Synthetic data only. Usage: python -I mcp_stdio.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, subprocess, sys, time

sys.stdout.reconfigure(encoding="utf-8")
exe, box = sys.argv[1], sys.argv[2]
proj = os.path.join(box, "work", "alpha")
os.makedirs(proj)
env = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "data"), CLAUDE_PROJECT_DIR=proj)

def run(*args, stdin=b""):
    return subprocess.run([exe, *args], input=stdin, capture_output=True, env=env)

run("project", "add", proj, "--capture", "conversation")
body = json.dumps({"session_id": "s", "cwd": proj, "hook_event_name": "UserPromptSubmit", "prompt": "We decided deploy key rotation happens on the first Monday."})
run("hook", "UserPromptSubmit", stdin=body.encode())
run("ingest")

p = subprocess.Popen([exe, "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
lines = []

def send(obj):
    p.stdin.write((json.dumps(obj) + "\n").encode()); p.stdin.flush()

def recv():
    line = p.stdout.readline()
    lines.append(line)
    return json.loads(line)

def rss_kb():
    out = subprocess.run(["powershell", "-NoProfile", "-Command", f"(Get-Process -Id {p.pid}).WorkingSet64"], capture_output=True, text=True).stdout.strip()
    return int(out) // 1024

send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "evidence", "version": "0"}}})
init = recv()
print("initialize ->", "protocolVersion", init["result"]["protocolVersion"], "| server", init["result"]["serverInfo"]["name"], init["result"]["serverInfo"]["version"])
send({"jsonrpc": "2.0", "method": "notifications/initialized"})
send({"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
tools = recv()["result"]["tools"]
print("tools/list ->", sorted(t["name"] for t in tools), "| all read-only:", all(t.get("annotations", {}).get("readOnlyHint") for t in tools))
print("memory after start, idle:", rss_kb(), "KiB")

send({"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "memory_search", "arguments": {"query": "deploy key rotation"}}})
r = recv()["result"]
print("memory_search -> isError", r.get("isError", False)); print("   ", r["content"][0]["text"].splitlines()[0][:120])
send({"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "memory_search", "arguments": {"query": "rotation", "all_projects": True}}})
r = recv()["result"]
print("memory_search all_projects -> isError", r.get("isError", False), "|", r["content"][0]["text"][:70])
send({"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "no_such_tool", "arguments": {}}})
r = recv()
print("unknown tool ->", "error" if "error" in r else "result", (r.get("error") or {}).get("message", "")[:60])
send({"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "project_status", "arguments": {}}})
r = recv()["result"]
print("project_status ->", r["content"][0]["text"].splitlines()[0])
for i in range(30):
    send({"jsonrpc": "2.0", "id": 100 + i, "method": "tools/call", "params": {"name": "memory_search", "arguments": {"query": "rotation"}}})
    recv()
time.sleep(1)
print("memory after 34 tool calls, idle:", rss_kb(), "KiB")

p.stdin.close()
code = p.wait(timeout=10)
rest = p.stdout.read()
err = p.stderr.read()
bad = [l for l in lines if not l.strip().startswith(b"{")]
print("server exit code after stdin closed:", code)
print("stdout lines:", len(lines), "| non-JSON lines:", len(bad), "| trailing stdout bytes:", len(rest), "| stderr bytes:", len(err))
