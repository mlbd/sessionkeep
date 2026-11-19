"""Phase 0 proxy probes. Synthetic data only. Python/C-SQLite proxy, NOT the target Go/modernc stack."""
import os, sys, time, json, sqlite3, statistics, subprocess, tempfile, random, shutil, uuid

def pct(xs, p):
    xs = sorted(xs); return xs[min(len(xs) - 1, int(round(p / 100 * (len(xs) - 1))))]

def summ(name, xs):
    print(f"{name}: n={len(xs)} p50={pct(xs,50):.2f}ms p95={pct(xs,95):.2f}ms p99={pct(xs,99):.2f}ms max={max(xs):.2f}ms")

root = tempfile.mkdtemp(prefix="p0probe_", dir=sys.argv[1])
try:
    # 1. durable spool write: temp file -> write 2KB -> fsync -> atomic rename (+ no dir fsync on Windows)
    spool = os.path.join(root, "spool"); os.makedirs(spool)
    payload = json.dumps({"version": 1, "kind": "user_prompt", "payload": {"text": "x" * 1900}}).encode()
    for label, do_sync in (("spool write+fsync+rename", True), ("spool write+rename (no fsync)", False)):
        xs = []
        for i in range(300):
            t = time.perf_counter()
            tmp = os.path.join(spool, f".{uuid.uuid4().hex}.tmp")
            with open(tmp, "wb") as f:
                f.write(payload)
                if do_sync:
                    f.flush(); os.fsync(f.fileno())
            os.replace(tmp, os.path.join(spool, f"{uuid.uuid4().hex}.evt"))
            xs.append((time.perf_counter() - t) * 1000)
        summ(label, xs)

    # 2. process spawn floor for a tiny native exe, directly vs through Git Bash vs node
    def spawn(argv, n=40):
        xs = []
        for _ in range(n):
            t = time.perf_counter()
            subprocess.run(argv, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL)
            xs.append((time.perf_counter() - t) * 1000)
        return xs
    hn = r"C:\Windows\System32\hostname.exe"
    summ("spawn native exe direct (exec form proxy)", spawn([hn]))
    bash = r"C:\Program Files\Git\bin\bash.exe"
    if os.path.exists(bash):
        summ("spawn native exe via Git Bash -c (shell form proxy)", spawn([bash, "-c", "hostname.exe"]))
    summ("spawn powershell -NoProfile (shell fallback proxy)", spawn(["powershell.exe", "-NoProfile", "-Command", "hostname.exe"], n=15))
    node = shutil.which("node")
    if node:
        summ("spawn node -e '' (JS hook baseline)", spawn([node, "-e", ""]))

    # 3. SQLite (C library via Python) WAL + FTS5 at 100k synthetic rows
    print("python sqlite3 lib version:", sqlite3.sqlite_version)
    dbp = os.path.join(root, "t.db")
    con = sqlite3.connect(dbp); con.execute("PRAGMA journal_mode=WAL"); con.execute("PRAGMA synchronous=FULL")
    con.execute("CREATE TABLE m(id INTEGER PRIMARY KEY, project TEXT, body TEXT)")
    con.execute("CREATE VIRTUAL TABLE f USING fts5(body, content='m', content_rowid='id')")
    rnd = random.Random(42)
    words = [f"w{n:04d}" for n in range(4000)] + ["oauth", "token", "migration", "ECONNRESET", "tenant", "isolation"]
    t = time.perf_counter()
    with con:
        for i in range(100_000):
            body = " ".join(rnd.choice(words) for _ in range(60))
            con.execute("INSERT INTO m(id,project,body) VALUES(?,?,?)", (i + 1, f"p{i % 5}", body))
            con.execute("INSERT INTO f(rowid,body) VALUES(?,?)", (i + 1, body))
    print(f"bulk insert 100k rows (one txn): {(time.perf_counter()-t):.1f}s  db size={os.path.getsize(dbp)/1e6:.0f}MB")
    con.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    # single durable inserts (synchronous=FULL, one txn each) = ingest/commit cost
    xs = []
    for i in range(200):
        t = time.perf_counter()
        with con:
            con.execute("INSERT INTO m(project,body) VALUES(?,?)", ("p0", "single durable insert"))
        xs.append((time.perf_counter() - t) * 1000)
    summ("single-row durable commit WAL synchronous=FULL", xs)
    # FTS queries with scope join, from a second read-only connection while writer connection stays open
    ro = sqlite3.connect(f"file:{dbp}?mode=ro", uri=True)
    q = "SELECT m.id, snippet(f,0,'[',']','..',8) FROM f JOIN m ON m.id=f.rowid WHERE f MATCH ? AND m.project=? ORDER BY bm25(f) LIMIT 20"
    xs = []
    for i in range(200):
        term = rnd.choice(["oauth token", "ECONNRESET", "tenant AND isolation", '"oauth token"', rnd.choice(words)])
        t = time.perf_counter(); ro.execute(q, (term, f"p{i % 5}")).fetchall(); xs.append((time.perf_counter() - t) * 1000)
    summ("FTS5 bm25 top-20 scoped query @100k rows (warm)", xs)
    # cold open + one query in a fresh process = daemon-free hook read path proxy
    code = ("import sqlite3,sys;c=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True);"
            "c.execute(\"SELECT body FROM m WHERE project='p1' ORDER BY id DESC LIMIT 20\").fetchall()")
    summ("fresh python process: open ro + indexed-less read (includes python startup)", spawn([sys.executable, "-I", "-c", code, dbp], n=20))
    summ("fresh python process: startup only (subtract)", spawn([sys.executable, "-I", "-c", "pass"], n=20))
    ro.close(); con.close()
finally:
    shutil.rmtree(root, ignore_errors=True)
