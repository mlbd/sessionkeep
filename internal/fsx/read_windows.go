//go:build windows

package fsx

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// readOnce opens the file allowing other processes to replace or delete it while it is open.
// Go's os.Open does not request FILE_SHARE_DELETE, which makes a concurrent WriteAtomic fail
// with "Access is denied" for as long as any reader holds the file.
func readOnce(path string) ([]byte, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	return io.ReadAll(f)
}
