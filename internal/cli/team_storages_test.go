package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sessionkeep/internal/s3lite"
)

// Each project goes to the storage chosen for it: the personal one unless another is chosen, the
// shared one, or a bucket of its own. Two people who each keep their own personal bucket work
// together through the shared one. The buckets are stand-ins, not a real storage service.
func TestEachProjectGoesToTheStorageChosenForIt(t *testing.T) {
	type bucket struct {
		fake *s3lite.Fake
		url  string
	}
	open := func(secret string) bucket {
		f := s3lite.NewFake("AK", secret)
		srv := httptest.NewServer(f)
		t.Cleanup(srv.Close)
		return bucket{f, srv.URL}
	}
	mine, company, client, theirs := open("MINE-SECRET"), open("COMPANY-SECRET"), open("CLIENT-SECRET"), open("THEIR-SECRET")
	const pass = "correct horse battery staple"
	type reply struct {
		Data  json.RawMessage
		Error struct{ Code, Message string }
	}
	type overview struct {
		Data struct {
			Storages map[string]struct {
				Set    bool
				Bucket string
			}
			Projects map[string]struct{ Name, Storage, Bucket string }
		}
	}
	keys := func(b bucket, name, secret, from string) map[string]string {
		return map[string]string{"endpoint": b.url, "bucket": name, "from": from, "access_key": "AK", "secret_key": secret}
	}

	a := newSandbox(t)
	own, work, gig := a.dir("own"), a.dir("work"), a.dir("gig")
	for _, d := range []string{own, work, gig} {
		a.ok("project", "add", d, "--capture", "conversation")
	}
	a.say("UserPromptSubmit", "t-1", own, "My side project OWNWORD.")
	a.say("UserPromptSubmit", "t-2", work, "Company work WORKWORD.")
	a.say("UserPromptSubmit", "t-3", gig, "Client work GIGWORD.")
	a.ok("ingest")
	ba := signIn(t, a)
	var r reply

	// The shared storage cannot be used before it is saved, and it is never taken from the personal one.
	if code := ba.do("POST", "/api/team/storage", keys(mine, "mine", "MINE-SECRET", "Sam"), nil, &r); code != http.StatusOK {
		t.Fatalf("saving the personal storage: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/connect", map[string]string{"project": work, "name": "company-app", "passphrase": pass, "storage": "shared"}, nil, &r); code != http.StatusUnprocessableEntity || r.Error.Code != "storage_required" {
		t.Fatalf("sharing through a shared storage that is not saved: %d %+v", code, r.Error)
	}
	shared := keys(company, "company", "COMPANY-SECRET", "Sam")
	shared["storage"] = "shared"
	if code := ba.do("POST", "/api/team/storage", shared, nil, &r); code != http.StatusOK {
		t.Fatalf("saving the shared storage: %d %+v", code, r.Error)
	}

	// Personal by default, shared when chosen, and a bucket of its own for the client project.
	if code := ba.do("POST", "/api/team/connect", map[string]string{"project": own, "name": "side-project", "passphrase": pass}, nil, &r); code != http.StatusOK {
		t.Fatalf("sharing through the personal storage: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/connect", map[string]string{"project": work, "name": "company-app", "passphrase": pass, "storage": "shared"}, nil, &r); code != http.StatusOK {
		t.Fatalf("sharing through the shared storage: %d %+v", code, r.Error)
	}
	if code := ba.do("POST", "/api/team/connect", map[string]string{"project": gig, "name": "client-site", "passphrase": pass, "storage": "custom"}, nil, &r); code != http.StatusBadRequest {
		t.Fatalf("a bucket of its own without an address: %d %+v", code, r.Error)
	}
	custom := keys(client, "client", "CLIENT-SECRET", "")
	for k, v := range map[string]string{"project": gig, "name": "client-site", "passphrase": pass, "storage": "custom"} {
		custom[k] = v
	}
	if code := ba.do("POST", "/api/team/connect", custom, nil, &r); code != http.StatusOK {
		t.Fatalf("sharing through a bucket of its own: %d %+v", code, r.Error)
	}

	var o overview
	ba.do("GET", "/api/team/overview", nil, nil, &o)
	if o.Data.Storages["personal"].Bucket != "mine" || o.Data.Storages["shared"].Bucket != "company" || len(o.Data.Projects) != 3 {
		t.Fatalf("overview: %+v", o.Data)
	}
	for id, want := range map[string]string{"side-project": "personal", "company-app": "shared", "client-site": "custom"} {
		found := false
		for _, p := range o.Data.Projects {
			if p.Name == id {
				found = p.Storage == want
			}
		}
		if !found {
			t.Errorf("project %s is not shown with the %s storage: %+v", id, want, o.Data.Projects)
		}
	}
	var raw reply
	ba.do("GET", "/api/team/overview", nil, nil, &raw)
	for _, secret := range []string{"MINE-SECRET", "COMPANY-SECRET", "CLIENT-SECRET", pass} {
		if strings.Contains(string(raw.Data), secret) {
			t.Fatalf("the overview shows a secret: %s", raw.Data)
		}
	}

	// Each sync lands in its own bucket and nowhere else.
	for _, p := range []string{own, work, gig} {
		if code := ba.do("POST", "/api/team/sync", map[string]any{"project": p, "confirm": true}, nil, &r); code != http.StatusOK {
			t.Fatalf("sync of %s: %d %+v", p, code, r.Error)
		}
	}
	if len(mine.fake.Objects) != 1 || len(company.fake.Objects) != 1 || len(client.fake.Objects) != 1 {
		t.Fatalf("bundles per bucket: mine %d, company %d, client %d; want one each", len(mine.fake.Objects), len(company.fake.Objects), len(client.fake.Objects))
	}

	// A colleague with a personal bucket of their own joins the company project through the shared one.
	b := newSandbox(t)
	copyOfWork := b.dir("work-copy")
	b.ok("project", "add", copyOfWork, "--capture", "conversation")
	bb := signIn(t, b)
	if code := bb.do("POST", "/api/team/storage", keys(theirs, "theirs", "THEIR-SECRET", "Kim"), nil, &r); code != http.StatusOK {
		t.Fatalf("the colleague's personal storage: %d %+v", code, r.Error)
	}
	theirShared := keys(company, "company", "COMPANY-SECRET", "Kim")
	theirShared["storage"] = "shared"
	if code := bb.do("POST", "/api/team/storage", theirShared, nil, &r); code != http.StatusOK {
		t.Fatalf("the colleague's shared storage: %d %+v", code, r.Error)
	}
	if code := bb.do("POST", "/api/team/connect", map[string]string{"project": copyOfWork, "name": "company-app", "passphrase": pass, "storage": "shared"}, nil, &r); code != http.StatusOK || !strings.Contains(string(r.Data), `"bundles_there":1`) {
		t.Fatalf("the colleague joins through the shared storage: %d %s %+v", code, r.Data, r.Error)
	}
	if code := bb.do("POST", "/api/team/sync", map[string]any{"project": copyOfWork, "confirm": true}, nil, &r); code != http.StatusOK {
		t.Fatalf("the colleague's sync: %d %+v", code, r.Error)
	}
	if hits := b.search("WORKWORD", "--project", copyOfWork).Data.Hits; len(hits) != 1 {
		t.Errorf("the colleague finds the company work %d times, want once", len(hits))
	}
	for _, word := range []string{"OWNWORD", "GIGWORD"} {
		if hits := b.search(word, "--all-projects").Data.Hits; len(hits) != 0 {
			t.Errorf("%s reached the colleague, who only shares the company project", word)
		}
	}
	if len(theirs.fake.Objects) != 0 {
		t.Error("the company project was sent to the colleague's personal bucket")
	}

	// Forgetting the shared storage leaves the personal one and every shared project alone.
	if code := ba.do("POST", "/api/team/storage", map[string]any{"forget": true, "storage": "shared"}, nil, &r); code != http.StatusOK {
		t.Fatalf("forgetting the shared storage: %d %+v", code, r.Error)
	}
	var after overview
	ba.do("GET", "/api/team/overview", nil, nil, &after)
	if after.Data.Storages["shared"].Set || !after.Data.Storages["personal"].Set || len(after.Data.Projects) != 3 {
		t.Errorf("after forgetting the shared storage: %+v", after.Data)
	}
	if code := ba.do("POST", "/api/team/sync", map[string]any{"project": work}, nil, &r); code != http.StatusOK && code != http.StatusConflict {
		t.Errorf("the company project stopped working when the shared storage was forgotten: %d %+v", code, r.Error)
	}

	// From a terminal: --storage and a bucket of its own cannot be given together.
	a.env[envTeamPassphrase] = pass
	if code, _, errOut := a.run("team", "connect", "--name", "x-y", "--storage", "shared", "--endpoint", company.url, "--bucket", "company", "--project", own); code == ExitOK || !strings.Contains(errOut, "not both") {
		t.Errorf("--storage with --endpoint: exit %d, %s", code, errOut)
	}
	if out := a.ok("team", "list"); !strings.Contains(out, "personal  storage: bucket mine") || !strings.Contains(out, "(custom storage") {
		t.Errorf("team list: %s", out)
	}
}
