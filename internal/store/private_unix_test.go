//go:build !windows

package store

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"sessionkeep/internal/config"
)

// A data folder that existed before SessionKeep (as on macOS, where the installer may create it
// first with 0755) is made private, and the database and its WAL are readable by the user only,
// whatever the umask (L4). Unix only; Windows relies on the folder's ACL.
func TestTheDataFolderAndDatabaseArePrivate(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	dir := filepath.Join(t.TempDir(), "SessionKeep")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := config.Paths{DataDir: dir, SpoolDir: filepath.Join(dir, "spool"), CacheDir: filepath.Join(dir, "cache"), DBPath: filepath.Join(dir, "sessionkeep.db")}
	if err := config.Ensure(p); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.DataDir, p.SpoolDir, p.CacheDir} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v (%v), want 0700", d, fi.Mode().Perm(), err)
		}
	}
	s, err := Open(context.Background(), p.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, f := range []string{p.DBPath, p.DBPath + "-wal"} {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v (%v), want 0600", f, fi.Mode().Perm(), err)
		}
	}
}
