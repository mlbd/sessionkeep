//go:build windows

package fsx

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// The POSIX-semantics rename must itself succeed while a reader holds the target open; otherwise
// WriteAtomic would only be passing by falling back and retrying.
func TestPosixRenameReplacesAFileThatIsOpen(t *testing.T) {
	dir := t.TempDir()
	dst, tmp := filepath.Join(dir, "target é.json"), filepath.Join(dir, "new.tmp")
	os.WriteFile(dst, []byte("old"), 0o600)
	os.WriteFile(tmp, []byte("new"), 0o600)

	p, _ := windows.UTF16PtrFromString(dst)
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(h), dst)
	defer reader.Close()

	if err := posixRename(tmp, dst); err != nil {
		t.Fatalf("POSIX rename failed with a reader holding the target: %v", err)
	}
	if got, _ := ReadFile(dst); string(got) != "new" {
		t.Fatalf("target holds %q, want new", got)
	}
	buf := make([]byte, 8)
	n, _ := reader.Read(buf)
	if string(buf[:n]) != "old" {
		t.Fatalf("the open reader should still see the old content, got %q", buf[:n])
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("temp file still exists after rename")
	}
}

// A reader opened the ordinary way (no delete-sharing) blocks the replace; this documents why
// files written with WriteAtomic must be read with ReadFile.
func TestOrdinaryOpenBlocksReplace(t *testing.T) {
	dir := t.TempDir()
	dst, tmp := filepath.Join(dir, "target.json"), filepath.Join(dir, "new.tmp")
	os.WriteFile(dst, []byte("old"), 0o600)
	os.WriteFile(tmp, []byte("new"), 0o600)
	f, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := posixRename(tmp, dst); err == nil {
		t.Skip("this Windows build allows the replace even without delete-sharing")
	}
}
