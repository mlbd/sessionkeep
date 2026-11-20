package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/history"
	"sessionkeep/internal/project"
	"sessionkeep/internal/redact"
	"sessionkeep/internal/store"
)

// historyReport is what `sessionkeep history import` found and, with --yes, did.
type historyReport struct {
	Imported        bool           `json:"imported"`
	Source          string         `json:"source"`
	Files           int            `json:"files"`
	UnreadableFiles int            `json:"unreadable_files"`
	Parsed          history.Stats  `json:"parsed"`
	Projects        []historyCount `json:"projects"`
	Turns           int            `json:"turns"`            // importable turns
	Sessions        int            `json:"sessions"`         // sessions they belong to
	Stored          int            `json:"stored"`           // with --yes: newly stored
	AlreadyStored   int            `json:"already_imported"` // with --yes: stored by an earlier import
	LiveSessions    int            `json:"sessions_captured_live_skipped"`
	Unregistered    int            `json:"turns_in_unregistered_folders"`
	Excluded        int            `json:"turns_in_excluded_folders"`
	PausedOrOff     int            `json:"turns_in_paused_or_text_free_projects"`
	OtherProject    int            `json:"turns_in_other_projects"`
	Redactions      int            `json:"secrets_removed"`
	Truncated       int            `json:"turns_cut_to_size_limit"`
	Folders         []historyCount `json:"unregistered_folders"`
	ParserVersion   int            `json:"parser_version"`
}

type historyCount struct {
	Name  string `json:"name"`
	Turns int    `json:"turns"`
}

// claudeDir is where Claude Code keeps its own files: --from, else the account the owner chose,
// else the one this terminal runs with (CLAUDE_CONFIG_DIR), else ~/.claude.
func claudeDir(inv invocation, st state) string {
	if v := inv.flags["from"]; v != "" {
		return v
	}
	if homeDir(inv.getenv) == "" && inv.getenv("CLAUDE_CONFIG_DIR") == "" && !st.settings.ClaudeAccount.Chosen {
		return ""
	}
	a, _ := effectiveAccount(inv, st)
	return a.ConfigDir
}

// historyCmd imports earlier Claude Code sessions for registered projects. Without --yes it
// only reports what it would import. It never registers a project, never imports an excluded,
// paused or text-free project, and skips sessions SessionKeep already captured as they happened.
func historyCmd(inv invocation, rest []string) error {
	if len(rest) != 1 || rest[0] != "import" {
		return usageErr("usage: sessionkeep history import [--project ID_OR_PATH] [--from CLAUDE_CONFIG_DIR] [--yes]\n       Without --yes it only reports what would be imported.")
	}
	if err := inv.only("project", "from", "yes"); err != nil {
		return err
	}
	apply := inv.flags["yes"] != ""
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	dir := claudeDir(inv, st)
	if dir == "" {
		return failErr("no_source", "cannot tell where Claude Code keeps its files; pass --from with its configuration folder")
	}
	files, err := filepath.Glob(filepath.Join(dir, "projects", "*", "*.jsonl"))
	if err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(dir, "projects")); err != nil || !fi.IsDir() {
		return failErr("no_source", "no Claude Code session folder at %s; pass --from with Claude Code's configuration folder", filepath.Join(dir, "projects"))
	}
	sort.Strings(files)
	rep := historyReport{Imported: apply, Source: dir, Files: len(files), ParserVersion: history.ParserVersion}
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		var only store.Project
		if key := inv.flags["project"]; key != "" {
			if only, err = resolveProject(ctx, inv, st, s, key); err != nil {
				return err
			}
		}
		all, err := s.ListProjects(ctx, st.settings.DeviceID)
		if err != nil {
			return err
		}
		names := map[string]string{}
		var roots []project.Root
		for _, p := range all {
			names[p.ID] = p.Name
			for _, r := range p.Roots {
				roots = append(roots, project.Root{Path: r, ProjectID: p.ID, Level: p.CapturePolicy, Paused: p.Paused})
			}
		}
		live, err := s.LiveSessionIDs(ctx)
		if err != nil {
			return err
		}
		// History never registers a folder, whatever the auto-add mode is.
		settings := st.settings
		settings.AutoAdd.Mode = config.ModeManual
		env := st.env(inv.getenv)
		perProject, folders := map[string]int{}, map[string]int{}
		sessions, liveSeen := map[string]bool{}, map[string]bool{}

		for _, path := range files {
			f, err := os.Open(path)
			if err != nil {
				rep.UnreadableFiles++
				continue
			}
			var pending []store.IngestInput
			stats, perr := history.Parse(f, func(t history.Turn) {
				d := project.Decide(env, settings, roots, t.Cwd)
				switch {
				case d.Reason == "excluded by the user":
					rep.Excluded++
					return
				case d.ProjectID == "":
					rep.Unregistered++
					if n, err := project.Normalize(runtime.GOOS, t.Cwd); err == nil {
						folders[n]++
					}
					return
				case d.Action != project.ActionCapture || (d.Level != config.LevelConversation && d.Level != config.LevelExtended):
					rep.PausedOrOff++
					return
				case only.ID != "" && d.ProjectID != only.ID:
					rep.OtherProject++
					return
				case live[t.SessionID]:
					liveSeen[t.SessionID] = true
					return
				}
				body, completeness := t.Text, "complete"
				if len(body) > capture.MaxTextBytes {
					cut := capture.MaxTextBytes
					for cut > 0 && !utf8.RuneStart(body[cut]) {
						cut--
					}
					body, completeness = body[:cut], "truncated"
					rep.Truncated++
				}
				body, n := redact.Text(body)
				rep.Redactions += n
				rep.Turns++
				perProject[d.ProjectID]++
				sessions[t.SessionID] = true
				if !apply {
					return
				}
				kind := "user_prompt"
				if t.Who == "assistant" {
					kind = "assistant_turn"
				}
				payload, _ := json.Marshal(map[string]any{"hook_event": "history-import", "cwd": t.Cwd, "level": d.Level, "imported": true, "parser_version": history.ParserVersion})
				status := "none"
				if n > 0 {
					status = "redacted"
				}
				pending = append(pending, store.IngestInput{
					EventID: config.NewID(), EventKey: "transcript:" + t.SessionID + ":" + t.UUID,
					DeviceID: st.settings.DeviceID, ProjectID: d.ProjectID, Agent: "claude-code", SessionExternal: t.SessionID,
					NewSessionID: config.NewID(), AdapterVersion: capture.AdapterVersion, Kind: kind,
					ObservedAt: t.At.Format(capture.TimeFormat), SafePayload: string(payload), RedactionStatus: status,
					Evidence: &store.EvidenceRow{ID: config.NewID(), Origin: "transcript", Locator: "claude-code:" + t.SessionID + ":" + t.UUID,
						Text: body, Completeness: completeness, Trust: t.Who, RedactionCount: n},
				})
			})
			f.Close()
			rep.Parsed.Add(stats)
			if perr != nil {
				rep.UnreadableFiles++
				continue // nothing from a file that could not be read to the end
			}
			now := time.Now().UTC().Format(capture.TimeFormat)
			for i := range pending {
				pending[i].ReceivedAt = now
				pending[i].EndsSession = i == len(pending)-1 // an imported session is over
				res, err := s.IngestEvent(ctx, pending[i])
				if err != nil {
					return fmt.Errorf("store imported text from %s: %w", filepath.Base(path), err)
				}
				switch res.Outcome {
				case store.Inserted:
					rep.Stored++
				case store.Replayed:
					rep.AlreadyStored++
				}
			}
		}
		rep.Sessions, rep.LiveSessions = len(sessions), len(liveSeen)
		for id, n := range perProject {
			rep.Projects = append(rep.Projects, historyCount{names[id], n})
		}
		for f, n := range folders {
			rep.Folders = append(rep.Folders, historyCount{f, n})
		}
		byTurns := func(l []historyCount) {
			sort.Slice(l, func(i, j int) bool {
				return l[i].Turns > l[j].Turns || (l[i].Turns == l[j].Turns && l[i].Name < l[j].Name)
			})
		}
		byTurns(rep.Projects)
		byTurns(rep.Folders)
		if len(rep.Folders) > 15 {
			rep.Folders = rep.Folders[:15]
		}
		return nil
	})
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Looked at %d session file(s) in %s\n", rep.Files, rep.Source)
	verb := "Would import"
	if apply {
		verb = "Imported"
	}
	fmt.Fprintf(&b, "%s: %d piece(s) of text from %d session(s)", verb, rep.Turns, rep.Sessions)
	if apply {
		fmt.Fprintf(&b, " (%d new, %d already there from an earlier import)", rep.Stored, rep.AlreadyStored)
	}
	b.WriteString("\n")
	for _, p := range rep.Projects {
		fmt.Fprintf(&b, "  %-28s %d\n", p.Name, p.Turns)
	}
	fmt.Fprintf(&b, "Left out: %d session(s) SessionKeep already captured live; %d piece(s) in excluded folders; %d in paused or text-free projects; %d in folders that are not projects",
		rep.LiveSessions, rep.Excluded, rep.PausedOrOff, rep.Unregistered)
	if rep.OtherProject > 0 {
		fmt.Fprintf(&b, "; %d in other projects", rep.OtherProject)
	}
	fmt.Fprintf(&b, ".\nSecrets removed: %d. Cut to the size limit: %d. Lines that could not be read: %d; files: %d.\n", rep.Redactions, rep.Truncated, rep.Parsed.BadLines+rep.Parsed.Oversize+rep.Parsed.MissingField, rep.UnreadableFiles)
	if len(rep.Folders) > 0 {
		b.WriteString("Folders with history that are not projects (add one with `sessionkeep project add`, then run this again):\n")
		for _, f := range rep.Folders {
			fmt.Fprintf(&b, "  %6d  %s\n", f.Turns, f.Name)
		}
	}
	b.WriteString("Not imported, ever: Claude's private reasoning, tool input and output, images, subagent sessions, and text Claude Code inserted itself.\n")
	if !apply {
		b.WriteString("NOTHING IMPORTED YET. Run the same command with --yes to import.\n")
		inv.emit(rep, b.String())
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was imported; add --yes to confirm"}
	}
	return inv.emit(rep, b.String())
}
