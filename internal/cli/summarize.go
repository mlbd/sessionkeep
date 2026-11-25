package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sessionkeep/internal/capture"
	"sessionkeep/internal/config"
	"sessionkeep/internal/lease"
	"sessionkeep/internal/redact"
	"sessionkeep/internal/store"
	"sessionkeep/internal/summarize"
)

const (
	defaultDailySummaries = 20
	// A session with less text than this is not worth a request.
	minSummaryChars = 200
	// One background run writes at most this many summaries.
	autoBatch = 5
	// An open session that has been silent this long is treated as over.
	summaryIdle = 6 * time.Hour
	// After a background attempt that did not finish cleanly, the next one waits this long.
	autoBackoff = 30 * time.Minute
	// EnvClaudeBin overrides where the Claude Code program is (absolute path).
	EnvClaudeBin = "SESSIONKEEP_CLAUDE_BIN"
)

type summarizeStatus struct {
	Enabled    bool   `json:"enabled"`
	Auto       bool   `json:"auto"`
	Model      string `json:"model"`
	DailyLimit int    `json:"daily_limit"`
	AskedToday int    `json:"requests_today"`
	Total      int    `json:"summaries"`
	Waiting    int    `json:"sessions_waiting"`
	Program    string `json:"program,omitempty"`
	Account    string `json:"account"`
	AccountSet bool   `json:"account_decided"`
}

type summarizeReport struct {
	Written  int                      `json:"written"`
	Skipped  int                      `json:"skipped_changed_meanwhile"`
	Failed   int                      `json:"failed"`
	Errors   []string                 `json:"errors,omitempty"`
	Waiting  []store.SummaryCandidate `json:"sessions,omitempty"` // with --dry-run: what would be sent
	Model    string                   `json:"model"`
	LimitHit bool                     `json:"daily_limit_reached"`
}

func summaryModel(s config.Settings) string {
	if s.Summaries.Model != "" {
		return s.Summaries.Model
	}
	return summarize.DefaultModel
}

func summaryLimit(s config.Settings) int {
	if s.Summaries.DailyLimit > 0 {
		return s.Summaries.DailyLimit
	}
	return defaultDailySummaries
}

func claudeProgram(getenv func(string) string) (string, error) {
	if v := getenv(EnvClaudeBin); v != "" {
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("%s must be an absolute path", EnvClaudeBin)
		}
		return v, nil
	}
	p, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New("the Claude Code program (`claude`) was not found on PATH")
	}
	return filepath.Abs(p)
}

// The daily limit counts requests made, not summaries kept: a failed request and a replaced
// summary used the plan's allowance too. The count lives in a small file beside the caches.
type requestCount struct {
	Day      string `json:"day"`
	Requests int    `json:"requests"`
}

func requestsPath(st state) string { return filepath.Join(st.paths.CacheDir, "summarize.count") }

func requestsToday(st state, now time.Time) int {
	var c requestCount
	raw, err := os.ReadFile(requestsPath(st))
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Day != now.UTC().Format("2006-01-02") {
		return 0
	}
	return c.Requests
}

// countRequest records one request before it is made. If the count cannot be saved the request
// is not made, so a broken file can never mean unlimited requests.
func countRequest(st state, now time.Time) error {
	raw, _ := json.Marshal(requestCount{Day: now.UTC().Format("2006-01-02"), Requests: requestsToday(st, now) + 1})
	return os.WriteFile(requestsPath(st), raw, 0o600)
}

func openRead(ctx context.Context, st state) (*store.Store, error) {
	s, err := store.OpenReadOnly(ctx, st.paths.DBPath)
	if errors.Is(err, store.ErrNeedsSetup) || errors.Is(err, store.ErrSchemaTooNew) {
		return nil, &cmdError{ExitUnavailable, "database_unavailable", err.Error()}
	}
	return s, err
}

// summarizeCmd is `sessionkeep summarize status|on|off|run|delete`.
func summarizeCmd(inv invocation, rest []string) error {
	usageText := `usage: sessionkeep summarize status
       sessionkeep summarize on [--model MODEL] [--daily-limit N] [--manual] [--yes]
       sessionkeep summarize off
       sessionkeep summarize run [--project ID_OR_PATH] [--limit N] [--dry-run]
       sessionkeep summarize delete [--project ID_OR_PATH] [--yes]`
	if len(rest) != 1 {
		return usageErr("%s", usageText)
	}
	switch rest[0] {
	case "status":
		if err := inv.only(); err != nil {
			return err
		}
		st, err := load(inv, false)
		if err != nil {
			return err
		}
		out := summarizeStatus{Enabled: st.settings.Summaries.Enabled, Auto: st.settings.Summaries.Auto, Model: summaryModel(st.settings), DailyLimit: summaryLimit(st.settings)}
		out.Program, _ = claudeProgram(inv.getenv)
		out.AskedToday = requestsToday(st, time.Now())
		acct, decided := effectiveAccount(inv, st)
		out.Account, out.AccountSet = acct.Name, decided
		ctx := context.Background()
		if s, err := openRead(ctx, st); err == nil {
			defer s.Close()
			if out.Total, err = s.SummaryCount(ctx); err != nil {
				return err
			}
			waiting, err := s.SessionsToSummarize(ctx, "", time.Now().Add(-summaryIdle).UTC().Format(capture.TimeFormat), 2, minSummaryChars, 1000)
			if err != nil {
				return err
			}
			out.Waiting = len(waiting)
		}
		state := "off (no session text is given to a model)"
		if out.Enabled {
			state = "on, written in the background after sessions end"
			if !out.Auto {
				state = "on, written only when you run `sessionkeep summarize run`"
			}
		}
		acctWords := out.Account
		if !out.AccountSet {
			acctWords = "not chosen: whichever account a session runs with pays (here: " + out.Account + "); choose with `sessionkeep account use NAME`"
		}
		prog := out.Program
		if prog == "" {
			prog = "not found on PATH"
		}
		return inv.emit(out, fmt.Sprintf("summaries: %s\nmodel: %s; at most %d request(s) a day (%d made today)\nstored: %d; finished sessions without a current summary: %d\nClaude Code program: %s\nClaude Code account: %s\n",
			state, out.Model, out.DailyLimit, out.AskedToday, out.Total, out.Waiting, prog, acctWords))

	case "on":
		if err := inv.only("model", "daily-limit", "manual", "yes"); err != nil {
			return err
		}
		st, err := load(inv, true)
		if err != nil {
			return err
		}
		acct, decided := effectiveAccount(inv, st)
		if !decided {
			var names []string
			for _, a := range accounts(inv, st) {
				names = append(names, a.Name)
			}
			return failErr("account_not_chosen", "this computer has more than one Claude Code account (%s). Say whose plan pays for summaries first: sessionkeep account use NAME", strings.Join(names, ", "))
		}
		next := st.settings
		next.Summaries.Enabled = true
		next.Summaries.Auto = inv.flags["manual"] == ""
		if v := inv.flags["model"]; v != "" {
			if !summarize.ValidModel(v) {
				return usageErr("--model must be a model name or alias such as sonnet or claude-sonnet-5-5 (letters, digits and . _ : @ / [ ] -, starting with a letter or digit); got %q", v)
			}
			next.Summaries.Model = v
		}
		if v := inv.flags["daily-limit"]; v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > config.MaxDailySummaries {
				return usageErr("--daily-limit must be a number from 1 to %d (got %q)", config.MaxDailySummaries, v)
			}
			next.Summaries.DailyLimit = n
		}
		if err := next.Validate(); err != nil {
			return usageErr("%v", err)
		}
		when := "in the background, by itself, after a session has ended"
		if !next.Summaries.Auto {
			when = "only when you run `sessionkeep summarize run`"
		}
		text := fmt.Sprintf(`Turning summaries on means:
  - When a session is over, SessionKeep gives its stored text (secrets already removed) to the Claude
    Code program on this computer, which sends it to Anthropic under your own sign-in. SessionKeep
    holds no key and connects to nothing itself.
  - Model: %s. Each summary is one request and uses the plan of the Claude Code account
    "%s". At most %d a day.
  - Written %s.
  - Only projects that save conversation text and are not paused. Excluded folders hold no text.
  - The model gets no tools. Its summary is stored on this computer, marked as written by a model.
    It never replaces the original text and is never treated as an instruction.
`, summaryModel(next), acct.Name, summaryLimit(next), when)
		if inv.flags["yes"] == "" {
			inv.emit(map[string]any{"changed": false, "would_set": next.Summaries}, text+"NOTHING CHANGED. Run the same command with --yes to turn summaries on.\n")
			return &cmdError{ExitNeedsConsent, "confirmation_required", "summaries are still off; add --yes to confirm"}
		}
		if err := config.SaveSettings(st.paths, next); err != nil {
			return err
		}
		return inv.emit(map[string]any{"changed": true, "summaries": next.Summaries}, text+"Summaries are ON. Turn them off with `sessionkeep summarize off`.\n")

	case "off":
		if err := inv.only(); err != nil {
			return err
		}
		st, err := load(inv, true)
		if err != nil {
			return err
		}
		st.settings.Summaries.Enabled, st.settings.Summaries.Auto = false, false
		if err := config.SaveSettings(st.paths, st.settings); err != nil {
			return err
		}
		return inv.emit(map[string]any{"changed": true, "summaries": st.settings.Summaries},
			"Summaries are OFF. No session text will be given to a model. Summaries already written are kept; remove them with `sessionkeep summarize delete`.\n")

	case "run":
		if inv.flags["detached"] != "" { // how the background drain starts this program; not for use by hand
			summarizeDetached(inv)
			return nil
		}
		if err := inv.only("project", "limit", "dry-run"); err != nil {
			return err
		}
		st, err := load(inv, false)
		if err != nil {
			return err
		}
		limit := 0
		if v := inv.flags["limit"]; v != "" {
			if limit, err = strconv.Atoi(v); err != nil || limit < 1 {
				return usageErr("--limit must be a positive number (got %q)", v)
			}
		}
		rep, err := summarizeRun(inv, st, inv.flags["project"], limit, inv.flags["dry-run"] != "")
		if err != nil {
			return err
		}
		var b strings.Builder
		if inv.flags["dry-run"] != "" {
			fmt.Fprintf(&b, "Would summarise %d session(s) with model %s:\n", len(rep.Waiting), rep.Model)
			for _, c := range rep.Waiting {
				fmt.Fprintf(&b, "  %.8s  %.16s  %3d text(s)  %s\n", c.SessionID, strings.Replace(c.LastAt, "T", " ", 1), c.Texts, c.ProjectName)
			}
			b.WriteString("NOTHING SENT. Run without --dry-run to write them.\n")
		} else {
			fmt.Fprintf(&b, "Summaries written: %d (model %s). Sessions that changed meanwhile and wait for the next run: %d. Failed: %d.\n", rep.Written, rep.Model, rep.Skipped, rep.Failed)
			for _, e := range rep.Errors {
				fmt.Fprintf(&b, "  failed: %s\n", e)
			}
		}
		if rep.LimitHit {
			b.WriteString("The daily limit is reached; the rest wait for tomorrow or a higher --daily-limit.\n")
		}
		if err := inv.emit(rep, b.String()); err != nil {
			return err
		}
		if rep.Failed > 0 && rep.Written == 0 {
			return failErr("summaries_failed", "no summary could be written; the original text is untouched")
		}
		return nil

	case "delete":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
		st, err := load(inv, false)
		if err != nil {
			return err
		}
		if inv.flags["yes"] == "" {
			inv.emit(map[string]any{"deleted": false}, "This deletes the model-written summaries (of the named project, or of every project). The original text is not touched.\nNOTHING DELETED. Add --yes to confirm.\n")
			return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was deleted; add --yes to confirm"}
		}
		n := 0
		err = withStore(st, func(ctx context.Context, s *store.Store) error {
			id := ""
			if key := inv.flags["project"]; key != "" {
				p, err := resolveProject(ctx, inv, st, s, key)
				if err != nil {
					return err
				}
				id = p.ID
			}
			n, err = s.DeleteSummaries(ctx, id)
			return err
		})
		if err != nil {
			return err
		}
		return inv.emit(map[string]any{"deleted": true, "summaries": n}, fmt.Sprintf("Deleted %d summary(ies). The original text is untouched.\n", n))
	}
	return usageErr("%s", usageText)
}

// summarizeRun writes summaries for finished sessions that have none, newest first, within the
// daily limit. The model runs without the database lease held, so capture is never kept waiting;
// the lease is taken only for the moment each summary is stored.
func summarizeRun(inv invocation, st state, projectKey string, limit int, dry bool) (summarizeReport, error) {
	rep := summarizeReport{Model: summaryModel(st.settings)}
	if !st.settings.Summaries.Enabled {
		return rep, failErr("summaries_off", "summaries are off; `sessionkeep summarize on` explains what turning them on means")
	}
	ctx := context.Background()
	// One summariser at a time, so two processes never pay for the same session.
	lock, err := lease.Acquire(filepath.Join(st.paths.CacheDir, "summarize.lock"))
	if errors.Is(err, lease.ErrHeld) {
		return rep, &cmdError{ExitUnavailable, "busy", "another SessionKeep process is already writing summaries"}
	}
	if err != nil {
		return rep, err
	}
	defer lock.Release()

	s, err := openRead(ctx, st)
	if err != nil {
		return rep, err
	}
	defer s.Close()
	projectID := ""
	if projectKey != "" {
		p, err := resolveProject(ctx, inv, st, s, projectKey)
		if err != nil {
			return rep, err
		}
		projectID = p.ID
	}
	dailyLeft := summaryLimit(st.settings) - requestsToday(st, time.Now())
	budget := dailyLeft
	if limit > 0 && limit < budget {
		budget = limit
	}
	candidates, err := s.SessionsToSummarize(ctx, projectID, time.Now().Add(-summaryIdle).UTC().Format(capture.TimeFormat), 2, minSummaryChars, 1000)
	if err != nil {
		return rep, err
	}
	if budget <= 0 {
		rep.LimitHit = len(candidates) > 0
		return rep, nil
	}
	if dry {
		rep.LimitHit = len(candidates) > dailyLeft
		if len(candidates) > budget {
			candidates = candidates[:budget]
		}
		rep.Waiting = candidates
		return rep, nil
	}
	bin, err := claudeProgram(inv.getenv)
	if err != nil {
		return rep, failErr("claude_not_found", "%v; summaries need Claude Code installed and signed in", err)
	}
	dir := filepath.Join(st.paths.CacheDir, "summarizer") // an empty folder for the program to start in
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return rep, err
	}
	acct, _ := effectiveAccount(inv, st)
	run := summarize.ClaudeCLI(bin, dir, accountEnv(acct, os.Environ()))

	asked := 0
	for _, c := range candidates {
		if asked >= budget {
			rep.LimitHit = asked >= dailyLeft
			break
		}
		if rep.Failed >= 2 { // most likely a limit or a sign-in problem: stop spending requests
			break
		}
		_, turns, err := s.SessionTurns(ctx, c.SessionID, c.ProjectID, false)
		if err != nil {
			return rep, err
		}
		var pieces []summarize.Piece
		for _, t := range turns {
			if t.Text != "" {
				pieces = append(pieces, summarize.Piece{Who: t.Who, At: t.ObservedAt, Text: t.Text})
			}
		}
		if len(pieces) != c.Texts { // the session changed since it was listed
			rep.Skipped++
			continue
		}
		input, used := summarize.BuildInput(pieces)
		if err := countRequest(st, time.Now()); err != nil {
			return rep, fmt.Errorf("cannot record the request count, so no request was made: %w", err)
		}
		asked++
		text, model, err := run(ctx, rep.Model, input)
		if err != nil {
			rep.Failed++
			rep.Errors = append(rep.Errors, fmt.Sprintf("session %.8s: %v", c.SessionID, err))
			continue
		}
		text, redactions := redact.Text(text)
		sum := store.Summary{ID: config.NewID(), ProjectID: c.ProjectID, SessionID: c.SessionID, Text: text, Provider: summarize.Provider, Model: model,
			PromptVersion: summarize.PromptVersion, SourceCount: len(pieces), SourceUsed: used, Redactions: redactions, CreatedAt: time.Now().UTC().Format(capture.TimeFormat)}
		err = withStore(st, func(ctx context.Context, w *store.Store) error { return w.PutSummary(ctx, sum) })
		if errors.Is(err, store.ErrSourceChanged) {
			rep.Skipped++ // the session moved on; it will be picked up again later
			continue
		}
		if err != nil {
			return rep, err
		}
		rep.Written++
	}
	return rep, nil
}

// spawnSummarizer starts this program again as a detached process that writes the summaries.
// It is a variable so tests can run that work in-process instead of starting the test binary.
var spawnSummarizer = func(getenv func(string) string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return startDetached(exe, []string{"summarize", "run", "--detached"}, os.Environ())
}

func attemptMarker(st state) string { return filepath.Join(st.paths.CacheDir, "summarize.attempt") }

// autoSummarize is called at the end of the background drain. It does nothing unless summaries
// are on and automatic, a finished session is waiting, today's limit is not used up and no
// recent attempt went wrong. Then it starts a detached process and returns at once: the host may
// end a background hook at any time, and a request cut short would be paid for and lost.
func autoSummarize(getenv func(string) string) {
	if getenv(summarize.GuardEnv) != "" {
		return
	}
	inv := invocation{flags: map[string]string{}, stdout: io.Discard, getenv: getenv}
	st, err := load(inv, false)
	if err != nil || !st.settings.Summaries.Enabled || !st.settings.Summaries.Auto {
		return
	}
	if fi, err := os.Stat(attemptMarker(st)); err == nil && time.Since(fi.ModTime()) < autoBackoff {
		return
	}
	if summaryLimit(st.settings)-requestsToday(st, time.Now()) <= 0 {
		return
	}
	ctx := context.Background()
	s, err := store.OpenReadOnly(ctx, st.paths.DBPath)
	if err != nil {
		return
	}
	waiting, err := s.SessionsToSummarize(ctx, "", time.Now().Add(-summaryIdle).UTC().Format(capture.TimeFormat), 2, minSummaryChars, 1)
	s.Close()
	if err != nil || len(waiting) == 0 {
		return
	}
	spawnSummarizer(getenv)
}

// summarizeDetached is the detached process. It prints nothing. A marker file is written before
// the first request and removed after a clean run, so an attempt that failed or was killed is
// not repeated for a while.
func summarizeDetached(inv invocation) {
	inv.stdout = io.Discard
	st, err := load(inv, false)
	if err != nil || !st.settings.Summaries.Enabled || !st.settings.Summaries.Auto {
		return
	}
	if err := os.WriteFile(attemptMarker(st), nil, 0o600); err != nil {
		return
	}
	if rep, err := summarizeRun(inv, st, "", autoBatch, false); err == nil && rep.Failed == 0 {
		os.Remove(attemptMarker(st))
	}
}
