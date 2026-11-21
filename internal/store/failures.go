package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Failure is one failed tool call whose error text was stored.
type Failure struct {
	Seq        int64  `json:"-"`
	SessionID  string `json:"session_id"`
	External   string `json:"-"` // the host's own session id
	ObservedAt string `json:"observed_at"`
	RecordID   string `json:"record_id"`
	Text       string `json:"text"`
	Tool       string `json:"tool"`
	Command    string `json:"command,omitempty"`
}

// FailureStamp changes whenever the project's stored failed tool calls change.
func (s *Store) FailureStamp(ctx context.Context, projectID string) (string, error) {
	var n, last int64
	err := s.db.QueryRowContext(ctx, `select count(*), coalesce(max(rowid),0) from events where project_id = ? and kind = 'tool_failure'`, projectID).Scan(&n, &last)
	return fmt.Sprintf("%d:%d", n, last), err
}

// Failures returns the project's failed tool calls that have stored error text, newest first.
func (s *Store) Failures(ctx context.Context, projectID string, limit int) ([]Failure, error) {
	rows, err := s.db.QueryContext(ctx, `select v.rowid, v.session_id, s.external_session_id, v.observed_at, e.id, e.text,
		coalesce(json_extract(v.safe_payload, '$.tool_name'),''), coalesce(json_extract(v.safe_payload, '$.tool_input.command'),'')
		from events v
		join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
		join sessions s on s.id = v.session_id and s.project_id = v.project_id
		where v.project_id = ? and v.kind = 'tool_failure' and e.text is not null
		order by v.rowid desc limit ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Failure
	for rows.Next() {
		var f Failure
		if err := rows.Scan(&f.Seq, &f.SessionID, &f.External, &f.ObservedAt, &f.RecordID, &f.Text, &f.Tool, &f.Command); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReplyAfter returns the first stored reply of the assistant that follows a failure in its
// session: what was said once the turn that hit the error had ended. Empty when there is none.
func (s *Store) ReplyAfter(ctx context.Context, projectID string, f Failure) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx, `select e.text from events v
		join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
		where v.project_id = ? and v.session_id = ? and v.kind = 'assistant_turn' and v.rowid > ? and e.text is not null
		order by v.rowid limit 1`, projectID, f.SessionID, f.Seq).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return text, err
}
