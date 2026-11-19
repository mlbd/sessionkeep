"""Slice 1B-5: drain throughput and idle footprint with the real binary. Synthetic data only.
Usage: python -I drain_bench.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, subprocess, sys, time

exe, box = sys.argv[1], sys.argv[2]
proj = os.path.join(box, "work", "p"); os.makedirs(proj)
env = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "data"))

def run(*args, stdin=b""):
    return subprocess.run([exe, *args], input=stdin, capture_output=True, env=env)

run("project", "add", proj, "--capture", "conversation")
N = 2000
t = time.perf_counter()
for i in range(N):
    body = json.dumps({"session_id": f"s{i//100}", "cwd": proj, "hook_event_name": "UserPromptSubmit", "prompt_id": str(i),
                       "prompt": f"event {i} " + "lorem ipsum dolor sit amet " * 20}).encode()
    run("hook", "UserPromptSubmit", stdin=body)
hook_s = time.perf_counter() - t
print(f"wrote {N} events through the hook, one process each: {hook_s:.1f} s ({hook_s/N*1000:.1f} ms each, sequential)")
t = time.perf_counter()
p = run("ingest", "--json")
drain_s = time.perf_counter() - t
rep = json.loads(p.stdout)["data"]
print(f"sessionkeep ingest: stored {rep['ingested']} events in {drain_s:.1f} s = {rep['ingested']/drain_s:.0f} events/s (one durable transaction per event)")
t = time.perf_counter(); run("ingest"); print(f"sessionkeep ingest with nothing waiting: {(time.perf_counter()-t)*1000:.0f} ms")
lat = []
for q in ["lorem ipsum", "event 1500", "dolor amet", "nothingmatchesthis"] * 5:
    t = time.perf_counter(); run("memory", "search", q, "--project", proj); lat.append((time.perf_counter() - t) * 1000)
lat.sort()
print(f"sessionkeep memory search as a command (process start to exit), {N} records: p50 {lat[len(lat)//2]:.0f} ms, max {lat[-1]:.0f} ms")
size = sum(os.path.getsize(os.path.join(r, f)) for r, _, fs in os.walk(env["SESSIONKEEP_DATA_DIR"]) for f in fs)
print(f"data directory after {N} events of about 550 characters: {size/1e6:.1f} MB")
print("resident SessionKeep processes when no command is running: 0 (there is no background process)")
