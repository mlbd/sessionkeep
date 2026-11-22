package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrRootRegistered means the directory already belongs to a project on this device.
	ErrRootRegistered = errors.New("directory is already registered")
	// ErrNoProject means no project matches the given id or root path.
	ErrNoProject = errors.New("no such project")
	// ErrAmbiguousProject means an id prefix matched more than one project.
	ErrAmbiguousProject = errors.New("more than one project matches")
)

// Project is a registered project with its roots on this device.
type Project struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	CapturePolicy string   `json:"capture_level"`
	Paused        bool     `json:"paused"`
	AddedBy       string   `json:"added_by"`
	CreatedAt     string   `json:"created_at"`
	Roots         []string `json:"roots"`
	// Recall is how saved sessions reach a new session: "asked" or "automatic".
	Recall string `json:"recall"`
}

// NewProject is the input to AddProject. Root must already be normalized; Level must be valid.
type NewProject struct {
	ID, RootID      string
	Name, Root      string
	Level, DeviceID string
	AddedBy         string // manual | auto
	Now             time.Time
}

// AddProject creates a project and its first root in one transaction.
func (s *Store) AddProject(ctx context.Context, n NewProject) (Project, error) {
	ts := n.Now.UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `insert into projects(id,name,capture_policy,created_at,added_by) values(?,?,?,?,?)`,
		n.ID, n.Name, n.Level, ts, n.AddedBy); err != nil {
		return Project{}, fmt.Errorf("add project: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into project_roots(id,project_id,device_id,normalized_path,last_seen) values(?,?,?,?,?)`,
		n.RootID, n.ID, n.DeviceID, n.Root, ts); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Project{}, fmt.Errorf("%w: %s", ErrRootRegistered, n.Root)
		}
		return Project{}, fmt.Errorf("add project root: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return Project{ID: n.ID, Name: n.Name, CapturePolicy: n.Level, AddedBy: n.AddedBy, CreatedAt: ts, Roots: []string{n.Root}, Recall: "asked"}, nil
}

// ListProjects returns all non-archived projects with their roots on the given device, oldest first.
func (s *Store) ListProjects(ctx context.Context, deviceID string) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `select p.id, p.name, p.capture_policy, p.paused_at is not null, p.added_by, p.created_at, p.recall, r.normalized_path
		from projects p left join project_roots r on r.project_id = p.id and r.device_id = ?
		where p.archived_at is null order by p.created_at, p.id, r.normalized_path`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		var root sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.CapturePolicy, &p.Paused, &p.AddedBy, &p.CreatedAt, &p.Recall, &root); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].ID != p.ID {
			p.Roots = []string{}
			out = append(out, p)
		}
		if root.Valid {
			out[len(out)-1].Roots = append(out[len(out)-1].Roots, root.String)
		}
	}
	return out, rows.Err()
}

// FindProject resolves a project by exact id, by unique id prefix (at least 4 characters), or by
// an exactly matching normalized root path on this device.
func (s *Store) FindProject(ctx context.Context, deviceID, idOrRoot string) (Project, error) {
	all, err := s.ListProjects(ctx, deviceID)
	if err != nil {
		return Project{}, err
	}
	var hits []Project
	for _, p := range all {
		if p.ID == idOrRoot {
			return p, nil
		}
		match := len(idOrRoot) >= 4 && strings.HasPrefix(p.ID, idOrRoot)
		for _, r := range p.Roots {
			if r == idOrRoot {
				match = true
			}
		}
		if match {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 0:
		return Project{}, ErrNoProject
	case 1:
		return hits[0], nil
	}
	return Project{}, ErrAmbiguousProject
}

// MoveRoot points one of a project's folders on this device at another place, and renames the
// project when name is not empty. Both paths must already be normalized. Everything saved for
// the project stays with it.
func (s *Store) MoveRoot(ctx context.Context, projectID, deviceID, from, to, name string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `update project_roots set normalized_path = ?, last_seen = ? where project_id = ? and device_id = ? and normalized_path = ?`,
		to, now.UTC().Format(time.RFC3339), projectID, deviceID, from)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("%w: %s", ErrRootRegistered, to)
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoProject
	}
	if name != "" {
		if _, err := tx.ExecContext(ctx, `update projects set name = ? where id = ?`, name, projectID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetCaptureLevel changes a project's capture level. The schema rejects unknown levels.
func (s *Store) SetCaptureLevel(ctx context.Context, id, level string) error {
	return s.updateProject(ctx, `update projects set capture_policy = ? where id = ?`, level, id)
}

// SetPaused pauses or resumes a project without changing its capture level.
func (s *Store) SetPaused(ctx context.Context, id string, paused bool, now time.Time) error {
	if paused {
		return s.updateProject(ctx, `update projects set paused_at = coalesce(paused_at, ?) where id = ?`, now.UTC().Format(time.RFC3339), id)
	}
	return s.updateProject(ctx, `update projects set paused_at = null where id = ?`, id)
}

func (s *Store) updateProject(ctx context.Context, q string, args ...any) error {
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoProject
	}
	return nil
}
