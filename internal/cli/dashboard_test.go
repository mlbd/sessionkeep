package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"sessionkeep/internal/web"
)

// browser is a minimal stand-in for a signed-in browser tab talking to a real server.
type browser struct {
	t      *testing.T
	srv    *web.Server
	base   string
	cookie *http.Cookie
	csrf   string
}

func startDashboard(t *testing.T, sb *sandbox) *web.Server {
	t.Helper()
	srv, err := web.Listen(runner(func(k string) string { return sb.env[k] }), "test")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return srv
}

var noRedirect = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func signIn(t *testing.T, sb *sandbox) *browser {
	t.Helper()
	srv := startDashboard(t, sb)
	b := &browser{t: t, srv: srv, base: "http://" + srv.Addr().String()}
	res, err := noRedirect.Get(srv.BootstrapURL())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || len(res.Cookies()) != 1 {
		t.Fatalf("sign-in: status %d, %d cookies", res.StatusCode, len(res.Cookies()))
	}
	b.cookie = res.Cookies()[0]
	if !b.cookie.HttpOnly || b.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie must be HttpOnly and SameSite=Strict: %+v", b.cookie)
	}
	if loc := res.Header.Get("Location"); loc != "/" {
		t.Fatalf("sign-in must redirect to a URL without the token, got %q", loc)
	}
	var s struct {
		Data struct {
			CSRF string `json:"csrf"`
		}
	}
	b.do("GET", "/api/session", nil, nil, &s)
	b.csrf = s.Data.CSRF
	return b
}

// do sends a request as the signed-in tab unless edit changes it, and returns the status.
func (b *browser) do(method, path string, body any, edit func(*http.Request), into any) int {
	b.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, b.base+path, rd)
	if b.cookie != nil {
		req.AddCookie(b.cookie)
	}
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-SessionKeep-CSRF", b.csrf)
		req.Header.Set("Origin", b.base)
	}
	if edit != nil {
		edit(req)
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			b.t.Fatalf("%s %s: status %d, body is not JSON: %q", method, path, res.StatusCode, raw)
		}
	}
	return res.StatusCode
}

func TestDashboardBindsLoopbackOnlyAndSetsSecurityHeaders(t *testing.T) {
	sb := newSandbox(t)
	srv := startDashboard(t, sb)
	addr := srv.Addr().(*net.TCPAddr)
	if !addr.IP.IsLoopback() || addr.IP.To4() == nil {
		t.Fatalf("server is listening on %v, not the IPv4 loopback address", addr)
	}
	res, err := http.Get("http://" + addr.String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "<title>SessionKeep</title>") {
		t.Fatalf("page: %d", res.StatusCode)
	}
	csp := res.Header.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "frame-ancestors 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP lacks %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe") || strings.Contains(csp, "*") || strings.Contains(csp, "http") {
		t.Errorf("CSP is too loose: %s", csp)
	}
	for h, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store"} {
		if res.Header.Get(h) != want {
			t.Errorf("%s = %q, want %q", h, res.Header.Get(h), want)
		}
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("the server must not send CORS allow headers")
	}
}

func TestDashboardRefusesEverythingThatIsNotTheSignedInTab(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation")
	before := sb.ok("project", "list")
	b := signIn(t, sb)

	// The page itself holds no data and is reachable; the API is not, without the cookie.
	stranger := &browser{t: t, base: b.base}
	for _, path := range []string{"/api/session", "/api/status", "/api/projects", "/api/search?q=x&all=1", "/api/instructions"} {
		if code := stranger.do("GET", path, nil, nil, nil); code != http.StatusUnauthorized {
			t.Errorf("GET %s without a session: %d, want 401", path, code)
		}
	}
	if code := stranger.do("POST", "/api/projects/x/pause", map[string]string{}, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("POST without a session: %d, want 401", code)
	}
	forged := &browser{t: t, base: b.base, cookie: &http.Cookie{Name: b.cookie.Name, Value: strings.Repeat("0", 64)}}
	if code := forged.do("GET", "/api/status", nil, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("guessed cookie: %d, want 401", code)
	}

	// The sign-in link works once.
	for _, link := range []string{b.srv.BootstrapURL(), b.base + "/auth?t=" + strings.Repeat("a", 64), b.base + "/auth", b.base + "/auth?t="} {
		res, err := noRedirect.Get(link)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden || len(res.Cookies()) != 0 {
			t.Errorf("reused or wrong sign-in link %q: status %d, cookies %d", link[len(b.base):], res.StatusCode, len(res.Cookies()))
		}
	}

	pause := "/api/projects/" + url.PathEscape(app) + "/pause"
	attacks := map[string]func(*http.Request){
		"no CSRF token":             func(r *http.Request) { r.Header.Del("X-SessionKeep-CSRF") },
		"wrong CSRF token":          func(r *http.Request) { r.Header.Set("X-SessionKeep-CSRF", strings.Repeat("f", 64)) },
		"another origin":            func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"another port on localhost": func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:1") },
		"null origin":               func(r *http.Request) { r.Header.Set("Origin", "null") },
		"cross-site fetch metadata": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"rebound host name":         func(r *http.Request) { r.Host = "evil.example" },
		"host without the port":     func(r *http.Request) { r.Host = "127.0.0.1" },
		"form post":                 func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") },
	}
	for name, edit := range attacks {
		if code := b.do("POST", pause, map[string]string{}, edit, nil); code < 400 {
			t.Errorf("%s: request was accepted (%d)", name, code)
		}
	}
	// Reads are refused for the same reasons, so a hostile page cannot even probe.
	if code := b.do("GET", "/api/status", nil, func(r *http.Request) { r.Host = "evil.example" }, nil); code != http.StatusMisdirectedRequest {
		t.Errorf("GET with a rebound host: %d", code)
	}
	if code := b.do("GET", "/api/status", nil, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, nil); code != http.StatusForbidden {
		t.Errorf("GET from another origin: %d", code)
	}
	if after := sb.ok("project", "list"); after != before {
		t.Fatalf("a refused request changed data:\n%s\n%s", before, after)
	}

	// Malformed bodies.
	for name, raw := range map[string]string{"unknown field": `{"path":"x","level":"off","admin":true}`, "trailing data": `{"path":"x"} {"y":1}`, "not JSON": `path=x`,
		"too large": `{"path":"` + strings.Repeat("a", 2<<20) + `"}`} {
		req, _ := http.NewRequest("POST", b.base+"/api/projects", strings.NewReader(raw))
		req.AddCookie(b.cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-SessionKeep-CSRF", b.csrf)
		res, err := noRedirect.Do(req)
		if err != nil {
			continue // the server may cut an oversized upload
		}
		res.Body.Close()
		if res.StatusCode < 400 || res.StatusCode >= 500 {
			t.Errorf("%s: status %d, want a 4xx", name, res.StatusCode)
		}
	}
	for _, path := range []string{"/api/nope", "/api/projects/x/delete", "/api/instructions/x/destroy"} {
		if code := b.do("POST", path, map[string]string{}, nil, nil); code != http.StatusNotFound {
			t.Errorf("POST %s: %d, want 404", path, code)
		}
	}
	for _, path := range []string{"/..%2f..%2fgo.mod", "/%2e%2e/server.go", "/ui/app.js", "/server.go"} {
		res, err := http.Get(b.base + path)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if strings.Contains(string(raw), "package web") || strings.Contains(string(raw), "module sessionkeep") {
			t.Errorf("GET %s served a file from outside the page directory", path)
		}
	}
}

// The whole owner journey through the API, against real storage: onboarding, capture, search,
// instructions with preview and confirmation, checkpoint and resume.
func TestDashboardJourneyThroughTheAPI(t *testing.T) {
	sb := newSandbox(t)
	app, client := sb.dir("app"), sb.dir("client")
	b := signIn(t, sb)

	type env struct {
		Data  json.RawMessage
		Error struct{ Code, Message string }
	}
	var e env
	// Adding a project without choosing a level is refused: there is no default in the UI either.
	if code := b.do("POST", "/api/projects", map[string]string{"path": app}, nil, &e); code != http.StatusConflict || e.Error.Code != "capture_level_required" {
		t.Fatalf("add without level: %d %+v", code, e.Error)
	}
	if code := b.do("POST", "/api/projects", map[string]string{"path": app, "level": "conversation", "name": "Demo"}, nil, &e); code != 200 {
		t.Fatalf("add project: %d %+v", code, e.Error)
	}
	var list struct {
		Data []struct {
			ID, Name     string
			Paused       bool
			CaptureLevel string `json:"capture_level"`
		}
	}
	b.do("GET", "/api/projects", nil, nil, &list)
	if len(list.Data) != 1 || list.Data[0].Name != "Demo" || list.Data[0].CaptureLevel != "conversation" {
		t.Fatalf("projects: %+v", list.Data)
	}
	pid := list.Data[0].ID

	// Onboarding settings and exclusions.
	if code := b.do("POST", "/api/settings/auto-add", map[string]string{"mode": "auto"}, nil, &e); code != http.StatusBadRequest {
		t.Fatalf("auto without a level: %d", code)
	}
	if code := b.do("POST", "/api/settings/auto-add", map[string]string{"mode": "auto", "level": "metadata"}, nil, nil); code != 200 {
		t.Fatalf("set auto mode: %d", code)
	}
	if code := b.do("POST", "/api/exclusions", map[string]any{"path": client}, nil, nil); code != 200 {
		t.Fatalf("exclude: %d", code)
	}
	var set struct {
		Data struct {
			AutoAdd    struct{ Mode, Level string } `json:"auto_add"`
			Exclusions []string
		}
	}
	b.do("GET", "/api/settings", nil, nil, &set)
	if set.Data.AutoAdd.Mode != "auto" || set.Data.AutoAdd.Level != "metadata" || len(set.Data.Exclusions) != 1 {
		t.Fatalf("settings: %+v", set.Data)
	}
	if a, _, _ := sb.which(client); a != "none" {
		t.Fatalf("the exclusion set in the UI is not honoured by the hook path: %s", a)
	}

	// Capture through the real hook, store through the API, then search.
	sb.say("UserPromptSubmit", "s-1", app, "The invoice export must use ISO dates. <script>alert(1)</script>")
	sb.say("Stop", "s-1", app, "Understood: ISO dates in the invoice export.")
	var status struct {
		Data struct {
			SpoolPending int `json:"spool_pending"`
		}
	}
	b.do("GET", "/api/status", nil, nil, &status)
	if status.Data.SpoolPending != 2 {
		t.Fatalf("status: %+v", status.Data)
	}
	if code := b.do("POST", "/api/ingest", nil, nil, nil); code != 200 {
		t.Fatalf("ingest: %d", code)
	}
	var found struct {
		Data struct {
			Hits []struct{ ID, Who, Snippet string }
		}
	}
	if code := b.do("GET", "/api/search?q="+url.QueryEscape("invoice export")+"&project="+pid, nil, nil, &found); code != 200 || len(found.Data.Hits) != 2 {
		t.Fatalf("search: %d %+v", code, found.Data)
	}
	// Option-looking input from the page is data, not options.
	if code := b.do("GET", "/api/search?q="+url.QueryEscape("--all-projects")+"&project="+pid, nil, nil, &found); code != 200 || len(found.Data.Hits) != 0 {
		t.Fatalf("option-looking query: %d %+v", code, found.Data)
	}
	if code := b.do("GET", "/api/search?q=x&project="+pid+"&limit=abc", nil, nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad limit: %d", code)
	}
	b.do("GET", "/api/search?q=ISO&project="+pid+"&who=user", nil, nil, &found)
	var rec struct{ Data struct{ Text, Who string } }
	if code := b.do("GET", "/api/records/"+found.Data.Hits[0].ID+"?project="+pid, nil, nil, &rec); code != 200 || !strings.Contains(rec.Data.Text, "<script>alert(1)</script>") {
		t.Fatalf("record: %d %+v", code, rec.Data)
	}

	// Instructions: preview first, nothing saved until confirmed.
	var prev struct {
		Data struct {
			Saved bool
			Text  string
		}
		Error struct{ Code string }
	}
	rule := "Always use ISO 8601 dates in exports."
	if code := b.do("POST", "/api/instructions", map[string]any{"text": rule, "scope": "project", "project": pid}, nil, &prev); code != http.StatusConflict || prev.Data.Saved || prev.Data.Text != rule {
		t.Fatalf("preview: %d %+v", code, prev)
	}
	var ins struct {
		Data []struct {
			ID, Text, State string
			Revision        int
		}
	}
	b.do("GET", "/api/instructions", nil, nil, &ins)
	if len(ins.Data) != 0 {
		t.Fatal("the preview saved the instruction")
	}
	if code := b.do("POST", "/api/instructions", map[string]any{"text": rule, "scope": "project", "project": pid, "confirm": true}, nil, nil); code != 200 {
		t.Fatalf("confirm: %d", code)
	}
	if code := b.do("POST", "/api/instructions", map[string]any{"text": "x", "confirm": true}, nil, &prev); code != http.StatusConflict || prev.Error.Code != "scope_required" {
		t.Fatalf("no scope: %d %+v", code, prev.Error)
	}
	b.do("GET", "/api/instructions", nil, nil, &ins)
	id := ins.Data[0].ID
	if ctx := sb.inject("SessionStart", "s-9", app); !strings.Contains(ctx, rule) {
		t.Fatal("an instruction recorded in the UI is not injected into sessions")
	}
	if code := b.do("POST", "/api/instructions/"+id+"/supersede", map[string]any{"text": "Always use ISO 8601 dates, in UTC.", "reason": "clarified"}, nil, nil); code != http.StatusConflict {
		t.Fatalf("supersede without confirm: %d, want 409", code)
	}
	if code := b.do("POST", "/api/instructions/"+id+"/supersede", map[string]any{"text": "Always use ISO 8601 dates, in UTC.", "reason": "clarified", "confirm": true}, nil, nil); code != 200 {
		t.Fatalf("supersede: %d", code)
	}
	var hist struct {
		Data struct {
			Versions []struct {
				Revision     int
				Status, Text string
			}
		}
	}
	b.do("GET", "/api/instructions/"+id, nil, nil, &hist)
	if len(hist.Data.Versions) != 2 || hist.Data.Versions[0].Status != "superseded" || hist.Data.Versions[0].Text != rule {
		t.Fatalf("history: %+v", hist.Data.Versions)
	}
	if code := b.do("POST", "/api/instructions/"+id+"/archive", map[string]any{"confirm": true}, nil, nil); code != 200 {
		t.Fatalf("archive: %d", code)
	}
	if ctx := sb.inject("SessionStart", "s-10", app); ctx != "" {
		t.Fatalf("archived instruction still injected: %q", ctx)
	}

	// Checkpoint and resume; pause.
	if code := b.do("POST", "/api/checkpoints", map[string]string{"note": "Export done. Next: tests.", "project": pid}, nil, nil); code != 200 {
		t.Fatalf("checkpoint: %d", code)
	}
	var res struct {
		Data struct {
			Checkpoints []struct{ Note string }
			Recent      []struct{ Text string }
		}
	}
	if code := b.do("GET", "/api/resume?project="+pid, nil, nil, &res); code != 200 || res.Data.Checkpoints[0].Note != "Export done. Next: tests." || len(res.Data.Recent) != 2 {
		t.Fatalf("resume: %d %+v", code, res.Data)
	}
	if code := b.do("POST", "/api/projects/"+pid+"/pause", map[string]string{}, nil, nil); code != 200 {
		t.Fatalf("pause: %d", code)
	}
	if a, _, _ := sb.which(app); a != "none" {
		t.Fatalf("pausing in the UI did not stop capture: %s", a)
	}
	var doc struct {
		Data struct {
			Healthy bool
			Checks  []struct{ Name string }
		}
	}
	if code := b.do("GET", "/api/doctor", nil, nil, &doc); code != 200 || !doc.Data.Healthy || len(doc.Data.Checks) < 3 {
		t.Fatalf("doctor: %d %+v", code, doc.Data)
	}
	if code := b.do("POST", "/api/projects/no-such-project/pause", map[string]string{}, nil, &e); code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown project: %d", code)
	}
}
