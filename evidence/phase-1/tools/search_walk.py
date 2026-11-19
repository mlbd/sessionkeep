"""Slice 1A-8 real-binary walkthrough of search. Synthetic data only.
Usage: python -I search_walk.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, re, subprocess, sys

exe, box = sys.argv[1], sys.argv[2]
sys.stdout.reconfigure(encoding="utf-8")
env = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "data"))
alpha, beta = os.path.join(box, "work", "alpha"), os.path.join(box, "work", "beta")
os.makedirs(alpha); os.makedirs(beta)

def scrub(s):
    s = s.replace(box, "<BOX>").replace(box.lower(), "<box>")
    return re.sub(r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b", "<UUID>", s)

def run(*args, stdin=b"", show=True):
    p = subprocess.run([exe, *args], input=stdin, capture_output=True, env=env)
    if show:
        print("$ sessionkeep " + " ".join(scrub(a) if " " not in a else '"' + scrub(a) + '"' for a in args))
        out = (p.stdout + p.stderr).decode("utf-8", "replace").rstrip()
        print(re.sub(r"(?m)^[0-9a-f]{8}  ", "<id>      ", scrub(out)))
        print(f"[exit {p.returncode}]")
    return p

def say(event, session, cwd, text):
    body = {"session_id": session, "cwd": cwd, "hook_event_name": event}
    body["last_assistant_message" if event == "Stop" else "prompt"] = text
    run("hook", event, stdin=json.dumps(body).encode(), show=False)

run("project", "add", alpha, "--capture", "conversation", show=False)
run("project", "add", beta, "--capture", "conversation", show=False)
say("UserPromptSubmit", "s1", alpha, "We decided the deploy key rotation happens on the first Monday of each month.")
say("Stop", "s1", alpha, "Recorded: deploy key rotation on the first Monday.")
say("UserPromptSubmit", "s1", alpha, r"The build fails with ECONNRESET: socket hang up at POST /api/v2/users?id=7 in C:\Work\App\src\auth_service.ts")
for i in range(12):
    say("UserPromptSubmit", "s2", beta, f"BETA-CONFIDENTIAL-{i} deploy key rotation deploy key rotation first Monday")

run("memory", "search", "deploy key rotation", "--project", alpha)
run("ingest")
run("memory", "search", "deploy key rotation", "--project", alpha)
run("memory", "search", "ECONNRESET: socket hang up at POST /api/v2/users?id=7", "--project", alpha)
run("memory", "search", r"c:\work\app\src\AUTH_SERVICE.TS", "--project", alpha)
run("memory", "search", "BETA-CONFIDENTIAL-3", "--project", alpha, "--exact")
run("memory", "search", "kubernetes helm chart", "--project", alpha)
run("memory", "search", "NEAR( \" * ) AND OR", "--project", alpha)
run("memory", "search", "first Monday", "--project", beta, "--limit", "3")
hit = json.loads(run("memory", "search", "rotation", "--project", alpha, "--who", "user", "--json", show=False).stdout)["data"]["hits"][0]["id"]
run("memory", "get", hit[:12], "--project", alpha)
run("memory", "get", hit, "--project", beta)
