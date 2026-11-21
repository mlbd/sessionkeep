package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sessionkeep/internal/gitstate"
	"sessionkeep/internal/store"
)

type recapProject struct {
	store.RecapProject
	Commits []gitstate.Commit `json:"commits"`
}

type recapOut struct {
	Day      string         `json:"day"`
	Label    string         `json:"label"`
	From     string         `json:"from"`
	To       string         `json:"to"`
	Projects []recapProject `json:"projects"`
	Sessions int            `json:"sessions"`
	Requests int            `json:"requests"`
}

func clockOf(ts string, loc *time.Location, withDay bool) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	if withDay {
		return t.In(loc).Format("Mon 15:04")
	}
	return t.In(loc).Format("15:04")
}

// recapCmd prints what was done in a day (or the last seven): sessions with their first request
// and last reply, checkpoints and the commits made in that time. It reads only.
func recapCmd(inv invocation, rest []string) error {
	const usageText = "usage: sessionkeep recap [today | yesterday | week | YYYY-MM-DD] [--project ID_OR_PATH | --all-projects]"
	if err := inv.only("project", "all-projects"); err != nil {
		return err
	}
	if len(rest) > 1 {
		return usageErr("%s", usageText)
	}
	day := "today"
	if len(rest) == 1 {
		day = rest[0]
	}
	loc := time.Local
	from, to, label, ok := store.RecapSpan(day, time.Now(), loc)
	if !ok {
		return usageErr("%q is not a day; %s", day, usageText)
	}
	all := inv.flags["all-projects"] != ""
	if all && inv.flags["project"] != "" {
		return usageErr("--project and --all-projects cannot be combined")
	}
	st, err := load(inv, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	s, err := store.OpenReadOnly(ctx, st.paths.DBPath)
	if err != nil {
		if errors.Is(err, store.ErrNeedsSetup) || errors.Is(err, store.ErrSchemaTooNew) {
			return &cmdError{ExitUnavailable, "database_unavailable", err.Error()}
		}
		return err
	}
	defer s.Close()
	var proj store.Project
	if !all {
		if proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
			return err
		}
	}
	found, err := s.Recap(ctx, proj.ID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	// Commits are read from each project's folder; a project with nothing else that day is
	// listed only when it has commits.
	known, err := s.ListProjects(ctx, st.settings.DeviceID)
	if err != nil {
		return err
	}
	out := recapOut{Day: day, Label: label, From: from.Format(time.RFC3339), To: to.Format(time.RFC3339), Projects: []recapProject{}}
	listed := map[string]bool{}
	commitsOf := func(p store.Project) []gitstate.Commit {
		if len(p.Roots) == 0 {
			return []gitstate.Commit{}
		}
		return gitstate.Commits(p.Roots[0], from, to, 40)
	}
	for _, rp := range found {
		listed[rp.ID] = true
		x := recapProject{RecapProject: rp, Commits: []gitstate.Commit{}}
		for _, p := range known {
			if p.ID == rp.ID {
				x.Commits = commitsOf(p)
			}
		}
		out.Projects = append(out.Projects, x)
	}
	for _, p := range known {
		if listed[p.ID] || (!all && p.ID != proj.ID) {
			continue
		}
		if c := commitsOf(p); len(c) > 0 {
			out.Projects = append(out.Projects, recapProject{RecapProject: store.RecapProject{ID: p.ID, Name: p.Name, Sessions: []store.RecapSession{}, Checkpoints: []store.Checkpoint{}}, Commits: c})
		}
	}

	week := day == "week"
	var b strings.Builder
	for _, p := range out.Projects {
		reqs, fails := 0, 0
		for _, x := range p.Sessions {
			reqs += x.Requests
			fails += x.Failures
		}
		out.Sessions += len(p.Sessions)
		out.Requests += reqs
		fmt.Fprintf(&b, "\n%s: %d session(s), %d request(s), %d failed command(s), %d checkpoint(s), %d commit(s)\n", p.Name, len(p.Sessions), reqs, fails, len(p.Checkpoints), len(p.Commits))
		for _, x := range p.Sessions {
			fmt.Fprintf(&b, "  %s-%s  session %.8s, %d request(s)\n", clockOf(x.From, loc, week), clockOf(x.To, loc, false), x.ID, x.Requests)
			if x.First != "" {
				fmt.Fprintf(&b, "      began with: %s\n", x.First)
			}
			if x.Summary != "" {
				fmt.Fprintf(&b, "      summary (written by a model): %s\n", strings.Join(strings.Fields(x.Summary), " "))
			} else if x.LastReply != "" {
				fmt.Fprintf(&b, "      last reply: %s\n", x.LastReply)
			}
			if len(x.Files) > 0 {
				more := ""
				if x.MoreFiles > 0 {
					more = fmt.Sprintf(" and %d more", x.MoreFiles)
				}
				fmt.Fprintf(&b, "      files: %s%s\n", strings.Join(x.Files, ", "), more)
			}
		}
		for _, c := range p.Checkpoints {
			fmt.Fprintf(&b, "  %s  checkpoint: %s\n", clockOf(c.CreatedAt, loc, week), strings.Join(strings.Fields(c.Note), " "))
		}
		for _, c := range p.Commits {
			fmt.Fprintf(&b, "  %s  commit %s  %s\n", clockOf(c.At, loc, week), c.ID, c.Subject)
		}
	}
	scopeName := "all projects"
	if !all {
		scopeName = fmt.Sprintf("project %q", proj.Name)
	}
	head := fmt.Sprintf("Recap for %s, in %s (times are local): %d session(s), %d request(s).\n", label, scopeName, out.Sessions, out.Requests)
	if len(out.Projects) == 0 {
		head += "Nothing was saved in that time.\n"
	}
	return inv.emit(out, head+b.String())
}
