package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Checkpoint is a note of where work stood, with the Git state at that moment.
type Checkpoint struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Note         string `json:"note"`
	GitRepo      bool   `json:"git_repo"`
	Branch       string `json:"branch,omitempty"`
	Head         string `json:"head,omitempty"`
	ChangedFiles int    `json:"changed_files"`
	DirtyDigest  string `json:"dirty_digest,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// AddCheckpoint stores a checkpoint. CreatedAt is set from now.
func (s *Store) AddCheckpoint(ctx context.Context, c Checkpoint, now time.Time) (Checkpoint, error) {
	c.CreatedAt = now.UTC().Format("2006-01-02T15:04:05.000000000Z")
	null := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	if _, err := s.db.ExecContext(ctx, `insert into checkpoints(id,project_id,note,git_repo,git_branch,git_head,changed_files,dirty_digest,created_at)
		values(?,?,?,?,?,?,?,?,?)`, c.ID, c.ProjectID, c.Note, c.GitRepo, null(c.Branch), null(c.Head), c.ChangedFiles, null(c.DirtyDigest), c.CreatedAt); err != nil {
		return Checkpoint{}, fmt.Errorf("store checkpoint: %w", err)
	}
	return c, nil
}

// Checkpoints returns a project's checkpoints, newest first, at most limit.
func (s *Store) Checkpoints(ctx context.Context, projectID string, limit int) ([]Checkpoint, error) {
	rows, err := s.db.QueryContext(ctx, `select id, project_id, note, git_repo, coalesce(git_branch,''), coalesce(git_head,''), changed_files, coalesce(dirty_digest,''), created_at
		from checkpoints where project_id = ? order by created_at desc, id limit ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Checkpoint{}
	for rows.Next() {
		var c Checkpoint
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Note, &c.GitRepo, &c.Branch, &c.Head, &c.ChangedFiles, &c.DirtyDigest, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Exchange is one recent piece of conversation text.
type Exchange struct {
	ID         string `json:"id"`
	Who        string `json:"who"`
	ObservedAt string `json:"observed_at"`
	Text       string `json:"text"`
}

// RecentExchanges returns the newest user and assistant texts of a project, oldest first, each
// cut to maxRunes characters (the full text stays available through its id).
func (s *Store) RecentExchanges(ctx context.Context, projectID string, n, maxRunes int) ([]Exchange, error) {
	rows, err := s.db.QueryContext(ctx, `select id, trust_level, observed_at, text from evidence
		where project_id = ? and text is not null and trust_level in ('user','assistant') order by seq desc limit ?`, projectID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exchange
	for rows.Next() {
		var e Exchange
		var text sql.NullString
		if err := rows.Scan(&e.ID, &e.Who, &e.ObservedAt, &text); err != nil {
			return nil, err
		}
		if r := []rune(text.String); len(r) > maxRunes {
			e.Text = string(r[:maxRunes]) + " ... [cut; full text: sessionkeep memory get " + e.ID[:min(12, len(e.ID))] + "]"
		} else {
			e.Text = text.String
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if out == nil {
		out = []Exchange{}
	}
	return out, nil
}

// ErrNoCheckpoint means the project has no checkpoint yet.
var ErrNoCheckpoint = errors.New("no checkpoint recorded for this project")

// ImportCheckpoint stores a checkpoint that was written elsewhere, keeping its own id and time.
// It reports false when a checkpoint with that id is already stored.
func (s *Store) ImportCheckpoint(ctx context.Context, c Checkpoint) (bool, error) {
	null := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	res, err := s.db.ExecContext(ctx, `insert or ignore into checkpoints(id,project_id,note,git_repo,git_branch,git_head,changed_files,dirty_digest,created_at)
		values(?,?,?,?,?,?,0,null,?)`, c.ID, c.ProjectID, c.Note, c.GitRepo, null(c.Branch), null(c.Head), c.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("store checkpoint: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
