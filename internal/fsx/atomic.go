// Package fsx holds small filesystem helpers shared by packages that persist state.
package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Replacing a file that other processes are reading needs care on Windows: the classic rename
// fails while any reader has the file open. WriteAtomic therefore uses a POSIX-semantics rename
// (rename_windows.go) and ReadFile opens with delete-sharing (read_windows.go), which that rename
// requires. Both sides also retry briefly — 40 tries, 2 ms apart — as a fallback.
// Files replaced with WriteAtomic must be read with ReadFile, not os.ReadFile.
const (
	retries   = 40
	retryWait = 2 * time.Millisecond
)

// WriteAtomic replaces path with data so that readers see either the old or the new content,
// never a partial file: write a temp file in the same directory, flush it, then rename over.
func WriteAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		for i := 0; ; i++ {
			if err = replace(tmp, path); err == nil || i >= retries {
				break
			}
			time.Sleep(retryWait)
		}
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// ReadFile reads a file that another process may be replacing with WriteAtomic. "Does not exist"
// is returned at once; any other failure is retried briefly before it is reported.
func ReadFile(path string) ([]byte, error) {
	for i := 0; ; i++ {
		raw, err := readOnce(path)
		if err == nil || errors.Is(err, os.ErrNotExist) || i >= retries {
			return raw, err
		}
		time.Sleep(retryWait)
	}
}
