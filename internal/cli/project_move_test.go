package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// After a project's folder is renamed, `project move` points the project at the new place: what
// was saved stays with it, new sessions there are captured for it, and the old place is forgotten.
func TestProjectMoveFollowsARenamedFolder(t *testing.T) {
	sb := newSandbox(t)
	old := sb.dir("old-name")
	sb.ok("project", "add", old, "--capture", "conversation")
	_, _, id := sb.which(old)
	sb.say("UserPromptSubmit", "t-1", old, "Add totals MOVEDWORD.")
	sb.ok("ingest")

	renamed := filepath.Join(sb.work, "new-name")
	if code, _, errOut := sb.run("project", "move", old, renamed); code == ExitOK || !strings.Contains(errOut, "not an existing directory") {
		t.Fatalf("moving to a folder that does not exist: exit %d, %s", code, errOut)
	}
	if err := os.Rename(old, renamed); err != nil {
		t.Fatal(err)
	}
	if a, _, _ := sb.which(renamed); a == "capture" {
		t.Fatal("the renamed folder is captured before the project was moved")
	}
	out := sb.ok("project", "move", old, renamed, "--name", "New name")
	if !strings.Contains(out, "New name") || !strings.Contains(out, "now lives in") {
		t.Errorf("move said: %s", out)
	}
	if a, _, got := sb.which(renamed); a != "capture" || got != id {
		t.Errorf("after the move the new folder gives action %q project %q, want capture %q", a, got, id)
	}
	if a, _, _ := sb.which(old); a == "capture" {
		t.Error("the old folder is still captured")
	}
	if hits := sb.search("MOVEDWORD", "--project", renamed).Data.Hits; len(hits) != 1 {
		t.Errorf("what was saved before the move is found %d times from the new folder, want once", len(hits))
	}

	// By id as well, and never onto a folder that is a project of its own.
	other := sb.dir("other")
	sb.ok("project", "add", other, "--capture", "conversation")
	if code, _, errOut := sb.run("project", "move", id, other); code == ExitOK || !strings.Contains(errOut, "already registered") {
		t.Errorf("moving onto another project's folder: exit %d, %s", code, errOut)
	}
	third := sb.dir("third")
	sb.ok("project", "move", id[:8], third)
	if _, _, got := sb.which(third); got != id {
		t.Errorf("move by id: the folder belongs to %q, want %q", got, id)
	}
	if code, _, _ := sb.run("project", "move", "no-such-thing", third); code == ExitOK {
		t.Error("moving a project that does not exist succeeded")
	}
}
