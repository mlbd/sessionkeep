"""Slice 1B-5: time from process start to exit for the SessionStart hook when it returns 20
recorded instructions and a checkpoint. Synthetic data only.
Usage: python -I recall_bench.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, subprocess, sys, time

exe, box = sys.argv[1], sys.argv[2]
proj = os.path.join(box, "work", "p"); os.makedirs(proj)
env = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "data"))

def run(*args, stdin=b""):
    return subprocess.run([exe, *args], input=stdin, capture_output=True, env=env)

run("project", "add", proj, "--capture", "conversation")
for i in range(20):
    run("remember", f"Rule {i:02d}: " + "always check the migration suite and the release notes before tagging. " * 3, "--scope", "project", "--project", proj, "--yes")
run("checkpoint", "Twenty rules recorded. Next: measure recall.", "--project", proj)
body = json.dumps({"session_id": "bench", "cwd": proj, "hook_event_name": "SessionStart", "source": "startup"}).encode()
lat, size = [], 0
for _ in range(300):
    t = time.perf_counter()
    p = run("hook", "SessionStart", stdin=body)
    lat.append((time.perf_counter() - t) * 1000)
    size = len(json.loads(p.stdout)["hookSpecificOutput"]["additionalContext"])
lat.sort()
pct = lambda p: lat[int(round(p / 100 * (len(lat) - 1)))]
print(f"SessionStart hook returning 20 instructions + checkpoint ({size} characters), spawn->exit: n={len(lat)} p50={pct(50):.1f}ms p95={pct(95):.1f}ms p99={pct(99):.1f}ms max={lat[-1]:.1f}ms")
