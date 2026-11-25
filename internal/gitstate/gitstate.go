// Package gitstate reads where a working tree stands, without changing anything.
//
// Every git invocation uses --no-optional-locks so that even the index refresh `git status`
// normally performs is skipped: SessionKeep must be able to look at a repository in the middle of
// someone's work and leave no trace.
//
// Nor may reading a repository run a program named by the repository itself. A copied or
// unzipped project can carry a .git/config that sets core.fsmonitor, a filter or textconv
// driver, an external diff or signature checking, each of which makes git run a command; every
// invocation switches those off (see guard and guarded).
package gitstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// State is a snapshot of a working tree. Repo is false when the directory is not inside a Git
// repository or git is not available; Reason then says why and the other fields are empty.
type State struct {
	Repo   bool   `json:"repo"`
	Reason string `json:"reason,omitempty"`
	Branch string `json:"branch,omitempty"` // "HEAD" when detached
	Head   string `json:"head,omitempty"`   // full commit id, empty before the first commit
	// Changed is the number of entries `git status` lists (modified, staged, deleted, untracked).
	Changed int `json:"changed_files"`
	// Dirty identifies the uncommitted state: a hash over the status listing, the diff against
	// HEAD and the content of untracked files. It is empty when the tree is clean.
	Dirty string `json:"dirty_digest,omitempty"`
}

// guard switches off the settings with which a repository could make git run a program. An
// empty core.fsmonitor means off in every git version; hooks do not run for the commands used
// here, and pointing them at the null device makes sure. Filter and diff drivers have names, so
// guarded switches them off one by one.
var guard = []string{
	"-c", "core.fsmonitor=",
	"-c", "core.hooksPath=" + os.DevNull,
	"-c", "diff.external=",
	"-c", "log.showSignature=false",
}

// errUnusualSettings is returned when a repository names a filter or diff driver that cannot be
// switched off on git's command line.
var errUnusualSettings = errors.New("this repository's git settings name a filter that SessionKeep cannot switch off, so its files were not read")

func gitRun(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// repo runs git in one folder with the guard and the overrides for that repository's drivers.
type repo struct {
	dir  string
	args []string
}

func (r repo) git(ctx context.Context, args ...string) ([]byte, error) {
	return gitRun(ctx, r.dir, append(append([]string{}, r.args...), args...)...)
}

// guarded prepares git for dir. Every filter driver (clean, smudge, process) and diff driver
// (textconv, command) that the repository's own settings define is overridden with an empty
// command. Drivers from the user's global or system settings, such as Git LFS, are left alone:
// the user installed those. Reading settings runs nothing.
func guarded(ctx context.Context, dir string) (repo, error) {
	r := repo{dir: dir, args: guard}
	const drivers = `^(filter|diff)\.`
	out, err := r.git(ctx, "config", "-z", "--show-scope", "--get-regexp", drivers)
	scoped := true
	if err != nil && !noMatch(err) {
		// git before 2.26 has no --show-scope: then every driver is switched off.
		scoped = false
		if out, err = r.git(ctx, "config", "-z", "--get-regexp", drivers); err != nil && !noMatch(err) {
			return r, fmt.Errorf("could not read the repository's git settings: %w", err)
		}
	}
	seen := map[string]bool{}
	entries := bytes.Split(out, []byte{0})
	for i := 0; i < len(entries); i++ {
		scope := "local"
		if scoped {
			scope = string(entries[i])
			if i++; i >= len(entries) {
				break
			}
		}
		key, _, _ := strings.Cut(string(entries[i]), "\n")
		if key == "" || scope == "global" || scope == "system" {
			continue
		}
		section, rest, _ := strings.Cut(key, ".")
		dot := strings.LastIndex(rest, ".")
		if dot <= 0 {
			continue // a variable directly under filter or diff is not a driver
		}
		name := rest[:dot]
		if strings.ContainsAny(name, "=\n") {
			return r, errUnusualSettings
		}
		if seen[section+"."+name] {
			continue
		}
		seen[section+"."+name] = true
		vars := []string{"textconv=", "command="}
		if section == "filter" {
			vars = []string{"clean=", "smudge=", "process=", "required=false"}
		}
		for _, v := range vars {
			r.args = append(r.args, "-c", section+"."+name+"."+v)
		}
	}
	return r, nil
}

// noMatch reports git config's exit status 1, which means nothing matched.
func noMatch(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 1
}

// Read inspects dir. It never returns an error: anything that prevents reading becomes Reason.
func Read(dir string) State {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := exec.LookPath("git"); err != nil {
		return State{Reason: "git is not installed or not on PATH"}
	}
	g := repo{dir: dir, args: guard}
	if out, err := g.git(ctx, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return State{Reason: "not a Git repository"}
	}
	s := State{Repo: true}
	if out, err := g.git(ctx, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		s.Branch = strings.TrimSpace(string(out))
	} else if out, err := g.git(ctx, "symbolic-ref", "--short", "HEAD"); err == nil {
		s.Branch = strings.TrimSpace(string(out)) // a repository with no commit yet
	}
	if out, err := g.git(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		s.Head = strings.TrimSpace(string(out))
	}
	g, err := guarded(ctx, dir)
	if err != nil {
		s.Reason = err.Error() + "; uncommitted state unknown"
		return s
	}
	// Submodules are not entered: their own settings are not checked here.
	status, err := g.git(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		s.Reason = "git status failed; uncommitted state unknown"
		return s
	}
	for _, e := range bytes.Split(status, []byte{0}) {
		if len(e) > 3 {
			s.Changed++
		}
	}
	if s.Changed > 0 {
		h := sha256.New()
		h.Write(status)
		if s.Head != "" {
			diff, _ := g.git(ctx, "diff", "--no-ext-diff", "--no-textconv", "--ignore-submodules=all", "HEAD")
			h.Write(diff)
		}
		hashUntracked(ctx, g, h)
		s.Dirty = hex.EncodeToString(h.Sum(nil))
	}
	return s
}

// Limits for fingerprinting untracked files, so a folder full of build output cannot stall a hook
// or a command. Beyond them a file contributes its size and modification time instead of content.
const (
	maxUntrackedFiles = 2000
	maxUntrackedBytes = 4 << 20
)

// hashUntracked adds the content of untracked, non-ignored files to h. `git diff` does not cover
// them, and "I rewrote the new file" is exactly the kind of change a checkpoint must notice.
func hashUntracked(ctx context.Context, g repo, h hash.Hash) {
	top, err := g.git(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return
	}
	root := strings.TrimSpace(string(top))
	list, err := repo{dir: root, args: g.args}.git(ctx, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return
	}
	for i, name := range bytes.Split(list, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(string(name)))
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		h.Write(name)
		if i >= maxUntrackedFiles || fi.Size() > maxUntrackedBytes {
			fmt.Fprintf(h, "\x00%d\x00%d\x00", fi.Size(), fi.ModTime().UnixNano())
			continue
		}
		if raw, err := os.ReadFile(path); err == nil {
			sum := sha256.Sum256(raw)
			h.Write(sum[:])
		}
	}
}

// Compare lists, in plain words, how now differs from then. An empty result means nothing that
// SessionKeep tracks has changed.
func Compare(then, now State) []string {
	var w []string
	switch {
	case !then.Repo && !now.Repo:
		return nil
	case then.Repo && !now.Repo:
		return []string{"the checkpoint was taken in a Git repository, but this folder is not one now (" + now.Reason + ")"}
	case !then.Repo && now.Repo:
		return []string{"this folder is a Git repository now; it was not when the checkpoint was taken"}
	}
	if then.Branch != now.Branch {
		w = append(w, "branch changed: checkpoint was on "+quote(then.Branch)+", now on "+quote(now.Branch))
	}
	if then.Head != now.Head {
		w = append(w, "commit changed: checkpoint was at "+short(then.Head)+", now at "+short(now.Head))
	}
	switch {
	case then.Dirty == now.Dirty:
	case then.Dirty == "":
		w = append(w, "there are uncommitted changes now; the tree was clean at the checkpoint")
	case now.Dirty == "":
		w = append(w, "the tree is clean now; there were uncommitted changes at the checkpoint (committed, stashed or discarded since)")
	default:
		w = append(w, "the uncommitted changes differ from those at the checkpoint")
	}
	return w
}

func quote(s string) string {
	if s == "" {
		return "(none)"
	}
	return `"` + s + `"`
}

func short(h string) string {
	if h == "" {
		return "(no commit)"
	}
	if len(h) > 10 {
		return h[:10]
	}
	return h
}

// Commit is one commit, as listed for a span of time.
type Commit struct {
	ID      string `json:"id"`
	At      string `json:"at"`
	Subject string `json:"subject"`
}

// Commits lists the commits reachable from HEAD that were made in [since, until), oldest first,
// at most limit. It returns an empty list where there is no repository or git is unavailable.
func Commits(dir string, since, until time.Time, limit int) []Commit {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out := []Commit{}
	g := repo{dir: dir, args: guard}
	raw, err := g.git(ctx, "log", "--no-merges", "--no-show-signature", fmt.Sprintf("--max-count=%d", limit*4),
		"--since="+since.Add(-time.Second).UTC().Format(time.RFC3339), "--until="+until.UTC().Format(time.RFC3339), "--format=%h%x09%cI%x09%s")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		at, err := time.Parse(time.RFC3339, f[1])
		if err != nil || at.Before(since) || !at.Before(until) {
			continue
		}
		out = append([]Commit{{ID: f[0], At: at.UTC().Format(time.RFC3339), Subject: f[2]}}, out...)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// CommitInfo returns one commit and the files it changed (paths from the repository root).
// rev must already be checked by the caller to be a plain revision name.
func CommitInfo(dir, rev string) (Commit, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	g, err := guarded(ctx, dir)
	if err != nil {
		return Commit{}, nil, err
	}
	raw, err := g.git(ctx, "show", "--no-patch", "--no-show-signature", "--format=%h%x09%cI%x09%s", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return Commit{}, nil, err
	}
	f := strings.SplitN(strings.TrimSpace(string(raw)), "\t", 3)
	if len(f) != 3 {
		return Commit{}, nil, fmt.Errorf("unexpected answer from git")
	}
	at, err := time.Parse(time.RFC3339, f[1])
	if err != nil {
		return Commit{}, nil, err
	}
	c := Commit{ID: f[0], At: at.UTC().Format(time.RFC3339), Subject: f[2]}
	names, err := g.git(ctx, "show", "--no-renames", "--no-show-signature", "--name-only", "--format=", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return c, nil, err
	}
	files := []string{}
	for _, n := range strings.Split(string(names), "\n") {
		if n = strings.TrimSpace(n); n != "" {
			files = append(files, n)
		}
	}
	return c, files, nil
}

// BlameLine returns the commit that last changed one line of a file.
func BlameLine(dir, file string, line int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	g, err := guarded(ctx, dir)
	if err != nil {
		return "", err
	}
	raw, err := g.git(ctx, "blame", "--porcelain", "--no-textconv", fmt.Sprintf("-L%d,%d", line, line), "--", file)
	if err != nil {
		return "", err
	}
	id, _, _ := strings.Cut(string(raw), " ")
	if len(id) < 40 || strings.Trim(id, "0") == "" {
		return "", fmt.Errorf("the line is not committed yet")
	}
	return id, nil
}
