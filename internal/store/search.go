package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// ErrNeedsSetup means the database is missing or its schema does not match this binary; a command
// that holds the lease (for example `sessionkeep setup`) has to create or migrate it first.
var ErrNeedsSetup = errors.New("the database is missing or needs migrating; run `sessionkeep setup`")

// ErrNoEvidence means no stored text matches the id within the allowed scope.
var ErrNoEvidence = errors.New("no such record in this scope")

// ErrBadCursor means the cursor was not produced by an earlier search with the same query.
var ErrBadCursor = errors.New("invalid cursor")

// OpenReadOnly opens an existing database for reading. It takes no lease, runs no migration and
// cannot write; searches use it so they never wait for, or block, the ingester.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNeedsSetup
	}
	p := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(filepath.ToSlash(path))
	db, err := sql.Open("sqlite", "file:"+p+"?mode=ro&_pragma=busy_timeout(3000)&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	known, err := fs.ReadDir(embedded, "migrations")
	if err != nil {
		db.Close()
		return nil, err
	}
	var have int
	if err := db.QueryRowContext(ctx, "select coalesce(max(version),0) from schema_migrations").Scan(&have); err != nil || have != len(known) {
		db.Close()
		if have > len(known) {
			return nil, ErrSchemaTooNew
		}
		return nil, ErrNeedsSetup
	}
	return &Store{db: db}, nil
}

// SearchParams describes one search. Exactly one of ProjectID and AllProjects must be set: a
// search without a scope is refused rather than treated as "everything".
type SearchParams struct {
	ProjectID   string
	AllProjects bool
	Query       string
	Exact       bool   // match the query as one literal string instead of as words
	Who         string // "", user, assistant or tool
	Since       string // inclusive lower bound on observed_at, RFC 3339 prefix
	Until       string // exclusive upper bound
	Limit       int
	Cursor      string
}

// Hit is one search result. Snippet marks matches with ** on both sides (plain ASCII, so it
// survives any console code page).
type Hit struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Who        string `json:"who"`
	Origin     string `json:"origin"`
	ObservedAt string `json:"observed_at"`
	Snippet    string `json:"snippet"`
}

// SearchResult is one page of hits.
type SearchResult struct {
	Hits []Hit `json:"hits"`
	// Method says how the hits were found: "words" (every word present, word endings ignored),
	// "exact" (the query as one literal string) or "any" (only some of the words present).
	Method     string `json:"method"`
	NextCursor string `json:"next_cursor,omitempty"`
}

const (
	defaultLimit = 10
	maxLimit     = 50
)

// ftsQuery turns free text into a safe FTS5 expression: every word becomes a quoted string, so
// punctuation and words like AND, OR, NEAR are searched for rather than interpreted. A query
// wrapped in double quotes becomes one phrase. It returns "" when the text has no searchable word.
func ftsQuery(q string) string {
	q = strings.TrimSpace(q)
	phrase := len(q) >= 2 && strings.HasPrefix(q, `"`) && strings.HasSuffix(q, `"`)
	var terms []string
	for _, f := range strings.Fields(strings.Trim(q, `"`)) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
			continue
		}
		terms = append(terms, strings.ReplaceAll(f, `"`, `""`))
	}
	if len(terms) == 0 {
		return ""
	}
	if phrase {
		return `"` + strings.Join(terms, " ") + `"`
	}
	return `"` + strings.Join(terms, `" "`) + `"`
}

// ftsAnyQuery is the fallback form: records containing any of the words, best matches first.
// It returns "" when there is only one word (the all-words search already covered it).
func ftsAnyQuery(q string) string {
	all := ftsQuery(strings.Trim(strings.TrimSpace(q), `"`))
	if !strings.Contains(all, `" "`) {
		return ""
	}
	return strings.ReplaceAll(all, `" "`, `" OR "`)
}

// Search finds stored text. The project filter is part of the query itself, applied before
// ranking, never a post-filter. Word search uses the full-text index; if it finds nothing, or
// Exact is set, the query is matched as a literal string (case-insensitive for ASCII), which is
// what finds error messages, paths and ids that the word index splits apart.
func (s *Store) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	if (p.ProjectID == "") == !p.AllProjects {
		return SearchResult{}, errors.New("a search needs exactly one scope: a project or the explicit all-projects flag")
	}
	if strings.TrimSpace(p.Query) == "" {
		return SearchResult{}, errors.New("empty query")
	}
	switch p.Who {
	case "", "user", "assistant", "tool":
	default:
		return SearchResult{}, fmt.Errorf("who must be user, assistant or tool (got %q)", p.Who)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	// The cursor pins the method, the offset and the newest row that existed when the first page
	// was fetched, so later pages are not shifted by text captured in the meantime.
	method, offset, maxSeq := "", 0, int64(0)
	if p.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(p.Cursor)
		parts := strings.Split(string(raw), ":")
		if err != nil || len(parts) != 3 || (parts[0] != "words" && parts[0] != "exact" && parts[0] != "any") {
			return SearchResult{}, ErrBadCursor
		}
		method = parts[0]
		offset, err = strconv.Atoi(parts[1])
		if err != nil || offset < 0 {
			return SearchResult{}, ErrBadCursor
		}
		if maxSeq, err = strconv.ParseInt(parts[2], 10, 64); err != nil {
			return SearchResult{}, ErrBadCursor
		}
	} else if err := s.db.QueryRowContext(ctx, "select coalesce(max(seq),0) from evidence").Scan(&maxSeq); err != nil {
		return SearchResult{}, err
	}

	where := []string{"e.seq <= ?", "e.text is not null"}
	args := []any{maxSeq}
	if !p.AllProjects {
		where = append(where, "e.project_id = ?")
		args = append(args, p.ProjectID)
	}
	if p.Who != "" {
		where = append(where, "e.trust_level = ?")
		args = append(args, p.Who)
	}
	if p.Since != "" {
		where = append(where, "e.observed_at >= ?")
		args = append(args, p.Since)
	}
	if p.Until != "" {
		where = append(where, "e.observed_at < ?")
		args = append(args, p.Until)
	}
	cond := strings.Join(where, " and ")

	run := func(m string) ([]Hit, error) {
		var q string
		var qa []any
		if m == "words" || m == "any" {
			q = `select e.id, e.project_id, e.trust_level, e.origin, e.observed_at, snippet(evidence_fts, 0, '**', '**', ' ... ', 18)
				from evidence_fts join evidence e on e.seq = evidence_fts.rowid
				where evidence_fts match ? and ` + cond + ` order by bm25(evidence_fts), e.seq desc limit ? offset ?`
			match := ftsQuery(p.Query)
			if m == "any" {
				match = ftsAnyQuery(p.Query)
			}
			qa = append([]any{match}, args...)
		} else {
			needle := strings.Trim(strings.TrimSpace(p.Query), `"`)
			q = `select e.id, e.project_id, e.trust_level, e.origin, e.observed_at, e.text
				from evidence e where instr(lower(e.text), lower(?)) > 0 and ` + cond + ` order by e.observed_at desc, e.seq desc limit ? offset ?`
			qa = append([]any{needle}, args...)
		}
		rows, err := s.db.QueryContext(ctx, q, append(qa, limit+1, offset)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var hits []Hit
		for rows.Next() {
			var h Hit
			if err := rows.Scan(&h.ID, &h.ProjectID, &h.Who, &h.Origin, &h.ObservedAt, &h.Snippet); err != nil {
				return nil, err
			}
			if m == "exact" {
				h.Snippet = around(h.Snippet, strings.Trim(strings.TrimSpace(p.Query), `"`))
			}
			hits = append(hits, h)
		}
		return hits, rows.Err()
	}

	if method == "" {
		method = "words"
		if p.Exact || ftsQuery(p.Query) == "" {
			method = "exact"
		}
	}
	hits, err := run(method)
	if err != nil {
		return SearchResult{}, err
	}
	// Nothing contains all the words: try the whole query as one literal string, and if that
	// fails too, records that contain only some of the words. The result says which it was.
	if len(hits) == 0 && method == "words" && p.Cursor == "" {
		method = "exact"
		if hits, err = run(method); err != nil {
			return SearchResult{}, err
		}
		if len(hits) == 0 && ftsAnyQuery(p.Query) != "" {
			method = "any"
			if hits, err = run(method); err != nil {
				return SearchResult{}, err
			}
		}
		if len(hits) == 0 {
			method = "words"
		}
	}
	res := SearchResult{Hits: hits, Method: method}
	if len(hits) > limit {
		res.Hits = hits[:limit]
		res.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%d:%d", method, offset+limit, maxSeq)))
	}
	if res.Hits == nil {
		res.Hits = []Hit{}
	}
	return res, nil
}

// around returns the part of text surrounding the first case-insensitive occurrence of needle,
// with the match marked like the word-search snippets.
func around(text, needle string) string {
	i := strings.Index(strings.ToLower(text), strings.ToLower(needle))
	if i < 0 || len(strings.ToLower(text)) != len(text) { // lower-casing changed byte offsets: give the start instead
		r := []rune(text)
		if len(r) > 160 {
			return string(r[:160]) + " ..."
		}
		return text
	}
	start, end := i, i+len(needle)
	for n := 0; start > 0 && n < 60; n++ {
		start--
		for start > 0 && text[start]&0xC0 == 0x80 {
			start--
		}
	}
	for n := 0; end < len(text) && n < 60; n++ {
		end++
		for end < len(text) && text[end]&0xC0 == 0x80 {
			end++
		}
	}
	out := text[start:i] + "**" + text[i:i+len(needle)] + "**" + text[i+len(needle):end]
	if start > 0 {
		out = "... " + out
	}
	if end < len(text) {
		out += " ..."
	}
	return strings.ReplaceAll(out, "\n", " ")
}

// Record is one stored text with where it came from.
type Record struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	ProjectName    string `json:"project_name"`
	Who            string `json:"who"`
	Origin         string `json:"origin"`
	ObservedAt     string `json:"observed_at"`
	Session        string `json:"session_id,omitempty"` // the host's session id
	EventKind      string `json:"event_kind,omitempty"`
	Completeness   string `json:"completeness"`
	RedactionCount int    `json:"redactions"`
	Text           string `json:"text"`
}

// GetEvidence returns one stored text by id or unique id prefix (at least 8 characters). With a
// projectID the record must belong to that project; knowing an id is not enough to read across
// projects.
func (s *Store) GetEvidence(ctx context.Context, idOrPrefix, projectID string, allProjects bool) (Record, error) {
	if (projectID == "") == !allProjects {
		return Record{}, errors.New("a lookup needs exactly one scope: a project or the explicit all-projects flag")
	}
	if len(idOrPrefix) < 8 || strings.ContainsAny(idOrPrefix, `%_\`) {
		return Record{}, ErrNoEvidence
	}
	q := `select e.id, e.project_id, p.name, e.trust_level, e.origin, e.observed_at, coalesce(s.external_session_id,''), coalesce(v.kind,''),
		e.completeness, e.redaction_count, coalesce(e.text,'')
		from evidence e join projects p on p.id = e.project_id
		left join events v on v.evidence_id = e.id and v.project_id = e.project_id
		left join sessions s on s.id = v.session_id and s.project_id = v.project_id
		where e.id >= ? and e.id < ?`
	// A range on the id index finds both the exact id and every id starting with the prefix.
	// (An "id = ? or id like ?" condition made SQLite read every row: 204 ms per lookup at
	// 100,000 records, measured in the Phase 1 scale run.)
	args := []any{idOrPrefix, idOrPrefix + "\U0010FFFF"}
	if !allProjects {
		q += ` and e.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, q+` order by e.id limit 2`, args...)
	if err != nil {
		return Record{}, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.ProjectName, &r.Who, &r.Origin, &r.ObservedAt, &r.Session, &r.EventKind, &r.Completeness, &r.RedactionCount, &r.Text); err != nil {
			return Record{}, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return Record{}, err
	}
	for _, r := range out {
		if r.ID == idOrPrefix {
			return r, nil
		}
	}
	switch len(out) {
	case 0:
		return Record{}, ErrNoEvidence
	case 1:
		return out[0], nil
	}
	return Record{}, fmt.Errorf("%w: the id prefix matches more than one record; use more characters", ErrNoEvidence)
}
