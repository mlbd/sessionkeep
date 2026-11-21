// Package gitstate reads where a working tree stands, without changing anything.
//
// Every git invocation uses --no-optional-locks so that even the index refresh `git status`
// normally performs is skipped: SessionKeep must be able to look at a repository in the middle of
// someone's work and leave no trace.
package gitstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// Read inspects dir. It never returns an error: anything that prevents reading becomes Reason.
func Read(dir string) State {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := exec.LookPath("git"); err != nil {
		return State{Reason: "git is not installed or not on PATH"}
	}
	if out, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return State{Reason: "not a Git repository"}
	}
	s := State{Repo: true}
	if out, err := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		s.Branch = strings.TrimSpace(string(out))
	} else if out, err := git(ctx, dir, "symbolic-ref", "--short", "HEAD"); err == nil {
		s.Branch = strings.TrimSpace(string(out)) // a repository with no commit yet
	}
	if out, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		s.Head = strings.TrimSpace(string(out))
	}
	status, err := git(ctx, dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
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
			diff, _ := git(ctx, dir, "diff", "--no-ext-diff", "--no-textconv", "HEAD")
			h.Write(diff)
		}
		hashUntracked(ctx, dir, h)
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
func hashUntracked(ctx context.Context, dir string, h hash.Hash) {
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return
	}
	root := strings.TrimSpace(string(top))
	list, err := git(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
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
	raw, err := git(ctx, dir, "log", "--no-merges", fmt.Sprintf("--max-count=%d", limit*4),
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
