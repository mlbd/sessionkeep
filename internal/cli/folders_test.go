package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type foldersReply struct {
	Data struct {
		Path, Parent string
		Places       []struct{ Name, Path string }
		Folders      []struct {
			Name, Path             string
			Git, Project, Excluded bool
		}
	}
}

func (sb *sandbox) folders(args ...string) foldersReply {
	sb.t.Helper()
	var r foldersReply
	if err := json.Unmarshal([]byte(sb.ok(append([]string{"--json", "folders"}, args...)...)), &r); err != nil {
		sb.t.Fatal(err)
	}
	return r
}

// The folder chooser sees folder names only: never files, never hidden folders. Git repositories,
// registered projects and excluded folders are marked.
func TestFoldersListsSubfoldersOnly(t *testing.T) {
	sb := newSandbox(t)
	root := sb.dir("pick")
	app := sb.dir("pick", "app")
	sb.dir("pick", "Billing", ".git")
	sb.dir("pick", ".hidden")
	private := sb.dir("pick", "private")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("SECRETWORD"), 0o600); err != nil {
		t.Fatal(err)
	}
	sb.ok("project", "add", app, "--capture", "conversation")
	sb.ok("exclude", "add", private)

	raw := sb.ok("--json", "folders", root)
	if strings.Contains(raw, "notes.txt") || strings.Contains(raw, "SECRETWORD") || strings.Contains(raw, ".hidden") {
		t.Fatalf("the listing shows a file or a hidden folder: %s", raw)
	}
	r := sb.folders(root)
	var names []string
	for _, f := range r.Data.Folders {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "app,Billing,private" {
		t.Fatalf("folders %v, want app, Billing, private in that order", names)
	}
	if f := r.Data.Folders; !f[0].Project || f[0].Git || !f[1].Git || f[1].Project || !f[2].Excluded {
		t.Errorf("marks are wrong: %+v", f)
	}
	if r.Data.Parent != filepath.Dir(root) {
		t.Errorf("parent %q, want %q", r.Data.Parent, filepath.Dir(root))
	}
	if r.Data.Folders[0].Path != app {
		t.Errorf("path %q, want %q", r.Data.Folders[0].Path, app)
	}

	// Without a folder, the home folder is listed, and it is the first place offered.
	home := sb.env["HOME"]
	if err := os.MkdirAll(filepath.Join(home, "Documents"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := sb.folders()
	if h.Data.Path != home || len(h.Data.Places) < 2 || h.Data.Places[0].Path != home || h.Data.Places[1].Name != "Documents" {
		t.Errorf("home listing: path %q, places %+v", h.Data.Path, h.Data.Places)
	}
}

func TestFoldersRefusesWhatIsNotAFolder(t *testing.T) {
	sb := newSandbox(t)
	file := filepath.Join(sb.dir("x"), "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{file, filepath.Join(sb.work, "missing")} {
		if code, _, errOut := sb.run("folders", p); code == ExitOK || !strings.Contains(errOut, "is not a folder") {
			t.Errorf("folders %s: exit %d, %s", p, code, errOut)
		}
	}
}
