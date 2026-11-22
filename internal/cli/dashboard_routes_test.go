package cli

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// The redesigned dashboard's own routes: the folder chooser, moving a renamed project folder, and
// the switch for fetching pages to check facts.
func TestDashboardFoldersMoveAndResearchSwitch(t *testing.T) {
	sb := newSandbox(t)
	old := sb.dir("code", "old-name")
	sb.ok("project", "add", old, "--capture", "conversation")
	_, _, id := sb.which(old)
	b := signIn(t, sb)

	var list struct {
		Data struct {
			Path    string
			Folders []struct {
				Name    string
				Project bool
			}
		}
	}
	if code := b.do("GET", "/api/folders?path="+url.QueryEscape(filepath.Join(sb.work, "code")), nil, nil, &list); code != http.StatusOK || len(list.Data.Folders) != 1 || list.Data.Folders[0].Name != "old-name" || !list.Data.Folders[0].Project {
		t.Fatalf("folders: %d %+v", code, list.Data)
	}
	var bad struct{ Error struct{ Code string } }
	if code := b.do("GET", "/api/folders?path="+url.QueryEscape(filepath.Join(sb.work, "nope")), nil, nil, &bad); code == http.StatusOK || bad.Error.Code != "not_a_directory" {
		t.Errorf("folders of a missing path: %d %+v", code, bad.Error)
	}

	renamed := filepath.Join(sb.work, "code", "new-name")
	if err := os.Rename(old, renamed); err != nil {
		t.Fatal(err)
	}
	var moved struct {
		Error struct{ Code, Message string }
	}
	if code := b.do("POST", "/api/projects/"+id+"/move", map[string]string{"to": renamed, "name": "new-name"}, nil, &moved); code != http.StatusOK {
		t.Fatalf("move: %d %+v", code, moved.Error)
	}
	if a, _, got := sb.which(renamed); a != "capture" || got != id {
		t.Errorf("after the move the new folder gives %q %q, want capture %q", a, got, id)
	}

	var st struct{ Data struct{ Enabled bool } }
	for _, on := range []bool{true, false} {
		if code := b.do("POST", "/api/research/fetching", map[string]bool{"on": on}, nil, nil); code != http.StatusOK {
			t.Fatalf("research switch %v: %d", on, code)
		}
		if b.do("GET", "/api/research/fetching", nil, nil, &st); st.Data.Enabled != on {
			t.Errorf("after switching to %v the status says %v", on, st.Data.Enabled)
		}
	}
}
