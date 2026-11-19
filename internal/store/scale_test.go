//go:build scale

package store

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// Run with: go test -tags scale -run TestScale -v -timeout 60m ./internal/store/
// It stores 100,000 synthetic text records through the same path the ingester uses and measures
// storing and searching. Numbers are for the machine it runs on; nothing here asserts a budget.
func TestScale(t *testing.T) {
	const total = 100_000
	s, path := openTemp(t)
	rnd := rand.New(rand.NewSource(42))
	vocab := make([]string, 6000)
	for i := range vocab {
		vocab[i] = fmt.Sprintf("%s%d", []string{"alpha", "deploy", "invoice", "cache", "token", "route", "schema", "queue", "render", "index"}[i%10], i)
	}
	sentence := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(vocab[rnd.Intn(len(vocab))])
			b.WriteByte(' ')
		}
		return b.String()
	}
	for p := 0; p < 5; p++ {
		mustExec(t, s, `insert into projects(id,name,capture_policy,created_at) values(?,?, 'conversation','2026-01-01T00:00:00Z')`, fmt.Sprintf("proj-%d", p), fmt.Sprintf("P%d", p))
	}

	start := time.Now()
	var commit []time.Duration
	for i := 0; i < total; i++ {
		text := sentence(40 + rnd.Intn(80))
		switch {
		case i == 77_777:
			text += " UNIQUEMARKERZQ9 the staging database is refreshed every Tuesday night"
		case i%10_000 == 5:
			text += " ECONNRESET: socket hang up at POST /api/v2/users?id=7"
		}
		t0 := time.Now()
		res, err := s.IngestEvent(ctx, IngestInput{
			EventID: fmt.Sprintf("ev-%d", i), EventKey: fmt.Sprintf("hook:ev-%d", i), DeviceID: "dev", ProjectID: fmt.Sprintf("proj-%d", i%5),
			Agent: "claude-code", SessionExternal: fmt.Sprintf("s-%d", i/200), NewSessionID: fmt.Sprintf("sess-%d", i), AdapterVersion: 1,
			Kind: "user_prompt", ObservedAt: fmt.Sprintf("2026-%02d-%02dT10:00:00.%09dZ", 1+i%12, 1+i%28, i), ReceivedAt: "2026-11-20T00:00:00Z",
			SafePayload: `{"prompt_id":"p"}`, RedactionStatus: "none",
			Evidence: &EvidenceRow{ID: fmt.Sprintf("evid-%07d", i), Origin: "hook:UserPromptSubmit", Locator: "l", Text: text, Completeness: "complete", Trust: []string{"user", "assistant"}[i%2]},
		})
		if err != nil || res.Outcome != Inserted {
			t.Fatalf("event %d: %+v %v", i, res, err)
		}
		if i%50 == 0 {
			commit = append(commit, time.Since(t0))
		}
	}
	elapsed := time.Since(start)
	s.db.Exec("pragma wal_checkpoint(TRUNCATE)")
	fi, _ := os.Stat(path)
	t.Logf("STORED %d records in %.0f s = %.0f records/s; per-record commit p50 %.2f ms p95 %.2f ms; database %.0f MB",
		total, elapsed.Seconds(), float64(total)/elapsed.Seconds(), ms(commit, 50), ms(commit, 95), float64(fi.Size())/1e6)

	measure := func(label string, n int, fn func(i int) int) {
		var d []time.Duration
		hits := 0
		for i := 0; i < n; i++ {
			t0 := time.Now()
			hits += fn(i)
			d = append(d, time.Since(t0))
		}
		t.Logf("SEARCH %-58s n=%d p50 %7.2f ms  p95 %7.2f ms  max %7.2f ms  avg hits %.1f", label, n, ms(d, 50), ms(d, 95), ms(d, 100), float64(hits)/float64(n))
	}
	q := func(p SearchParams) int {
		r, err := s.Search(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return len(r.Hits)
	}
	measure("two common words, one project (all words)", 60, func(i int) int {
		return q(SearchParams{ProjectID: fmt.Sprintf("proj-%d", i%5), Query: vocab[rnd.Intn(200)] + " " + vocab[rnd.Intn(200)]})
	})
	measure("one word, one project", 60, func(i int) int {
		return q(SearchParams{ProjectID: fmt.Sprintf("proj-%d", i%5), Query: vocab[rnd.Intn(len(vocab))]})
	})
	measure("one word, all projects, limit 50", 60, func(i int) int {
		return q(SearchParams{AllProjects: true, Query: vocab[rnd.Intn(len(vocab))], Limit: 50})
	})
	measure("unique marker (word index)", 30, func(int) int { return q(SearchParams{ProjectID: "proj-2", Query: "UNIQUEMARKERZQ9"}) })
	measure("different word endings: 'staging database refresh'", 30, func(int) int { return q(SearchParams{ProjectID: "proj-2", Query: "staging database refresh"}) })
	measure("exact error string (literal scan of one project)", 15, func(i int) int {
		return q(SearchParams{ProjectID: "proj-0", Query: "ECONNRESET: socket hang up at POST /api/v2/users?id=7", Exact: true})
	})
	measure("literal scan that matches nothing, all projects (worst case)", 8, func(int) int {
		return q(SearchParams{AllProjects: true, Query: "this exact sentence is not stored anywhere", Exact: true})
	})
	measure("no word matches -> falls through to literal and partial", 15, func(int) int {
		return q(SearchParams{ProjectID: "proj-1", Query: "zzzunknownword yyyunknownword"})
	})
	measure("read one record by id", 60, func(i int) int {
		if _, err := s.GetEvidence(ctx, fmt.Sprintf("evid-%07d", rnd.Intn(total)), "", true); err != nil {
			t.Fatal(err)
		}
		return 1
	})
	measure("open read-only + one-word search + close (what one command does)", 30, func(i int) int {
		ro, err := OpenReadOnly(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer ro.Close()
		r, err := ro.Search(ctx, SearchParams{ProjectID: "proj-3", Query: vocab[rnd.Intn(len(vocab))]})
		if err != nil {
			t.Fatal(err)
		}
		return len(r.Hits)
	})
	t0 := time.Now()
	if err := s.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("COMPACT (search index rebuild + file rewrite, as after a deletion): %.1f s", time.Since(t0).Seconds())
	t0 = time.Now()
	out := path + ".export"
	if err := s.ExportTo(ctx, out); err != nil {
		t.Fatal(err)
	}
	t.Logf("EXPORT COPY of the database: %.1f s", time.Since(t0).Seconds())
}

func ms(d []time.Duration, p int) float64 {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	i := (len(d) - 1) * p / 100
	return float64(d[i].Microseconds()) / 1000
}
