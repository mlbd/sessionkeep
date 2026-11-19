"""Driver for the spool spike. Usage: python -I drive.py <spike.exe> <empty work dir>. Synthetic data only."""
import json, os, random, subprocess, sys, time, uuid

exe, work = sys.argv[1], sys.argv[2]
os.makedirs(os.path.join(work, "spool"), exist_ok=True)
rnd = random.Random(7)

def pct(xs, p):
    xs = sorted(xs); return xs[int(round(p / 100 * (len(xs) - 1)))]

# 1. hook latency from spawn to exit, 2 KB payload on stdin, 1000 events
ids, lat = [], []
for i in range(1000):
    eid = uuid.uuid4().hex
    ids.append(eid)
    data = json.dumps({"event_id": eid, "kind": "user_prompt", "payload": {"text": "x" * 1900}}).encode()
    t = time.perf_counter()
    r = subprocess.run([exe, "hook", work], input=data, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    lat.append((time.perf_counter() - t) * 1000)
    assert r.returncode == 0
print(f"hook spawn->exit incl. fsync+rename: n={len(lat)} p50={pct(lat,50):.1f}ms p95={pct(lat,95):.1f}ms p99={pct(lat,99):.1f}ms max={max(lat):.1f}ms")
print("spool files after 1000 hooks:", len([f for f in os.listdir(os.path.join(work, 'spool')) if f.endswith('.evt')]))

# 2. hook must fail open on garbage input and on a missing spool directory
r1 = subprocess.run([exe, "hook", work], input=b"{not json", capture_output=True)
r2 = subprocess.run([exe, "hook", os.path.join(work, "does-not-exist")], input=b'{"event_id":"a","kind":"k","payload":{}}', capture_output=True)
print("fail-open: garbage exit", r1.returncode, "stdout", len(r1.stdout), "| missing dir exit", r2.returncode, "stdout", len(r2.stdout))

# 3. two consumers at once: exactly one should get the lease
env = dict(os.environ, SLOW_MS="2")
a = subprocess.Popen([exe, "consume", work], env=env, stdout=subprocess.PIPE)
time.sleep(0.3)
b = subprocess.run([exe, "consume", work], capture_output=True)
print("second consumer while first runs: exit", b.returncode, b.stdout.decode().strip())
time.sleep(0.4)
a.kill(); a.wait()  # hard kill (TerminateProcess) mid-drain
print("first consumer hard-killed mid-drain")

# 4. crash/replay loop: injected exits before/after commit, plus hard kills, until the spool is empty
crashes = {"before_commit": 0, "after_commit": 0, "kill": 0}
rounds = 0
while True:
    rounds += 1
    left = len([f for f in os.listdir(os.path.join(work, "spool")) if f.endswith(".evt")])
    if left == 0:
        break
    # only crash modes while more than 60 files remain, so every phase of the drain is interrupted
    mode = rnd.choice(["before_commit", "after_commit", "kill"]) if left > 60 else "clean"
    if mode == "kill":
        p = subprocess.Popen([exe, "consume", work], env=dict(os.environ, SLOW_MS="1"), stdout=subprocess.DEVNULL)
        time.sleep(rnd.uniform(0.05, 0.25)); p.kill(); p.wait(); crashes["kill"] += 1
    elif mode == "clean":
        subprocess.run([exe, "consume", work], env=dict(os.environ, SLOW_MS="0"), stdout=subprocess.DEVNULL)
    else:
        e = dict(os.environ, CRASH_AFTER=str(rnd.randint(1, 40)), CRASH_POINT=mode)
        rc = subprocess.run([exe, "consume", work], env=e, stdout=subprocess.DEVNULL).returncode
        if rc == 3: crashes[mode] += 1
print("rounds:", rounds, "injected crashes:", crashes)
out = subprocess.run([exe, "verify", work], capture_output=True).stdout.decode().strip()
print("verify:", out, "| expected rows=1000 distinct=1000")
