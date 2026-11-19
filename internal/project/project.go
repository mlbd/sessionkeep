// Package project decides, for a working directory, whether SessionKeep may capture anything and
// under which project. The decision uses only the settings file and the roots cache, so the hook
// path can make it without opening the database.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sessionkeep/internal/config"
	"sessionkeep/internal/fsx"
)

// Root is one registered directory as the hook path sees it.
type Root struct {
	Path      string `json:"path"` // normalized
	ProjectID string `json:"project_id"`
	Level     string `json:"level"`
	Paused    bool   `json:"paused"`
}

// Actions a Decision can carry.
const (
	ActionCapture      = "capture"       // registered project, capture at Level
	ActionNone         = "none"          // capture nothing, say nothing
	ActionRemind       = "remind"        // capture nothing; tell the user the folder is not registered
	ActionAutoRegister = "auto_register" // owner enabled auto mode: register Root at Level, then capture
)

// Decision is what to do for one working directory.
type Decision struct {
	Action    string `json:"action"`
	ProjectID string `json:"project_id,omitempty"`
	Level     string `json:"level,omitempty"`
	Root      string `json:"root,omitempty"`
	Reason    string `json:"reason"`
}

// Normalize turns an absolute path into the comparison form for goos: one separator style, no
// "." or ".." segments, no trailing separator, and on Windows lower case without the \\?\ prefix.
//
// Limits: it is lexical. It does not resolve symlinks, junctions or 8.3 short names, treats macOS
// paths as case-sensitive, and uses Unicode lower-casing rather than NTFS's own case table.
func Normalize(goos, p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", errors.New("path contains a NUL byte")
	}
	if goos != "windows" {
		if !strings.HasPrefix(p, "/") {
			return "", fmt.Errorf("not an absolute path: %q", p)
		}
		return "/" + strings.Join(cleanSegments(strings.Split(p, "/")), "/"), nil
	}
	p = strings.ReplaceAll(p, "/", `\`)
	switch {
	case strings.HasPrefix(strings.ToUpper(p), `\\?\UNC\`):
		p = `\\` + p[8:]
	case strings.HasPrefix(p, `\\?\`):
		p = p[4:]
	}
	p = strings.ToLower(p)
	var prefix string
	switch {
	case len(p) >= 3 && p[1] == ':' && p[2] == '\\' && p[0] >= 'a' && p[0] <= 'z':
		prefix, p = p[:2], p[3:]
	case strings.HasPrefix(p, `\\`):
		parts := strings.SplitN(strings.TrimLeft(p, `\`), `\`, 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("incomplete UNC path: %q", p)
		}
		prefix = `\\` + parts[0] + `\` + parts[1]
		p = ""
		if len(parts) == 3 {
			p = parts[2]
		}
	default:
		return "", fmt.Errorf("not an absolute path: %q", p)
	}
	segs := cleanSegments(strings.Split(p, `\`))
	if len(segs) == 0 {
		return prefix + `\`, nil
	}
	return prefix + `\` + strings.Join(segs, `\`), nil
}

func cleanSegments(in []string) []string {
	out := in[:0:0]
	for _, s := range in {
		switch s {
		case "", ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, s)
		}
	}
	return out
}

// within reports whether normalized path p is dir or lies below it, on a segment boundary
// (so C:\work\app2 is not inside C:\work\app).
func within(goos, p, dir string) bool {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	if p == dir {
		return true
	}
	if !strings.HasSuffix(dir, sep) {
		dir += sep
	}
	return strings.HasPrefix(p, dir)
}

// depth counts the directory levels below the filesystem root of a normalized path: 0 for "/",
// `c:\` or `\server\share`, 1 for a top-level folder, and so on.
func depth(goos, p string) int {
	if goos != "windows" {
		return len(cleanSegments(strings.Split(p, "/")))
	}
	n := len(cleanSegments(strings.Split(p, `\`)))
	if strings.HasPrefix(p, `\`) {
		return n - 2 // server and share
	}
	return n - 1 // drive
}

// Env supplies the two facts Decide cannot derive from a path string.
type Env struct {
	GOOS string
	Home string // user's home directory, any form; never auto-registered
	// RepoRoot returns the enclosing repository root for a directory, or "" if there is none.
	RepoRoot func(dir string) string
}

// Decide applies the rules in order: invalid path, exclusion, registered root, then the auto-add
// mode. Exclusions win over everything, including a registered root.
func Decide(env Env, s config.Settings, roots []Root, cwd string) Decision {
	p, err := Normalize(env.GOOS, cwd)
	if err != nil {
		return Decision{Action: ActionNone, Reason: "working directory is not a usable absolute path"}
	}
	for _, ex := range s.Exclusions {
		if within(env.GOOS, p, ex) {
			return Decision{Action: ActionNone, Reason: "excluded by the user"}
		}
	}
	var best *Root
	for i := range roots {
		if within(env.GOOS, p, roots[i].Path) && (best == nil || len(roots[i].Path) > len(best.Path)) {
			best = &roots[i]
		}
	}
	if best != nil {
		switch {
		case best.Paused:
			return Decision{Action: ActionNone, ProjectID: best.ProjectID, Root: best.Path, Reason: "project is paused"}
		case best.Level == config.LevelOff || !config.ValidLevel(best.Level):
			return Decision{Action: ActionNone, ProjectID: best.ProjectID, Root: best.Path, Reason: "capture is off for this project"}
		}
		return Decision{Action: ActionCapture, ProjectID: best.ProjectID, Level: best.Level, Root: best.Path, Reason: "registered project"}
	}
	switch s.AutoAdd.Mode {
	case config.ModeAsk:
		return Decision{Action: ActionRemind, Reason: "folder is not registered"}
	case config.ModeAuto:
		root := p
		if env.RepoRoot != nil {
			if r, err := Normalize(env.GOOS, env.RepoRoot(cwd)); err == nil && within(env.GOOS, p, r) {
				root = r
			}
		}
		// A repository root above an excluded folder is fine; a root that IS excluded was handled above.
		home, _ := Normalize(env.GOOS, env.Home)
		if depth(env.GOOS, root) < 2 || (home != "" && within(env.GOOS, home, root)) {
			return Decision{Action: ActionRemind, Reason: "folder is too broad to add automatically (a drive root, a top-level folder, the home directory or above it); add it explicitly if intended"}
		}
		return Decision{Action: ActionAutoRegister, Level: s.AutoAdd.Level, Root: root, Reason: "auto-add is enabled by the user"}
	}
	return Decision{Action: ActionNone, Reason: "folder is not registered"}
}

// FindRepoRoot walks up from dir looking for a .git entry and returns the directory holding it,
// or "" when there is none. It reads directory metadata only.
func FindRepoRoot(dir string) string {
	for i := 0; i < 64; i++ {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

type rootsFile struct {
	Version   int    `json:"version"`
	WrittenAt string `json:"written_at"`
	Roots     []Root `json:"roots"`
}

// WriteRootsCache atomically replaces the hook-readable list of registered roots.
func WriteRootsCache(p config.Paths, roots []Root, now time.Time) error {
	sort.Slice(roots, func(i, j int) bool { return roots[i].Path < roots[j].Path })
	if roots == nil {
		roots = []Root{}
	}
	// Leave the file alone when nothing changed: hooks read it constantly, and every replace is a
	// moment in which a reader can collide with the writer.
	if current, err := ReadRootsCache(p); err == nil && current != nil && sameRoots(current, roots) {
		return nil
	}
	raw, err := json.MarshalIndent(rootsFile{Version: 1, WrittenAt: now.UTC().Format(time.RFC3339), Roots: roots}, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(p.RootsCachePath(), append(raw, '\n'))
}

// ReadRootsCache returns the registered roots. A missing file means none are registered. A file
// that cannot be parsed is an error: the caller must capture nothing rather than guess.
func ReadRootsCache(p config.Paths) ([]Root, error) {
	raw, err := fsx.ReadFile(p.RootsCachePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f rootsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("roots cache is not valid JSON: %w", err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("roots cache version %d is not supported", f.Version)
	}
	return f.Roots, nil
}

// Excluded reports whether dir is at or below one of the user's excluded folders. A path that
// cannot be normalized counts as excluded: when in doubt, keep nothing.
func Excluded(goos string, exclusions []string, dir string) bool {
	p, err := Normalize(goos, dir)
	if err != nil {
		return true
	}
	for _, ex := range exclusions {
		if within(goos, p, ex) {
			return true
		}
	}
	return false
}

func sameRoots(a, b []Root) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
