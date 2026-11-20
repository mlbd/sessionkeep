package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const stamp = "2006-01-02T15:04:05.000000000Z"

var (
	// ErrNoClaim means no claim matches the id or id prefix in that project.
	ErrNoClaim = errors.New("no such claim")
	// ErrAmbiguousClaim means an id prefix matched more than one claim.
	ErrAmbiguousClaim = errors.New("more than one claim matches")
	// ErrClaimClosed means the claim was withdrawn or superseded and cannot change any more.
	ErrClaimClosed = errors.New("the claim is withdrawn or superseded")
)

// Snapshot is what one successful fetch of a source returned. The page text is not stored.
type Snapshot struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	FinalURL     string `json:"final_url"`
	Status       int    `json:"http_status"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Hash         string `json:"content_hash"`
	Bytes        int    `json:"bytes"`
	Truncated    bool   `json:"truncated"`
	Title        string `json:"title,omitempty"`
	Extractor    int    `json:"extractor_version"`
	FetchedAt    string `json:"fetched_at"`
}

// ClaimEvidence is a quote found in a fetched source. Last is the newest snapshot in which the
// quote was found (the first one, until a later check confirms it against changed content).
type ClaimEvidence struct {
	ID    string   `json:"id"`
	Quote string   `json:"quote"`
	First Snapshot `json:"found_in"`
	Last  Snapshot `json:"last_confirmed_in"`
}

// ClaimCheck is the outcome of looking at one quote's source again.
type ClaimCheck struct {
	EvidenceID string `json:"evidence_id"`
	CheckedAt  string `json:"checked_at"`
	Result     string `json:"result"` // confirmed | unchanged | quote_missing | unavailable
	Detail     string `json:"detail,omitempty"`
}

// Claim is a statement about the outside world. Status is State, except that a verified claim
// past its ExpiresAt reads as "stale".
type Claim struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	Subject      string          `json:"subject"`
	Statement    string          `json:"statement"`
	Category     string          `json:"category"`
	AppliesTo    string          `json:"applies_to,omitempty"`
	State        string          `json:"state"`
	Status       string          `json:"status"`
	VerifiedAt   string          `json:"verified_at,omitempty"`
	ExpiresAt    string          `json:"expires_at,omitempty"`
	SupersededBy string          `json:"superseded_by,omitempty"`
	Note         string          `json:"note,omitempty"`
	CreatedAt    string          `json:"created_at"`
	Evidence     []ClaimEvidence `json:"evidence"`
	LastChecks   []ClaimCheck    `json:"last_checks,omitempty"`
}

// NewEvidence is a quote together with the fetch it was found in. IDs are supplied by the caller.
type NewEvidence struct {
	ID, SourceID string
	Snap         Snapshot
	Quote        string
}

// NewClaim describes a claim to store. With evidence it is stored verified until Now+Window;
// without, as a candidate. Supersedes names an open claim of the same project that it replaces.
type NewClaim struct {
	ID, ProjectID, Subject, Statement, Category, AppliesTo, Supersedes string
	Evidence                                                           []NewEvidence
	Now                                                                time.Time
	Window                                                             time.Duration
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func putSnapshot(ctx context.Context, tx *sql.Tx, projectID, sourceID string, sn Snapshot) error {
	_, err := tx.ExecContext(ctx, `insert into source_snapshots(id,project_id,source_id,fetched_at,final_url,http_status,etag,last_modified,content_hash,bytes,truncated,title,extractor_version)
		values(?,?,?,?,?,?,?,?,?,?,?,?,?)`, sn.ID, projectID, sourceID, sn.FetchedAt, sn.FinalURL, sn.Status, nullable(sn.ETag), nullable(sn.LastModified), sn.Hash, sn.Bytes, sn.Truncated, nullable(sn.Title), sn.Extractor)
	return err
}

// AddClaim stores a claim, its sources, snapshots and quotes in one transaction.
func (s *Store) AddClaim(ctx context.Context, n NewClaim) (Claim, error) {
	ts := n.Now.UTC().Format(stamp)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `insert into claims(id,project_id,subject,statement,category,applies_to,state,created_at) values(?,?,?,?,?,?,'candidate',?)`,
		n.ID, n.ProjectID, n.Subject, n.Statement, n.Category, nullable(n.AppliesTo), ts); err != nil {
		return Claim{}, fmt.Errorf("store claim: %w", err)
	}
	for _, e := range n.Evidence {
		sourceID := e.SourceID
		if _, err := tx.ExecContext(ctx, `insert into sources(id,project_id,url,created_at) values(?,?,?,?) on conflict(project_id,url) do nothing`, sourceID, n.ProjectID, e.Snap.URL, ts); err != nil {
			return Claim{}, err
		}
		if err := tx.QueryRowContext(ctx, `select id from sources where project_id = ? and url = ?`, n.ProjectID, e.Snap.URL).Scan(&sourceID); err != nil {
			return Claim{}, err
		}
		if err := putSnapshot(ctx, tx, n.ProjectID, sourceID, e.Snap); err != nil {
			return Claim{}, fmt.Errorf("store snapshot: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `insert into claim_evidence(id,project_id,claim_id,snapshot_id,quote,created_at) values(?,?,?,?,?,?)`, e.ID, n.ProjectID, n.ID, e.Snap.ID, e.Quote, ts); err != nil {
			return Claim{}, fmt.Errorf("store evidence: %w", err)
		}
	}
	if len(n.Evidence) > 0 {
		if _, err := tx.ExecContext(ctx, `update claims set state = 'verified', verified_at = ?, expires_at = ? where id = ?`, ts, n.Now.Add(n.Window).UTC().Format(stamp), n.ID); err != nil {
			return Claim{}, err
		}
	}
	if n.Supersedes != "" {
		res, err := tx.ExecContext(ctx, `update claims set state = 'superseded', superseded_by = ? where project_id = ? and id = ? and state not in ('withdrawn','superseded')`, n.ID, n.ProjectID, n.Supersedes)
		if err != nil {
			return Claim{}, err
		}
		if c, _ := res.RowsAffected(); c == 0 {
			return Claim{}, ErrClaimClosed
		}
	}
	if err := tx.Commit(); err != nil {
		return Claim{}, err
	}
	return s.Claim(ctx, n.ProjectID, n.ID, n.Now)
}

// ClaimFilter narrows Claims. Closed includes withdrawn and superseded claims.
type ClaimFilter struct {
	Query  string
	Closed bool
	Limit  int
}

const claimCols = `c.id, c.project_id, c.subject, c.statement, c.category, coalesce(c.applies_to,''), c.state, coalesce(c.verified_at,''), coalesce(c.expires_at,''), coalesce(c.superseded_by,''), coalesce(c.note,''), c.created_at`

func scanClaim(row interface{ Scan(...any) error }, now time.Time) (Claim, error) {
	var c Claim
	if err := row.Scan(&c.ID, &c.ProjectID, &c.Subject, &c.Statement, &c.Category, &c.AppliesTo, &c.State, &c.VerifiedAt, &c.ExpiresAt, &c.SupersededBy, &c.Note, &c.CreatedAt); err != nil {
		return Claim{}, err
	}
	c.Status = c.State
	if c.State == "verified" && c.ExpiresAt <= now.UTC().Format(stamp) {
		c.Status = "stale"
	}
	c.Evidence = []ClaimEvidence{}
	return c, nil
}

// Claims lists a project's claims, the ones needing attention first, with their evidence.
func (s *Store) Claims(ctx context.Context, projectID string, f ClaimFilter, now time.Time) ([]Claim, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	q, args := `select `+claimCols+` from claims c where c.project_id = ?`, []any{projectID}
	if f.Query != "" {
		q = `select ` + claimCols + ` from claims_fts join claims c on c.seq = claims_fts.rowid where claims_fts match ? and c.project_id = ?`
		args = []any{ftsQuery(f.Query), projectID}
	}
	if !f.Closed {
		q += ` and c.state not in ('withdrawn','superseded')`
	}
	q += ` order by case c.state when 'disputed' then 0 when 'verified' then 1 when 'candidate' then 2 else 3 end, coalesce(c.expires_at,''), c.seq desc limit ?`
	rows, err := s.db.QueryContext(ctx, q, append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	out := []Claim{}
	for rows.Next() {
		c, err := scanClaim(rows, now)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fillClaim(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Claim returns one claim of a project by id or unique id prefix.
func (s *Store) Claim(ctx context.Context, projectID, idOrPrefix string, now time.Time) (Claim, error) {
	if strings.ContainsAny(idOrPrefix, "%_") || idOrPrefix == "" {
		return Claim{}, ErrNoClaim
	}
	rows, err := s.db.QueryContext(ctx, `select `+claimCols+` from claims c where c.project_id = ? and c.id like ? || '%' limit 2`, projectID, idOrPrefix)
	if err != nil {
		return Claim{}, err
	}
	var found []Claim
	for rows.Next() {
		c, err := scanClaim(rows, now)
		if err != nil {
			rows.Close()
			return Claim{}, err
		}
		found = append(found, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Claim{}, err
	}
	switch len(found) {
	case 0:
		return Claim{}, ErrNoClaim
	case 1:
		return found[0], s.fillClaim(ctx, &found[0])
	}
	return Claim{}, ErrAmbiguousClaim
}

const snapCols = `sn.id, so.url, sn.final_url, sn.http_status, coalesce(sn.etag,''), coalesce(sn.last_modified,''), sn.content_hash, sn.bytes, sn.truncated, coalesce(sn.title,''), sn.extractor_version, sn.fetched_at`

func (s *Store) snapshot(ctx context.Context, id string) (Snapshot, error) {
	var sn Snapshot
	err := s.db.QueryRowContext(ctx, `select `+snapCols+` from source_snapshots sn join sources so on so.id = sn.source_id where sn.id = ?`, id).
		Scan(&sn.ID, &sn.URL, &sn.FinalURL, &sn.Status, &sn.ETag, &sn.LastModified, &sn.Hash, &sn.Bytes, &sn.Truncated, &sn.Title, &sn.Extractor, &sn.FetchedAt)
	return sn, err
}

func (s *Store) fillClaim(ctx context.Context, c *Claim) error {
	rows, err := s.db.QueryContext(ctx, `select e.id, e.quote, e.snapshot_id,
		coalesce((select k.snapshot_id from claim_checks k where k.evidence_id = e.id and k.result = 'confirmed' order by k.checked_at desc limit 1), e.snapshot_id)
		from claim_evidence e where e.project_id = ? and e.claim_id = ? order by e.created_at, e.id`, c.ProjectID, c.ID)
	if err != nil {
		return err
	}
	type ref struct{ first, last string }
	var refs []ref
	for rows.Next() {
		var e ClaimEvidence
		var r ref
		if err := rows.Scan(&e.ID, &e.Quote, &r.first, &r.last); err != nil {
			rows.Close()
			return err
		}
		c.Evidence, refs = append(c.Evidence, e), append(refs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i, r := range refs {
		if c.Evidence[i].First, err = s.snapshot(ctx, r.first); err != nil {
			return err
		}
		if c.Evidence[i].Last, err = s.snapshot(ctx, r.last); err != nil {
			return err
		}
		var k ClaimCheck
		err := s.db.QueryRowContext(ctx, `select evidence_id, checked_at, result, coalesce(detail,'') from claim_checks where evidence_id = ? order by checked_at desc limit 1`, c.Evidence[i].ID).
			Scan(&k.EvidenceID, &k.CheckedAt, &k.Result, &k.Detail)
		if err == nil {
			c.LastChecks = append(c.LastChecks, k)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// EvidenceCheck is the outcome for one quote. Confirmed carries the new snapshot (IDs set by
// the caller); the other results carry none.
type EvidenceCheck struct {
	ID, EvidenceID, Result, Detail string
	Snap                           *Snapshot
}

// ApplyCheck records one check per quote and moves the claim accordingly: every quote confirmed
// or unchanged makes it verified until now+window; any quote missing makes it disputed; if a
// source could not be reached and no quote is missing, the claim is left exactly as it was.
// It returns the claim after the change.
func (s *Store) ApplyCheck(ctx context.Context, c Claim, checks []EvidenceCheck, now time.Time, window time.Duration) (Claim, error) {
	if c.State != "verified" && c.State != "disputed" {
		return Claim{}, ErrClaimClosed
	}
	if len(checks) != len(c.Evidence) || len(checks) == 0 {
		return Claim{}, fmt.Errorf("a check needs one result per quote (%d results for %d quotes)", len(checks), len(c.Evidence))
	}
	ts := now.UTC().Format(stamp)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback()
	missing, unavailable := false, false
	for _, k := range checks {
		var snapID any
		last := ""
		for _, e := range c.Evidence {
			if e.ID == k.EvidenceID {
				last = e.Last.ID
			}
		}
		if last == "" {
			return Claim{}, fmt.Errorf("quote %s does not belong to claim %s", k.EvidenceID, c.ID)
		}
		switch k.Result {
		case "confirmed":
			if k.Snap == nil {
				return Claim{}, errors.New("a confirmed check needs its snapshot")
			}
			var sourceID string
			if err := tx.QueryRowContext(ctx, `select source_id from source_snapshots where id = ?`, last).Scan(&sourceID); err != nil {
				return Claim{}, err
			}
			if err := putSnapshot(ctx, tx, c.ProjectID, sourceID, *k.Snap); err != nil {
				return Claim{}, fmt.Errorf("store snapshot: %w", err)
			}
			snapID = k.Snap.ID
		case "unchanged":
			snapID = last
		case "quote_missing":
			missing = true
		case "unavailable":
			unavailable = true
		default:
			return Claim{}, fmt.Errorf("unknown check result %q", k.Result)
		}
		if _, err := tx.ExecContext(ctx, `insert into claim_checks(id,project_id,claim_id,evidence_id,checked_at,result,snapshot_id,detail) values(?,?,?,?,?,?,?,?)`,
			k.ID, c.ProjectID, c.ID, k.EvidenceID, ts, k.Result, snapID, nullable(k.Detail)); err != nil {
			return Claim{}, err
		}
	}
	switch {
	case missing:
		_, err = tx.ExecContext(ctx, `update claims set state = 'disputed' where id = ?`, c.ID)
	case !unavailable:
		_, err = tx.ExecContext(ctx, `update claims set state = 'verified', verified_at = ?, expires_at = ? where id = ?`, ts, now.Add(window).UTC().Format(stamp), c.ID)
	}
	if err != nil {
		return Claim{}, err
	}
	if err := tx.Commit(); err != nil {
		return Claim{}, err
	}
	return s.Claim(ctx, c.ProjectID, c.ID, now)
}

// WithdrawClaim marks an open claim as withdrawn, with the reason.
func (s *Store) WithdrawClaim(ctx context.Context, projectID, id, reason string) error {
	res, err := s.db.ExecContext(ctx, `update claims set state = 'withdrawn', note = ? where project_id = ? and id = ? and state not in ('withdrawn','superseded')`, nullable(reason), projectID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrClaimClosed
	}
	return nil
}

// ClaimCounts says how many open claims a project has in each status.
type ClaimCounts struct {
	Verified  int `json:"verified"`
	Stale     int `json:"stale"`
	Disputed  int `json:"disputed"`
	Candidate int `json:"candidate"`
}

// CountClaims counts a project's open claims by status at the given time.
func (s *Store) CountClaims(ctx context.Context, projectID string, now time.Time) (ClaimCounts, error) {
	var c ClaimCounts
	err := s.db.QueryRowContext(ctx, `select
		coalesce(sum(state = 'verified' and expires_at > ?1),0), coalesce(sum(state = 'verified' and expires_at <= ?1),0),
		coalesce(sum(state = 'disputed'),0), coalesce(sum(state = 'candidate'),0) from claims where project_id = ?2`, now.UTC().Format(stamp), projectID).
		Scan(&c.Verified, &c.Stale, &c.Disputed, &c.Candidate)
	return c, err
}
