"""Slice 1A-5 process-level check: concurrent hook processes, then drains interrupted by hard
kills, with the real binary. Synthetic data only.
Usage: python -I ingest_kill.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, random, subprocess, sys, time
from concurrent.futures import ThreadPoolExecutor

exe, box = sys.argv[1], sys.argv[2]
data = os.path.join(box, "data dir")
env = dict(os.environ, SESSIONKEEP_DATA_DIR=data)
rnd = random.Random(11)

def run(args, stdin=b""):
    return subprocess.run([exe] + args, input=stdin, capture_output=True, env=env)

def status():
    return json.loads(run(["status", "--json"]).stdout)["data"]

projects = []
for i in range(5):
    d = os.path.join(box, "work", f"proj {i}")
    os.makedirs(d)
    assert run(["project", "add", d, "--capture", "conversation"]).returncode == 0
    projects.append(d)

# 1. 20 concurrent writers, 50 events each, across 5 projects
def writer(w):
    bad = 0
    for i in range(50):
        body = json.dumps({"session_id": f"sess-{w}", "cwd": projects[w % 5], "hook_event_name": "UserPromptSubmit",
                           "prompt_id": f"{w}-{i}", "prompt": f"writer {w} prompt {i} " + "x" * 500}).encode()
        p = run(["hook", "UserPromptSubmit"], body)
        bad += p.returncode != 0 or p.stdout != b""
    return bad
t = time.perf_counter()
with ThreadPoolExecutor(20) as ex:
    bad = sum(ex.map(writer, range(20)))
print(f"20 concurrent hook writers x 50 events: {time.perf_counter()-t:.1f}s, non-silent or failed hooks: {bad}")
s = status()
print("after hooks: spool_pending", s["spool_pending"], "stored events", s["stored"]["events"], "gaps not imported", s["gaps_not_yet_imported"])

# 2. two drains at once: exactly one may hold the lease
a = subprocess.Popen([exe, "ingest", "--json"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
time.sleep(0.15)
b = run(["ingest", "--json"])
print("second ingest while the first runs: exit", b.returncode, "|", b.stderr.decode().strip())
a.kill(); a.wait()
print("first ingest hard-killed mid-drain; stored so far:", status()["stored"]["events"])

# 3. drains interrupted by hard kills at random times until the spool is empty
kills = rounds = 0
while status()["spool_pending"] > 0:
    rounds += 1
    left = status()["spool_pending"]
    if left > 40 and rounds < 200:
        p = subprocess.Popen([exe, "ingest"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)
        time.sleep(rnd.uniform(0.03, 0.25))
        if p.poll() is None:
            p.kill(); kills += 1
        p.wait()
    else:
        run(["ingest"])
s = status()
print(f"drain rounds: {rounds}, hard kills that interrupted a running drain: {kills}")
print("final:", json.dumps(s["stored"]), "| spool_pending", s["spool_pending"], "| quarantined", s["quarantined_files"])
print("expected: events 1000, evidence 1000, sessions 20, projects 5, spool 0, quarantined 0")
p = run(["ingest", "--json"])
print("one more ingest:", p.stdout.decode().strip())
