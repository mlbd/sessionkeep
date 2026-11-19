package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sessionkeep/internal/archive"
	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/fsx"
	"sessionkeep/internal/ingest"
	"sessionkeep/internal/instruct"
	"sessionkeep/internal/lease"
	"sessionkeep/internal/store"
)

// exportCmd writes everything SessionKeep holds into one file. Waiting events are stored first, so
// the export is as complete as it can be; the copy is taken while holding the lease.
func exportCmd(inv invocation, rest []string) error {
	if err := inv.only(); err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: sessionkeep export FILE")
	}
	dst, err := filepath.Abs(rest[0])
	if err != nil {
		return usageErr("cannot resolve path %q: %v", rest[0], err)
	}
	if _, err := os.Stat(dst); err == nil {
		return failErr("exists", "%s already exists; choose another name (exports are never overwritten)", dst)
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var man archive.Manifest
	var totals store.Totals
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		if _, err := ingest.Drain(ctx, ingest.Deps{Paths: st.paths, Settings: st.settings, Store: s, GOOS: runtime.GOOS, Now: time.Now, NewID: config.NewID}); err != nil {
			return fmt.Errorf("store waiting events before exporting: %w", err)
		}
		tmp := filepath.Join(st.paths.DataDir, ".export-"+config.NewID()+".db")
		defer os.Remove(tmp)
		if err := s.ExportTo(ctx, tmp); err != nil {
			return err
		}
		schema, t, err := store.VerifyFile(ctx, tmp)
		if err != nil {
			return fmt.Errorf("the copy failed verification and was not exported: %w", err)
		}
		totals = t
		raw, _ := json.Marshal(t)
		man, err = archive.Write(dst, tmp, st.paths.SettingsPath(), archive.Manifest{
			CreatedAt: time.Now().UTC().Format(time.RFC3339), Version: buildinfo.Get().Version, SchemaVersion: schema, SQLite: s.Engine().Version, Totals: raw,
			NotIncluded: []string{
				fmt.Sprintf("events captured while this export was running (%d waiting now)", pendingEvents(st.paths)),
				"quarantined spool files",
				"Claude Code's own session files, which SessionKeep does not own",
				"the SessionKeep program and the Claude Code plugin",
			},
		})
		return err
	})
	if err != nil {
		return err
	}
	fi, _ := os.Stat(dst)
	text := fmt.Sprintf("exported to %s (%d bytes)\n%d project(s), %d session(s), %d event(s), %d text record(s), %d instruction(s), %d checkpoint(s)\nThe file is not encrypted. It contains your conversation text; keep it somewhere private.\nCheck it at any time with: sessionkeep import \"%s\" --dry-run\n",
		dst, fi.Size(), totals.Projects, totals.Sessions, totals.Events, totals.Evidence, totals.Instructions, totals.Checkpoints, dst)
	return inv.emit(map[string]any{"file": dst, "bytes": fi.Size(), "manifest": man}, text)
}

// importCmd restores an export into a data directory that has no database yet. With --dry-run
// it verifies the file and reports what would happen, changing nothing.
func importCmd(inv invocation, rest []string) error {
	if err := inv.only("dry-run"); err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("usage: sessionkeep import FILE [--dry-run]")
	}
	dry := inv.flags["dry-run"] != ""
	src, err := filepath.Abs(rest[0])
	if err != nil {
		return usageErr("cannot resolve path %q: %v", rest[0], err)
	}
	p, err := config.Resolve(runtime.GOOS, inv.getenv)
	if err != nil {
		return &cmdError{ExitUnavailable, "no_data_dir", err.Error()}
	}
	work, err := os.MkdirTemp("", "sessionkeep-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	man, err := archive.Extract(src, work)
	if err != nil {
		if errors.Is(err, archive.ErrInvalid) || errors.Is(err, os.ErrNotExist) {
			return failErr("invalid_export", "%v", err)
		}
		return err
	}
	ctx := context.Background()
	schema, totals, err := store.VerifyFile(ctx, filepath.Join(work, archive.DBName))
	if err != nil {
		return failErr("invalid_export", "%v", err)
	}
	var claimed store.Totals
	json.Unmarshal(man.Totals, &claimed)
	problems := []string{}
	if claimed != totals {
		problems = append(problems, "the contents do not match the counts recorded in the export")
	}
	if schema != man.SchemaVersion {
		problems = append(problems, "the database version does not match the one recorded in the export")
	}
	if schema > store.SchemaVersion() {
		problems = append(problems, fmt.Sprintf("the export was made by a newer SessionKeep (data version %d; this program understands up to %d)", schema, store.SchemaVersion()))
	}
	existing := false
	for _, f := range []string{p.DBPath, p.DBPath + "-wal"} {
		if _, err := os.Stat(f); err == nil {
			existing = true
		}
	}
	if existing {
		problems = append(problems, "this data directory already has a database; import only restores into an empty one, so nothing is overwritten or merged")
	}
	_, hasSettings := os.Stat(p.SettingsPath())
	settingsNote := "settings from the export will be restored"
	if hasSettings == nil {
		settingsNote = "this data directory already has settings; they are kept, and the export's settings are ignored"
	}
	report := map[string]any{"file": src, "created_at": man.CreatedAt, "made_by": man.Version, "data_version": schema, "totals": totals,
		"target": p.DataDir, "problems": problems, "would_restore": len(problems) == 0, "restored": false, "settings": settingsNote}
	text := fmt.Sprintf("export made %s by sessionkeep %s, data version %d\ncontains: %d project(s), %d session(s), %d event(s), %d text record(s), %d instruction(s), %d checkpoint(s)\nfile integrity: every entry matches its recorded digest; database integrity check passed\ntarget: %s\n%s\n",
		man.CreatedAt, man.Version, schema, totals.Projects, totals.Sessions, totals.Events, totals.Evidence, totals.Instructions, totals.Checkpoints, p.DataDir, settingsNote)
	for _, pr := range problems {
		text += "PROBLEM: " + pr + "\n"
	}
	if len(problems) > 0 {
		inv.emit(report, text+"Nothing was changed.\n")
		return &cmdError{ExitFailure, "import_refused", "the import cannot proceed: " + problems[0]}
	}
	if dry {
		return inv.emit(report, text+"DRY RUN: this import would succeed. Nothing was changed. Run again without --dry-run to restore.\n")
	}

	if err := config.Ensure(p); err != nil {
		return &cmdError{ExitUnavailable, "data_dir_unusable", err.Error()}
	}
	l, err := lease.Acquire(p.LeasePath)
	if errors.Is(err, lease.ErrHeld) {
		return &cmdError{ExitUnavailable, "busy", "another SessionKeep process is using this data directory"}
	}
	if err != nil {
		return err
	}
	if err := copyFile(filepath.Join(work, archive.DBName), p.DBPath); err != nil {
		l.Release()
		return fmt.Errorf("place the database: %w", err)
	}
	if hasSettings != nil {
		if raw, err := os.ReadFile(filepath.Join(work, archive.SettingsName)); err == nil {
			if err := fsx.WriteAtomic(p.SettingsPath(), raw); err != nil {
				l.Release()
				return err
			}
		}
	}
	l.Release()
	// Opening migrates an older export forward; then the hook-readable caches are rebuilt.
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var after store.Totals
	if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
		after, err = s.Totals(ctx)
		return err
	}); err != nil {
		return fmt.Errorf("the database was placed but could not be opened: %w", err)
	}
	if after != totals {
		return fmt.Errorf("after restoring, the counts differ from the export (%+v vs %+v)", after, totals)
	}
	report["restored"] = true
	return inv.emit(report, text+"RESTORED. Counts after restoring match the export.\nRegistered folders are restored as they were on the machine that made the export.\n")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}

const forgetLimits = "Not affected: exports you made earlier, and Claude Code's own session files. If you made exports that contain this, delete those files yourself.\n"

// forgetCmd deletes stored data. Without --yes it shows exactly what would be removed.
func forgetCmd(inv invocation, rest []string) error {
	usageText := `usage: sessionkeep forget record ID [--project ID_OR_PATH | --all-projects] [--yes]
       sessionkeep forget captured --project ID_OR_PATH [--yes]     (all captured text of a project; keeps the project, its instructions and checkpoints)
       sessionkeep forget project ID_OR_PATH [--yes]                (the project and everything that belongs to it)`
	if len(rest) == 0 {
		return usageErr("%s", usageText)
	}
	kind, args := rest[0], rest[1:]
	switch {
	case kind == "record" && len(args) == 1:
		if err := inv.only("project", "all-projects", "yes"); err != nil {
			return err
		}
	case kind == "captured" && len(args) == 0 && inv.flags["project"] != "":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
	case kind == "project" && len(args) == 1:
		if err := inv.only("yes"); err != nil {
			return err
		}
	default:
		return usageErr("%s", usageText)
	}
	all := inv.flags["all-projects"] != ""
	if all && inv.flags["project"] != "" {
		return usageErr("--project and --all-projects cannot be combined")
	}
	apply := inv.flags["yes"] != ""
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var removed store.Removal
	var what string
	var proj store.Project
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		switch kind {
		case "record":
			if !all {
				if proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
					return err
				}
			}
			rec, err := s.GetEvidence(ctx, args[0], proj.ID, all)
			if err != nil {
				return err
			}
			what = fmt.Sprintf("the record %.12s in project %q, said by %s at %s:\n----\n%s\n----", rec.ID, rec.ProjectName, rec.Who, rec.ObservedAt, firstRunes(rec.Text, 300))
			removed, err = s.ForgetRecord(ctx, rec.ID, apply)
		case "captured":
			if proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
				return err
			}
			what = fmt.Sprintf("all captured text of project %q (%.8s); the project, its instructions and its checkpoints stay", proj.Name, proj.ID)
			removed, err = s.ForgetCaptured(ctx, proj.ID, apply)
		default:
			if proj, err = resolveProject(ctx, inv, st, s, args[0]); err != nil {
				return err
			}
			what = fmt.Sprintf("project %q (%.8s) and everything that belongs to it", proj.Name, proj.ID)
			removed, err = s.ForgetProject(ctx, proj.ID, apply)
		}
		if err != nil || !apply {
			return err
		}
		if kind != "record" {
			dropSpooled(st.paths, proj.ID)
		}
		if kind == "project" {
			instruct.RemoveCache(st.paths, proj.ID)
		}
		return s.Compact(ctx)
	})
	if errors.Is(err, store.ErrNoEvidence) {
		return failErr("not_found", "no stored record matches %q in that scope", args[0])
	}
	if err != nil {
		return err
	}
	counts := fmt.Sprintf("%d event(s), %d text record(s), %d session(s), %d instruction(s), %d checkpoint(s), %d project(s)",
		removed.Events, removed.Evidence, removed.Sessions, removed.Instructions, removed.Checkpoints, removed.Projects)
	if !apply {
		inv.emit(map[string]any{"deleted": false, "would_remove": removed, "target": what},
			fmt.Sprintf("NOT DELETED YET. This would permanently delete %s\nThat is: %s.\nIt cannot be undone. %sRun the same command with --yes to delete.\n", what, counts, forgetLimits))
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was deleted; add --yes to confirm"}
	}
	return inv.emit(map[string]any{"deleted": true, "removed": removed},
		fmt.Sprintf("deleted: %s.\nThe search index no longer contains it and the database file was rewritten without it.\n%s", counts, forgetLimits))
}

func firstRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + " ..."
	}
	return s
}

// dropSpooled removes events of a project that are still waiting in the spool, so deleted data
// is not stored again by the next drain.
func dropSpooled(p config.Paths, projectID string) {
	entries, err := os.ReadDir(p.SpoolDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".evt") {
			continue
		}
		path := filepath.Join(p.SpoolDir, e.Name())
		raw, err := os.ReadFile(path)
		var ev capture.Event
		if err == nil && json.Unmarshal(raw, &ev) == nil && ev.ProjectID == projectID {
			os.Remove(path)
		}
	}
}
