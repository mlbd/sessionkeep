package cli

import (
	"fmt"
	"strings"
	"testing"
)

// A small synthetic gold set (P2-06). Each fact is stored once among unrelated text; each
// question must bring its fact back in the first five results. This measures keyword recall
// only: the last group asks with different words on purpose, to keep the known limit visible.
func TestRecallOnSyntheticGoldSet(t *testing.T) {
	sb := newSandbox(t)
	app := sb.dir("app")
	sb.ok("project", "add", app, "--capture", "conversation", "--name", "App")
	facts := []struct{ mark, text, query string }{
		{"G01", "We decided invoices are retained for seven years.", "invoice retention"},
		{"G02", "The export job is scheduled at 02:00 UTC every night.", "export job schedule"},
		{"G03", "Never round money until it is displayed.", "rounding money"},
		{"G04", "The staging database lives on host db-staging-7.", "staging database host"},
		{"G05", "Login failed because the session cookie lacked the Secure flag.", "cookie secure flag"},
		{"G06", "We chose Postgres over MySQL for the reporting service.", "chose Postgres"},
		{"G07", "The flaky test is TestCheckoutTotals and it fails on leap days.", "TestCheckoutTotals"},
		{"G08", "Deploys are frozen from 20 December to 2 January.", "deploy freeze December"},
		{"G09", "The error was ECONNRESET: socket hang up during uploads.", "ECONNRESET uploads"},
		{"G10", "Feature flags are stored in config/flags.yaml.", "config/flags.yaml"},
		{"G11", "Reviewers must approve migrations before merging.", "approving migrations"},
		{"G12", "The cache expires after fifteen minutes.", "cache expiring"},
	}
	for i, f := range facts {
		sb.say("UserPromptSubmit", fmt.Sprintf("s-%d", i%4), app, f.text+" "+f.mark)
		sb.say("Stop", fmt.Sprintf("s-%d", i%4), app, fmt.Sprintf("Understood, noted item %d about the work in progress.", i))
		sb.say("UserPromptSubmit", fmt.Sprintf("s-%d", i%4), app, fmt.Sprintf("Unrelated filler %d: rename the helper and tidy the imports.", i))
	}
	sb.ok("ingest")

	found := 0
	for _, f := range facts {
		hit := false
		for i, h := range sb.search(f.query, "--project", app).Data.Hits {
			if i < 5 && strings.Contains(sb.ok("memory", "get", h.ID, "--project", app), f.mark) {
				hit = true
			}
		}
		if hit {
			found++
		} else {
			t.Errorf("recall miss: %q did not return fact %s in the first five results", f.query, f.mark)
		}
	}
	t.Logf("keyword recall@5 on the synthetic gold set: %d/%d", found, len(facts))

	// Different words for the same thing: keyword search is expected to miss these. Not a
	// failure; logged so the limit stays measured until meaning-based search exists.
	missed := 0
	for _, q := range []struct{ query, mark string }{{"how long do we keep bills", "G01"}, {"when does the nightly dump run", "G02"}, {"which SQL engine did we pick", "G06"}} {
		hit := false
		for i, h := range sb.search(q.query, "--project", app).Data.Hits {
			if i < 5 && strings.Contains(sb.ok("memory", "get", h.ID, "--project", app), q.mark) {
				hit = true
			}
		}
		if !hit {
			missed++
		}
	}
	t.Logf("questions in different words that did not find their fact in the first five: %d/3 (known limit of keyword search)", missed)
}
