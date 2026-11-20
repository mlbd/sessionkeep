package store

import (
	"errors"
	"testing"
)

func sum(project, session string, sources int) Summary {
	return Summary{ID: "sum-" + session + "-" + string(rune('0'+sources)), ProjectID: project, SessionID: session, Text: "Goal. Rotate the tenant key. Done.",
		Provider: "test", Model: "model-x", PromptVersion: 1, SourceCount: sources, SourceUsed: sources, CreatedAt: "2026-11-20T00:00:00.000000000Z"}
}

func TestSummaryIsRefusedWhenTheSessionChangedAndReplacedOtherwise(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	mustExec(t, s, insEvidence, "evi-1", "proj-A", "loc-1", "first text")
	mustExec(t, s, insEvent, "ev-1", "proj-A", "sess-A", "k-1", "evi-1")

	// Written from two pieces, but the session holds one: refused, nothing stored.
	if err := s.PutSummary(ctx, sum("proj-A", "sess-A", 2)); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("stale summary: %v, want ErrSourceChanged", err)
	}
	if _, ok, _ := s.SummaryOf(ctx, "proj-A", "sess-A"); ok {
		t.Fatal("a refused summary was stored")
	}
	if err := s.PutSummary(ctx, sum("proj-A", "sess-A", 1)); err != nil {
		t.Fatal(err)
	}
	// A session of another project cannot be given this project's id.
	if err := s.PutSummary(ctx, sum("proj-B", "sess-A", 1)); err == nil {
		t.Fatal("a summary was stored for a session in another project")
	}
	wantFail(t, s, "never edited", `update summaries set text = 'changed'`)

	// More text arrives: the old summary no longer covers the session, so it is a candidate
	// again, and the new summary replaces the old one.
	mustExec(t, s, insEvidence, "evi-2", "proj-A", "loc-2", "second text")
	mustExec(t, s, insEvent, "ev-2", "proj-A", "sess-A", "k-2", "evi-2")
	mustExec(t, s, `update sessions set state = 'ended' where id = 'sess-A'`)
	c, err := s.SessionsToSummarize(ctx, "", "2000-01-01", 2, 10, 10)
	if err != nil || len(c) != 1 || c[0].SessionID != "sess-A" || c[0].Texts != 2 {
		t.Fatalf("candidates = %+v, %v", c, err)
	}
	if err := s.PutSummary(ctx, sum("proj-A", "sess-A", 2)); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.SummaryCount(ctx); n != 1 {
		t.Fatalf("%d summaries, want 1 (replaced)", n)
	}
	if c, _ := s.SessionsToSummarize(ctx, "", "2000-01-01", 2, 10, 10); len(c) != 0 {
		t.Fatalf("a session with a current summary is still a candidate: %+v", c)
	}
	mustExec(t, s, `delete from summaries`)
	if c, _ := s.SessionsToSummarize(ctx, "", "2000-01-01", 2, 1000, 10); len(c) != 0 {
		t.Fatalf("a session with almost no text is a candidate: %+v", c)
	}
	if err := s.PutSummary(ctx, sum("proj-A", "sess-A", 2)); err != nil {
		t.Fatal(err)
	}

	// Found by a word of the summary, inside the project only, and gone from the index when a
	// source event is deleted.
	hits, partial, err := s.SearchSummaries(ctx, "proj-A", false, "rotating keys", 5) // word endings are ignored
	if err != nil || len(hits) != 1 || partial {
		t.Fatalf("summary search = %+v partial=%v err=%v", hits, partial, err)
	}
	if hits, partial, _ := s.SearchSummaries(ctx, "proj-A", false, "rotate the certificate", 5); len(hits) != 1 || !partial {
		t.Fatalf("partial summary search = %+v partial=%v", hits, partial)
	}
	if hits, _, _ := s.SearchSummaries(ctx, "proj-B", false, "tenant key", 5); len(hits) != 0 {
		t.Fatalf("a summary was found from another project: %+v", hits)
	}
	if _, _, err := s.SearchSummaries(ctx, "", false, "tenant", 5); err == nil {
		t.Fatal("a summary search without a scope was accepted")
	}
	mustExec(t, s, `delete from events where id = 'ev-2'`)
	if n, _ := s.SummaryCount(ctx); n != 0 {
		t.Fatal("the summary outlived an event it was written from")
	}
	if hits, _, _ := s.SearchSummaries(ctx, "", true, "tenant key", 5); len(hits) != 0 {
		t.Fatalf("a deleted summary is still in the index: %+v", hits)
	}
}

func TestDeletingAProjectDeletesItsSummaries(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	for _, p := range []string{"A", "B"} {
		mustExec(t, s, insEvidence, "evi-"+p, "proj-"+p, "loc-"+p, "text")
		mustExec(t, s, insEvent, "ev-"+p, "proj-"+p, "sess-"+p, "k-"+p, "evi-"+p)
		if err := s.PutSummary(ctx, sum("proj-"+p, "sess-"+p, 1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ForgetProject(ctx, "proj-A", true); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.SummaryOf(ctx, "proj-A", "sess-A"); ok {
		t.Fatal("the deleted project's summary is still there")
	}
	if _, ok, _ := s.SummaryOf(ctx, "proj-B", "sess-B"); !ok {
		t.Fatal("another project's summary was deleted too")
	}
	if err := s.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	if hits, _, _ := s.SearchSummaries(ctx, "", true, "tenant", 5); len(hits) != 1 {
		t.Fatalf("after compaction the summary index holds %d match(es), want 1", len(hits))
	}
}

func batchEvent(n int, evidenceID string) IngestInput {
	id := string(rune('a' + n))
	return IngestInput{EventID: "bev-" + id, EventKey: "batch:" + id, DeviceID: "dev-1", ProjectID: "proj-A", Agent: "claude-code", SessionExternal: "ext-batch",
		NewSessionID: "sess-batch", AdapterVersion: 1, Kind: "user_prompt", ObservedAt: "2026-11-19T00:00:00.000000000Z", ReceivedAt: "2026-11-19T00:00:01.000000000Z",
		SafePayload: "{}", RedactionStatus: "none", EndsSession: n == 2,
		Evidence: &EvidenceRow{ID: evidenceID, Origin: "transcript", Locator: "loc-" + id, Text: "batch text " + id, Completeness: "complete", Trust: "user"}}
}

func TestIngestEventsStoresAWholeBatchOrNothing(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	count := func() (n int) {
		s.db.QueryRow("select count(*) from events where event_key like 'batch:%'").Scan(&n)
		return
	}
	// The third event reuses the first one's evidence id, which the database refuses.
	if _, err := s.IngestEvents(ctx, []IngestInput{batchEvent(0, "be-0"), batchEvent(1, "be-1"), batchEvent(2, "be-0")}); err == nil {
		t.Fatal("a batch with a bad event was accepted")
	}
	var sessions int
	s.db.QueryRow("select count(*) from sessions where external_session_id = 'ext-batch'").Scan(&sessions)
	if count() != 0 || sessions != 0 {
		t.Fatalf("after a failed batch: %d event(s) and %d session(s) remain, want none", count(), sessions)
	}
	res, err := s.IngestEvents(ctx, []IngestInput{batchEvent(0, "be-0"), batchEvent(1, "be-1"), batchEvent(2, "be-2")})
	if err != nil || len(res) != 3 || res[0].Outcome != Inserted || res[2].Outcome != Inserted || count() != 3 {
		t.Fatalf("batch: %+v, %v, %d stored", res, err, count())
	}
	var state string
	s.db.QueryRow("select state from sessions where external_session_id = 'ext-batch'").Scan(&state)
	if state != "ended" {
		t.Errorf("session state %q, want ended", state)
	}
	// The same batch again plus one new event: three replays, one insert.
	res, err = s.IngestEvents(ctx, []IngestInput{batchEvent(0, "be-0"), batchEvent(1, "be-1"), batchEvent(2, "be-2"), batchEvent(3, "be-3")})
	if err != nil || res[0].Outcome != Replayed || res[2].Outcome != Replayed || res[3].Outcome != Inserted || count() != 4 {
		t.Fatalf("replayed batch: %+v, %v, %d stored", res, err, count())
	}
}
