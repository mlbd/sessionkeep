//go:build windows

package ingest

import "golang.org/x/sys/windows"

// lockExclusive opens a file with no sharing at all, the way a scanner or indexer can, and
// returns a function that releases it.
func lockExclusive(path string) (func(), error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}
