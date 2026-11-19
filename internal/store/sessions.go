package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// SessionInfo summarises one captured session.
type SessionInfo struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	External    string `json:"host_session_id"`
	Agent       string `json:"agent"`
	StartedAt   string `json:"started_at"`
	LastAt      string `json:"last_activity_at"`
	State       string `json:"state"`
	Events      int    `json:"events"`
	Texts       int    `json:"text_records"`
	FirstPrompt string `json:"first_prompt"`
}

// ErrNoSession means no session matches within the allowed scope.
var ErrNoSession = errors.New("no such session in this scope")

const sessionSelect = `select s.id, s.project_id, p.name, s.external_session_id, s.agent, s.started_at,
	coalesce((select max(observed_at) from events v where v.session_id = s.id and v.project_id = s.project_id), s.started_at),
	s.state,
	(select count(*) from events v where v.session_id = s.id and v.project_id = s.project_id),
	(select count(*) from events v where v.session_id = s.id and v.project_id = s.project_id and v.evidence_id is not null),
	coalesce((select substr(e.text, 1, 160) from events v join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
		where v.session_id = s.id and v.project_id = s.project_id and e.trust_level = 'user' order by v.observed_at limit 1), '')
	from sessions s join projects p on p.id = s.project_id`

func scanSessions(rows *sql.Rows) ([]SessionInfo, error) {
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.ProjectName, &s.External, &s.Agent, &s.StartedAt, &s.LastAt, &s.State, &s.Events, &s.Texts, &s.FirstPrompt); err != nil {
			return nil, err
		}
		s.FirstPrompt = strings.Join(strings.Fields(s.FirstPrompt), " ")
		out = append(out, s)
	}
	return out, rows.Err()
}

// Sessions lists sessions, newest first. An empty projectID lists every project's sessions and
// is only for the owner's own views.
func (s *Store) Sessions(ctx context.Context, projectID string, limit int) ([]SessionInfo, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q, args := sessionSelect, []any{}
	if projectID != "" {
		q += ` where s.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, q+` order by s.started_at desc, s.id limit ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	return scanSessions(rows)
}

// Turn is one stored piece of a session, in order.
type Turn struct {
	ID         string `json:"id,omitempty"` // record id when there is stored text
	Kind       string `json:"kind"`
	Who        string `json:"who,omitempty"`
	ObservedAt string `json:"observed_at"`
	Text       string `json:"text,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Redactions int    `json:"redactions,omitempty"`
}

// SessionTurns returns a session and everything stored for it, oldest first. The session must
// belong to projectID unless allProjects is set.
func (s *Store) SessionTurns(ctx context.Context, idOrPrefix, projectID string, allProjects bool) (SessionInfo, []Turn, error) {
	if (projectID == "") == !allProjects {
		return SessionInfo{}, nil, errors.New("a lookup needs exactly one scope: a project or the explicit all-projects flag")
	}
	if len(idOrPrefix) < 8 || strings.ContainsAny(idOrPrefix, `%_\`) {
		return SessionInfo{}, nil, ErrNoSession
	}
	q := sessionSelect + ` where s.id >= ? and s.id < ?`
	args := []any{idOrPrefix, idOrPrefix + "\U0010FFFF"}
	if !allProjects {
		q += ` and s.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, q+` order by s.id limit 2`, args...)
	if err != nil {
		return SessionInfo{}, nil, err
	}
	found, err := scanSessions(rows)
	if err != nil {
		return SessionInfo{}, nil, err
	}
	if len(found) != 1 {
		return SessionInfo{}, nil, ErrNoSession
	}
	info := found[0]
	trows, err := s.db.QueryContext(ctx, `select coalesce(e.id,''), v.kind, coalesce(e.trust_level,''), v.observed_at, coalesce(e.text,''),
		coalesce(json_extract(v.safe_payload, '$.tool_name'),''), coalesce(e.redaction_count,0)
		from events v left join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
		where v.session_id = ? and v.project_id = ? order by v.observed_at, v.id`, info.ID, info.ProjectID)
	if err != nil {
		return info, nil, err
	}
	defer trows.Close()
	turns := []Turn{}
	for trows.Next() {
		var t Turn
		if err := trows.Scan(&t.ID, &t.Kind, &t.Who, &t.ObservedAt, &t.Text, &t.Tool, &t.Redactions); err != nil {
			return info, nil, err
		}
		turns = append(turns, t)
	}
	return info, turns, trows.Err()
}
