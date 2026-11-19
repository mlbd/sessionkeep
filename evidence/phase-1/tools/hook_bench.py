"""Slice 1A-4 process-level check of `sessionkeep hook`. Synthetic data only.
Usage: python -I hook_bench.py <sessionkeep.exe> <empty sandbox dir>"""
import json, os, subprocess, sys, time

exe, box = sys.argv[1], sys.argv[2]
data, proj = os.path.join(box, "data dir"), os.path.join(box, "work", "Demo App")
os.makedirs(os.path.join(proj, "src"))
env = dict(os.environ, SESSIONKEEP_DATA_DIR=data)

def run(args, stdin=b""):
    t = time.perf_counter()
    p = subprocess.run([exe] + args, input=stdin, capture_output=True, env=env)
    return p, (time.perf_counter() - t) * 1000

def pct(xs, p):
    xs = sorted(xs); return xs[int(round(p / 100 * (len(xs) - 1)))]

def spool():
    d = os.path.join(data, "spool")
    return sorted(os.listdir(d)) if os.path.isdir(d) else []

def payload(cwd, text):
    return json.dumps({"session_id": "bench-session", "cwd": cwd, "hook_event_name": "UserPromptSubmit",
                       "prompt_id": "p", "prompt": text}).encode()

# 1. before setup: silent, creates nothing
p, _ = run(["hook", "UserPromptSubmit"], payload(proj, "hello"))
print("before setup: exit", p.returncode, "stdout", len(p.stdout), "stderr", len(p.stderr), "data dir exists", os.path.exists(data))

p, _ = run(["project", "add", proj, "--capture", "conversation"])
assert p.returncode == 0, p.stderr

# 2. latency: 500 captures of a 2 KB prompt, timed from process start to exit
lat = []
body = payload(os.path.join(proj, "src"), "x" * 1900)
for _ in range(500):
    p, ms = run(["hook", "UserPromptSubmit"], body)
    assert p.returncode == 0 and p.stdout == b"" and p.stderr == b""
    lat.append(ms)
print(f"hook spawn->exit, registered project, 2 KB prompt: n={len(lat)} p50={pct(lat,50):.1f}ms p95={pct(lat,95):.1f}ms p99={pct(lat,99):.1f}ms max={max(lat):.1f}ms")
print("spool files:", len(spool()), "| temp files:", len([f for f in spool() if not f.endswith('.evt')]))

# 3. latency when nothing is captured (unregistered folder): decision cost only
lat = []
other = payload(os.path.join(box, "work", "Other"), "x" * 1900)
for _ in range(200):
    p, ms = run(["hook", "UserPromptSubmit"], other)
    assert p.returncode == 0 and p.stdout == b""
    lat.append(ms)
print(f"hook spawn->exit, unregistered folder: n={len(lat)} p50={pct(lat,50):.1f}ms p95={pct(lat,95):.1f}ms p99={pct(lat,99):.1f}ms")
print("spool files after unregistered runs:", len(spool()))

# 4. secrets: none of these may be in any file under the data directory
secret_prompt = ("key -----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAbenchSYNTHETICbody\n-----END RSA PRIVATE KEY----- "
                 "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJiZW5jaCJ9.c2lnbmF0dXJlLWJlbmNo "
                 "postgres://admin:Bench-Pw-123456@db.example.test/x curl -u bob:Bench-Curl-Pw https://example.test")
run(["hook", "UserPromptSubmit"], payload(proj, secret_prompt))
blob = b""
for root, _, files in os.walk(data):
    for f in files:
        with open(os.path.join(root, f), "rb") as fh:
            blob += fh.read()
leaks = [s for s in ("MIIEowIBAAKCAQEAbenchSYNTHETIC", "eyJhbGciOiJIUzI1NiJ9", "Bench-Pw-123456", "Bench-Curl-Pw") if s.encode() in blob]
print("secret strings found under the data directory:", leaks or "none", "| marker present:", b"[REDACTED:" in blob)

# 5. bad input and reminder
p, _ = run(["hook", "UserPromptSubmit"], b"{not json")
print("garbage input: exit", p.returncode, "stdout", len(p.stdout), "stderr", len(p.stderr))
p, _ = run(["hook", "SessionStart"], other)
print("unregistered SessionStart stdout:", p.stdout.decode().strip())
p, _ = run(["project", "pause", proj])
before = len(spool())
p, _ = run(["hook", "UserPromptSubmit"], payload(proj, "while paused"))
print("paused project: exit", p.returncode, "new spool files", len(spool()) - before)
