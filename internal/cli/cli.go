// Package cli implements the sessionkeep command line. It takes its streams and environment as
// arguments so every command is testable without a process.
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

	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/project"
	"sessionkeep/internal/store"
)

// Exit statuses (docs/09-INTERFACE_CONTRACTS.md). The hook entry point, added in a later slice,
// deliberately returns ExitOK for every recoverable problem.
const (
	ExitOK           = 0
	ExitFailure      = 1
	ExitUsage        = 2
	ExitUnavailable  = 3
	ExitNeedsConsent = 4
)

const usage = `sessionkeep — local-first developer memory (pre-release; events are captured to the spool but not yet ingested)

Usage:
  sessionkeep setup [--auto-add manual|ask|auto] [--level LEVEL]
                                   Create the data directory and choose what happens in folders
                                   SessionKeep has not seen: manual (nothing), ask (a reminder; the
                                   default) or auto (register at LEVEL). No flags: show settings.
  sessionkeep project add PATH --capture LEVEL [--name NAME]
                                   Register a folder. LEVEL is required; there is no default.
  sessionkeep project list
  sessionkeep project pause|resume ID_OR_PATH
  sessionkeep project capture ID_OR_PATH LEVEL
  sessionkeep project which [PATH]     Show what SessionKeep would do in a folder (reads only)
  sessionkeep exclude add|remove PATH  Never capture, register or remind at or below PATH
  sessionkeep exclude list
  sessionkeep hook EVENT               Claude Code hook entry point (reads the hook JSON on stdin;
                                   always exits 0; not meant to be run by hand)
  sessionkeep doctor                   Read-only checks: build, data location, SQLite engine gate
  sessionkeep paths                    Show where SessionKeep keeps its data (creates nothing)
  sessionkeep version | help

LEVEL is one of: off, metadata, conversation, extended.
Add --json to any command for machine-readable output.
Only the commands listed here exist. Environment: SESSIONKEEP_DATA_DIR overrides the data
directory (absolute path; intended for test sandboxes).
`

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// cmdError carries an exit status and a stable machine code.
type cmdError struct {
	exit int
	code string
	msg  string
}

func (e *cmdError) Error() string { return e.msg }

func usageErr(format string, a ...any) error {
	return &cmdError{ExitUsage, "usage", fmt.Sprintf(format, a...)}
}
func failErr(code, format string, a ...any) error {
	return &cmdError{ExitFailure, code, fmt.Sprintf(format, a...)}
}

type invocation struct {
	pos    []string
	flags  map[string]string
	json   bool
	stdout io.Writer
	getenv func(string) string
}

// Run executes one command and returns the process exit status. Results go to stdout,
// diagnostics to stderr; with --json, errors are also a JSON envelope on stdout.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if args[0] == "hook" {
		return hook(args[1:], stdin, stdout, getenv)
	}
	inv, err := parse(args, "auto-add", "level", "capture", "name")
	if err == nil {
		inv.stdout, inv.getenv = stdout, getenv
		err = dispatch(inv)
	}
	if err == nil {
		return ExitOK
	}
	var ce *cmdError
	if !errors.As(err, &ce) {
		ce = &cmdError{ExitFailure, "failed", err.Error()}
	}
	fmt.Fprintln(stderr, "sessionkeep:", ce.msg)
	if inv.json {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.Encode(map[string]any{"error": errorBody{Code: ce.code, Message: ce.msg}})
	}
	return ce.exit
}

// parse separates positionals from flags. Value flags accept "--name value" and "--name=value".
func parse(args []string, valueFlags ...string) (invocation, error) {
	inv := invocation{flags: map[string]string{}}
	for _, a := range args {
		if a == "--json" {
			inv.json = true
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" {
			continue
		}
		if !strings.HasPrefix(a, "--") {
			inv.pos = append(inv.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(a[2:], "=")
		known := false
		for _, f := range valueFlags {
			known = known || f == name
		}
		if !known {
			return inv, usageErr("unknown option --%s; run `sessionkeep help`", name)
		}
		if !hasVal {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return inv, usageErr("option --%s needs a value", name)
			}
			i++
			val = args[i]
		}
		if _, dup := inv.flags[name]; dup {
			return inv, usageErr("option --%s given more than once", name)
		}
		inv.flags[name] = val
	}
	return inv, nil
}

// only rejects flags a command does not take, so a typo cannot be silently ignored.
func (inv invocation) only(allowed ...string) error {
	for f := range inv.flags {
		ok := false
		for _, a := range allowed {
			ok = ok || a == f
		}
		if !ok {
			return usageErr("option --%s does not apply to `%s`", f, strings.Join(inv.pos, " "))
		}
	}
	return nil
}

func (inv invocation) emit(data any, text string) error {
	if inv.json {
		enc := json.NewEncoder(inv.stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]any{"data": data})
	}
	_, err := fmt.Fprint(inv.stdout, text)
	return err
}

func dispatch(inv invocation) error {
	if len(inv.pos) == 0 {
		return usageErr("no command given; run `sessionkeep help`")
	}
	cmd, rest := inv.pos[0], inv.pos[1:]
	switch cmd {
	case "help":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("help takes no arguments")
		}
		_, err := fmt.Fprint(inv.stdout, usage)
		return err
	case "version":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("version takes no arguments")
		}
		bi := buildinfo.Get()
		text := fmt.Sprintf("sessionkeep %s (%s/%s, %s)", bi.Version, bi.OS, bi.Arch, bi.Go)
		if bi.Commit != "" {
			text += fmt.Sprintf(" commit %.12s", bi.Commit)
			if bi.Modified {
				text += "+modified"
			}
		}
		return inv.emit(bi, text+"\n")
	case "paths":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("paths takes no arguments")
		}
		p, err := config.Resolve(runtime.GOOS, inv.getenv)
		if err != nil {
			return &cmdError{ExitUnavailable, "no_data_dir", err.Error()}
		}
		return inv.emit(p, fmt.Sprintf("data directory: %s (%s)\ndatabase:       %s\nspool:          %s\ncache:          %s\n", p.DataDir, p.Source, p.DBPath, p.SpoolDir, p.CacheDir))
	case "doctor":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("doctor takes no arguments")
		}
		return doctor(inv)
	case "setup":
		if len(rest) != 0 {
			return usageErr("setup takes options only")
		}
		return setup(inv)
	case "exclude":
		return exclude(inv, rest)
	case "project":
		return projectCmd(inv, rest)
	}
	return usageErr("unknown command %q; run `sessionkeep help`", cmd)
}

// hook is the Claude Code hook entry point: `sessionkeep hook <Event>` with the host's JSON on stdin.
// It always exits 0 and prints nothing but a valid hook response, whatever goes wrong: a memory
// tool must never block or disturb the coding session (ADR-0002).
func hook(args []string, stdin io.Reader, stdout io.Writer, getenv func(string) string) int {
	if len(args) != 1 {
		return ExitOK
	}
	done := make(chan capture.Result, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- capture.Result{}
			}
		}()
		raw, err := io.ReadAll(io.LimitReader(stdin, 8<<20))
		if err != nil {
			done <- capture.Result{}
			return
		}
		p, err := config.Resolve(runtime.GOOS, getenv)
		if err != nil {
			done <- capture.Result{}
			return
		}
		st := state{paths: p}
		done <- capture.Handle(capture.Request{HookEvent: args[0], Raw: raw, Paths: p, Env: st.env(getenv), Now: time.Now(), NewID: config.NewID})
	}()
	select {
	case res := <-done:
		stdout.Write(res.Stdout)
	case <-time.After(capture.Deadline):
		// Give up silently; an unfinished temp file in the spool is ignored by the ingester.
	}
	return ExitOK
}

// state is what the mutating commands share: resolved paths, settings with a device id, and the
// environment facts the decision needs.
type state struct {
	paths    config.Paths
	settings config.Settings
}

// load resolves paths and settings. With create it also makes the data directory and assigns the
// device id on first use, so `project add` works without a separate `setup`.
func load(inv invocation, create bool) (state, error) {
	p, err := config.Resolve(runtime.GOOS, inv.getenv)
	if err != nil {
		return state{}, &cmdError{ExitUnavailable, "no_data_dir", err.Error()}
	}
	if create {
		if err := config.Ensure(p); err != nil {
			return state{}, &cmdError{ExitUnavailable, "data_dir_unusable", err.Error()}
		}
	}
	s, err := config.LoadSettings(p)
	if err != nil {
		return state{}, &cmdError{ExitUnavailable, "settings_unreadable", err.Error() + "; nothing will be captured until the settings file is repaired"}
	}
	if create && s.DeviceID == "" {
		s.DeviceID = config.NewID()
		if err := config.SaveSettings(p, s); err != nil {
			return state{}, err
		}
	}
	return state{paths: p, settings: s}, nil
}

func (st state) env(getenv func(string) string) project.Env {
	home := getenv("HOME")
	if runtime.GOOS == "windows" {
		home = getenv("USERPROFILE")
	}
	return project.Env{GOOS: runtime.GOOS, Home: home, RepoRoot: project.FindRepoRoot}
}

// absNorm turns a user-supplied path into the normalized comparison form.
func absNorm(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", usageErr("cannot resolve path %q: %v", p, err)
	}
	n, err := project.Normalize(runtime.GOOS, abs)
	if err != nil {
		return "", usageErr("%v", err)
	}
	return n, nil
}

// withStore opens the database, runs fn, then rewrites the roots cache the hook path reads.
//
// Until the ingest lease exists (slice 1A-5) these short administrative writes rely on SQLite's
// own locking; ADR-0003's "only the lease holder writes" is applied to them in that slice.
func withStore(st state, fn func(ctx context.Context, s *store.Store) error) error {
	ctx := context.Background()
	s, err := store.Open(ctx, st.paths.DBPath)
	if err != nil {
		if errors.Is(err, store.ErrIncompatibleEngine) {
			return &cmdError{ExitUnavailable, "incompatible", err.Error()}
		}
		return err
	}
	defer s.Close()
	if err := fn(ctx, s); err != nil {
		return err
	}
	all, err := s.ListProjects(ctx, st.settings.DeviceID)
	if err != nil {
		return err
	}
	var roots []project.Root
	for _, p := range all {
		for _, r := range p.Roots {
			roots = append(roots, project.Root{Path: r, ProjectID: p.ID, Level: p.CapturePolicy, Paused: p.Paused})
		}
	}
	if err := project.WriteRootsCache(st.paths, roots, time.Now()); err != nil {
		return fmt.Errorf("the change was saved but the roots cache could not be rewritten (run `sessionkeep setup` to refresh it): %w", err)
	}
	return nil
}

func setup(inv invocation) error {
	if err := inv.only("auto-add", "level"); err != nil {
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	mode, hasMode := inv.flags["auto-add"]
	level, hasLevel := inv.flags["level"]
	if hasLevel && !hasMode {
		return usageErr("--level is only used together with --auto-add auto")
	}
	if hasMode {
		st.settings.AutoAdd = config.AutoAdd{Mode: mode, Level: level}
		if mode == config.ModeAuto && !hasLevel {
			return usageErr("--auto-add auto needs --level metadata|conversation|extended; SessionKeep will not pick a level for you")
		}
		if err := st.settings.Validate(); err != nil {
			return usageErr("%v", err)
		}
		if err := config.SaveSettings(st.paths, st.settings); err != nil {
			return err
		}
	}
	if err := withStore(st, func(context.Context, *store.Store) error { return nil }); err != nil {
		return err
	}
	text := fmt.Sprintf("data directory: %s\nunregistered folders: %s", st.paths.DataDir, describeMode(st.settings.AutoAdd))
	text += fmt.Sprintf("\nexclusions: %d\n", len(st.settings.Exclusions))
	return inv.emit(map[string]any{"data_dir": st.paths.DataDir, "auto_add": st.settings.AutoAdd, "exclusions": st.settings.Exclusions, "device_id": st.settings.DeviceID}, text)
}

func describeMode(a config.AutoAdd) string {
	switch a.Mode {
	case config.ModeManual:
		return "manual — nothing is saved until you run `sessionkeep project add`"
	case config.ModeAuto:
		return fmt.Sprintf("auto — registered automatically at level %q (except excluded folders)", a.Level)
	}
	return "ask — nothing is saved; you get a reminder to run `sessionkeep project add`"
}

func exclude(inv invocation, rest []string) error {
	if err := inv.only(); err != nil {
		return err
	}
	if len(rest) == 1 && rest[0] == "list" {
		st, err := load(inv, false)
		if err != nil {
			return err
		}
		text := strings.Join(st.settings.Exclusions, "\n")
		if text == "" {
			text = "no exclusions"
		}
		return inv.emit(st.settings.Exclusions, text+"\n")
	}
	if len(rest) != 2 || (rest[0] != "add" && rest[0] != "remove") {
		return usageErr("usage: sessionkeep exclude add|remove PATH, or sessionkeep exclude list")
	}
	path, err := absNorm(rest[1])
	if err != nil {
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	kept := st.settings.Exclusions[:0:0]
	found := false
	for _, e := range st.settings.Exclusions {
		if e == path {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	switch {
	case rest[0] == "add":
		kept = append(kept, path)
	case !found:
		return failErr("not_excluded", "%s is not in the exclusion list", path)
	}
	st.settings.Exclusions = kept
	if err := config.SaveSettings(st.paths, st.settings); err != nil {
		return err
	}
	verb := map[string]string{"add": "excluded", "remove": "no longer excluded"}[rest[0]]
	return inv.emit(st.settings.Exclusions, fmt.Sprintf("%s: %s\n", verb, path))
}

func projectCmd(inv invocation, rest []string) error {
	if len(rest) == 0 {
		return usageErr("usage: sessionkeep project add|list|pause|resume|capture|which …")
	}
	sub, args := rest[0], rest[1:]
	switch sub {
	case "which":
		if err := inv.only(); err != nil {
			return err
		}
		if len(args) > 1 {
			return usageErr("usage: sessionkeep project which [PATH]")
		}
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return usageErr("cannot resolve path %q: %v", dir, err)
		}
		st, err := load(inv, false)
		if err != nil {
			return err
		}
		roots, err := project.ReadRootsCache(st.paths)
		if err != nil {
			return &cmdError{ExitUnavailable, "roots_cache_unreadable", err.Error() + "; nothing will be captured until `sessionkeep setup` rewrites it"}
		}
		d := project.Decide(st.env(inv.getenv), st.settings, roots, abs)
		text := fmt.Sprintf("%s: %s", d.Action, d.Reason)
		if d.ProjectID != "" {
			text += fmt.Sprintf("\nproject: %s", d.ProjectID)
		}
		if d.Level != "" {
			text += fmt.Sprintf("\nlevel:   %s", d.Level)
		}
		if d.Root != "" {
			text += fmt.Sprintf("\nroot:    %s", d.Root)
		}
		return inv.emit(d, text+"\n")

	case "list":
		if err := inv.only(); err != nil {
			return err
		}
		if len(args) != 0 {
			return usageErr("project list takes no arguments")
		}
		st, err := load(inv, true)
		if err != nil {
			return err
		}
		var all []store.Project
		if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
			all, err = s.ListProjects(ctx, st.settings.DeviceID)
			return err
		}); err != nil {
			return err
		}
		var b strings.Builder
		for _, p := range all {
			state := p.CapturePolicy
			if p.Paused {
				state += " (paused)"
			}
			fmt.Fprintf(&b, "%.8s  %-22s %-10s %s\n", p.ID, state, p.AddedBy, strings.Join(p.Roots, ", "))
		}
		if len(all) == 0 {
			b.WriteString("no projects registered\n")
		}
		return inv.emit(all, b.String())

	case "add":
		if err := inv.only("capture", "name"); err != nil {
			return err
		}
		if len(args) != 1 {
			return usageErr("usage: sessionkeep project add PATH --capture off|metadata|conversation|extended [--name NAME]")
		}
		level, ok := inv.flags["capture"]
		if !ok {
			return &cmdError{ExitNeedsConsent, "capture_level_required", "choose what to save for this project: --capture off|metadata|conversation|extended (there is no default)"}
		}
		if !config.ValidLevel(level) {
			return usageErr("--capture must be off, metadata, conversation or extended (got %q)", level)
		}
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return usageErr("cannot resolve path %q: %v", args[0], err)
		}
		if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
			return failErr("not_a_directory", "%s is not an existing directory", abs)
		}
		root, err := absNorm(abs)
		if err != nil {
			return err
		}
		st, err := load(inv, true)
		if err != nil {
			return err
		}
		if d := project.Decide(st.env(inv.getenv), st.settings, nil, abs); d.Reason == "excluded by the user" {
			return failErr("excluded", "%s is inside an excluded folder; run `sessionkeep exclude remove` first if you really want to register it", root)
		}
		name := inv.flags["name"]
		if name == "" {
			name = filepath.Base(abs)
		}
		var added store.Project
		if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
			added, err = s.AddProject(ctx, store.NewProject{ID: config.NewID(), RootID: config.NewID(), Name: name, Root: root, Level: level, DeviceID: st.settings.DeviceID, AddedBy: "manual", Now: time.Now()})
			return err
		}); err != nil {
			if errors.Is(err, store.ErrRootRegistered) {
				return failErr("already_registered", "%v", err)
			}
			return err
		}
		return inv.emit(added, fmt.Sprintf("registered %s\nproject: %s\nlevel:   %s\n", root, added.ID, level))

	case "pause", "resume", "capture":
		if err := inv.only(); err != nil {
			return err
		}
		want := 1
		if sub == "capture" {
			want = 2
		}
		if len(args) != want {
			return usageErr("usage: sessionkeep project pause|resume ID_OR_PATH, or sessionkeep project capture ID_OR_PATH LEVEL")
		}
		if sub == "capture" && !config.ValidLevel(args[1]) {
			return usageErr("LEVEL must be off, metadata, conversation or extended (got %q)", args[1])
		}
		st, err := load(inv, true)
		if err != nil {
			return err
		}
		var after store.Project
		if err := withStore(st, func(ctx context.Context, s *store.Store) error {
			p, err := s.FindProject(ctx, st.settings.DeviceID, args[0])
			if errors.Is(err, store.ErrNoProject) {
				if n, nerr := absNorm(args[0]); nerr == nil {
					p, err = s.FindProject(ctx, st.settings.DeviceID, n)
				}
			}
			if err != nil {
				return err
			}
			switch sub {
			case "pause":
				err = s.SetPaused(ctx, p.ID, true, time.Now())
			case "resume":
				err = s.SetPaused(ctx, p.ID, false, time.Now())
			default:
				err = s.SetCaptureLevel(ctx, p.ID, args[1])
			}
			if err != nil {
				return err
			}
			after, err = s.FindProject(ctx, st.settings.DeviceID, p.ID)
			return err
		}); err != nil {
			switch {
			case errors.Is(err, store.ErrNoProject):
				return failErr("no_such_project", "no project matches %q (use an id from `sessionkeep project list` or a registered folder)", args[0])
			case errors.Is(err, store.ErrAmbiguousProject):
				return failErr("ambiguous_project", "%q matches more than one project; use more of the id", args[0])
			}
			return err
		}
		state := after.CapturePolicy
		if after.Paused {
			state += " (paused)"
		}
		return inv.emit(after, fmt.Sprintf("project %.8s: %s\n", after.ID, state))
	}
	return usageErr("unknown project command %q; run `sessionkeep help`", sub)
}

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | failed
	Detail string `json:"detail"`
}

// notChecked is stated in every doctor report so a short list of passes is not read as full health.
const notChecked = "database file, spool, ingest lease, capture hooks, plugin registration"

// doctor runs read-only checks. It opens no database file and creates nothing; later slices add
// checks for the spool, lease, database and plugin registration as those parts are built.
func doctor(inv invocation) error {
	bi := buildinfo.Get()
	checks := []check{{"build", "ok", fmt.Sprintf("sessionkeep %s %s/%s %s", bi.Version, bi.OS, bi.Arch, bi.Go)}}

	p, err := config.Resolve(runtime.GOOS, inv.getenv)
	if err != nil {
		checks = append(checks, check{"data_location", "failed", err.Error()})
	} else {
		checks = append(checks, check{"data_location", "ok", fmt.Sprintf("%s (%s)", p.DataDir, p.Source)})
		if s, err := config.LoadSettings(p); err != nil {
			checks = append(checks, check{"settings", "failed", err.Error()})
		} else {
			checks = append(checks, check{"settings", "ok", fmt.Sprintf("unregistered folders: %s; %d exclusion(s)", s.AutoAdd.Mode, len(s.Exclusions))})
		}
	}

	if e, err := store.ProbeEngine(context.Background()); err != nil {
		checks = append(checks, check{"sqlite_engine", "failed", err.Error()})
	} else if err := store.CheckEngine(e); err != nil {
		checks = append(checks, check{"sqlite_engine", "failed", err.Error()})
	} else {
		checks = append(checks, check{"sqlite_engine", "ok", fmt.Sprintf("SQLite %s, FTS5 present, at or above %d.%d.%d", e.Version, store.MinSQLite[0], store.MinSQLite[1], store.MinSQLite[2])})
	}

	healthy := true
	var b strings.Builder
	for _, c := range checks {
		healthy = healthy && c.Status == "ok"
		fmt.Fprintf(&b, "%-14s %-7s %s\n", c.Name, c.Status, c.Detail)
	}
	fmt.Fprintf(&b, "not checked (not built yet): %s\n", notChecked)
	if err := inv.emit(map[string]any{"healthy": healthy, "checks": checks, "not_checked": notChecked}, b.String()); err != nil {
		return err
	}
	if !healthy {
		return &cmdError{ExitFailure, "unhealthy", "one or more checks failed"}
	}
	return nil
}
