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

// Totals counts everything a user might want to know is in (or gone from) the database.
type Totals struct {
	Projects     int `json:"projects"`
	Sessions     int `json:"sessions"`
	Events       int `json:"events"`
	Evidence     int `json:"text_records"`
	Instructions int `json:"instructions"`
	Versions     int `json:"instruction_versions"`
	Checkpoints  int `json:"checkpoints"`
	Gaps         int `json:"capture_gaps"`
}

const totalsQuery = `select (select count(*) from projects), (select count(*) from sessions), (select count(*) from events),
	(select count(*) from evidence), (select count(*) from instructions), (select count(*) from instruction_versions),
	(select count(*) from checkpoints), (select count(*) from capture_gaps)`

// Totals counts all rows.
func (s *Store) Totals(ctx context.Context) (Totals, error) {
	var t Totals
	err := s.db.QueryRowContext(ctx, totalsQuery).Scan(&t.Projects, &t.Sessions, &t.Events, &t.Evidence, &t.Instructions, &t.Versions, &t.Checkpoints, &t.Gaps)
	return t, err
}

// ExportTo writes a complete, consistent, compacted copy of the database to path, which must not
// exist. It runs on the writer's connection, so it sees one moment in time and is not restarted
// by other writes (Phase 0 finding B1). Deleted content is not carried into the copy.
func (s *Store) ExportTo(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	if _, err := s.db.ExecContext(ctx, `vacuum into ?`, path); err != nil {
		os.Remove(path)
		return fmt.Errorf("copy database: %w", err)
	}
	return nil
}

// VerifyFile opens a database file read-only, checks its integrity and returns its schema
// version and totals. It does not need the file to match this binary's schema.
func VerifyFile(ctx context.Context, path string) (schema int, t Totals, err error) {
	p := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(filepath.ToSlash(path))
	db, err := sql.Open("sqlite", "file:"+p+"?mode=ro&immutable=1")
	if err != nil {
		return 0, t, err
	}
	defer db.Close()
	var ic string
	if err := db.QueryRowContext(ctx, "pragma integrity_check").Scan(&ic); err != nil || ic != "ok" {
		return 0, t, fmt.Errorf("the database copy is damaged (integrity check: %q, %v)", ic, err)
	}
	var fk int
	if err := db.QueryRowContext(ctx, "select count(*) from pragma_foreign_key_check").Scan(&fk); err != nil || fk != 0 {
		return 0, t, fmt.Errorf("the database copy has %d broken reference(s) (%v)", fk, err)
	}
	if err := db.QueryRowContext(ctx, "select coalesce(max(version),0) from schema_migrations").Scan(&schema); err != nil {
		return 0, t, fmt.Errorf("not a SessionKeep database: %w", err)
	}
	// Older schemas lack some tables; count what exists.
	has := func(table string) bool {
		var n int
		db.QueryRowContext(ctx, "select count(*) from sqlite_master where type='table' and name=?", table).Scan(&n)
		return n == 1
	}
	count := func(table string) int {
		var n int
		if has(table) {
			db.QueryRowContext(ctx, "select count(*) from "+table).Scan(&n)
		}
		return n
	}
	t = Totals{Projects: count("projects"), Sessions: count("sessions"), Events: count("events"), Evidence: count("evidence"),
		Instructions: count("instructions"), Versions: count("instruction_versions"), Checkpoints: count("checkpoints"), Gaps: count("capture_gaps")}
	return schema, t, nil
}

// SchemaVersion is the number of migrations this binary knows.
func SchemaVersion() int {
	known, _ := loadMigrations(embedded, "migrations")
	return len(known)
}

// Removal says what a forget operation deletes (or would delete).
type Removal struct {
	Events       int `json:"events"`
	Evidence     int `json:"text_records"`
	Sessions     int `json:"sessions"`
	Instructions int `json:"instructions"`
	Checkpoints  int `json:"checkpoints"`
	Projects     int `json:"projects"`
}

func (s *Store) count(ctx context.Context, tx *sql.Tx, q string, args ...any) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// forget runs fn in a transaction; with apply false it rolls back, so the returned counts are an
// exact preview of what applying would remove.
func (s *Store) forget(ctx context.Context, apply bool, audit string, fn func(tx *sql.Tx) (Removal, error)) (Removal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Removal{}, err
	}
	defer tx.Rollback()
	r, err := fn(tx)
	if err != nil || !apply {
		return r, err
	}
	// A record that something was deleted, with counts and no content.
	if _, err := tx.ExecContext(ctx, `insert into capture_gaps(id, source, reason, occurred_at, count, recoverable)
		values(lower(hex(randomblob(16))), 'user:forget', ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'), ?, 0)`, audit, max(1, r.Events+r.Evidence+r.Instructions+r.Checkpoints+r.Projects)); err != nil {
		return r, err
	}
	return r, tx.Commit()
}

// ForgetRecord deletes one stored text and the event that carried it.
func (s *Store) ForgetRecord(ctx context.Context, evidenceID string, apply bool) (Removal, error) {
	return s.forget(ctx, apply, "record_deleted", func(tx *sql.Tx) (Removal, error) {
		var r Removal
		res, err := tx.ExecContext(ctx, `delete from events where evidence_id = ?`, evidenceID)
		if err != nil {
			return r, err
		}
		n, _ := res.RowsAffected()
		r.Events = int(n)
		res, err = tx.ExecContext(ctx, `delete from evidence where id = ?`, evidenceID)
		if err != nil {
			return r, err
		}
		if n, _ = res.RowsAffected(); n == 0 {
			return r, ErrNoEvidence
		}
		r.Evidence = int(n)
		return r, nil
	})
}

// ForgetCaptured deletes everything captured from sessions of a project — events, stored text,
// sessions — and keeps the project, its instructions and its checkpoints.
func (s *Store) ForgetCaptured(ctx context.Context, projectID string, apply bool) (Removal, error) {
	return s.forget(ctx, apply, "captured_text_deleted", func(tx *sql.Tx) (Removal, error) {
		var r Removal
		var err error
		for _, step := range []struct {
			into *int
			q    string
		}{
			{&r.Events, `delete from events where project_id = ?`},
			{&r.Evidence, `delete from evidence where project_id = ?`},
			{&r.Sessions, `delete from sessions where project_id = ?`},
		} {
			res, e := tx.ExecContext(ctx, step.q, projectID)
			if e != nil {
				return r, e
			}
			n, _ := res.RowsAffected()
			*step.into = int(n)
		}
		if _, err = tx.ExecContext(ctx, `delete from capture_gaps where project_id = ?`, projectID); err != nil {
			return r, err
		}
		if n, e := s.count(ctx, tx, `select count(*) from projects where id = ?`, projectID); e != nil || n == 0 {
			return r, errors.Join(e, ErrNoProject)
		}
		return r, nil
	})
}

// ForgetProject deletes a project and everything that belongs to it: captured text, sessions,
// its instructions and their history, its checkpoints and its registered folders.
func (s *Store) ForgetProject(ctx context.Context, projectID string, apply bool) (Removal, error) {
	return s.forget(ctx, apply, "project_deleted", func(tx *sql.Tx) (Removal, error) {
		var r Removal
		var err error
		for _, c := range []struct {
			into *int
			q    string
		}{
			{&r.Events, `select count(*) from events where project_id = ?`},
			{&r.Evidence, `select count(*) from evidence where project_id = ?`},
			{&r.Sessions, `select count(*) from sessions where project_id = ?`},
			{&r.Instructions, `select count(*) from instructions where project_id = ?`},
			{&r.Checkpoints, `select count(*) from checkpoints where project_id = ?`},
		} {
			if *c.into, err = s.count(ctx, tx, c.q, projectID); err != nil {
				return r, err
			}
		}
		// Events first: they reference evidence without a cascade.
		if _, err := tx.ExecContext(ctx, `delete from events where project_id = ?`, projectID); err != nil {
			return r, err
		}
		res, err := tx.ExecContext(ctx, `delete from projects where id = ?`, projectID)
		if err != nil {
			return r, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return r, ErrNoProject
		}
		r.Projects = 1
		return r, nil
	})
}

// Compact makes deleted content physically absent from the database file. Three steps are needed:
// the search index is rebuilt from the remaining text (deleting a row only marks its words as
// removed inside the index; the old words stay in its internal segments until then), the file is
// rewritten without unused pages, and the write-ahead log is emptied.
func (s *Store) Compact(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `insert into evidence_fts(evidence_fts) values('rebuild')`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `vacuum`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `pragma wal_checkpoint(TRUNCATE)`)
	return err
}
