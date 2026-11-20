package store

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

var day0 = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

const twoWeeks = 14 * 24 * time.Hour

var snapN int

func snap(url string, at time.Time) Snapshot {
	snapN++
	return Snapshot{ID: fmt.Sprintf("snap-%d", snapN), URL: url, FinalURL: url, Status: 200, ETag: `"v1"`, Hash: "h1", Bytes: 100, Extractor: 1, FetchedAt: at.UTC().Format(stamp)}
}

func claim(t *testing.T, s *Store, id, project string, withSource bool) Claim {
	t.Helper()
	n := NewClaim{ID: id, ProjectID: project, Subject: "payments API", Statement: "The refund endpoint accepts at most 100 items per call.", Category: "api", Now: day0, Window: twoWeeks}
	if withSource {
		n.Evidence = []NewEvidence{{ID: "evi-" + id, SourceID: "src-" + id, Snap: snap("https://docs.example.test/refunds", day0), Quote: "at most 100 items per call"}}
	}
	c, err := s.AddClaim(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAClaimIsVerifiedOnlyWithAQuoteAndGoesStaleByItself(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	if c := claim(t, s, "c-plain", "proj-A", false); c.Status != "candidate" || c.VerifiedAt != "" || len(c.Evidence) != 0 {
		t.Fatalf("a claim without a source: %+v, want an unverified candidate", c)
	}
	c := claim(t, s, "c-src", "proj-A", true)
	if c.Status != "verified" || len(c.Evidence) != 1 || c.Evidence[0].First.URL != "https://docs.example.test/refunds" || c.Evidence[0].Last.ID != c.Evidence[0].First.ID {
		t.Fatalf("a claim with a source: %+v", c)
	}
	// One second before the window ends it is verified; at the end it is stale. Reading it
	// later does not move any date.
	if got, _ := s.Claim(ctx, "proj-A", "c-src", day0.Add(twoWeeks-time.Second)); got.Status != "verified" {
		t.Fatalf("status just inside the window = %q", got.Status)
	}
	late, err := s.Claim(ctx, "proj-A", "c-src", day0.Add(twoWeeks))
	if err != nil || late.Status != "stale" || late.State != "verified" || late.VerifiedAt != c.VerifiedAt || late.ExpiresAt != c.ExpiresAt {
		t.Fatalf("after the window: %+v (%v), want stale with unchanged dates", late, err)
	}
	if again, _ := s.Claim(ctx, "proj-A", "c-src", day0); again.Status != "verified" || again.VerifiedAt != c.VerifiedAt {
		t.Fatal("reading a stale claim changed it")
	}
	n, _ := s.CountClaims(ctx, "proj-A", day0.Add(twoWeeks))
	if n != (ClaimCounts{Stale: 1, Candidate: 1}) {
		t.Fatalf("counts = %+v", n)
	}
}

func TestTheDatabaseRefusesAVerifiedClaimWithoutEvidence(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	claim(t, s, "c1", "proj-A", false)
	claim(t, s, "c2", "proj-A", true)
	wantFail(t, s, "only after its evidence", `insert into claims(id,project_id,subject,statement,category,state,verified_at,expires_at,created_at)
		values('x','proj-A','s','t','api','verified','2026-11-20T00:00:00Z','2026-12-20T00:00:00Z','2026-11-20T00:00:00Z')`)
	wantFail(t, s, "needs a quote", `update claims set state = 'verified', verified_at = '2026-11-20T00:00:00Z', expires_at = '2026-12-20T00:00:00Z' where id = 'c1'`)
	wantFail(t, s, "CHECK", `update claims set verified_at = null where id = 'c2'`)
	wantFail(t, s, "never rewritten", `update claims set statement = 'something else' where id = 'c2'`)
	wantFail(t, s, "immutable", `update source_snapshots set content_hash = 'other'`)
	wantFail(t, s, "immutable", `update claim_evidence set quote = 'other words'`)
	// A quote cannot be attached to a claim of another project.
	wantFail(t, s, "FOREIGN KEY", `insert into claim_evidence(id,project_id,claim_id,snapshot_id,quote,created_at) select 'e9','proj-B','c1',id,'q','2026-11-20T00:00:00Z' from source_snapshots limit 1`)
}

func check(t *testing.T, s *Store, c Claim, result string, at time.Time) Claim {
	t.Helper()
	snapN++
	k := EvidenceCheck{ID: fmt.Sprintf("chk-%d", snapN), EvidenceID: c.Evidence[0].ID, Result: result}
	if result == "confirmed" {
		sn := snap(c.Evidence[0].First.URL, at)
		sn.ETag = `"v2"`
		k.Snap = &sn
	}
	if result == "unavailable" {
		k.Detail = "http_status 503"
	}
	got, err := s.ApplyCheck(ctx, c, []EvidenceCheck{k}, at, twoWeeks)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestChecksMoveAClaimOnlyOnWhatTheSourceSays(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	c := claim(t, s, "c1", "proj-A", true)
	later := day0.Add(20 * 24 * time.Hour)

	// The source cannot be reached: nothing about the claim changes, so it stays stale.
	got := check(t, s, c, "unavailable", later)
	if got.Status != "stale" || got.VerifiedAt != c.VerifiedAt || got.ExpiresAt != c.ExpiresAt || len(got.LastChecks) != 1 || got.LastChecks[0].Detail != "http_status 503" {
		t.Fatalf("after an unreachable source: %+v", got)
	}
	// The server says the page is unchanged: verified again from the day of the check.
	got = check(t, s, got, "unchanged", later)
	if got.Status != "verified" || got.VerifiedAt != later.Format(stamp) || got.ExpiresAt != later.Add(twoWeeks).Format(stamp) || got.Evidence[0].Last.ID != c.Evidence[0].First.ID {
		t.Fatalf("after an unchanged page: %+v", got)
	}
	// The quote is gone: disputed, and the date it was last verified is kept.
	got = check(t, s, got, "quote_missing", later.Add(time.Hour))
	if got.Status != "disputed" || got.VerifiedAt != later.Format(stamp) {
		t.Fatalf("after the quote went missing: %+v", got)
	}
	// An unreachable source does not clear a dispute.
	if got = check(t, s, got, "unavailable", later.Add(2*time.Hour)); got.Status != "disputed" {
		t.Fatalf("an unreachable source changed a disputed claim to %q", got.Status)
	}
	// The page changed and holds the quote again: verified, and re-checks use the new snapshot.
	got = check(t, s, got, "confirmed", later.Add(3*time.Hour))
	if got.Status != "verified" || got.Evidence[0].Last.ETag != `"v2"` || got.Evidence[0].First.ETag != `"v1"` {
		t.Fatalf("after the quote was found again: %+v", got)
	}
	if _, err := s.ApplyCheck(ctx, got, nil, later, twoWeeks); err == nil {
		t.Fatal("a check without a result per quote was accepted")
	}
	if _, err := s.ApplyCheck(ctx, got, []EvidenceCheck{{ID: "k", EvidenceID: "evi-of-another-claim", Result: "unchanged"}}, later, twoWeeks); err == nil {
		t.Fatal("a check for a quote of another claim was accepted")
	}
}

func TestSupersedeWithdrawAndProjectScope(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	old := claim(t, s, "c-old", "proj-A", true)
	other := claim(t, s, "c-other", "proj-B", true)

	// A claim of project B cannot be superseded from project A, and nothing is stored.
	_, err := s.AddClaim(ctx, NewClaim{ID: "c-bad", ProjectID: "proj-A", Subject: "s", Statement: "t", Category: "api", Supersedes: other.ID, Now: day0, Window: twoWeeks})
	if !errors.Is(err, ErrClaimClosed) {
		t.Fatalf("superseding another project's claim: %v", err)
	}
	if _, err := s.Claim(ctx, "proj-A", "c-bad", day0); !errors.Is(err, ErrNoClaim) {
		t.Fatal("the refused claim was stored anyway")
	}
	if _, err := s.Claim(ctx, "proj-A", other.ID, day0); !errors.Is(err, ErrNoClaim) {
		t.Fatal("a claim can be read from another project")
	}
	if _, err := s.Claim(ctx, "proj-A", "%", day0); !errors.Is(err, ErrNoClaim) {
		t.Fatal("a wildcard was accepted as an id")
	}

	if _, err := s.AddClaim(ctx, NewClaim{ID: "c-new", ProjectID: "proj-A", Subject: "payments API", Statement: "The limit is now 250 items.", Category: "api", Supersedes: old.ID, Now: day0, Window: twoWeeks}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, "proj-A", "c-", day0); !errors.Is(err, ErrAmbiguousClaim) {
		t.Fatalf("an ambiguous prefix: %v", err)
	}
	got, _ := s.Claim(ctx, "proj-A", old.ID, day0)
	if got.Status != "superseded" || got.SupersededBy != "c-new" {
		t.Fatalf("the replaced claim: %+v", got)
	}
	if _, err := s.ApplyCheck(ctx, got, []EvidenceCheck{{ID: "k1", EvidenceID: got.Evidence[0].ID, Result: "unchanged"}}, day0, twoWeeks); !errors.Is(err, ErrClaimClosed) {
		t.Fatalf("a superseded claim was checked: %v", err)
	}
	if err := s.WithdrawClaim(ctx, "proj-A", old.ID, "x"); !errors.Is(err, ErrClaimClosed) {
		t.Fatalf("a superseded claim was withdrawn: %v", err)
	}
	if err := s.WithdrawClaim(ctx, "proj-A", other.ID, "x"); !errors.Is(err, ErrClaimClosed) {
		t.Fatal("a claim was withdrawn from another project")
	}
	if err := s.WithdrawClaim(ctx, "proj-A", "c-new", "the vendor removed the page"); err != nil {
		t.Fatal(err)
	}
	open, _ := s.Claims(ctx, "proj-A", ClaimFilter{}, day0)
	all, _ := s.Claims(ctx, "proj-A", ClaimFilter{Closed: true}, day0)
	if len(open) != 0 || len(all) != 2 {
		t.Fatalf("open=%d all=%d, want 0 and 2", len(open), len(all))
	}
	if hits, err := s.Claims(ctx, "proj-B", ClaimFilter{Query: "refunds endpoint"}, day0); err != nil || len(hits) != 1 {
		t.Fatalf("search by other word forms found %d (%v), want 1", len(hits), err)
	}
	if hits, _ := s.Claims(ctx, "proj-A", ClaimFilter{Query: "refund"}, day0); len(hits) != 0 {
		t.Fatal("search returned a closed claim or one of another project")
	}
}

func TestDeletingAProjectDeletesItsResearch(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	a := claim(t, s, "c-a", "proj-A", true)
	check(t, s, a, "confirmed", day0.Add(time.Hour))
	claim(t, s, "c-b", "proj-B", true)
	if _, err := s.ForgetProject(ctx, "proj-A", true); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"claims", "claim_evidence", "claim_checks", "sources", "source_snapshots"} {
		var n int
		if err := s.db.QueryRowContext(ctx, `select count(*) from `+table+` where project_id = 'proj-A'`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s still holds %d row(s) of the deleted project (%v)", table, n, err)
		}
	}
	if err := s.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.Claims(ctx, "proj-B", ClaimFilter{Query: "refund"}, day0); len(left) != 1 || left[0].Status != "verified" {
		t.Fatalf("the other project's claim after deletion and compaction: %+v", left)
	}
}
