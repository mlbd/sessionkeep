// Package capture is the Claude Code hook adapter: it turns one hook payload into at most one
// durable spool file. It runs inside a short-lived process on the user's critical path, so it
// never writes to the database, never touches the network, and never returns an error to the
// host (ADR-0002, ADR-0003). It reads the database in one case only: a request in a project
// whose owner chose automatic recall is searched for, read-only and for a fraction of a second
// at most (D123).
package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"sessionkeep/internal/config"
	"sessionkeep/internal/instruct"
	"sessionkeep/internal/project"
	"sessionkeep/internal/redact"
)

// AdapterVersion identifies this mapping from host payloads to normalized events.
const AdapterVersion = 1

// MaxTextBytes bounds each captured text field. Longer text is cut and the event says so.
const MaxTextBytes = 256 << 10

// maxSnippetBytes bounds tool input values and tool output snippets.
const maxSnippetBytes = 4 << 10

// TimeFormat is how observed times are stored: always nine fractional digits, so comparing two
// stored times as text agrees with comparing the instants. (time.RFC3339Nano drops trailing
// zeros, which misorders events inside one second.) It still parses as RFC 3339.
const TimeFormat = "2006-01-02T15:04:05.000000000Z"

// Deadline is how long a hook may spend before it gives up silently. Typical runs take about
// 15 ms; the bound exists for a stalled disk, and is generous because giving up loses the event.
// A variable so that a slow test run can wait longer.
var Deadline = 2 * time.Second

// kinds maps supported hook events to normalized event kinds. Anything else is ignored.
var kinds = map[string]string{
	"SessionStart":       "session_start",
	"UserPromptSubmit":   "user_prompt",
	"PostToolUse":        "tool_use",
	"PostToolUseFailure": "tool_failure",
	"Stop":               "assistant_turn",
	"StopFailure":        "turn_failure",
	"PostCompact":        "compaction",
	"SessionEnd":         "session_end",
}

// hookInput lists every host field SessionKeep reads. Fields not named here are never persisted.
type hookInput struct {
	SessionID            string          `json:"session_id"`
	Cwd                  string          `json:"cwd"`
	TranscriptPath       string          `json:"transcript_path"`
	PromptID             string          `json:"prompt_id"`
	Source               string          `json:"source"`
	Prompt               string          `json:"prompt"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	StopHookActive       *bool           `json:"stop_hook_active"`
	ToolName             string          `json:"tool_name"`
	ToolUseID            string          `json:"tool_use_id"`
	ToolInput            map[string]any  `json:"tool_input"`
	ToolResponse         json.RawMessage `json:"tool_response"`
	DurationMS           *float64        `json:"duration_ms"`
	Error                string          `json:"error"`
	IsInterrupt          *bool           `json:"is_interrupt"`
	Reason               string          `json:"reason"`
	Trigger              string          `json:"trigger"`
	CompactSummary       string          `json:"compact_summary"`
}

// toolInputKeys are the tool input fields kept at conversation level and above: where the tool
// acted and what it was asked, never file contents.
var toolInputKeys = []string{"file_path", "path", "notebook_path", "command", "description", "pattern", "glob", "url", "query", "subagent_type"}

// Event is the normalized record written to the spool.
type Event struct {
	Version        int            `json:"version"`
	EventID        string         `json:"event_id"`
	SourceKey      string         `json:"source_key"`
	Agent          string         `json:"agent"`
	AdapterVersion int            `json:"adapter_version"`
	DeviceID       string         `json:"device_id"`
	ProjectID      string         `json:"project_id,omitempty"`
	Register       *Register      `json:"register,omitempty"`
	Level          string         `json:"level"`
	SessionID      string         `json:"session_id"`
	Kind           string         `json:"kind"`
	HookEvent      string         `json:"hook_event"`
	ObservedAt     string         `json:"observed_at"`
	Cwd            string         `json:"cwd"`
	Payload        map[string]any `json:"payload"`
	Redaction      Redaction      `json:"redaction"`
	Completeness   string         `json:"completeness"`
	DroppedBytes   int            `json:"dropped_bytes,omitempty"`
}

// Register asks the ingester to create the project first (auto-add mode, ADR-0007).
type Register struct {
	Root  string `json:"root"`
	Level string `json:"level"`
}

// Redaction says what the redactor did to this event.
type Redaction struct {
	Applied     bool `json:"applied"`
	Count       int  `json:"count"`
	RuleVersion int  `json:"rule_version"`
}

// Request is one hook invocation.
type Request struct {
	HookEvent string
	Raw       []byte
	Paths     config.Paths
	Env       project.Env
	Now       time.Time
	NewID     func() string
}

// Result reports what happened, for tests and diagnostics. Stdout is the only thing the host sees.
type Result struct {
	Stdout    []byte
	Action    string // decision action, or "ignored" / "unavailable"
	SpoolFile string // set when an event was written
	Gap       string // set when an event should have been written but was not
}

// Handle processes one hook payload. It never panics outward and never reports failure to the
// host: every problem ends as "write nothing" plus, where possible, a gap record without content.
func Handle(req Request) (res Result) {
	defer func() {
		if r := recover(); r != nil {
			res = Result{Action: "unavailable", Gap: gap(req, "internal_error")}
		}
	}()

	kind, ok := kinds[req.HookEvent]
	if !ok {
		return Result{Action: "ignored"}
	}
	var in hookInput
	if err := json.Unmarshal(req.Raw, &in); err != nil || in.Cwd == "" || in.SessionID == "" {
		return Result{Action: "ignored"}
	}

	// Fail closed: a damaged settings or roots file must not be read as "no exclusions".
	settings, err := config.LoadSettings(req.Paths)
	if err != nil {
		return Result{Action: "unavailable", Gap: gap(req, "settings_unreadable")}
	}
	roots, err := project.ReadRootsCache(req.Paths)
	if err != nil {
		return Result{Action: "unavailable", Gap: gap(req, "roots_cache_unreadable")}
	}
	d := project.Decide(req.Env, settings, roots, in.Cwd)

	// Recall is independent of capture: a registered project gets its recorded instructions even
	// while capture is paused or off. Excluded and unregistered folders get nothing.
	recall := recallContext(req, in, d)

	switch d.Action {
	case project.ActionCapture, project.ActionAutoRegister:
	case project.ActionRemind:
		if req.HookEvent != "SessionStart" {
			return Result{Action: d.Action}
		}
		msg := "SessionKeep: this folder is not registered, so nothing is being saved. To start remembering it, run: sessionkeep project add . --capture conversation"
		if strings.HasPrefix(d.Reason, "folder is too broad") {
			msg = "SessionKeep: " + d.Reason + ". Nothing is being saved here."
		}
		out, _ := json.Marshal(map[string]string{"systemMessage": msg})
		return Result{Action: d.Action, Stdout: append(out, '\n')}
	default:
		return Result{Action: d.Action, Stdout: recall}
	}
	if settings.DeviceID == "" {
		return Result{Action: "unavailable", Stdout: recall, Gap: gap(req, "no_device_id")}
	}

	ev := Event{
		Version: 1, EventID: req.NewID(), Agent: "claude-code", AdapterVersion: AdapterVersion,
		DeviceID: settings.DeviceID, ProjectID: d.ProjectID, Level: d.Level, SessionID: in.SessionID,
		Kind: kind, HookEvent: req.HookEvent, ObservedAt: req.Now.UTC().Format(TimeFormat),
		Cwd: in.Cwd, Payload: map[string]any{}, Completeness: "complete",
		Redaction: Redaction{RuleVersion: redact.Version},
	}
	ev.SourceKey = "hook:" + ev.EventID
	if d.Action == project.ActionAutoRegister {
		ev.Register = &Register{Root: d.Root, Level: d.Level}
	}
	build(&ev, in)

	raw, err := json.Marshal(ev)
	if err != nil {
		return Result{Action: d.Action, Stdout: recall, Gap: gap(req, "encode_failed")}
	}
	name := fmt.Sprintf("%020d-%s.evt", req.Now.UnixNano(), ev.EventID)
	if err := writeDurable(req.Paths.SpoolDir, name, raw); err != nil {
		return Result{Action: d.Action, Stdout: recall, Gap: gap(req, "spool_write_failed")}
	}
	// A command that failed with an error an earlier session already met: say what was said then.
	if s, _ := ev.Payload["error"].(string); req.HookEvent == "PostToolUseFailure" && d.ProjectID != "" && (in.IsInterrupt == nil || !*in.IsInterrupt) {
		if note := instruct.KnownErrorNote(req.Paths, d.ProjectID, in.SessionID, in.ToolName, s); note != "" {
			if out, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": req.HookEvent, "additionalContext": note}}); err == nil {
				recall = append(out, '\n')
			}
		}
	}
	// Keep the exchange for the hook path too, so it can be given back after a compaction.
	if d.ProjectID != "" {
		if s, _ := ev.Payload["prompt"].(string); req.HookEvent == "UserPromptSubmit" {
			instruct.NoteTurn(req.Paths, in.SessionID, d.ProjectID, "user", s, req.Now)
		}
		if s, _ := ev.Payload["text"].(string); req.HookEvent == "Stop" {
			instruct.NoteTurn(req.Paths, in.SessionID, d.ProjectID, "assistant", s, req.Now)
		}
	}
	return Result{Action: d.Action, Stdout: recall, SpoolFile: filepath.Join(req.Paths.SpoolDir, name)}
}

// recallContext returns the hook response carrying the user's recorded instructions, or nil.
// SessionStart always sends them (that also covers resume and the restart after compaction).
// UserPromptSubmit sends them only when the set changed since it was last sent in this session.
// A start that follows a compaction also gets the session's last exchanges word for word, when
// the project saves text: the model is otherwise left with a summary of them only.
func recallContext(req Request, in hookInput, d project.Decision) []byte {
	session := in.SessionID
	if req.HookEvent != "SessionStart" && req.HookEvent != "UserPromptSubmit" {
		return nil
	}
	if d.ProjectID == "" && d.Action != project.ActionAutoRegister {
		return nil
	}
	respond := func(text string) []byte {
		out, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": req.HookEvent, "additionalContext": text}})
		if err != nil {
			return nil
		}
		return append(out, '\n')
	}
	cache, err := instruct.Load(req.Paths, d.ProjectID, true)
	if err != nil {
		gap(req, "instruction_cache_unreadable")
		if req.HookEvent != "SessionStart" {
			return nil
		}
		// Do not let a damaged file look like "the user recorded nothing".
		return respond("SessionKeep is a local memory tool this user installed. The user's recorded instructions could not be read in this session (the local cache file is damaged). Their content is not known here; the user can repair it by running: sessionkeep setup")
	}
	text := ""
	if cache != nil {
		digest := instruct.SetDigest(cache.Items)
		if req.HookEvent == "SessionStart" || !instruct.AlreadySent(req.Paths, session, digest) {
			// The note about stale researched facts is given once, when the session starts.
			var at time.Time
			if req.HookEvent == "SessionStart" {
				at = req.Now
			}
			if text, _ = instruct.Envelope(cache, at); text != "" {
				instruct.MarkSent(req.Paths, session, digest)
			}
		}
	}
	// Automatic recall, where the owner chose it for the project: what the latest sessions were
	// about at every start, and with every request the stored pieces that share its words.
	if d.Recall == project.RecallAutomatic && d.ProjectID != "" {
		switch {
		case req.HookEvent == "SessionStart" && cache != nil:
			text = instruct.WithHistory(cache, session, text)
		case req.HookEvent == "UserPromptSubmit":
			if note := instruct.RelatedNote(req.Paths, d.ProjectID, session, in.Prompt, instruct.Budget-instruct.Units(text)-1); note != "" {
				text = strings.TrimPrefix(text+"\n"+note, "\n")
			}
		}
	}
	savesText := d.Level == config.LevelConversation || d.Level == config.LevelExtended
	if req.HookEvent == "SessionStart" && in.Source == "compact" && d.Action == project.ActionCapture && savesText {
		text = instruct.WithRecent(req.Paths, session, d.ProjectID, text)
	}
	if text == "" {
		return nil
	}
	return respond(text)
}

// build fills the payload according to the capture level.
//
//	metadata      identifiers, tool names, durations, sizes — no text
//	conversation  + the user's prompt, the assistant's final message, compaction summary,
//	              allowlisted tool input fields, tool error text
//	extended      + a bounded snippet of tool output
func build(ev *Event, in hookInput) {
	p := ev.Payload
	set := func(k, v string) {
		if v != "" {
			p[k] = v
		}
	}
	// cut shortens v to at most limit bytes on a character boundary and records what was dropped.
	cut := func(v string, limit int) string {
		if len(v) <= limit {
			return v
		}
		at := limit
		for at > 0 && !utf8.RuneStart(v[at]) {
			at--
		}
		ev.DroppedBytes += len(v) - at
		ev.Completeness = "truncated"
		return v[:at]
	}
	// Text is redacted before it is cut to the limit: cutting first could leave the start of a
	// secret too short for any rule to recognise. Only a bounded margin beyond the limit is
	// redacted, so a huge output costs no more than a long one.
	text := func(k, v string, limit int) {
		if v == "" {
			return
		}
		out, n := redact.Text(cut(v, limit+redactMargin))
		ev.Redaction.Count += n
		ev.Redaction.Applied = ev.Redaction.Applied || n > 0
		p[k] = cut(out, limit)
	}
	withText := ev.Level == config.LevelConversation || ev.Level == config.LevelExtended

	set("prompt_id", in.PromptID)
	set("source", in.Source)
	set("tool_name", in.ToolName)
	set("tool_use_id", in.ToolUseID)
	set("reason", in.Reason)
	set("trigger", in.Trigger)
	if in.DurationMS != nil {
		p["duration_ms"] = *in.DurationMS
	}
	if in.IsInterrupt != nil {
		p["is_interrupt"] = *in.IsInterrupt
	}
	if in.StopHookActive != nil {
		p["stop_hook_active"] = *in.StopHookActive
	}
	if ev.HookEvent == "StopFailure" {
		set("error_type", in.Error) // a fixed set of type names, not free text
	}
	for k, v := range map[string]string{"prompt_chars": in.Prompt, "text_chars": in.LastAssistantMessage, "summary_chars": in.CompactSummary} {
		if v != "" {
			p[k] = utf8.RuneCountInString(v)
		}
	}
	if !withText {
		return
	}

	set("transcript_path", in.TranscriptPath)
	text("prompt", in.Prompt, MaxTextBytes)
	if ev.HookEvent == "Stop" {
		text("text", in.LastAssistantMessage, MaxTextBytes)
	}
	text("compact_summary", in.CompactSummary, MaxTextBytes)
	if ev.HookEvent == "PostToolUseFailure" {
		text("error", in.Error, maxSnippetBytes)
	}
	// The output of a tool that read a file which commonly holds secrets is not kept at all.
	withheld := false
	for _, k := range []string{"file_path", "path", "notebook_path"} {
		if s, ok := in.ToolInput[k].(string); ok && SensitiveFile(s) {
			withheld = true
		}
	}
	if c, ok := in.ToolInput["command"].(string); ok && mentionsSensitiveFile(c) {
		withheld = true
	}
	if len(in.ToolInput) > 0 {
		kept := map[string]any{}
		for _, k := range toolInputKeys {
			if s, ok := in.ToolInput[k].(string); ok && s != "" {
				saved := p
				p = kept
				text(k, s, maxSnippetBytes)
				p = saved
			}
		}
		if len(kept) > 0 {
			p["tool_input"] = kept
		}
	}
	if ev.Level == config.LevelExtended && len(in.ToolResponse) > 0 && string(in.ToolResponse) != "null" {
		if withheld {
			p["tool_response_withheld"] = "the tool read a file that commonly holds secrets"
		} else {
			text("tool_response", readable(in.ToolResponse), maxSnippetBytes)
		}
	}
}

// redactMargin is how far beyond a field's limit text is redacted before it is cut.
const redactMargin = 4 << 10

// readable turns a tool response into plain text: its strings, one after another. Redaction must
// see the text itself; in the JSON form a quoted value arrives as \"...\", which the rules for
// quoted values do not recognise. Numbers and true/false are left out.
func readable(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	var b strings.Builder
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			if t != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(t)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k])
			}
		}
	}
	walk(v)
	return b.String()
}

// SensitiveFile reports whether a path names a file that commonly holds secrets: .env files
// (except examples), private keys and certificates with keys, SSH keys, credential and token
// files, service-account JSON, Terraform variables and state. SessionKeep keeps no tool output
// from reading one, at any level.
func SensitiveFile(p string) bool {
	base := strings.ToLower(p)
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env") {
		for _, safe := range []string{"example", "sample", "template", "dist", "defaults"} {
			if strings.HasSuffix(base, "."+safe) {
				return false
			}
		}
		return true
	}
	switch base {
	case ".envrc", ".netrc", "_netrc", ".npmrc", ".pypirc", ".pgpass", ".git-credentials", ".dockercfg", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
		"secrets.json", "secrets.yml", "secrets.yaml", "terraform.tfstate", "terraform.tfstate.backup":
		return true
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".jks", ".keystore", ".kdbx", ".tfvars", ".tfstate", ".ppk"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	if strings.HasPrefix(base, "credentials") || (strings.HasSuffix(base, ".json") && (strings.Contains(base, "service-account") || strings.Contains(base, "serviceaccount") || strings.Contains(base, "service_account"))) {
		return true
	}
	return false
}

// mentionsSensitiveFile reports whether a shell command names such a file, as in `cat .env`.
func mentionsSensitiveFile(cmd string) bool {
	for _, w := range strings.FieldsFunc(cmd, func(r rune) bool { return strings.ContainsRune(" \t\r\n\"'`;|&<>(),=", r) }) {
		if SensitiveFile(w) {
			return true
		}
	}
	return false
}

// writeDurable writes name into dir so it is either fully present or absent: temp file, flush to
// disk, rename. It does not create dir; setup does that.
func writeDurable(dir, name string, data []byte) error {
	tmp := filepath.Join(dir, "."+name+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// GapLogName is the file, in the data directory, that records events which could not be captured.
// Lines hold a time, the hook event and a reason code — never content.
const GapLogName = "capture-gaps.jsonl"

// gap appends one line to the gap log and returns the reason. If even that fails, the reason is
// still returned to the caller; there is nowhere left to record it.
func gap(req Request, reason string) string {
	line, _ := json.Marshal(map[string]string{"at": req.Now.UTC().Format(TimeFormat), "hook_event": req.HookEvent, "reason": reason})
	if f, err := os.OpenFile(filepath.Join(req.Paths.DataDir, GapLogName), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err == nil {
		f.Write(append(line, '\n'))
		f.Close()
	}
	return reason
}
