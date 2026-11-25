package store

import (
	"context"
	"strings"
)

// Related is one stored piece of conversation that shares words with a request.
type Related struct {
	ID         string
	Who        string
	ObservedAt string
	Snippet    string
	Received   bool // from a teammate's handoff file or team bundle, not said on this computer
}

// Related returns the project's stored prompts and replies that best match any of terms, best
// first. Text of the session with the host id skipSession is left out: that is the conversation
// the request is part of. The project filter is part of the query, never a post-filter.
func (s *Store) Related(ctx context.Context, projectID string, terms []string, skipSession string, limit int) ([]Related, error) {
	if projectID == "" || len(terms) == 0 {
		return nil, nil
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	skip := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(skipSession)
	rows, err := s.db.QueryContext(ctx, `select e.id, e.trust_level, e.observed_at, snippet(evidence_fts, 0, '', '', ' ... ', 48), e.origin = 'handoff'
		from evidence_fts join evidence e on e.seq = evidence_fts.rowid
		where evidence_fts match ? and e.project_id = ? and e.text is not null and e.trust_level in ('user','assistant')
		and e.source_locator not like ? escape '!'
		order by bm25(evidence_fts), e.seq desc limit ?`, strings.Join(quoted, " OR "), projectID, "%:"+skip+":%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Related
	for rows.Next() {
		var r Related
		if err := rows.Scan(&r.ID, &r.Who, &r.ObservedAt, &r.Snippet, &r.Received); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetRecall changes how a project's saved sessions reach a new session. The schema rejects
// anything but 'asked' and 'automatic'.
func (s *Store) SetRecall(ctx context.Context, id, mode string) error {
	return s.updateProject(ctx, `update projects set recall = ? where id = ?`, mode, id)
}
