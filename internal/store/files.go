package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Touch is one stored tool call that named a file.
type Touch struct {
	Seq       int64
	SessionID string
	At        string
	Tool      string
	File      string
}

// ChangeTools are the tools that change a file; the others that name one only read it.
var ChangeTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// FileKey puts a path into the form paths are compared in: lower case, forward slashes, no
// leading "./" and no trailing slash.
func FileKey(path string) string {
	p := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), `\`, "/"))
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.Trim(strings.TrimPrefix(p, "./"), "/")
}

// Touches returns the project's stored tool calls on a file, oldest first. tail is a FileKey: a
// stored path matches when it is that path or ends with "/" + tail, so a path inside the project
// finds the file wherever the project folder was. since and until (RFC 3339, either may be
// empty) bound the time.
func (s *Store) Touches(ctx context.Context, projectID, tail, since, until string, limit int) ([]Touch, error) {
	if tail == "" {
		return nil, nil
	}
	esc := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(tail)
	const file = `lower(replace(coalesce(json_extract(v.safe_payload,'$.tool_input.file_path'), json_extract(v.safe_payload,'$.tool_input.notebook_path'), ''), '\', '/'))`
	q := `select v.rowid, v.session_id, v.observed_at, coalesce(json_extract(v.safe_payload,'$.tool_name'),''), ` + file + `
		from events v where v.project_id = ? and v.kind = 'tool_use' and (` + file + ` = ? or ` + file + ` like ? escape '!')`
	args := []any{projectID, tail, "%/" + esc}
	if since != "" {
		q += ` and julianday(v.observed_at) >= julianday(?)`
		args = append(args, since)
	}
	if until != "" {
		q += ` and julianday(v.observed_at) < julianday(?)`
		args = append(args, until)
	}
	rows, err := s.db.QueryContext(ctx, q+` order by v.rowid limit ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Touch
	for rows.Next() {
		var t Touch
		if err := rows.Scan(&t.Seq, &t.SessionID, &t.At, &t.Tool, &t.File); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RequestBefore returns the last thing the user said in a session before the event seq: the
// request that led to it. Empty when nothing was stored.
func (s *Store) RequestBefore(ctx context.Context, projectID, sessionID string, seq int64) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx, `select e.text from events v
		join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
		where v.project_id = ? and v.session_id = ? and v.rowid < ? and e.trust_level = 'user' and e.text is not null
		order by v.rowid desc limit 1`, projectID, sessionID, seq).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return text, err
}
