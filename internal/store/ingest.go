package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Outcomes of IngestEvent.
const (
	Inserted = "inserted"
	Replayed = "replayed" // the event was already stored; nothing changed
	Orphan   = "orphan"   // it names a project that does not exist
)

// AutoProject describes the project to create when an event carries an auto-add request and its
// root is not registered yet.
type AutoProject struct {
	ProjectID, RootID string // ids to use if a project has to be created
	Name, Root, Level string
}

// EvidenceRow is the permitted text that belongs to an event.
type EvidenceRow struct {
	ID, Origin, Locator, Text string
	Completeness, Trust       string
	RedactionCount            int
}

// IngestInput is one normalized event ready to store.
type IngestInput struct {
	EventID, EventKey      string
	DeviceID, ProjectID    string // ProjectID is empty when Auto is set
	Auto                   *AutoProject
	Agent, SessionExternal string
	NewSessionID           string // used if the session row has to be created
	AdapterVersion         int
	Kind                   string
	ObservedAt, ReceivedAt string
	SafePayload            string // JSON without the text that goes to Evidence
	Evidence               *EvidenceRow
	RedactionStatus        string
	EndsSession            bool
}

// IngestResult says what IngestEvent did.
type IngestResult struct {
	Outcome        string
	ProjectID      string
	ProjectCreated bool
}

// IngestEvent stores one event with its session, optional auto-added project and evidence in a
// single transaction. Storing the same event again changes nothing.
func (s *Store) IngestEvent(ctx context.Context, in IngestInput) (IngestResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestResult{}, err
	}
	defer tx.Rollback()
	res, err := ingestTx(ctx, tx, in)
	if err != nil {
		return IngestResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IngestResult{}, err
	}
	return res, nil
}

// IngestEvents stores several events in one transaction: all of them or, on any error, none.
// It is for bulk work such as importing a whole earlier session, where one durable write per
// event would be needlessly slow. Events already stored are reported as replayed, as usual.
func (s *Store) IngestEvents(ctx context.Context, ins []IngestInput) ([]IngestResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := make([]IngestResult, 0, len(ins))
	for _, in := range ins {
		res, err := ingestTx(ctx, tx, in)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func ingestTx(ctx context.Context, tx *sql.Tx, in IngestInput) (IngestResult, error) {
	var err error
	var seen int
	if err := tx.QueryRowContext(ctx, `select count(*) from events where event_key = ?`, in.EventKey).Scan(&seen); err != nil {
		return IngestResult{}, err
	}
	if seen > 0 {
		return IngestResult{Outcome: Replayed}, nil
	}

	res := IngestResult{Outcome: Inserted, ProjectID: in.ProjectID}
	if in.Auto != nil {
		err := tx.QueryRowContext(ctx, `select project_id from project_roots where device_id = ? and normalized_path = ?`, in.DeviceID, in.Auto.Root).Scan(&res.ProjectID)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `insert into projects(id,name,capture_policy,created_at,added_by) values(?,?,?,?, 'auto')`,
				in.Auto.ProjectID, in.Auto.Name, in.Auto.Level, in.ReceivedAt); err != nil {
				return IngestResult{}, fmt.Errorf("auto-add project: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `insert into project_roots(id,project_id,device_id,normalized_path,last_seen) values(?,?,?,?,?)`,
				in.Auto.RootID, in.Auto.ProjectID, in.DeviceID, in.Auto.Root, in.ReceivedAt); err != nil {
				return IngestResult{}, fmt.Errorf("auto-add root: %w", err)
			}
			res.ProjectID, res.ProjectCreated = in.Auto.ProjectID, true
		} else if err != nil {
			return IngestResult{}, err
		}
	} else {
		var n int
		if err := tx.QueryRowContext(ctx, `select count(*) from projects where id = ?`, in.ProjectID).Scan(&n); err != nil {
			return IngestResult{}, err
		}
		if n == 0 {
			return IngestResult{Outcome: Orphan}, nil
		}
	}

	var sessionID string
	err = tx.QueryRowContext(ctx, `select id from sessions where project_id = ? and agent = ? and device_id = ? and external_session_id = ?`,
		res.ProjectID, in.Agent, in.DeviceID, in.SessionExternal).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		sessionID = in.NewSessionID
		if _, err := tx.ExecContext(ctx, `insert into sessions(id,project_id,agent,external_session_id,device_id,started_at,adapter_version) values(?,?,?,?,?,?,?)`,
			sessionID, res.ProjectID, in.Agent, in.SessionExternal, in.DeviceID, in.ObservedAt, in.AdapterVersion); err != nil {
			return IngestResult{}, fmt.Errorf("create session: %w", err)
		}
	} else if err != nil {
		return IngestResult{}, err
	}
	if in.EndsSession {
		if _, err := tx.ExecContext(ctx, `update sessions set ended_at = ?, state = 'ended' where id = ?`, in.ObservedAt, sessionID); err != nil {
			return IngestResult{}, err
		}
	}

	var evidenceID any
	if e := in.Evidence; e != nil {
		if _, err := tx.ExecContext(ctx, `insert into evidence(id,project_id,origin,source_locator,observed_at,text,completeness,trust_level,redaction_count)
			values(?,?,?,?,?,?,?,?,?)`, e.ID, res.ProjectID, e.Origin, e.Locator, in.ObservedAt, e.Text, e.Completeness, e.Trust, e.RedactionCount); err != nil {
			return IngestResult{}, fmt.Errorf("store evidence: %w", err)
		}
		evidenceID = e.ID
	}
	if _, err := tx.ExecContext(ctx, `insert into events(id,project_id,session_id,event_key,kind,observed_at,received_at,payload_version,safe_payload,evidence_id,redaction_status)
		values(?,?,?,?,?,?,?,?,?,?,?)`, in.EventID, res.ProjectID, sessionID, in.EventKey, in.Kind, in.ObservedAt, in.ReceivedAt, in.AdapterVersion, in.SafePayload, evidenceID, in.RedactionStatus); err != nil {
		return IngestResult{}, fmt.Errorf("store event: %w", err)
	}
	return res, nil
}

// AddGap records that something could not be captured. The same id is stored once.
func (s *Store) AddGap(ctx context.Context, id, source, reason, occurredAt string, recoverable bool) error {
	_, err := s.db.ExecContext(ctx, `insert or ignore into capture_gaps(id,source,reason,occurred_at,recoverable) values(?,?,?,?,?)`, id, source, reason, occurredAt, recoverable)
	return err
}

// Counts is a summary of what the database holds.
type Counts struct {
	Projects int `json:"projects"`
	Sessions int `json:"sessions"`
	Events   int `json:"events"`
	Evidence int `json:"evidence"`
	Gaps     int `json:"capture_gaps"`
}

// ReadCounts opens the database read-only (no lease, no migration) and counts rows. A database
// that does not exist yet reports zeros.
func ReadCounts(ctx context.Context, path string) (Counts, error) {
	var c Counts
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	p := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(filepath.ToSlash(path))
	db, err := sql.Open("sqlite", "file:"+p+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		return c, err
	}
	defer db.Close()
	err = db.QueryRowContext(ctx, `select (select count(*) from projects), (select count(*) from sessions),
		(select count(*) from events), (select count(*) from evidence), (select count(*) from capture_gaps)`).
		Scan(&c.Projects, &c.Sessions, &c.Events, &c.Evidence, &c.Gaps)
	return c, err
}
