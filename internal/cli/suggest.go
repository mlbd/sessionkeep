package cli

import (
	"context"
	"fmt"
	"strings"

	"sessionkeep/internal/store"
	"sessionkeep/internal/suggest"
)

// suggestCmd lists requests the user made in nearly the same words in three or more sessions
// and that no instruction covers yet. It reads only and saves nothing: recording one is the
// user's own `sessionkeep remember`.
func suggestCmd(inv invocation, rest []string) error {
	if err := inv.only("project", "all-projects"); err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErr("usage: sessionkeep suggest [--project ID_OR_PATH | --all-projects]")
	}
	all := inv.flags["all-projects"] != ""
	if all && inv.flags["project"] != "" {
		return usageErr("--project and --all-projects cannot be combined")
	}
	st, s, err := openForRead(inv)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx := context.Background()
	var proj store.Project
	if !all {
		if proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
			return err
		}
	}
	rows, err := s.UserTexts(ctx, proj.ID, 20000)
	if err != nil {
		return err
	}
	said := make([]suggest.Said, 0, len(rows))
	for _, r := range rows {
		said = append(said, suggest.Said{Session: r.SessionID, Project: r.ProjectID, At: r.At, Text: r.Text})
	}
	// Instructions that already apply: the project's own and the global ones, or all of them.
	var existing []string
	if all {
		list, err := s.ListInstructions(ctx, false)
		if err != nil {
			return err
		}
		for _, i := range list {
			existing = append(existing, i.Text)
		}
	} else {
		list, err := s.ActiveInstructions(ctx, proj.ID)
		if err != nil {
			return err
		}
		for _, i := range list {
			existing = append(existing, i.Text)
		}
	}
	found := suggest.Find(said, existing, 3)
	scopeName, scopeFlag := "all projects", "--scope global"
	if !all {
		scopeName, scopeFlag = fmt.Sprintf("project %q", proj.Name), "--scope project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Requests you made in nearly the same words in 3 or more sessions of %s, and that no instruction covers (%d found, from %d request(s) read).\nThey are found by comparing words, not by a model: read each one. Nothing is saved by this command.\n", scopeName, len(found), len(said))
	for _, x := range found {
		fmt.Fprintf(&b, "\n%d sessions, last %.10s:  %s\n    to make it a standing instruction: sessionkeep remember %q %s\n", x.Sessions, x.LastAt, x.Text, x.Text, scopeFlag)
	}
	return inv.emit(map[string]any{"suggestions": found, "requests_read": len(said)}, b.String())
}
