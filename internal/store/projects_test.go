package store

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func newProj(id, root, level string) NewProject {
	return NewProject{ID: id, RootID: "root-" + id, Name: "n-" + id, Root: root, Level: level, DeviceID: "dev-1", AddedBy: "manual", Now: t0}
}

func TestAddListFindProject(t *testing.T) {
	s, _ := openTemp(t)
	if _, err := s.AddProject(ctx, newProj("aaaa1111-x", `c:\work\app`, "conversation")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProject(ctx, newProj("aaaa2222-y", `c:\work\other`, "off")); err != nil {
		t.Fatal(err)
	}

	// Same directory twice is refused and leaves no orphan project behind.
	if _, err := s.AddProject(ctx, newProj("cccc3333-z", `c:\work\app`, "metadata")); !errors.Is(err, ErrRootRegistered) {
		t.Fatalf("want ErrRootRegistered, got %v", err)
	}
	// An unknown level is rejected by the schema, not stored.
	if _, err := s.AddProject(ctx, newProj("dddd4444-w", `c:\work\bad`, "everything")); err == nil {
		t.Fatal("unknown capture level must be rejected")
	}
	all, err := s.ListProjects(ctx, "dev-1")
	if err != nil || len(all) != 2 {
		t.Fatalf("list: %+v, %v; want exactly the 2 valid projects", all, err)
	}
	if all[0].Roots[0] != `c:\work\app` || all[0].CapturePolicy != "conversation" || all[0].Paused || all[0].AddedBy != "manual" {
		t.Fatalf("unexpected first project: %+v", all[0])
	}

	// Roots registered on another device are not listed as this device's roots.
	other, _ := s.ListProjects(ctx, "dev-2")
	if len(other) != 2 || len(other[0].Roots) != 0 {
		t.Fatalf("other device must see the projects without roots: %+v", other)
	}

	for _, key := range []string{"aaaa1111-x", "aaaa1", `c:\work\app`} {
		if p, err := s.FindProject(ctx, "dev-1", key); err != nil || p.ID != "aaaa1111-x" {
			t.Errorf("FindProject(%q) = %+v, %v", key, p, err)
		}
	}
	if _, err := s.FindProject(ctx, "dev-1", "aaaa"); !errors.Is(err, ErrAmbiguousProject) {
		t.Errorf("ambiguous prefix: got %v", err)
	}
	for _, key := range []string{"zzzz", "aaa", `c:\work`, ""} {
		if _, err := s.FindProject(ctx, "dev-1", key); !errors.Is(err, ErrNoProject) {
			t.Errorf("FindProject(%q): want ErrNoProject, got %v", key, err)
		}
	}
}

func TestPauseKeepsLevelAndResumeRestoresIt(t *testing.T) {
	s, _ := openTemp(t)
	s.AddProject(ctx, newProj("p1", `c:\work\app`, "extended"))
	if err := s.SetPaused(ctx, "p1", true, t0); err != nil {
		t.Fatal(err)
	}
	p, _ := s.FindProject(ctx, "dev-1", "p1")
	if !p.Paused || p.CapturePolicy != "extended" {
		t.Fatalf("after pause: %+v", p)
	}
	if err := s.SetPaused(ctx, "p1", false, t0); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.FindProject(ctx, "dev-1", "p1"); p.Paused || p.CapturePolicy != "extended" {
		t.Fatalf("after resume: %+v", p)
	}
	if err := s.SetCaptureLevel(ctx, "p1", "metadata"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCaptureLevel(ctx, "p1", "all-of-it"); err == nil {
		t.Fatal("unknown level must be rejected")
	}
	if p, _ = s.FindProject(ctx, "dev-1", "p1"); p.CapturePolicy != "metadata" {
		t.Fatalf("level after failed update: %+v", p)
	}
	if err := s.SetPaused(ctx, "nope", true, t0); !errors.Is(err, ErrNoProject) {
		t.Fatalf("want ErrNoProject, got %v", err)
	}
}
