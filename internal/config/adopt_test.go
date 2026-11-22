package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdoptOldMovesDataSavedUnderTheEarlierName(t *testing.T) {
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) string {
		b, _ := os.ReadFile(path)
		return string(b)
	}
	paths := func(base, name string) Paths {
		dir := filepath.Join(base, name)
		return Paths{DataDir: dir, DBPath: filepath.Join(dir, "sessionkeep.db"), Source: "localappdata"}
	}

	// The whole folder moves, and the database with the files beside it changes name.
	base := t.TempDir()
	old := filepath.Join(base, "DevWeft")
	write(filepath.Join(old, "devweft.db"), "DB")
	write(filepath.Join(old, "devweft.db-wal"), "WAL")
	write(filepath.Join(old, "settings.json"), "SETTINGS")
	write(filepath.Join(old, "team", "state.json"), "STATE")
	p := paths(base, "SessionKeep")
	from, err := AdoptOld(p)
	if err != nil || from != old {
		t.Fatalf("AdoptOld = %q, %v", from, err)
	}
	if read(p.DBPath) != "DB" || read(p.DBPath+"-wal") != "WAL" || read(filepath.Join(p.DataDir, "settings.json")) != "SETTINGS" || read(filepath.Join(p.DataDir, "team", "state.json")) != "STATE" {
		t.Error("something did not arrive")
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("the old folder is still there")
	}
	if from, err := AdoptOld(p); from != "" || err != nil {
		t.Errorf("a second run did something: %q, %v", from, err)
	}

	// The lower-case name used outside Windows and macOS.
	base = t.TempDir()
	write(filepath.Join(base, "devweft", "devweft.db"), "DB")
	p = paths(base, "sessionkeep")
	if from, err := AdoptOld(p); err != nil || from == "" || read(p.DBPath) != "DB" {
		t.Errorf("lower-case name: %q, %v", from, err)
	}

	// An empty new folder is no obstacle; one that holds data is, and then nothing is touched.
	base = t.TempDir()
	write(filepath.Join(base, "DevWeft", "devweft.db"), "OLD")
	p = paths(base, "SessionKeep")
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if from, err := AdoptOld(p); err != nil || from == "" || read(p.DBPath) != "OLD" {
		t.Errorf("empty new folder: %q, %v", from, err)
	}
	base = t.TempDir()
	write(filepath.Join(base, "DevWeft", "devweft.db"), "OLD")
	p = paths(base, "SessionKeep")
	write(p.DBPath, "NEW")
	if from, err := AdoptOld(p); err == nil || from != "" {
		t.Errorf("both hold data: %q, %v", from, err)
	}
	if read(p.DBPath) != "NEW" || read(filepath.Join(base, "DevWeft", "devweft.db")) != "OLD" {
		t.Error("data was touched although both folders hold some")
	}

	// A data directory chosen by hand is never touched.
	base = t.TempDir()
	write(filepath.Join(base, "DevWeft", "devweft.db"), "OLD")
	p = paths(base, "SessionKeep")
	p.Source = "override"
	if from, err := AdoptOld(p); from != "" || err != nil {
		t.Errorf("override: %q, %v", from, err)
	}
	if read(filepath.Join(base, "DevWeft", "devweft.db")) != "OLD" {
		t.Error("an overridden data directory led to a move")
	}
}
