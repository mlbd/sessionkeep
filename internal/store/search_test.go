package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fill stores texts as evidence for a project, alternating who said them, one minute apart.
func fill(t *testing.T, s *Store, project string, texts ...string) []string {
	t.Helper()
	var ids []string
	var n int
	s.db.QueryRow("select count(*) from evidence").Scan(&n)
	for i, tx := range texts {
		id := fmt.Sprintf("evid-%04d-%s", n+i, project)
		who := []string{"user", "assistant", "tool"}[i%3]
		mustExec(t, s, `insert into evidence(id,project_id,origin,source_locator,observed_at,text,completeness,trust_level)
			values(?,?, 'hook:test', ?, ?, ?, 'complete', ?)`, id, project, id, fmt.Sprintf("2026-11-%02dT10:%02d:00Z", 10+(n+i)/60, (n+i)%60), tx, who)
		ids = append(ids, id)
	}
	return ids
}

func search(t *testing.T, s *Store, p SearchParams) SearchResult {
	t.Helper()
	res, err := s.Search(ctx, p)
	if err != nil {
		t.Fatalf("search %+v: %v", p, err)
	}
	return res
}

func ids(r SearchResult) []string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, h.ID)
	}
	return out
}

func TestSearchIsScopedBeforeRanking(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	fill(t, s, "proj-A", "the tenant isolation check lives in auth_service.ts", "unrelated note about fonts")
	// Project B has many more, better-ranking matches for the same words.
	var bTexts []string
	for i := 0; i < 30; i++ {
		bTexts = append(bTexts, "tenant isolation tenant isolation tenant isolation SECRET-B-"+fmt.Sprint(i))
	}
	fill(t, s, "proj-B", bTexts...)

	res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "tenant isolation", Limit: 50})
	if len(res.Hits) != 1 || res.Hits[0].ProjectID != "proj-A" || res.Method != "words" {
		t.Fatalf("project A search: %+v", res)
	}
	for _, h := range res.Hits {
		if strings.Contains(h.Snippet, "SECRET-B") {
			t.Fatal("project B text leaked into a project A search")
		}
	}
	if !strings.Contains(res.Hits[0].Snippet, "**tenant**") {
		t.Fatalf("snippet should mark the match: %q", res.Hits[0].Snippet)
	}
	// Exact (literal) mode is scoped the same way.
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "SECRET-B-7", Exact: true}); len(res.Hits) != 0 {
		t.Fatalf("exact search crossed projects: %+v", res)
	}
	if res := search(t, s, SearchParams{AllProjects: true, Query: "tenant isolation", Limit: 50}); len(res.Hits) != 31 {
		t.Fatalf("explicit all-projects search found %d, want 31", len(res.Hits))
	}
	// No scope at all, or both, is refused.
	for _, p := range []SearchParams{{Query: "tenant"}, {ProjectID: "proj-A", AllProjects: true, Query: "tenant"}} {
		if _, err := s.Search(ctx, p); err == nil {
			t.Fatalf("search without exactly one scope was accepted: %+v", p)
		}
	}
	if res := search(t, s, SearchParams{ProjectID: "no-such-project", Query: "tenant"}); len(res.Hits) != 0 {
		t.Fatal("unknown project must return nothing")
	}
}

func TestExactStringsThatTheWordIndexSplits(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	want := fill(t, s, "proj-A",
		`Request failed: ECONNRESET: socket hang up at POST /api/v2/users?id=7&retry=true (attempt 3/5)`,
		`edited C:\Work\App\src\auth_service.ts and src/components/LoginForm.tsx`,
		`see commit a019b3b51b07 and event 6f9619ff-8b86-d011-b42d-00c04fc964ff`,
		`TypeError: Cannot read properties of undefined (reading 'map')`,
		`run: go test ./... -run 'TestFoo|TestBar' -count=1`,
	)
	fill(t, s, "proj-A", "ordinary sentence with nothing special", "another ordinary one")
	cases := map[string]string{
		`ECONNRESET: socket hang up`:            want[0],
		`/api/v2/users?id=7&retry=true`:         want[0],
		`(attempt 3/5)`:                         want[0],
		`C:\Work\App\src\auth_service.ts`:       want[1],
		`c:\work\app\src\AUTH_SERVICE.TS`:       want[1],
		`src/components/LoginForm.tsx`:          want[1],
		`a019b3b`:                               want[2],
		`6f9619ff-8b86-d011-b42d-00c04fc964ff`:  want[2],
		`(reading 'map')`:                       want[3],
		`'TestFoo|TestBar'`:                     want[4],
		`"Cannot read properties of undefined"`: want[3],
	}
	for q, id := range cases {
		res := search(t, s, SearchParams{ProjectID: "proj-A", Query: q})
		found := false
		for _, h := range res.Hits {
			found = found || h.ID == id
		}
		if !found {
			t.Errorf("query %q did not find %s (method %s, hits %v)", q, id, res.Method, ids(res))
		}
	}
	// Forced literal search marks the match in its snippet.
	res := search(t, s, SearchParams{ProjectID: "proj-A", Query: `users?id=7`, Exact: true})
	if len(res.Hits) != 1 || !strings.Contains(res.Hits[0].Snippet, "**users?id=7**") || res.Method != "exact" {
		t.Fatalf("%+v", res)
	}
}

func TestPunctuationAndOperatorWordsNeverBreakTheQuery(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	fill(t, s, "proj-A", "alpha AND beta OR gamma NOT delta NEAR epsilon", `she said "hello" (twice) * and left`, "c++ and c# and f#")
	for _, q := range []string{`"`, `""`, `(`, `)`, `*`, `^`, `:`, `-`, `+`, `a"b`, `"unterminated`, `NEAR(`, `NEAR(a b, 3)`, `AND`, `OR NOT`, `col:value`, `{a b}`,
		`a AND`, `-alpha`, `alpha*`, `^alpha`, `"hello" (twice)`, `c++`, `%`, `_`, `\`, `'; drop table evidence; --`, strings.Repeat("x ", 400), "tab\there", "new\nline", "日本語", "🚀"} {
		if _, err := s.Search(ctx, SearchParams{ProjectID: "proj-A", Query: q}); err != nil {
			t.Errorf("query %q failed: %v", q, err)
		}
	}
	// Operator words are searched for, not interpreted: "OR" must not turn the query into a union.
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "alpha OR nonexistentword"}); len(res.Hits) != 0 {
		t.Fatalf("OR was interpreted as an operator: %v", ids(res))
	}
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "alpha NOT delta"}); len(res.Hits) != 1 {
		t.Fatalf("NOT was interpreted as an operator: %v", ids(res))
	}
	var n int
	s.db.QueryRow("select count(*) from evidence").Scan(&n)
	if n != 3 {
		t.Fatalf("evidence table has %d rows after hostile queries, want 3", n)
	}
	for _, q := range []string{"", "   ", "\t\n"} {
		if _, err := s.Search(ctx, SearchParams{ProjectID: "proj-A", Query: q}); err == nil {
			t.Errorf("blank query %q was accepted", q)
		}
	}
}

func TestNoMatchReturnsNothing(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	fill(t, s, "proj-A", "something stored")
	res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "zebracrossing unicornword"})
	if len(res.Hits) != 0 || res.NextCursor != "" || res.Hits == nil {
		t.Fatalf("%+v", res)
	}
}

func TestPaginationReturnsEveryRowExactlyOnce(t *testing.T) {
	for _, exact := range []bool{false, true} {
		s, _ := openTemp(t)
		seed(t, s)
		var texts []string
		for i := 0; i < 25; i++ {
			texts = append(texts, fmt.Sprintf("deploy note number %d about the deploy pipeline", i))
		}
		want := fill(t, s, "proj-A", texts...)
		fill(t, s, "proj-B", "deploy note in another project")

		seen := map[string]int{}
		cursor, pages := "", 0
		for {
			res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "deploy note", Exact: exact, Limit: 7, Cursor: cursor})
			pages++
			for _, h := range res.Hits {
				seen[h.ID]++
			}
			if pages == 1 {
				// Text captured between pages must not shift or join the result set.
				fill(t, s, "proj-A", "deploy note added while paging", "another deploy note added while paging")
			}
			if cursor = res.NextCursor; cursor == "" {
				break
			}
			if pages > 10 {
				t.Fatal("pagination does not terminate")
			}
		}
		if pages != 4 || len(seen) != 25 {
			t.Fatalf("exact=%v: %d pages, %d distinct rows; want 4 pages, 25 rows", exact, pages, len(seen))
		}
		for _, id := range want {
			if seen[id] != 1 {
				t.Fatalf("exact=%v: row %s returned %d times", exact, id, seen[id])
			}
		}
	}
}

func TestCursorAndLimitValidation(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	var texts []string
	for i := 0; i < 120; i++ {
		texts = append(texts, fmt.Sprintf("row %d common", i))
	}
	fill(t, s, "proj-A", texts...)
	for _, c := range []string{"garbage", "!!!", "d29yZHM6eDow", "bm9wZTowOjA"} {
		if _, err := s.Search(ctx, SearchParams{ProjectID: "proj-A", Query: "common", Cursor: c}); !errors.Is(err, ErrBadCursor) {
			t.Errorf("cursor %q: want ErrBadCursor, got %v", c, err)
		}
	}
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "common", Limit: 100000}); len(res.Hits) != maxLimit || res.NextCursor == "" {
		t.Fatalf("limit must be capped at %d: got %d", maxLimit, len(res.Hits))
	}
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "common"}); len(res.Hits) != defaultLimit {
		t.Fatalf("default limit: got %d", len(res.Hits))
	}
}

func TestFilters(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	fill(t, s, "proj-A", "topic one", "topic two", "topic three", "topic four", "topic five", "topic six") // user, assistant, tool, user, …
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "topic", Who: "assistant"}); len(res.Hits) != 2 {
		t.Fatalf("who=assistant: %d hits", len(res.Hits))
	}
	if res := search(t, s, SearchParams{ProjectID: "proj-A", Query: "topic", Since: "2026-11-10T10:02:00Z", Until: "2026-11-10T10:04:00Z"}); len(res.Hits) != 2 {
		t.Fatalf("time window: %d hits, want 2", len(res.Hits))
	}
	if _, err := s.Search(ctx, SearchParams{ProjectID: "proj-A", Query: "topic", Who: "everyone"}); err == nil {
		t.Fatal("unknown who filter accepted")
	}
}

func TestGetEvidenceEnforcesScopeEvenWithTheID(t *testing.T) {
	s, _ := openTemp(t)
	seed(t, s)
	a := fill(t, s, "proj-A", "text that belongs to project A")[0]
	b := fill(t, s, "proj-B", "text that belongs to project B")[0]
	mustExec(t, s, insEvent, "ev-a", "proj-A", "sess-A", "k-a", a)

	r, err := s.GetEvidence(ctx, a, "proj-A", false)
	if err != nil || r.Text != "text that belongs to project A" || r.Session != "ext-A" || r.EventKind != "user_prompt" || r.ProjectName != "Project A" || r.Completeness != "complete" {
		t.Fatalf("%+v %v", r, err)
	}
	if r, err := s.GetEvidence(ctx, a[:10], "proj-A", false); err != nil || r.ID != a {
		t.Fatalf("prefix lookup: %+v %v", r, err)
	}
	if _, err := s.GetEvidence(ctx, b, "proj-A", false); !errors.Is(err, ErrNoEvidence) {
		t.Fatalf("project B's record was readable from project A: %v", err)
	}
	if r, err := s.GetEvidence(ctx, b, "", true); err != nil || r.ProjectID != "proj-B" {
		t.Fatalf("explicit all-projects lookup: %+v %v", r, err)
	}
	for _, key := range []string{"evid", "short", "evid-%%%%", "________", "nonexistent-id-123"} {
		if _, err := s.GetEvidence(ctx, key, "proj-A", false); !errors.Is(err, ErrNoEvidence) {
			t.Errorf("key %q: want ErrNoEvidence, got %v", key, err)
		}
	}
	if _, err := s.GetEvidence(ctx, a, "", false); err == nil {
		t.Fatal("lookup without a scope accepted")
	}
}

func TestOpenReadOnlyCannotWriteAndNeedsACurrentSchema(t *testing.T) {
	s, path := openTemp(t)
	seed(t, s)
	fill(t, s, "proj-A", "readable text")
	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if res, err := ro.Search(ctx, SearchParams{ProjectID: "proj-A", Query: "readable"}); err != nil || len(res.Hits) != 1 {
		t.Fatalf("read-only search: %+v %v", res, err)
	}
	if _, err := ro.db.Exec("delete from evidence"); err == nil {
		t.Fatal("a read-only store accepted a write")
	}
	if _, err := OpenReadOnly(ctx, path+".missing"); !errors.Is(err, ErrNeedsSetup) {
		t.Fatalf("missing database: %v", err)
	}
	mustExec(t, s, "insert into schema_migrations values(9999,'future','x','t')")
	if _, err := OpenReadOnly(ctx, path); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("newer schema: %v", err)
	}
	mustExec(t, s, "delete from schema_migrations where version >= 2")
	if _, err := OpenReadOnly(ctx, path); !errors.Is(err, ErrNeedsSetup) {
		t.Fatalf("older schema: %v", err)
	}
}
