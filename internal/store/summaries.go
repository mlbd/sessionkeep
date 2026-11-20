package store

import (
	"context"
	"database/sql"
	"errors"
)

// Summary is a model-written summary of one session. It is derived text, not evidence.
type Summary struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	SessionID     string `json:"session_id"`
	Text          string `json:"text"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	PromptVersion int    `json:"prompt_version"`
	SourceCount   int    `json:"source_records"`
	SourceUsed    int    `json:"source_records_given_to_model"`
	Redactions    int    `json:"redactions"`
	CreatedAt     string `json:"created_at"`
}

// SummaryOf returns the summary of a session, if there is one.
func (s *Store) SummaryOf(ctx context.Context, projectID, sessionID string) (Summary, bool, error) {
	var m Summary
	err := s.db.QueryRowContext(ctx, `select id, project_id, session_id, text, provider, model, prompt_version, source_count, source_used, redaction_count, created_at
		from summaries where project_id = ? and session_id = ?`, projectID, sessionID).
		Scan(&m.ID, &m.ProjectID, &m.SessionID, &m.Text, &m.Provider, &m.Model, &m.PromptVersion, &m.SourceCount, &m.SourceUsed, &m.Redactions, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Summary{}, false, nil
	}
	return m, err == nil, err
}

// ErrSourceChanged means the session gained or lost text while the summary was being written.
var ErrSourceChanged = errors.New("the session changed while it was being summarised")

// PutSummary stores a summary, replacing an older one for the same session. It refuses when the
// session no longer holds exactly SourceCount text records, so a summary can never describe
// text that was deleted, or miss text that arrived, while the model was working.
func (s *Store) PutSummary(ctx context.Context, m Summary) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var now int
	if err := tx.QueryRowContext(ctx, `select count(*) from events where project_id = ? and session_id = ? and evidence_id is not null`, m.ProjectID, m.SessionID).Scan(&now); err != nil {
		return err
	}
	if now != m.SourceCount {
		return ErrSourceChanged
	}
	if _, err := tx.ExecContext(ctx, `delete from summaries where project_id = ? and session_id = ?`, m.ProjectID, m.SessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into summaries(id, project_id, session_id, text, provider, model, prompt_version, source_count, source_used, redaction_count, created_at)
		values(?,?,?,?,?,?,?,?,?,?,?)`, m.ID, m.ProjectID, m.SessionID, m.Text, m.Provider, m.Model, m.PromptVersion, m.SourceCount, m.SourceUsed, m.Redactions, m.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// SummaryCandidate is a session that has no summary of its current text.
type SummaryCandidate struct {
	SessionID   string `json:"session_id"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Texts       int    `json:"text_records"`
	LastAt      string `json:"last_activity_at"`
}

// SessionsToSummarize lists sessions that are over (ended, or silent since idleBefore), hold at
// least minTexts text records and minChars characters of text, belong to a project that is saving conversation text and is not
// paused, and have no summary of exactly that text. Newest first. An empty projectID means every
// project.
func (s *Store) SessionsToSummarize(ctx context.Context, projectID, idleBefore string, minTexts, minChars, limit int) ([]SummaryCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `select sid, pid, name, texts, last_at from (
		select s.id as sid, s.project_id as pid, p.name, s.state,
			(select count(*) from events v where v.session_id = s.id and v.project_id = s.project_id and v.evidence_id is not null) as texts,
			coalesce((select max(observed_at) from events v where v.session_id = s.id and v.project_id = s.project_id), s.started_at) as last_at,
			coalesce((select sum(length(e.text)) from events v join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
				where v.session_id = s.id and v.project_id = s.project_id), 0) as chars
		from sessions s join projects p on p.id = s.project_id
		where (?1 = '' or s.project_id = ?1) and p.capture_policy in ('conversation','extended') and p.paused_at is null and p.archived_at is null)
		where texts >= ?2 and chars >= ?5 and (state <> 'open' or last_at < ?3)
			and not exists (select 1 from summaries m where m.session_id = sid and m.project_id = pid and m.source_count = texts)
		order by last_at desc, sid limit ?4`, projectID, minTexts, idleBefore, limit, minChars)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SummaryCandidate{}
	for rows.Next() {
		var c SummaryCandidate
		if err := rows.Scan(&c.SessionID, &c.ProjectID, &c.ProjectName, &c.Texts, &c.LastAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SummaryCount returns how many summaries exist.
func (s *Store) SummaryCount(ctx context.Context) (n int, err error) {
	err = s.db.QueryRowContext(ctx, `select count(*) from summaries`).Scan(&n)
	return
}

// DeleteSummaries deletes every summary of a project, or of every project when projectID is empty.
func (s *Store) DeleteSummaries(ctx context.Context, projectID string) (int, error) {
	res, err := s.db.ExecContext(ctx, `delete from summaries where ?1 = '' or project_id = ?1`, projectID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SummaryHit is a summary that matched a search.
type SummaryHit struct {
	SessionID string `json:"session_id"`
	ProjectID string `json:"project_id"`
	CreatedAt string `json:"created_at"`
	Model     string `json:"model"`
	Snippet   string `json:"snippet"`
}

// SearchSummaries finds summaries containing all the words of the query, or, when none does,
// some of them (partial is then true). The scope rule is the same as for Search.
func (s *Store) SearchSummaries(ctx context.Context, projectID string, allProjects bool, query string, limit int) (hits []SummaryHit, partial bool, err error) {
	if (projectID == "") == !allProjects {
		return nil, false, errors.New("a search needs exactly one scope: a project or the explicit all-projects flag")
	}
	run := func(match string) ([]SummaryHit, error) {
		rows, err := s.db.QueryContext(ctx, `select m.session_id, m.project_id, m.created_at, m.model, snippet(summaries_fts, 0, '**', '**', ' ... ', 24)
			from summaries_fts join summaries m on m.seq = summaries_fts.rowid
			where summaries_fts match ?1 and (?2 = '' or m.project_id = ?2) order by bm25(summaries_fts), m.seq desc limit ?3`, match, projectID, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []SummaryHit{}
		for rows.Next() {
			var h SummaryHit
			if err := rows.Scan(&h.SessionID, &h.ProjectID, &h.CreatedAt, &h.Model, &h.Snippet); err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		return out, rows.Err()
	}
	match := ftsQuery(query)
	if match == "" {
		return []SummaryHit{}, false, nil
	}
	if hits, err = run(match); err != nil || len(hits) > 0 {
		return hits, false, err
	}
	if anyMatch := ftsAnyQuery(query); anyMatch != "" {
		hits, err = run(anyMatch)
		return hits, len(hits) > 0, err
	}
	return hits, false, nil
}
