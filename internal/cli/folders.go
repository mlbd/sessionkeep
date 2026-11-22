package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"sessionkeep/internal/project"
)

// folderEntry is one folder inside the folder being looked at.
type folderEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Git      bool   `json:"git"`      // holds a .git folder or file
	Project  bool   `json:"project"`  // already a registered project folder on this computer
	Excluded bool   `json:"excluded"` // inside a folder the user excluded
}

type place struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// foldersCmd lists the folders inside PATH (the home folder when none is given), for the
// dashboard's folder chooser: a web page cannot read full paths on its own. Only folder names are
// listed, never files or their contents, and nothing is changed.
func foldersCmd(inv invocation, rest []string) error {
	if err := inv.only(); err != nil {
		return err
	}
	if len(rest) > 1 {
		return usageErr("usage: sessionkeep folders [PATH]")
	}
	home := homeOf(inv)
	target := home
	if len(rest) == 1 && rest[0] != "" {
		target = rest[0]
	}
	if target == "" {
		return failErr("no_home", "the home folder is not known; give a folder")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return usageErr("cannot resolve path %q: %v", target, err)
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return failErr("not_a_directory", "%s is not a folder on this computer", abs)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return failErr("cannot_read", "the folder %s cannot be read: %v", abs, err)
	}
	st, err := load(inv, false)
	if err != nil {
		return err
	}
	roots, _ := project.ReadRootsCache(st.paths) // a missing cache only means no folder is marked
	isRoot := map[string]bool{}
	for _, r := range roots {
		isRoot[r.Path] = true
	}
	env := st.env(inv.getenv)
	folders := []folderEntry{}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || hiddenFolder(name) {
			continue
		}
		p := filepath.Join(abs, name)
		f := folderEntry{Name: name, Path: p}
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			f.Git = true
		}
		if n, err := project.Normalize(runtime.GOOS, p); err == nil {
			f.Project = isRoot[n]
		}
		f.Excluded = project.Decide(env, st.settings, nil, p).Reason == "excluded by the user"
		folders = append(folders, f)
	}
	sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	out := map[string]any{"path": abs, "parent": parent, "places": places(home), "folders": folders}
	var b strings.Builder
	b.WriteString(abs + "\n")
	for _, f := range folders {
		mark := ""
		switch {
		case f.Project:
			mark = "  (already a project)"
		case f.Excluded:
			mark = "  (excluded)"
		case f.Git:
			mark = "  (Git repository)"
		}
		b.WriteString("  " + f.Name + mark + "\n")
	}
	if len(folders) == 0 {
		b.WriteString("  (no folders inside)\n")
	}
	return inv.emit(out, b.String())
}

// hiddenFolder leaves out folders people do not pick projects from: dot folders, and the
// Windows recycle bin and system folders.
func hiddenFolder(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || strings.EqualFold(name, "System Volume Information")
}

// places are the starting points offered beside the folder list: the home folder and two common
// folders in it, then the roots of this system's disks.
func places(home string) []place {
	out := []place{}
	isDir := func(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
	if home != "" && isDir(home) {
		out = append(out, place{"Home", home})
		for _, sub := range []string{"Desktop", "Documents"} {
			if p := filepath.Join(home, sub); isDir(p) {
				out = append(out, place{sub, p})
			}
		}
	}
	switch runtime.GOOS {
	case "windows":
		for c := 'C'; c <= 'Z'; c++ { // A: and B: are floppy letters; asking for them can stall
			if root := string(c) + `:\`; isDir(root) {
				out = append(out, place{"Drive " + string(c) + ":", root})
			}
		}
	default:
		out = append(out, place{"Computer", "/"})
		extra := []string{"/mnt", "/media"}
		if runtime.GOOS == "darwin" {
			extra = []string{"/Volumes"}
		}
		for _, p := range extra {
			if isDir(p) {
				out = append(out, place{filepath.Base(p), p})
			}
		}
	}
	return out
}
