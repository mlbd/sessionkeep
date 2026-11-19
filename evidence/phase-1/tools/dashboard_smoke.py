"""Slice 1B-2: start the real `sessionkeep dashboard`, sign in with the printed link and call the API,
using plain HTTP requests (no browser). Synthetic data only.
Usage: python -I dashboard_smoke.py <sessionkeep.exe> <empty sandbox dir>"""
import http.cookiejar, json, os, re, subprocess, sys, time, urllib.error, urllib.request

exe, box = sys.argv[1], sys.argv[2]
proj = os.path.join(box, "work", "demo")
os.makedirs(proj)
env = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "data"))
subprocess.run([exe, "project", "add", proj, "--capture", "conversation"], env=env, capture_output=True)

p = subprocess.Popen([exe, "dashboard"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
text = b""
while b"/auth?t=" not in text:
    text += p.stdout.readline()
link = re.search(rb"http://127\.0\.0\.1:\d+/auth\?t=[0-9a-f]+", text).group().decode()
base = link.split("/auth")[0]
print("server printed a sign-in link on", base.replace(base.split(":")[-1], "<port>"), "| token length", len(link.split("t=")[1]))

def get(opener, path, headers=None, method="GET", body=None):
    req = urllib.request.Request(base + path, method=method, headers=headers or {}, data=body)
    try:
        r = opener.open(req, timeout=10)
        return r.status, r.read(), r.headers
    except urllib.error.HTTPError as e:
        return e.code, e.read(), e.headers

anon = urllib.request.build_opener()
print("GET / (no session):", get(anon, "/")[0], "| GET /app.js:", get(anon, "/app.js")[0], "| GET /api/status (no session):", get(anon, "/api/status")[0])

jar = http.cookiejar.CookieJar()
me = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
status, _, _ = get(me, link[len(base):])
print("open sign-in link:", status, "| cookies set:", [c.name for c in jar], "| HttpOnly:", all(c.has_nonstandard_attr("HttpOnly") for c in jar))
print("open the same link again:", get(anon, link[len(base):])[0])
s, raw, hdr = get(me, "/api/session")
csrf = json.loads(raw)["data"]["csrf"]
print("GET /api/session:", s, "| CSP:", hdr.get("Content-Security-Policy")[:60], "...")
s, raw, _ = get(me, "/api/projects")
print("GET /api/projects:", s, [(x["name"], x["capture_level"]) for x in json.loads(raw)["data"]])
s, raw, _ = get(me, "/api/status")
print("GET /api/status:", s, json.loads(raw)["data"]["stored"])
pid = json.loads(get(me, "/api/projects")[1])["data"][0]["id"]
print("POST pause without the request token:", get(me, f"/api/projects/{pid}/pause", {"Content-Type": "application/json"}, "POST", b"{}")[0])
print("POST pause from another origin:", get(me, f"/api/projects/{pid}/pause", {"Content-Type": "application/json", "X-SessionKeep-CSRF": csrf, "Origin": "https://evil.example"}, "POST", b"{}")[0])
print("POST pause as the signed-in page:", get(me, f"/api/projects/{pid}/pause", {"Content-Type": "application/json", "X-SessionKeep-CSRF": csrf, "Origin": base}, "POST", b"{}")[0])
print("GET with a foreign Host header:", get(me, "/api/status", {"Host": "evil.example"})[0])

ps = subprocess.run(["powershell", "-NoProfile", "-Command", f"(Get-Process -Id {p.pid}).WorkingSet64; (Get-NetTCPConnection -OwningProcess {p.pid} -State Listen | Select-Object -ExpandProperty LocalAddress) -join ','"], capture_output=True, text=True).stdout.split()
print("memory while serving:", int(ps[0]) // 1024, "KiB | listening on:", ps[1] if len(ps) > 1 else "?")
p.terminate()
p.wait(timeout=10)
time.sleep(0.3)
try:
    urllib.request.urlopen(base + "/", timeout=2)
    print("after stop: still answering (unexpected)")
except Exception as e:
    print("after stop: no longer reachable")
