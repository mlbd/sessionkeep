package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sessionkeep/internal/config"
	"sessionkeep/internal/keep"
	"sessionkeep/internal/s3lite"
)

// Sharing is set up and run entirely from the dashboard, with no key in the environment: the
// storage is saved once, a project is switched on with a name and a passphrase, and one sync
// does both directions. A project that was never switched on is never sent. The bucket is a
// stand-in, not a real storage service.
func TestSharingAProjectFromTheDashboard(t *testing.T) {
	fake := s3lite.NewFake("AK", "SYNTHETIC-SECRET-KEY")
	srv := httptest.NewServer(fake)
	defer srv.Close()
	const pass = "correct horse battery staple"
	type reply struct {
		Data  json.RawMessage
		Error struct{ Code, Message string }
	}
	type overview struct {
		Data struct {
			Storage  struct{ Set, Saved bool }
			Projects map[string]struct{ Name, Bucket string }
		}
	}
	type synced struct {
		Data struct {
			Receive struct {
				Bundles      int
				Instructions []struct{ Text string }
			}
			Send struct {
				Sent   bool
				Counts struct {
					Turns int `json:"pieces_of_text"`
				}
			}
		}
	}
	storage := func(secret string) map[string]string {
		return map[string]string{"endpoint": srv.URL, "bucket": "team-bucket", "from": "Sam", "access_key": "AK", "secret_key": secret}
	}

	a := newSandbox(t)
	app, private := a.dir("app"), a.dir("private")
	a.ok("project", "add", app, "--capture", "conversation")
	a.ok("project", "add", private, "--capture", "conversation")
	a.remember("project", "Use tabs in this project DASHRULE.", "--project", app)
	a.say("UserPromptSubmit", "t-1", app, "Add totals APUBLIC.")
	a.say("Stop", "t-1", app, "Totals are added.")
	a.say("UserPromptSubmit", "t-9", private, "My own notes APRIVATE.")
	a.ok("ingest")
	ba := signIn(t, a)

	var r reply
	share := map[string]string{"project": app, "name": "shop-app", "passphrase": pass}
	if code := ba.do("POST", "/api/team/connect", share, nil, &r); code != http.StatusUnprocessableEntity || r.Error.Code != "storage_required" {
		t.Fatalf("sharing before any storage is saved: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/storage", storage("WRONG"), nil, &r); code != http.StatusUnprocessableEntity || r.Error.Code != "storage_refused" {
		t.Fatalf("storage with a wrong key: %d %+v", code, r.Error)
	}
	var o overview
	if ba.do("GET", "/api/team/overview", nil, nil, &o); o.Data.Storage.Set {
		t.Fatal("keys the bucket refused were saved")
	}
	if code := ba.do("POST", "/api/team/storage", storage("SYNTHETIC-SECRET-KEY"), nil, &r); code != http.StatusOK {
		t.Fatalf("saving the storage: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/connect", map[string]string{"project": app, "name": "shop-app", "passphrase": "short"}, nil, &r); code != http.StatusBadRequest {
		t.Errorf("a short passphrase: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/connect", share, nil, &r); code != http.StatusOK {
		t.Fatalf("turning sharing on: %d %+v", code, r.Error)
	}
	// No answer ever carries a key or the passphrase, and one project is shared, not two.
	var raw reply
	ba.do("GET", "/api/team/overview", nil, nil, &raw)
	if s := string(raw.Data); strings.Contains(s, "SYNTHETIC-SECRET-KEY") || strings.Contains(s, pass) {
		t.Fatalf("the overview shows a secret: %s", s)
	}
	if ba.do("GET", "/api/team/overview", nil, nil, &o); !o.Data.Storage.Set || !o.Data.Storage.Saved || len(o.Data.Projects) != 1 {
		t.Fatalf("overview after sharing one project: %+v", o.Data)
	}
	if keep.Protected {
		file, _ := os.ReadFile(filepath.Join(a.env[config.EnvDataDir], "team.json"))
		if len(file) == 0 || strings.Contains(string(file), pass) || strings.Contains(string(file), "SYNTHETIC-SECRET-KEY") {
			t.Fatalf("the settings file is missing or holds a secret in the clear: %s", file)
		}
	}

	// One sync: shown first, then done. The other project is refused and never leaves.
	var s synced
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": app}, nil, &s); code != http.StatusConflict || s.Data.Send.Counts.Turns != 2 || len(fake.Objects) != 0 {
		t.Fatalf("the sync preview: %d %+v, %d object(s)", code, s.Data, len(fake.Objects))
	}
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": app, "confirm": true}, nil, &s); code != http.StatusOK || !s.Data.Send.Sent || len(fake.Objects) != 1 {
		t.Fatalf("the sync: %d %+v, %d object(s)", code, s.Data, len(fake.Objects))
	}
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": private, "confirm": true}, nil, &r); code != http.StatusUnprocessableEntity || r.Error.Code != "not_connected" || len(fake.Objects) != 1 {
		t.Fatalf("syncing a project that is not shared: %d %+v, %d object(s)", code, r.Error, len(fake.Objects))
	}
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": app}, nil, &r); code != http.StatusOK {
		t.Errorf("a second sync with nothing new asks for confirmation: %d", code)
	}

	// A colleague who joins later does the same on their computer and gets everything.
	b := newSandbox(t)
	theirs := b.dir("their-copy")
	b.ok("project", "add", theirs, "--capture", "conversation")
	b.say("UserPromptSubmit", "t-2", theirs, "I looked at tax before joining BEARLIER.")
	b.ok("ingest")
	bb := signIn(t, b)
	kim := storage("SYNTHETIC-SECRET-KEY")
	kim["from"] = "Kim"
	if code := bb.do("POST", "/api/team/storage", kim, nil, &r); code != http.StatusOK {
		t.Fatalf("the colleague's storage: %d %+v", code, r.Error)
	}
	join := map[string]string{"project": theirs, "name": "shop-app", "passphrase": "another passphrase entirely"}
	if code := bb.do("POST", "/api/team/connect", join, nil, &r); code != http.StatusUnprocessableEntity || r.Error.Code != "wrong_passphrase" {
		t.Fatalf("joining with another passphrase: %d %+v", code, r.Error)
	}
	join["passphrase"] = pass
	if code := bb.do("POST", "/api/team/connect", join, nil, &r); code != http.StatusOK || !strings.Contains(string(r.Data), `"bundles_there":1`) {
		t.Fatalf("joining: %d %s %+v", code, r.Data, r.Error)
	}
	if code := bb.do("POST", "/api/team/sync", map[string]any{"project": theirs}, nil, &s); code != http.StatusConflict || s.Data.Receive.Bundles != 1 || len(s.Data.Receive.Instructions) != 1 || !strings.Contains(s.Data.Receive.Instructions[0].Text, "DASHRULE") || s.Data.Send.Counts.Turns != 1 {
		t.Fatalf("the colleague's preview: %d %+v", code, s.Data)
	}
	if hits := b.search("APUBLIC", "--project", theirs).Data.Hits; len(hits) != 0 {
		t.Fatal("a preview stored text")
	}
	if code := bb.do("POST", "/api/team/sync", map[string]any{"project": theirs, "confirm": true}, nil, &s); code != http.StatusOK || len(fake.Objects) != 2 {
		t.Fatalf("the colleague's sync: %d, %d object(s)", code, len(fake.Objects))
	}
	if hits := b.search("APUBLIC", "--project", theirs).Data.Hits; len(hits) != 1 {
		t.Errorf("the colleague finds APUBLIC %d times, want once", len(hits))
	}
	if hits := b.search("APRIVATE", "--all-projects").Data.Hits; len(hits) != 0 {
		t.Error("the project that was never shared reached the colleague")
	}
	// The earlier work of the one who joined later reaches the first, with the same one press.
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": app, "confirm": true}, nil, &s); code != http.StatusOK {
		t.Fatalf("the first one's second sync: %d", code)
	}
	if hits := a.search("BEARLIER", "--project", app).Data.Hits; len(hits) != 1 {
		t.Errorf("BEARLIER is found %d times by the first one, want once", len(hits))
	}

	// The same things from a terminal, with the saved storage.
	if out := a.ok("team", "list"); !strings.Contains(out, "team-bucket") || !strings.Contains(out, `"shop-app"`) {
		t.Errorf("team list: %s", out)
	}
	if out := a.ok("team", "sync", "--project", app); !strings.Contains(out, "Nothing new from the team") || !strings.Contains(out, "Nothing new to send") {
		t.Errorf("team sync with nothing to do: %s", out)
	}
	a.env[envTeamPassphrase] = pass
	if out := a.ok("team", "connect", "--name", "private-notes", "--project", private); !strings.Contains(out, "0 bundle(s) are there") {
		t.Errorf("connect with the saved storage: %s", out)
	}

	// Sharing the same project under another name starts over: everything counts as unsent there.
	other := map[string]string{"project": app, "name": "shop-app-two", "passphrase": pass}
	if code := ba.do("POST", "/api/team/connect", other, nil, &r); code != http.StatusOK {
		t.Fatalf("sharing under another name: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": app}, nil, &s); code != http.StatusConflict || s.Data.Send.Counts.Turns != 2 {
		t.Errorf("after sharing under another name the preview is: %d %+v", code, s.Data)
	}

	// Stop sharing: shown first, then done; forgetting the storage leaves shared projects alone.
	if code := ba.do("POST", "/api/team/disconnect", map[string]any{"project": app}, nil, &r); code != http.StatusConflict {
		t.Errorf("stop sharing without confirming: %d", code)
	}
	if code := ba.do("POST", "/api/team/disconnect", map[string]any{"project": app, "confirm": true}, nil, &r); code != http.StatusOK {
		t.Fatalf("stop sharing: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/storage", map[string]any{"forget": true}, nil, &r); code != http.StatusOK {
		t.Fatalf("forgetting the storage: %d %+v", code, r.Error)
	}
	var after overview // a fresh value: decoding into a used map would keep its old entries
	if ba.do("GET", "/api/team/overview", nil, nil, &after); after.Data.Storage.Saved || len(after.Data.Projects) != 1 {
		t.Errorf("after stopping one project and forgetting the storage: %+v", after.Data)
	}
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": private}, nil, &r); code != http.StatusConflict {
		t.Errorf("a project still shared stopped working when the storage was forgotten: %d %+v", code, r.Error)
	}
}
