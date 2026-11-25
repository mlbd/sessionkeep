package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sessionkeep/internal/config"
	"sessionkeep/internal/handoff"
	"sessionkeep/internal/s3lite"
	"sessionkeep/internal/team"
)

// teamSandbox is a computer with its own data directory and the team's keys in its environment.
func teamSandbox(t *testing.T, passphrase string) *sandbox {
	sb := newSandbox(t)
	sb.env[envTeamAccess], sb.env[envTeamSecret], sb.env[envTeamPassphrase] = "AK", "SYNTHETIC-SECRET-KEY", passphrase
	return sb
}

// Two colleagues connect the same shared name to one bucket (a stand-in, not R2) and exchange
// their work; a third with another passphrase is turned away; the bucket holds nothing readable.
func TestTwoColleaguesExchangeAProjectThroughABucket(t *testing.T) {
	fake := s3lite.NewFake("AK", "SYNTHETIC-SECRET-KEY")
	srv := httptest.NewServer(fake)
	defer srv.Close()
	const pass = "correct horse battery staple"
	connect := func(sb *sandbox, dir, as string) string {
		return sb.ok("team", "connect", "--name", "shop-app", "--endpoint", srv.URL, "--bucket", "team-bucket", "--from", as, "--project", dir)
	}

	a := teamSandbox(t, pass)
	app := a.dir("app")
	a.ok("project", "add", app, "--capture", "conversation")
	a.remember("project", "Use tabs in this project TEAMRULE.", "--project", app)
	a.say("UserPromptSubmit", "t-1", app, "Add totals to "+filepath.Join(app, "invoice.go")+" AMARK, key AKIAIOSFODNN7EXAMPLE.")
	a.say("Stop", "t-1", app, "Totals are added AREPLY.")
	a.ok("ingest")
	a.ok("checkpoint", "Done: totals. Next: tax ACHECK", "--project", app)

	if code, _, _ := a.run("team", "push", "--project", app, "--yes"); code != ExitFailure {
		t.Fatalf("a push without a connection: exit %d", code)
	}
	if out := connect(a, app, "Sam"); !strings.Contains(out, "0 bundle(s) are there") || !strings.Contains(out, "Nothing was sent") {
		t.Fatalf("connect: %s", out)
	}
	// The keys are not in the settings file in the clear where the system can protect them.
	raw, _ := os.ReadFile(filepath.Join(a.env[config.EnvDataDir], "team.json"))
	if len(raw) == 0 || strings.Contains(string(raw), pass) {
		t.Fatalf("the team settings are missing or hold the passphrase in the clear: %s", raw)
	}

	code, out, _ := a.run("team", "push", "--project", app)
	if code != ExitNeedsConsent || !strings.Contains(out, "NOTHING SENT YET") || !strings.Contains(out, "1 session(s) with 2 piece(s)") || len(fake.Objects) != 0 {
		t.Fatalf("a preview sent something or said too little: exit %d, %d object(s)\n%s", code, len(fake.Objects), out)
	}
	if out := a.ok("team", "push", "--project", app, "--yes"); !strings.Contains(out, "SENT: one encrypted bundle") || len(fake.Objects) != 1 {
		t.Fatalf("push: %d object(s)\n%s", len(fake.Objects), out)
	}
	for name, body := range fake.Objects {
		for _, clear := range []string{"TEAMRULE", "AMARK", "AREPLY", "ACHECK", "shop-app", "AKIAIOSFODNN7EXAMPLE", "Sam"} {
			if strings.Contains(string(body), clear) || strings.Contains(name, clear) {
				t.Errorf("the bucket shows %q in %s", clear, name)
			}
		}
	}
	if out := a.ok("team", "push", "--project", app, "--yes"); !strings.Contains(out, "Nothing new to send") || len(fake.Objects) != 1 {
		t.Errorf("a second push sent again: %s", out)
	}
	if out := a.ok("team", "pull", "--project", app, "--yes"); !strings.Contains(out, "Nothing new from the team") {
		t.Errorf("the sender received their own bundle: %s", out)
	}

	b := teamSandbox(t, pass)
	theirs := b.dir("their-copy")
	b.ok("project", "add", theirs, "--capture", "conversation")
	if out := connect(b, theirs, "Kim"); !strings.Contains(out, "1 bundle(s) are there") {
		t.Fatalf("second connect: %s", out)
	}
	code, out, _ = b.run("team", "pull", "--project", theirs)
	if code != ExitNeedsConsent || !strings.Contains(out, "TEAMRULE") || !strings.Contains(out, "Sam") || !strings.Contains(out, "NOTHING RECEIVED YET") {
		t.Fatalf("the pull preview: exit %d\n%s", code, out)
	}
	if hits := b.search("AMARK", "--project", theirs).Data.Hits; len(hits) != 0 {
		t.Fatal("a preview stored text")
	}
	if out := b.ok("team", "pull", "--project", theirs, "--yes"); !strings.Contains(out, "RECEIVED: 1 instruction(s) waiting for your approval on the dashboard (0 already there), 1 checkpoint(s) (0 already there), 2 piece(s) of text (0 already there)") {
		t.Errorf("pull: %s", out)
	}
	for _, word := range []string{"AMARK", "AREPLY"} {
		if hits := b.search(word, "--project", theirs).Data.Hits; len(hits) != 1 {
			t.Errorf("%s is found %d times by the receiver, want once", word, len(hits))
		}
	}
	if got := b.inject("SessionStart", "n-1", theirs); strings.Contains(got, "TEAMRULE") || !strings.Contains(got, "ACHECK") || !strings.Contains(got, "written by a teammate") {
		t.Errorf("before approval a new session must get the teammate's checkpoint, marked as theirs, and not their instruction:\n%s", got)
	}
	if w := b.waiting(); len(w) != 1 || w[0].Author != "received:Sam" {
		t.Fatalf("the received instruction should wait for approval, marked as Sam's: %+v", w)
	} else {
		b.ok("instruction", "approve", w[0].ID, "--yes")
	}
	if got := b.inject("SessionStart", "n-2", theirs); !strings.Contains(got, "TEAMRULE") || !strings.Contains(got, "ACHECK") {
		t.Errorf("after approval a new session of the receiver lacks the instruction or the checkpoint:\n%s", got)
	}
	if out := b.ok("team", "pull", "--project", theirs, "--yes"); !strings.Contains(out, "Nothing new from the team") {
		t.Errorf("a second pull: %s", out)
	}

	// The receiver works on, and sends only their own work back: not what they received.
	b.say("UserPromptSubmit", "t-2", theirs, "Now add tax BMARK.")
	b.say("Stop", "t-2", theirs, "Tax is added BREPLY.")
	if out := b.ok("team", "push", "--project", theirs, "--yes"); !strings.Contains(out, "0 instruction(s), 0 checkpoint(s), 1 session(s) with 2 piece(s)") || len(fake.Objects) != 2 {
		t.Errorf("the receiver's push: %d object(s)\n%s", len(fake.Objects), out)
	}
	if out := a.ok("team", "status", "--project", app); !strings.Contains(out, "To receive: 1 bundle(s)") || !strings.Contains(out, "To send: 0 instruction(s), 0 checkpoint(s), 0 piece(s)") {
		t.Errorf("status: %s", out)
	}
	if out := a.ok("team", "pull", "--project", app, "--yes"); !strings.Contains(out, "Kim") || !strings.Contains(out, "2 piece(s) of text (0 already there)") {
		t.Errorf("the first sender's pull: %s", out)
	}
	if hits := a.search("BMARK", "--project", app).Data.Hits; len(hits) != 1 {
		t.Errorf("BMARK is found %d times by the first sender", len(hits))
	}

	// A session that grows is sent again as only its new part.
	a.say("UserPromptSubmit", "t-1", app, "One more thing AMORE.")
	if out := a.ok("team", "push", "--project", app, "--yes"); !strings.Contains(out, "1 session(s) with 1 piece(s)") {
		t.Errorf("a grown session: %s", out)
	}
	if out := b.ok("team", "pull", "--project", theirs, "--yes"); !strings.Contains(out, "1 piece(s) of text (0 already there)") {
		t.Errorf("the new part: %s", out)
	}
	if list := b.sessions("--project", theirs); len(list) != 4 {
		t.Errorf("the receiver lists %d sessions, want 4 (the sender's one, grown, and their own three)", len(list))
	}

	// Another passphrase opens nothing and cannot even connect to this shared name.
	c := teamSandbox(t, "a different passphrase")
	third := c.dir("third")
	c.ok("project", "add", third, "--capture", "conversation")
	if code, _, errOut := c.run("team", "connect", "--name", "shop-app", "--endpoint", srv.URL, "--bucket", "team-bucket", "--project", third); code != ExitFailure || !strings.Contains(errOut, "passphrase does not open") {
		t.Errorf("a wrong passphrase connected: exit %d %q", code, errOut)
	}
	// A wrong storage secret is refused by the bucket.
	c.env[envTeamSecret] = "WRONG"
	if code, _, errOut := c.run("team", "connect", "--name", "other-name", "--endpoint", srv.URL, "--bucket", "team-bucket", "--project", third); code != ExitFailure || !strings.Contains(errOut, "403") {
		t.Errorf("a wrong storage secret: exit %d %q", code, errOut)
	}

	// A bundle someone altered in the bucket is skipped and said so; nothing is taken from it.
	for name := range fake.Objects {
		if !strings.Contains(name, "/b/") {
			continue
		}
		fake.Objects[name+"x.age"] = append([]byte(nil), fake.Objects[name][:len(fake.Objects[name])-9]...)
		break
	}
	d := teamSandbox(t, pass)
	fourth := d.dir("fourth")
	d.ok("project", "add", fourth, "--capture", "conversation")
	connect(d, fourth, "Lee")
	if out := d.ok("team", "pull", "--project", fourth, "--yes"); !strings.Contains(out, "is damaged or was altered") || !strings.Contains(out, "RECEIVED:") {
		t.Errorf("a pull with a damaged bundle: %s", out)
	}

	// Bad input, and disconnecting.
	for _, bad := range [][]string{
		{"team"}, {"team", "connect", "--name", "Shop App", "--endpoint", srv.URL, "--bucket", "team-bucket", "--project", app},
		{"team", "connect", "--name", "shop-app", "--endpoint", "http://example.com", "--bucket", "team-bucket", "--project", app},
		{"team", "connect", "--name", "shop-app", "--endpoint", srv.URL, "--bucket", "Bad_Bucket", "--project", app},
	} {
		if code, _, _ := a.run(bad...); code != ExitUsage {
			t.Errorf("%v: exit %d, want usage", bad, code)
		}
	}
	a.env[envTeamPassphrase] = "short"
	if code, _, _ := a.run("team", "connect", "--name", "shop-app", "--endpoint", srv.URL, "--bucket", "team-bucket", "--project", app); code != ExitUsage {
		t.Errorf("a short passphrase: exit %d", code)
	}
	if code, _, _ := a.run("team", "disconnect", "--project", app); code != ExitNeedsConsent {
		t.Errorf("disconnect without --yes: exit %d", code)
	}
	a.ok("team", "disconnect", "--project", app, "--yes")
	if code, _, _ := a.run("team", "status", "--project", app); code != ExitFailure {
		t.Errorf("status after disconnecting: exit %d", code)
	}
}

// Someone who can write to the bucket but has no passphrase puts bundles there whose names sort
// last. A colleague can still connect and sync; the planted bundles are skipped, said so once and
// not downloaded again; a wrong passphrase is still turned away (M5).
func TestABundleThatDoesNotOpenDoesNotStopTheTeam(t *testing.T) {
	fake := s3lite.NewFake("AK", "SYNTHETIC-SECRET-KEY")
	srv := httptest.NewServer(fake)
	defer srv.Close()
	const pass = "correct horse battery staple"
	connect := func(sb *sandbox, dir, as string) (int, string, string) {
		return sb.run("team", "connect", "--name", "shop-app", "--endpoint", srv.URL, "--bucket", "team-bucket", "--from", as, "--project", dir)
	}

	a := teamSandbox(t, pass)
	app := a.dir("app")
	a.ok("project", "add", app, "--capture", "conversation")
	a.ok("checkpoint", "Done: totals PLANTCHECK", "--project", app)
	if code, out, errOut := connect(a, app, "Sam"); code != ExitOK {
		t.Fatalf("connect: %d %s %s", code, out, errOut)
	}
	a.ok("team", "push", "--project", app, "--yes")

	locked, err := team.SealBundle(handoff.File{SourceDevice: "intruder"}, "not the team passphrase")
	if err != nil {
		t.Fatal(err)
	}
	prefix := "team-bucket/" + team.Prefix("shop-app")
	// The locked one sorts last: connect used to try only the newest bundle.
	fake.Objects[prefix+"99991231T235958Z-0000000000-junk.age"] = []byte("age-encryption.org/v1\nnot a bundle\n")
	fake.Objects[prefix+"99991231T235959Z-0000000000-lock.age"] = locked

	b := teamSandbox(t, pass)
	theirs := b.dir("their-copy")
	b.ok("project", "add", theirs, "--capture", "conversation")
	if code, out, errOut := connect(b, theirs, "Kim"); code != ExitOK || !strings.Contains(out, "3 bundle(s) are there") {
		t.Fatalf("a colleague with the right passphrase was kept out by planted bundles: exit %d\n%s%s", code, out, errOut)
	}
	out := b.ok("team", "sync", "--project", theirs, "--yes")
	for _, want := range []string{"PLANTCHECK", "0000000000-lock.age does not open with this project's passphrase", "0000000000-junk.age is damaged or was altered", "RECEIVED:"} {
		if !strings.Contains(out, want) {
			t.Errorf("the first sync lacks %q:\n%s", want, out)
		}
	}
	out = b.ok("team", "sync", "--project", theirs, "--yes")
	if strings.Contains(out, "Skipped: bundle") || !strings.Contains(out, "2 bundle(s) found earlier to be damaged or locked") {
		t.Errorf("the second sync looked at the planted bundles again or did not say they are skipped:\n%s", out)
	}
	// Connecting again (perhaps with a corrected passphrase) looks at them again.
	connect(b, theirs, "Kim")
	if out := b.ok("team", "pull", "--project", theirs, "--yes"); !strings.Contains(out, "lock.age does not open") {
		t.Errorf("after connecting again the planted bundles were not looked at again:\n%s", out)
	}

	c := teamSandbox(t, "a different passphrase")
	third := c.dir("third")
	c.ok("project", "add", third, "--capture", "conversation")
	if code, _, errOut := connect(c, third, "Lee"); code != ExitFailure || !strings.Contains(errOut, "passphrase does not open") {
		t.Errorf("a wrong passphrase connected: exit %d %q", code, errOut)
	}
}
