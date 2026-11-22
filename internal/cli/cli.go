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
	"strconv"
	"strings"
	"time"

	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/gitstate"
	"sessionkeep/internal/ingest"
	"sessionkeep/internal/instruct"
	"sessionkeep/internal/lease"
	"sessionkeep/internal/plugin"
	"sessionkeep/internal/project"
	"sessionkeep/internal/redact"
	"sessionkeep/internal/store"
	"sessionkeep/internal/summarize"
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

const usage = `sessionkeep - local-first developer memory for Claude Code

Usage:
  sessionkeep setup [--auto-add manual|ask|auto] [--level LEVEL]
                                   Create the data directory and choose what happens in folders
                                   SessionKeep has not seen: manual (nothing), ask (a reminder; the
                                   default) or auto (register at LEVEL). No flags: show settings.
  sessionkeep project add PATH --capture LEVEL [--name NAME]
                                   Register a folder. LEVEL is required; there is no default.
  sessionkeep project list
  sessionkeep project pause|resume ID_OR_PATH
  sessionkeep project recall ID_OR_PATH asked|automatic
                                   How saved sessions reach a new session. asked (the default):
                                   only when Claude looks them up. automatic: a note of the
                                   latest sessions at every start, and with every request the
                                   stored pieces that share its words
  sessionkeep project capture ID_OR_PATH LEVEL
  sessionkeep project move ID_OR_OLD_PATH NEW_PATH [--name NAME]
                                   After you renamed or moved a project's folder: point the
                                   project at the new place, so its saved sessions stay with it
  sessionkeep project which [PATH]     Show what SessionKeep would do in a folder (reads only)
  sessionkeep folders [PATH]           List the folders inside PATH (default: your home folder), for
                                   choosing a project folder; reads only, never lists files
  sessionkeep exclude add|remove PATH  Never capture, register or remind at or below PATH
  sessionkeep exclude list
  sessionkeep memory search "QUERY" [--project ID_OR_PATH | --all-projects] [--exact]
                 [--who user|assistant|tool] [--since DATE] [--until DATE] [--limit N] [--cursor C]
                                   Search stored conversation text. Scope is the current
                                   project unless you say otherwise.
  sessionkeep memory get ID            Show one stored record in full, with where it came from
  sessionkeep session list | show ID   Browse captured sessions and read one in full
  sessionkeep recap [today|yesterday|week|YYYY-MM-DD] [--project ID_OR_PATH | --all-projects]
                                   What was done in a day: sessions with their first request
                                   and last reply, checkpoints and commits. Reads only
  sessionkeep file PATH                The sessions that changed a file: what was asked, what was
                                   answered. Reads only
  sessionkeep why COMMIT | FILE:LINE   The conversation behind a commit, or behind the commit that
                                   last changed a line: the sessions that changed its files in
                                   the week before it. Reads only; changes nothing in Git
  sessionkeep checkpoint "TEXT"        Record where work stands (what is done, what is next) with the
                                   current Git branch, commit and uncommitted state
  sessionkeep resume                   Print a continuation pack: instructions, latest checkpoint, a
                                   warning if the code has moved since, and recent exchanges
  sessionkeep remember "TEXT" --scope global|project [--project ID_OR_PATH] [--yes]
                                   Record a standing instruction, word for word. Without --yes it
                                   only shows what would be saved. TEXT may be - to read stdin.
  sessionkeep suggest [--project ID_OR_PATH | --all-projects]
                                   Requests you made in nearly the same words in three or more
                                   sessions and that no instruction covers. Found by comparing
                                   words, not by a model; saves nothing
  sessionkeep instruction list [--all] | show ID
  sessionkeep instruction supersede ID "TEXT" [--reason R] [--yes]
  sessionkeep instruction archive ID [--reason R] [--yes]
  sessionkeep plugin write DIR [--with-mcp]
                                   Generate the Claude Code plugin directory for this binary;
                                   --with-mcp also registers the optional MCP server
                                   (changes no Claude Code setting)
  sessionkeep install [--with-mcp] [--enable-plugin]
                                   Copy this program to a stable per-user place and generate the
                                   Claude Code plugin beside it. It changes no Claude Code setting
                                   unless you add --enable-plugin, which runs Claude Code's own
                                   two commands that turn the plugin on for your account
  sessionkeep uninstall [--purge-data] [--yes]
                                   Remove what install created; your data is kept unless you
                                   ask for it to be deleted too
  sessionkeep autostart enable|disable|status
                                   Optional: store waiting events once when you log on
  sessionkeep history import [--yes]   Bring in earlier Claude Code sessions of your projects
  sessionkeep summarize status | on | off | run | delete
                                   Optional summaries of finished sessions, written by a model
                                   through your own Claude Code sign-in. Off unless you turn it
                                   on; "summarize on" explains what that means before it changes
                                   anything
  sessionkeep account list | use NAME | auto | plugins | enable NAME | disable NAME
                                   Which Claude Code sign-in SessionKeep acts for when this computer
                                   has more than one (summaries, history import, printed commands)
  sessionkeep research status | on | off | fetch URL | record | list | search | show | check | withdraw
                                   Facts about the outside world (an API, a price, a limit) with
                                   the page that says so and the date it was checked; a fact
                                   past its re-check date is shown as stale. Off unless you turn
                                   it on. Run "sessionkeep research" for the forms
  sessionkeep export FILE              Write everything SessionKeep holds into one file (not encrypted)
  sessionkeep import FILE [--dry-run]  Check an export, or restore it into an empty data directory
  sessionkeep handoff make FILE [--with-sessions] [--since DATE] [--from NAME] [--yes]
  sessionkeep handoff receive FILE [--yes]
                                   Give one project's memory to a teammate: its instructions,
                                   checkpoints and, if you ask, what was said in its sessions,
                                   in one readable file (not encrypted). Both show what would
                                   happen first; receive adds it to a project you registered
  sessionkeep team storage [--storage personal|shared] --endpoint URL --bucket BUCKET [--from YOUR-NAME]
  sessionkeep team connect --name SHARED-NAME [--storage shared] | sync [--yes] | list | status | disconnect [--yes]
                                   Keep a project's memory in a storage bucket you chose
                                   (Cloudflare R2 or another S3-compatible service) and exchange
                                   it with colleagues who connect the same shared name. Your
                                   personal storage is used unless you pick the shared one or
                                   give the project a bucket of its own (--endpoint, --bucket).
                                   What is sent is encrypted here first with the team passphrase;
                                   push and pull show what would happen and need --yes. Run
                                   "sessionkeep team" for how the keys are given
  sessionkeep forget record ID | captured --project P | project P   [--yes]
                                   Permanently delete stored data; without --yes it only shows
                                   what would be deleted
  sessionkeep dashboard                Serve the local web pages (this computer only) and print a
                                   one-time sign-in link; runs until you press Ctrl+C
  sessionkeep mcp                      Optional MCP server on stdin/stdout with read-only tools
                                   (started by Claude Code, not by hand)
  sessionkeep ingest                   Move captured events from the spool into the database
  sessionkeep status                   What is stored and what is still waiting (reads only)
  sessionkeep hook EVENT              Claude Code hook entry point (reads the hook JSON on stdin;
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
	stdin  io.Reader
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
		if getenv(summarize.GuardEnv) != "" {
			return ExitOK // a summary call must never be captured as a session
		}
		return hook(args[1:], stdin, stdout, getenv)
	}
	if len(args) == 2 && args[0] == "ingest" && args[1] == "--quiet" {
		// The form the plugin's background hook uses: print nothing and never report failure to
		// the host. A held lease simply means another drain is already doing the work.
		if inv, err := parse([]string{"ingest"}); err == nil {
			inv.stdout, inv.getenv = io.Discard, getenv
			if _, statErr := os.Stat(dataDirOf(getenv)); statErr == nil { // never create the data directory from a hook
				ingestCmd(inv)
				autoSummarize(getenv)
			}
		}
		return ExitOK
	}
	inv, err := parse(args, "auto-add", "level", "capture", "name", "scope", "project", "reason", "who", "since", "until", "limit", "offset", "cursor", "from", "model", "daily-limit", "subject", "category", "applies-to", "url", "quote", "find", "supersedes", "endpoint", "bucket", "region", "storage")
	if err == nil {
		inv.stdin, inv.stdout, inv.getenv = stdin, stdout, getenv
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
func parse(args []string, valueFlags ...string) (inv invocation, err error) {
	inv = invocation{flags: map[string]string{}}
	// A bare "--" ends the options: everything after it is taken literally, so text that happens
	// to start with "--" (a search query, an instruction) can never be read as an option.
	literal := []string(nil)
	for i, a := range args {
		if a == "--" {
			args, literal = args[:i], args[i+1:]
			break
		}
	}
	for _, a := range args {
		if a == "--json" {
			inv.json = true
		}
	}
	defer func() { inv.pos = append(inv.pos, literal...) }()
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
		if (name == "yes" || name == "all" || name == "all-projects" || name == "exact" || name == "with-mcp" || name == "enable-plugin" || name == "dry-run" || name == "purge-data" || name == "manual" || name == "detached" || name == "due" || name == "with-sessions" || name == "no-sessions" || name == "secrets-stdin") && !hasVal { // switches that take no value
			inv.flags[name] = "true"
			continue
		}
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
	case "ingest":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("ingest takes no arguments")
		}
		return ingestCmd(inv)
	case "status":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("status takes no arguments")
		}
		return status(inv)
	case "session":
		return sessionCmd(inv, rest)
	case "history":
		return historyCmd(inv, rest)
	case "summarize":
		return summarizeCmd(inv, rest)
	case "account":
		return accountCmd(inv, rest)
	case "research":
		return researchCmd(inv, rest)
	case "recap":
		return recapCmd(inv, rest)
	case "file":
		return fileCmd(inv, rest)
	case "why":
		return whyCmd(inv, rest)
	case "checkpoint":
		return checkpointCmd(inv, rest)
	case "resume":
		if len(rest) != 0 {
			return usageErr("usage: sessionkeep resume [--project ID_OR_PATH]")
		}
		return resumeCmd(inv)
	case "memory", "evidence":
		return memoryCmd(inv, cmd, rest)
	case "remember":
		return remember(inv, rest)
	case "instruction":
		return instructionCmd(inv, rest)
	case "suggest":
		return suggestCmd(inv, rest)
	case "install":
		if len(rest) != 0 {
			return usageErr("usage: sessionkeep install [--with-mcp] [--enable-plugin]")
		}
		return installCmd(inv)
	case "uninstall":
		if len(rest) != 0 {
			return usageErr("usage: sessionkeep uninstall [--purge-data] [--yes]")
		}
		return uninstallCmd(inv)
	case "autostart":
		return autostartCmd(inv, rest)
	case "export":
		return exportCmd(inv, rest)
	case "import":
		return importCmd(inv, rest)
	case "handoff":
		return handoffCmd(inv, rest)
	case "team":
		return teamCmd(inv, rest)
	case "forget":
		return forgetCmd(inv, rest)
	case "dashboard":
		if len(rest) != 0 || len(inv.flags) != 0 {
			return usageErr("usage: sessionkeep dashboard")
		}
		return dashboard(inv)
	case "mcp":
		if len(rest) != 0 {
			return usageErr("usage: sessionkeep mcp [--all-projects]")
		}
		if err := inv.only("all-projects"); err != nil {
			return err
		}
		return serveMCP(inv)
	case "plugin":
		if len(rest) != 2 || rest[0] != "write" {
			return usageErr("usage: sessionkeep plugin write DIR [--with-mcp]")
		}
		if err := inv.only("with-mcp"); err != nil {
			return err
		}
		return pluginWrite(inv, rest[1])
	case "folders":
		return foldersCmd(inv, rest)
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
		// Data saved under the program's earlier name moves here first. What was sent to and
		// received from a team is forgotten with it: bundles now live elsewhere in the bucket.
		from, err := config.AdoptOld(p)
		if err != nil {
			return state{}, &cmdError{ExitUnavailable, "old_data_not_moved", err.Error()}
		}
		if from != "" {
			os.RemoveAll(filepath.Join(p.DataDir, "team"))
		}
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
// It holds the ingest lease for the duration (ADR-0003: only the lease holder writes), waiting
// up to ten seconds if a drain is in progress.
func withStore(st state, fn func(ctx context.Context, s *store.Store) error) error {
	ctx := context.Background()
	l, err := lease.AcquireWait(st.paths.LeasePath, 10*time.Second)
	if errors.Is(err, lease.ErrHeld) {
		return &cmdError{ExitUnavailable, "busy", "another SessionKeep process is writing the database; try again in a moment"}
	}
	if err != nil {
		return fmt.Errorf("take the ingest lease: %w", err)
	}
	defer l.Release()
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
			roots = append(roots, project.Root{Path: r, ProjectID: p.ID, Level: p.CapturePolicy, Paused: p.Paused, Recall: recallOf(p)})
		}
	}
	if err := project.WriteRootsCache(st.paths, roots, time.Now()); err != nil {
		return fmt.Errorf("the change was saved but the roots cache could not be rewritten (run `sessionkeep setup` to refresh it): %w", err)
	}
	if err := instruct.RewriteCaches(ctx, s, st.paths, st.settings.DeviceID, time.Now()); err != nil {
		return fmt.Errorf("the change was saved but the instruction cache could not be rewritten (run `sessionkeep setup` to refresh it): %w", err)
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
		return "manual - nothing is saved until you run `sessionkeep project add`"
	case config.ModeAuto:
		return fmt.Sprintf("auto - registered automatically at level %q (except excluded folders)", a.Level)
	}
	return "ask - nothing is saved; you get a reminder to run `sessionkeep project add`"
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

var errWhichFolder = errors.New("the project has several folders and none was named")

func projectCmd(inv invocation, rest []string) error {
	if len(rest) == 0 {
		return usageErr("usage: sessionkeep project add|list|pause|resume|capture|move|which ...")
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

	case "move":
		if err := inv.only("name"); err != nil {
			return err
		}
		if len(args) != 2 {
			return usageErr("usage: sessionkeep project move ID_OR_OLD_PATH NEW_PATH [--name NAME]")
		}
		abs, err := filepath.Abs(args[1])
		if err != nil {
			return usageErr("cannot resolve path %q: %v", args[1], err)
		}
		if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
			return failErr("not_a_directory", "%s is not an existing directory (rename or move the folder first, then run this)", abs)
		}
		to, err := absNorm(abs)
		if err != nil {
			return err
		}
		st, err := load(inv, true)
		if err != nil {
			return err
		}
		if d := project.Decide(st.env(inv.getenv), st.settings, nil, abs); d.Reason == "excluded by the user" {
			return failErr("excluded", "%s is inside an excluded folder; run `sessionkeep exclude remove` first", to)
		}
		old, _ := absNorm(args[0])
		var after store.Project
		from := ""
		if err := withStore(st, func(ctx context.Context, s *store.Store) error {
			p, err := s.FindProject(ctx, st.settings.DeviceID, args[0])
			if errors.Is(err, store.ErrNoProject) && old != "" {
				p, err = s.FindProject(ctx, st.settings.DeviceID, old)
			}
			if err != nil {
				return err
			}
			for _, r := range p.Roots {
				if r == old || len(p.Roots) == 1 {
					from = r
				}
			}
			if from == "" {
				return errWhichFolder
			}
			if err := s.MoveRoot(ctx, p.ID, st.settings.DeviceID, from, to, inv.flags["name"], time.Now()); err != nil {
				return err
			}
			after, err = s.FindProject(ctx, st.settings.DeviceID, p.ID)
			return err
		}); err != nil {
			switch {
			case errors.Is(err, errWhichFolder):
				return failErr("which_folder", "this project has more than one folder on this computer; give the old folder's path instead of the id")
			case errors.Is(err, store.ErrNoProject):
				return failErr("no_such_project", "no project matches %q (use an id from `sessionkeep project list` or the folder's old path)", args[0])
			case errors.Is(err, store.ErrAmbiguousProject):
				return failErr("ambiguous_project", "%q matches more than one project; use more of the id", args[0])
			case errors.Is(err, store.ErrRootRegistered):
				return failErr("already_registered", "%s is already registered as a project of its own", to)
			}
			return err
		}
		return inv.emit(map[string]any{"project": after, "from": from, "to": to},
			fmt.Sprintf("project %.8s (%s) now lives in %s\nwas:     %s\nEverything saved for it stays with it.\n", after.ID, after.Name, to, from))

	case "pause", "resume", "capture", "recall":
		if err := inv.only(); err != nil {
			return err
		}
		want := 1
		if sub == "capture" || sub == "recall" {
			want = 2
		}
		if len(args) != want {
			return usageErr("usage: sessionkeep project pause|resume ID_OR_PATH, sessionkeep project capture ID_OR_PATH LEVEL, or sessionkeep project recall ID_OR_PATH asked|automatic")
		}
		if sub == "recall" && args[1] != "asked" && args[1] != project.RecallAutomatic {
			return usageErr("recall must be asked or automatic (got %q)", args[1])
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
			case "recall":
				err = s.SetRecall(ctx, p.ID, args[1])
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

// sessionCmd lists captured sessions and shows one in full. Read-only.
func sessionCmd(inv invocation, rest []string) error {
	if err := inv.only("project", "all-projects", "limit", "offset"); err != nil {
		return err
	}
	if !(len(rest) == 1 && rest[0] == "list") && !(len(rest) == 2 && rest[0] == "show") {
		return usageErr("usage: sessionkeep session list [--project ID_OR_PATH | --all-projects] [--limit N] [--offset N]\n       sessionkeep session show ID [--project ID_OR_PATH | --all-projects]")
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
	if rest[0] == "list" {
		limit := 0
		if v := inv.flags["limit"]; v != "" {
			if limit, err = strconv.Atoi(v); err != nil || limit < 1 {
				return usageErr("--limit must be a positive number (got %q)", v)
			}
		}
		offset := 0
		if v := inv.flags["offset"]; v != "" {
			if offset, err = strconv.Atoi(v); err != nil || offset < 0 {
				return usageErr("--offset must be zero or a positive number (got %q)", v)
			}
		}
		list, err := s.Sessions(ctx, proj.ID, limit, offset)
		if err != nil {
			return err
		}
		var b strings.Builder
		for _, x := range list {
			fmt.Fprintf(&b, "%.8s  %.16s  %-8s %3d text(s)  %s\n", x.ID, strings.Replace(x.StartedAt, "T", " ", 1), x.State, x.Texts, firstRunes(x.FirstPrompt, 90))
		}
		if len(list) == 0 {
			b.WriteString("no sessions captured yet\n")
		}
		return inv.emit(list, b.String())
	}
	info, turns, err := s.SessionTurns(ctx, rest[1], proj.ID, all)
	if errors.Is(err, store.ErrNoSession) {
		return failErr("not_found", "no captured session matches %q in that scope (use at least 8 characters of an id from `sessionkeep session list`)", rest[1])
	}
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "session %s in project %q\nstarted %s, last activity %s, %s\n", info.ID, info.ProjectName, info.StartedAt, info.LastAt, info.State)
	sum, hasSum, err := s.SummaryOf(ctx, info.ProjectID, info.ID)
	if err != nil {
		return err
	}
	out := map[string]any{"session": info, "turns": turns}
	if hasSum {
		out["summary"] = sum
		fmt.Fprintf(&b, "\nSUMMARY written by a model (%s, %.10s) from %d of %d piece(s) of text. It is not the original; the text below is.\n%s\n", sum.Model, sum.CreatedAt, sum.SourceUsed, sum.SourceCount, sum.Text)
	}
	for _, t := range turns {
		switch {
		case t.Text != "":
			fmt.Fprintf(&b, "\n[%s, %.19s]\n%s\n", t.Who, strings.Replace(t.ObservedAt, "T", " ", 1), t.Text)
		case t.Tool != "":
			fmt.Fprintf(&b, "\n(tool: %s)\n", t.Tool)
		}
	}
	return inv.emit(out, b.String())
}

// recallOf is the project's recall mode as the roots cache holds it: empty unless automatic.
func recallOf(p store.Project) string {
	if p.Recall == project.RecallAutomatic {
		return p.Recall
	}
	return ""
}

// workDir picks the folder whose Git state belongs to a project for this invocation: the folder
// the user named, else the current directory if it is inside the project, else the project's root.
func workDir(inv invocation, st state, p store.Project, key string) string {
	inside := func(dir string) bool {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return false
		}
		for _, r := range p.Roots {
			d := project.Decide(st.env(inv.getenv), config.DefaultSettings(), []project.Root{{Path: r, ProjectID: p.ID, Level: config.LevelMetadata}}, abs)
			if d.ProjectID == p.ID {
				return true
			}
		}
		return false
	}
	if key != "" {
		if fi, err := os.Stat(key); err == nil && fi.IsDir() && inside(key) {
			abs, _ := filepath.Abs(key)
			return abs
		}
	}
	if inside(".") {
		abs, _ := filepath.Abs(".")
		return abs
	}
	if len(p.Roots) > 0 {
		return p.Roots[0]
	}
	return "."
}

func gitOf(c store.Checkpoint) gitstate.State {
	return gitstate.State{Repo: c.GitRepo, Branch: c.Branch, Head: c.Head, Changed: c.ChangedFiles, Dirty: c.DirtyDigest}
}

// checkpointCmd records where work stands, with the Git state read (never changed) at that moment.
func checkpointCmd(inv invocation, rest []string) error {
	if err := inv.only("project"); err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr(`usage: sessionkeep checkpoint "what is done and what is next" [--project ID_OR_PATH]   (the text may be - to read stdin)`)
	}
	note, err := readText(inv, rest[0])
	if err != nil {
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var saved store.Checkpoint
	var proj store.Project
	var git gitstate.State
	if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
		if proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
			return err
		}
		git = gitstate.Read(workDir(inv, st, proj, inv.flags["project"]))
		saved, err = s.AddCheckpoint(ctx, store.Checkpoint{ID: config.NewID(), ProjectID: proj.ID, Note: note, GitRepo: git.Repo,
			Branch: git.Branch, Head: git.Head, ChangedFiles: git.Changed, DirtyDigest: git.Dirty}, time.Now())
		return err
	}); err != nil {
		return err
	}
	where := "no Git repository here (" + git.Reason + "); only the note was recorded"
	if git.Repo {
		where = fmt.Sprintf("branch %s at %.10s, %d uncommitted file(s)", git.Branch, git.Head, git.Changed)
	}
	return inv.emit(saved, fmt.Sprintf("checkpoint %.8s recorded for project %q\n%s\n", saved.ID, proj.Name, where))
}

// resumeCmd prints a continuation pack: the recorded instructions, the latest checkpoint with a
// comparison against the working tree as it is now, and the most recent exchanges. Read-only.
func resumeCmd(inv invocation) error {
	if err := inv.only("project"); err != nil {
		return err
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
	proj, err := resolveProject(ctx, inv, st, s, inv.flags["project"])
	if err != nil {
		return err
	}
	instr, err := s.ActiveInstructions(ctx, proj.ID)
	if err != nil {
		return err
	}
	cps, err := s.Checkpoints(ctx, proj.ID, 3)
	if err != nil {
		return err
	}
	recent, err := s.RecentExchanges(ctx, proj.ID, 6, 600)
	if err != nil {
		return err
	}
	now := gitstate.Read(workDir(inv, st, proj, inv.flags["project"]))
	pending := pendingEvents(st.paths)

	var b strings.Builder
	var warnings []string
	fmt.Fprintf(&b, "RESUME PACK for project %q (%.8s)\n", proj.Name, proj.ID)
	fmt.Fprintf(&b, "\n== Recorded instructions (%d active) ==\n", len(instr))
	for _, i := range instr {
		fmt.Fprintf(&b, "- [%s, %.8s] %s\n", i.Scope, i.ID, i.Text)
	}
	if len(instr) == 0 {
		b.WriteString("none recorded\n")
	}
	b.WriteString("\n== Latest checkpoint ==\n")
	if len(cps) == 0 {
		b.WriteString("none recorded; nothing here says where work stopped\n")
	} else {
		c := cps[0]
		fmt.Fprintf(&b, "recorded %s (%.8s)\n%s\n", c.CreatedAt, c.ID, c.Note)
		if c.GitRepo {
			fmt.Fprintf(&b, "at that time: branch %s at %.10s, %d uncommitted file(s)\n", c.Branch, c.Head, c.ChangedFiles)
		} else {
			b.WriteString("at that time: no Git repository\n")
		}
		warnings = gitstate.Compare(gitOf(c), now)
		if len(cps) > 1 {
			fmt.Fprintf(&b, "(%d earlier checkpoint(s) exist)\n", len(cps)-1)
		}
	}
	b.WriteString("\n== Working tree now ==\n")
	if now.Repo {
		fmt.Fprintf(&b, "branch %s at %.10s, %d uncommitted file(s)\n", now.Branch, now.Head, now.Changed)
	} else {
		fmt.Fprintf(&b, "no Git state available: %s\n", now.Reason)
	}
	for _, w := range warnings {
		fmt.Fprintf(&b, "WARNING: %s\n", w)
	}
	if len(cps) > 0 && len(warnings) == 0 {
		b.WriteString("matches the checkpoint: same branch, same commit, same uncommitted state\n")
	}
	if len(warnings) > 0 {
		b.WriteString("The checkpoint describes a different state of the code. Check the differences before relying on it. Nothing was changed by this command.\n")
	}
	fmt.Fprintf(&b, "\n== Most recent exchanges (%d) ==\n", len(recent))
	for _, e := range recent {
		fmt.Fprintf(&b, "[%s, %.16s] %s\n", e.Who, strings.Replace(e.ObservedAt, "T", " ", 1), e.Text)
	}
	if len(recent) == 0 {
		b.WriteString("nothing stored yet\n")
	}
	if pending > 0 {
		fmt.Fprintf(&b, "\nnote: %d captured event(s) are not stored yet and are missing above; run `sessionkeep ingest` and resume again for the latest state\n", pending)
	}
	b.WriteString("\nSearch older history with: sessionkeep memory search \"...\"\n")
	return inv.emit(map[string]any{"project": proj, "instructions": instr, "checkpoints": cps, "git_now": now, "warnings": warnings,
		"recent": recent, "not_included_pending_events": pending}, b.String())
}

// when turns a date (YYYY-MM-DD) or an RFC 3339 time into the form stored timestamps compare with.
func when(flag, v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", usageErr("--%s must be a date like 2026-11-19 or a time like 2026-11-19T14:30:00Z (got %q)", flag, v)
}

// pendingEvents counts captured events not yet in the database, so a search can say that its
// answer may be incomplete instead of implying it looked at everything.
func pendingEvents(p config.Paths) int {
	n := 0
	if entries, err := os.ReadDir(p.SpoolDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".evt") {
				n++
			}
		}
	}
	return n
}

// memoryCmd searches and reads stored text. It only reads: no lease, no writes.
// In this phase a "memory" is a stored piece of original text, so `evidence get` and `memory get`
// are the same lookup.
func memoryCmd(inv invocation, cmd string, rest []string) error {
	usageText := `usage: sessionkeep memory search "QUERY" [--project ID_OR_PATH | --all-projects] [--exact] [--who user|assistant|tool] [--since DATE] [--until DATE] [--limit N] [--cursor C]
       sessionkeep memory get ID [--project ID_OR_PATH | --all-projects]   (also: sessionkeep evidence get ID)`
	if len(rest) != 2 || (rest[0] != "search" && rest[0] != "get") || (cmd == "evidence" && rest[0] != "get") {
		return usageErr("%s", usageText)
	}
	if rest[0] == "search" {
		if err := inv.only("project", "all-projects", "exact", "who", "since", "until", "limit", "cursor"); err != nil {
			return err
		}
	} else if err := inv.only("project", "all-projects"); err != nil {
		return err
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
	scope := "all projects"
	if !all {
		scope = fmt.Sprintf("project %q (%.8s)", proj.Name, proj.ID)
	}
	pending := pendingEvents(st.paths)

	if rest[0] == "get" {
		rec, err := s.GetEvidence(ctx, rest[1], proj.ID, all)
		if errors.Is(err, store.ErrNoEvidence) {
			return failErr("not_found", "no stored record matches %q in %s (use at least 8 characters of an id from a search)", rest[1], scope)
		}
		if err != nil {
			return err
		}
		text := fmt.Sprintf("record %s\nproject: %s (%.8s)\nsaid by: %s (%s)\nwhen:    %s\nsession: %s\ncomplete: %s; redactions: %d\n----\n%s\n",
			rec.ID, rec.ProjectName, rec.ProjectID, rec.Who, rec.Origin, rec.ObservedAt, rec.Session, rec.Completeness, rec.RedactionCount, rec.Text)
		return inv.emit(rec, text)
	}

	limit := 0
	if v := inv.flags["limit"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return usageErr("--limit must be a positive number (got %q)", v)
		}
		limit = n
	}
	since, err := when("since", inv.flags["since"])
	if err != nil {
		return err
	}
	until, err := when("until", inv.flags["until"])
	if err != nil {
		return err
	}
	res, err := s.Search(ctx, store.SearchParams{ProjectID: proj.ID, AllProjects: all, Query: rest[1], Exact: inv.flags["exact"] != "",
		Who: inv.flags["who"], Since: since, Until: until, Limit: limit, Cursor: inv.flags["cursor"]})
	if errors.Is(err, store.ErrBadCursor) {
		return usageErr("the cursor is not valid; start the search again without --cursor")
	}
	if err != nil {
		return usageErr("%v", err)
	}
	var b strings.Builder
	for _, h := range res.Hits {
		line := fmt.Sprintf("%.8s  %.16s  %-9s", h.ID, strings.Replace(h.ObservedAt, "T", " ", 1), h.Who)
		if all {
			line += fmt.Sprintf("  [%.8s]", h.ProjectID)
		}
		fmt.Fprintf(&b, "%s  %s\n", line, strings.Join(strings.Fields(h.Snippet), " "))
	}
	if res.Method == "any" {
		b.WriteString("PARTIAL MATCHES ONLY: no stored record contains all of these words; the records above contain some of them.\n")
	}
	switch {
	case len(res.Hits) == 0:
		fmt.Fprintf(&b, "not found: nothing stored in %s matches this query\n", scope)
	case res.NextCursor != "":
		fmt.Fprintf(&b, "%d result(s) shown from %s; more exist - repeat with --cursor %s\n", len(res.Hits), scope, res.NextCursor)
	default:
		fmt.Fprintf(&b, "%d result(s) from %s; that is all of them\n", len(res.Hits), scope)
	}
	// Summaries are searched too, and shown apart: they are a model's words about a session, and
	// can match when the question uses other words than the session did.
	sums, sumsPartial := []store.SummaryHit{}, false
	plain := inv.flags["exact"] == "" && inv.flags["who"] == "" && since == "" && until == "" && inv.flags["cursor"] == ""
	if plain {
		if sums, sumsPartial, err = s.SearchSummaries(ctx, proj.ID, all, rest[1], 3); err != nil {
			return err
		}
	}
	if len(sums) > 0 {
		b.WriteString("Session summaries that match (written by a model, not original text; read the session before relying on one):\n")
		for _, h := range sums {
			fmt.Fprintf(&b, "  session %.8s  %.10s  %s\n", h.SessionID, h.CreatedAt, strings.Join(strings.Fields(h.Snippet), " "))
		}
		if sumsPartial {
			b.WriteString("  (these contain only some of the words)\n")
		}
		b.WriteString("  read a session with: sessionkeep session show <id>\n")
	}
	if plain && (len(res.Hits) == 0 || res.Method == "any") {
		b.WriteString("Text is matched by its words, not by its meaning. Before concluding that something was never said, search again with other words for the same thing: a synonym, the file or function name, or part of the error message.\n")
	}
	if pending > 0 {
		fmt.Fprintf(&b, "note: %d captured event(s) are not stored yet and were not searched; run `sessionkeep ingest` first for a complete answer\n", pending)
	}
	if len(res.Hits) > 0 {
		b.WriteString("read a full record with: sessionkeep memory get <id>\n")
	}
	return inv.emit(map[string]any{"hits": res.Hits, "method": res.Method, "next_cursor": res.NextCursor, "summaries": sums, "summaries_partial": sumsPartial, "scope": scope, "not_searched_pending_events": pending}, b.String())
}

// MaxInstructionChars bounds one instruction. An instruction longer than this could crowd every
// other one out of the session envelope (instruct.Budget is 9,000).
const MaxInstructionChars = 4000

// readText returns the instruction text exactly as given; "-" means read it from stdin.
func readText(inv invocation, arg string) (string, error) {
	text := arg
	if arg == "-" {
		raw, err := io.ReadAll(io.LimitReader(inv.stdin, 1<<20))
		if err != nil {
			return "", err
		}
		text = strings.TrimRight(string(raw), "\r\n")
	}
	switch n := len([]rune(text)); {
	case strings.TrimSpace(text) == "":
		return "", usageErr("the instruction text is empty")
	case n > MaxInstructionChars:
		return "", usageErr("the instruction is %d characters; the limit is %d - split it into separate instructions", n, MaxInstructionChars)
	case strings.ContainsRune(text, 0):
		return "", usageErr("the instruction text contains a NUL character")
	}
	if _, n := redact.Text(text); n > 0 {
		return "", failErr("looks_like_secret", "the text looks like it contains a password, key or token, so it was not saved; instructions are stored and replayed word for word")
	}
	return text, nil
}

// remember records a standing instruction. Scope has no default, and nothing is saved without
// --yes: the first run shows exactly what would be stored and where it would apply.
func remember(inv invocation, rest []string) error {
	if err := inv.only("scope", "project", "yes"); err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr(`usage: sessionkeep remember "TEXT" --scope global|project [--project ID_OR_PATH] [--yes]   (TEXT may be - to read stdin)`)
	}
	scope := inv.flags["scope"]
	if scope != "global" && scope != "project" {
		return &cmdError{ExitNeedsConsent, "scope_required", "say where this applies: --scope project (this project only) or --scope global (every registered project); there is no default"}
	}
	if scope == "global" && inv.flags["project"] != "" {
		return usageErr("--project cannot be combined with --scope global")
	}
	text, err := readText(inv, rest[0])
	if err != nil {
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var saved store.Instruction
	var target store.Project
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		if scope == "project" {
			if target, err = resolveProject(ctx, inv, st, s, inv.flags["project"]); err != nil {
				return err
			}
		}
		if inv.flags["yes"] == "" {
			return nil
		}
		saved, err = s.AddInstruction(ctx, store.NewInstruction{ID: config.NewID(), VersionID: config.NewID(), Scope: scope, ProjectID: target.ID, Text: text, Author: "owner", Now: time.Now()})
		return err
	})
	if errors.Is(err, store.ErrDuplicateInstruction) {
		return failErr("duplicate", "%v", err)
	}
	if err != nil {
		return err
	}
	where := "every registered project (global)"
	if scope == "project" {
		where = fmt.Sprintf("project %q (%.8s)", target.Name, target.ID)
	}
	if inv.flags["yes"] == "" {
		inv.emit(map[string]any{"saved": false, "scope": scope, "project_id": target.ID, "text": text},
			fmt.Sprintf("NOT SAVED YET. This would be recorded word for word:\n----\n%s\n----\napplies to: %s\nRun the same command with --yes to record it.\n", text, where))
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was saved; add --yes to record exactly this text"}
	}
	return inv.emit(saved, fmt.Sprintf("recorded instruction %.8s (revision 1)\napplies to: %s\nit will be given to new sessions from now on\n", saved.ID, where))
}

// resolveProject finds a project by id, id prefix or folder; with no key it uses the current
// directory. A folder inside a registered root resolves to that root's project.
func resolveProject(ctx context.Context, inv invocation, st state, s *store.Store, key string) (store.Project, error) {
	if key != "" {
		if p, err := s.FindProject(ctx, st.settings.DeviceID, key); err == nil || errors.Is(err, store.ErrAmbiguousProject) {
			return p, err
		}
	} else {
		key = "."
	}
	abs, err := filepath.Abs(key)
	if err != nil {
		return store.Project{}, usageErr("cannot resolve path %q: %v", key, err)
	}
	all, err := s.ListProjects(ctx, st.settings.DeviceID)
	if err != nil {
		return store.Project{}, err
	}
	var roots []project.Root
	for _, p := range all {
		for _, r := range p.Roots {
			roots = append(roots, project.Root{Path: r, ProjectID: p.ID, Level: p.CapturePolicy})
		}
	}
	none := config.DefaultSettings() // exclusions do not matter for naming a project
	d := project.Decide(st.env(inv.getenv), none, roots, abs)
	for _, p := range all {
		if p.ID == d.ProjectID && d.ProjectID != "" {
			return p, nil
		}
	}
	return store.Project{}, failErr("no_such_project", "%s is not inside a registered project; register it with `sessionkeep project add`, pass --project, or use --scope global", abs)
}

func instructionCmd(inv invocation, rest []string) error {
	if len(rest) == 0 {
		return usageErr("usage: sessionkeep instruction list [--all] | show ID | supersede ID \"TEXT\" [--reason R] [--yes] | archive ID [--reason R] [--yes]")
	}
	sub, args := rest[0], rest[1:]
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	notFound := func(err error, key string) error {
		switch {
		case errors.Is(err, store.ErrNoInstruction):
			return failErr("no_such_instruction", "no instruction matches %q (use at least 4 characters of an id from `sessionkeep instruction list`)", key)
		case errors.Is(err, store.ErrAmbiguousInstruction):
			return failErr("ambiguous_instruction", "%q matches more than one instruction; use more of the id", key)
		case errors.Is(err, store.ErrNotActive):
			return failErr("not_active", "instruction %q is archived and cannot be changed", key)
		case errors.Is(err, store.ErrDuplicateInstruction):
			return failErr("duplicate", "the new text is identical to the current text")
		}
		return err
	}
	switch sub {
	case "list":
		if err := inv.only("all"); err != nil {
			return err
		}
		if len(args) != 0 {
			return usageErr("instruction list takes no arguments")
		}
		var all []store.Instruction
		if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
			all, err = s.ListInstructions(ctx, inv.flags["all"] != "")
			return err
		}); err != nil {
			return err
		}
		var b strings.Builder
		for _, i := range all {
			scope := i.Scope
			if i.ProjectID != "" {
				scope += fmt.Sprintf(" %.8s", i.ProjectID)
			}
			first, _, more := strings.Cut(i.Text, "\n")
			if len([]rune(first)) > 70 {
				first, more = string([]rune(first)[:70]), true
			}
			if more {
				first += " ..."
			}
			fmt.Fprintf(&b, "%.8s  %-8s rev %d  %-16s %s\n", i.ID, i.State, i.Revision, scope, first)
		}
		if len(all) == 0 {
			b.WriteString("no instructions recorded\n")
		}
		return inv.emit(all, b.String())

	case "show":
		if err := inv.only(); err != nil {
			return err
		}
		if len(args) != 1 {
			return usageErr("usage: sessionkeep instruction show ID")
		}
		var ins store.Instruction
		var hist []store.InstructionVersion
		if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
			if ins, err = s.GetInstruction(ctx, args[0]); err != nil {
				return err
			}
			hist, err = s.InstructionHistory(ctx, ins.ID)
			return err
		}); err != nil {
			return notFound(err, args[0])
		}
		var b strings.Builder
		fmt.Fprintf(&b, "instruction %s\nscope: %s %s\nstate: %s\n", ins.ID, ins.Scope, ins.ProjectID, ins.State)
		for _, v := range hist {
			fmt.Fprintf(&b, "---- revision %d | %s | %s | sha256 %.12s", v.Revision, v.Status, v.CreatedAt, v.Digest)
			if v.Reason != "" {
				fmt.Fprintf(&b, " | reason: %s", v.Reason)
			}
			fmt.Fprintf(&b, "\n%s\n", v.Text)
		}
		return inv.emit(map[string]any{"instruction": ins, "versions": hist}, b.String())

	case "supersede", "archive":
		if err := inv.only("reason", "yes"); err != nil {
			return err
		}
		want := map[string]int{"supersede": 2, "archive": 1}[sub]
		if len(args) != want {
			return usageErr("usage: sessionkeep instruction supersede ID \"TEXT\" [--reason R] [--yes], or sessionkeep instruction archive ID [--reason R] [--yes]")
		}
		text := ""
		if sub == "supersede" {
			if text, err = readText(inv, args[1]); err != nil {
				return err
			}
		}
		var before, after store.Instruction
		if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
			if before, err = s.GetInstruction(ctx, args[0]); err != nil {
				return err
			}
			if before.State != "active" {
				return store.ErrNotActive
			}
			if inv.flags["yes"] == "" {
				return nil
			}
			if sub == "archive" {
				if err = s.ArchiveInstruction(ctx, before.ID, inv.flags["reason"]); err != nil {
					return err
				}
				after, err = s.GetInstruction(ctx, before.ID)
				return err
			}
			after, err = s.SupersedeInstruction(ctx, before.ID, config.NewID(), text, inv.flags["reason"], "owner", time.Now())
			return err
		}); err != nil {
			return notFound(err, args[0])
		}
		if inv.flags["yes"] == "" {
			preview := fmt.Sprintf("NOT CHANGED YET. Instruction %.8s currently reads:\n----\n%s\n----\n", before.ID, before.Text)
			if sub == "supersede" {
				preview += fmt.Sprintf("It would be replaced, word for word, by:\n----\n%s\n----\nThe old text stays in the history.\n", text)
			} else {
				preview += "It would be archived: kept in the history, no longer given to sessions.\n"
			}
			inv.emit(map[string]any{"changed": false, "current": before, "new_text": text}, preview+"Run the same command with --yes to do it.\n")
			return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was changed; add --yes to confirm"}
		}
		if sub == "archive" {
			return inv.emit(after, fmt.Sprintf("archived instruction %.8s; it is no longer given to sessions\n", after.ID))
		}
		return inv.emit(after, fmt.Sprintf("instruction %.8s is now revision %d; the previous text is kept in its history\n", after.ID, after.Revision))
	}
	return usageErr("unknown instruction command %q; run `sessionkeep help`", sub)
}

// dataDirOf returns the resolved data directory, or a path that cannot exist.
func dataDirOf(getenv func(string) string) string {
	p, err := config.Resolve(runtime.GOOS, getenv)
	if err != nil {
		return string([]byte{0})
	}
	return p.DataDir
}

// pluginWrite generates the Claude Code plugin directory for this binary. It changes no Claude
// Code setting: loading the plugin (`claude --plugin-dir DIR`, or installing it) is the user's step.
func pluginWrite(inv invocation, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return usageErr("cannot resolve path %q: %v", dir, err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find this program's own path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if err := plugin.Write(abs, exe, buildinfo.Get().Version, inv.flags["with-mcp"] != ""); err != nil {
		if errors.Is(err, plugin.ErrNotOurs) {
			return failErr("not_a_sessionkeep_plugin_dir", "%v; choose an empty or new directory", err)
		}
		return err
	}
	text := fmt.Sprintf("plugin written to %s\nhooks call: %s\nto try it for one session: claude --plugin-dir \"%s\"\nno Claude Code setting was changed\n", abs, exe, abs)
	return inv.emit(map[string]string{"plugin_dir": abs, "binary": exe}, text)
}

// ingestCmd drains the spool into the database once. With no engine running this is how captured
// events become searchable (ADR-0003, manual mode).
func ingestCmd(inv invocation) error {
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	ctx := context.Background()
	l, err := lease.Acquire(st.paths.LeasePath)
	if errors.Is(err, lease.ErrHeld) {
		return &cmdError{ExitUnavailable, "busy", "another SessionKeep process is already ingesting; nothing to do here"}
	}
	if err != nil {
		return fmt.Errorf("take the ingest lease: %w", err)
	}
	defer l.Release()
	s, err := store.Open(ctx, st.paths.DBPath)
	if err != nil {
		if errors.Is(err, store.ErrIncompatibleEngine) {
			return &cmdError{ExitUnavailable, "incompatible", err.Error()}
		}
		return err
	}
	defer s.Close()
	rep, err := ingest.Drain(ctx, ingest.Deps{Paths: st.paths, Settings: st.settings, Store: s, GOOS: runtime.GOOS, Now: time.Now, NewID: config.NewID})
	if err == nil {
		err = instruct.RewriteCaches(ctx, s, st.paths, st.settings.DeviceID, time.Now())
		instruct.PruneSessionState(st.paths, time.Now())
	}
	text := fmt.Sprintf("ingested %d, already stored %d, quarantined %d, discarded (excluded) %d, projects created %d, gaps imported %d\n",
		rep.Ingested, rep.Replayed, rep.Quarantined, rep.DiscardedExclude, rep.ProjectsCreated, rep.GapsImported)
	if rep.Recovered > 0 {
		text += fmt.Sprintf("%d event(s) were recovered from a capture that had been interrupted before it finished saving\n", rep.Recovered)
	}
	if rep.Deferred > 0 {
		text += fmt.Sprintf("%d file(s) were busy (held open by another program) and are left for the next run; nothing is lost\n", rep.Deferred)
	}
	if err != nil {
		inv.emit(rep, text)
		return fmt.Errorf("drain stopped with %d file(s) left in the spool; they will be picked up by the next run: %w", rep.Remaining, err)
	}
	return inv.emit(rep, text)
}

// status reports what is stored and what is waiting, without taking the lease or changing anything.
func status(inv invocation) error {
	st, err := load(inv, false)
	if err != nil {
		return err
	}
	counts, err := store.ReadCounts(context.Background(), st.paths.DBPath)
	if err != nil {
		return fmt.Errorf("read the database: %w", err)
	}
	pending, quarantined, oldest := 0, 0, ""
	if entries, err := os.ReadDir(st.paths.SpoolDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".evt") {
				if pending == 0 {
					if fi, err := e.Info(); err == nil {
						oldest = fi.ModTime().UTC().Format(time.RFC3339)
					}
				}
				pending++
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(st.paths.DataDir, ingest.QuarantineDir)); err == nil {
		quarantined = len(entries)
	}
	unimported := 0
	if raw, err := os.ReadFile(filepath.Join(st.paths.DataDir, capture.GapLogName)); err == nil {
		unimported = strings.Count(string(raw), "\n")
	}
	data := map[string]any{"stored": counts, "spool_pending": pending, "spool_oldest": oldest, "quarantined_files": quarantined,
		"gaps_not_yet_imported": unimported, "unregistered_folders": st.settings.AutoAdd.Mode}
	text := fmt.Sprintf("stored: %d project(s), %d session(s), %d event(s), %d text record(s), %d capture gap(s)\n", counts.Projects, counts.Sessions, counts.Events, counts.Evidence, counts.Gaps)
	text += fmt.Sprintf("waiting in the spool: %d event(s)", pending)
	if pending > 0 {
		text += fmt.Sprintf(" (oldest %s) - run `sessionkeep ingest` to store them", oldest)
	}
	text += fmt.Sprintf("\nquarantined files: %d; gaps not yet imported: %d\nunregistered folders: %s\n", quarantined, unimported, describeMode(st.settings.AutoAdd))
	return inv.emit(data, text)
}

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | failed
	Detail string `json:"detail"`
}

// notChecked is stated in every doctor report so a short list of passes is not read as full health.
const notChecked = "database integrity, spool backlog age, capture hooks and plugin registration (see `sessionkeep status` for counts)"

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

	checks = append(checks, installChecks(inv)...)

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
