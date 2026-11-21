package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

// RecapSession is what one session did inside a span of time.
type RecapSession struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Requests  int      `json:"requests"`
	Failures  int      `json:"failed_commands"`
	First     string   `json:"first_request,omitempty"`
	LastReply string   `json:"last_reply,omitempty"`
	Files     []string `json:"files,omitempty"`
	MoreFiles int      `json:"more_files,omitempty"`
	Summary   string   `json:"summary,omitempty"` // written by a model, when summaries are on
}

// RecapProject is one project's part of a recap.
type RecapProject struct {
	ID          string         `json:"project_id"`
	Name        string         `json:"name"`
	Sessions    []RecapSession `json:"sessions"`
	Checkpoints []Checkpoint   `json:"checkpoints"`
}

const (
	recapTextRunes = 300
	recapFiles     = 8
)

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + " ..."
	}
	return s
}

// Recap gathers what happened between since and until (RFC 3339, until exclusive) in one project,
// or in every project when projectID is empty. Projects come in name order, sessions oldest first.
func (s *Store) Recap(ctx context.Context, projectID, since, until string) ([]RecapProject, error) {
	q := `select v.session_id, v.project_id, p.name, v.kind, v.observed_at, coalesce(e.trust_level,''), coalesce(substr(e.text,1,2000),''),
		coalesce(json_extract(v.safe_payload,'$.tool_input.file_path'),'')
		from events v join projects p on p.id = v.project_id
		left join evidence e on e.id = v.evidence_id and e.project_id = v.project_id
		where julianday(v.observed_at) >= julianday(?) and julianday(v.observed_at) < julianday(?)`
	args := []any{since, until}
	if projectID != "" {
		q += ` and v.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, q+` order by julianday(v.observed_at), v.rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := map[string]*RecapProject{}
	sessions := map[string]*RecapSession{}
	owner := map[string]string{}
	seenFile := map[string]bool{}
	var order []string
	for rows.Next() {
		var sid, pid, name, kind, at, who, text, file string
		if err := rows.Scan(&sid, &pid, &name, &kind, &at, &who, &text, &file); err != nil {
			return nil, err
		}
		if projects[pid] == nil {
			projects[pid] = &RecapProject{ID: pid, Name: name, Sessions: []RecapSession{}, Checkpoints: []Checkpoint{}}
		}
		x := sessions[sid]
		if x == nil {
			x = &RecapSession{ID: sid, From: at}
			sessions[sid], owner[sid] = x, pid
			order = append(order, sid)
		}
		x.To = at
		switch {
		case kind == "tool_failure":
			x.Failures++
		case who == "user" && text != "":
			x.Requests++
			if x.First == "" {
				x.First = oneLine(text, recapTextRunes)
			}
		case who == "assistant" && kind != "compaction" && text != "":
			x.LastReply = oneLine(text, recapTextRunes)
		}
		if file != "" && !seenFile[sid+"\x00"+file] {
			seenFile[sid+"\x00"+file] = true
			if len(x.Files) < recapFiles {
				x.Files = append(x.Files, file)
			} else {
				x.MoreFiles++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, sid := range order {
		x, pid := sessions[sid], owner[sid]
		if sum, ok, err := s.SummaryOf(ctx, pid, sid); err != nil {
			return nil, err
		} else if ok {
			x.Summary = sum.Text
		}
		projects[pid].Sessions = append(projects[pid].Sessions, *x)
	}

	cq := `select c.id, c.project_id, p.name, c.note, c.git_repo, coalesce(c.git_branch,''), coalesce(c.git_head,''), c.changed_files, c.created_at
		from checkpoints c join projects p on p.id = c.project_id
		where julianday(c.created_at) >= julianday(?) and julianday(c.created_at) < julianday(?)`
	cargs := []any{since, until}
	if projectID != "" {
		cq += ` and c.project_id = ?`
		cargs = append(cargs, projectID)
	}
	crows, err := s.db.QueryContext(ctx, cq+` order by c.created_at, c.id`, cargs...)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var c Checkpoint
		var name string
		if err := crows.Scan(&c.ID, &c.ProjectID, &name, &c.Note, &c.GitRepo, &c.Branch, &c.Head, &c.ChangedFiles, &c.CreatedAt); err != nil {
			return nil, err
		}
		if projects[c.ProjectID] == nil {
			projects[c.ProjectID] = &RecapProject{ID: c.ProjectID, Name: name, Sessions: []RecapSession{}, Checkpoints: []Checkpoint{}}
		}
		projects[c.ProjectID].Checkpoints = append(projects[c.ProjectID].Checkpoints, c)
	}
	if err := crows.Err(); err != nil {
		return nil, err
	}

	out := make([]RecapProject, 0, len(projects))
	for _, p := range projects {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name); a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// RecapSpan turns "today", "yesterday", "week" or a date like 2026-11-20 into a span of whole
// days in loc, with a label for people. ok is false for anything else.
func RecapSpan(word string, now time.Time, loc *time.Location) (from, to time.Time, label string, ok bool) {
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	switch word {
	case "", "today":
		return today, today.AddDate(0, 0, 1), "today, " + today.Format("Monday 2 January 2006"), true
	case "yesterday":
		y := today.AddDate(0, 0, -1)
		return y, today, "yesterday, " + y.Format("Monday 2 January 2006"), true
	case "week":
		w := today.AddDate(0, 0, -6)
		return w, today.AddDate(0, 0, 1), "the last 7 days, " + w.Format("2 January") + " to " + today.Format("2 January 2006"), true
	}
	d, err := time.ParseInLocation("2006-01-02", word, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", false
	}
	return d, d.AddDate(0, 0, 1), d.Format("Monday 2 January 2006"), true
}
