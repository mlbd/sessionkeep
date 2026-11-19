"""Slice 1B-3 real-binary walkthrough: export, verify, restore into a fresh profile, delete.
Synthetic data only. Usage: python -I data_walk.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, re, subprocess, sys

sys.stdout.reconfigure(encoding="utf-8")
exe, box = sys.argv[1], sys.argv[2]
src = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "profile A"))
dst = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "profile B"))
proj = os.path.join(box, "work", "alpha"); os.makedirs(proj)
backup = os.path.join(box, "backup.sessionkeep")

def scrub(s):
    s = s.replace(box, "<BOX>").replace(box.lower(), "<box>")
    s = re.sub(r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b", "<UUID>", s)
    return re.sub(r"\b[0-9a-f]{8}-[0-9a-f]{3}\b", "<id>", s)

def run(env, *args, stdin=b"", show=True):
    p = subprocess.run([exe, *args], input=stdin, capture_output=True, env=env)
    if show:
        who = "A" if env is src else "B"
        print(f"[{who}] $ sessionkeep " + " ".join('"' + scrub(a) + '"' if " " in a else scrub(a) for a in args))
        print(scrub((p.stdout + p.stderr).decode("utf-8", "replace").rstrip()))
        print(f"[exit {p.returncode}]")
    return p

def say(text):
    body = {"session_id": "s1", "cwd": proj, "hook_event_name": "UserPromptSubmit", "prompt": text}
    run(src, "hook", "UserPromptSubmit", stdin=json.dumps(body).encode(), show=False)

run(src, "project", "add", proj, "--capture", "conversation", show=False)
say("Invoices are kept for seven years. Marker ZEBRAQUOKKA77.")
say("The export job runs nightly at 02:00 UTC.")
run(src, "ingest", show=False)
run(src, "remember", "Always use ISO 8601 dates.", "--scope", "global", "--yes", show=False)
run(src, "checkpoint", "Export job drafted. Next: retries.", "--project", proj, show=False)

run(src, "export", backup)
run(dst, "import", backup, "--dry-run")
run(dst, "import", backup)
run(dst, "memory", "search", "invoices seven years", "--project", proj)
run(dst, "import", backup)

hit = json.loads(run(src, "memory", "search", "ZEBRAQUOKKA77", "--project", proj, "--json", show=False).stdout)["data"]["hits"][0]["id"]
run(src, "forget", "record", hit, "--project", proj)
run(src, "forget", "record", hit, "--project", proj, "--yes")
run(src, "memory", "search", "ZEBRAQUOKKA77", "--project", proj, "--exact")
found = []
for root, _, files in os.walk(src["SESSIONKEEP_DATA_DIR"]):
    for f in files:
        if b"zebraquokka77" in open(os.path.join(root, f), "rb").read().lower():
            found.append(f)
print("files in profile A that still contain the deleted marker (any letter case):", found or "none")
print("the earlier export still contains it (as the command warns):", b"zebraquokka77" in open(backup, "rb").read().lower())
