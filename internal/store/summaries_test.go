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
