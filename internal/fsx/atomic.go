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

// AltSuffix names the second copy WriteAtomic keeps next to every file.
//
// Measured on Windows 10: even with a POSIX-semantics rename, a reader occasionally sees "file
// not found" for up to tens of milliseconds while a file is being replaced. For SessionKeep that
// would be dangerous — a hook that finds no roots cache concludes that no project is registered
// and captures nothing. So every file is written twice, one after the other, and a reader that
// cannot find the first copy uses the second. The two are never replaced at the same instant, so
// once a file exists a reader always finds one of them, without waiting.
const AltSuffix = ".alt"

// WriteAtomic replaces path with data so that readers using ReadFile see either the old or the
// new content, never a partial file and never a missing one.
func WriteAtomic(path string, data []byte) error {
	if err := writeOne(path, data); err != nil {
		return err
	}
	return writeOne(path+AltSuffix, data)
}

// writeOne writes a temp file in the same directory, flushes it, then renames it over path.
func writeOne(path string, data []byte) error {
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

// ReadFile reads a file written with WriteAtomic. If the file is momentarily absent because it
// is being replaced, the second copy is used (it may be one version older for an instant).
// "Does not exist" is returned, immediately, only when neither copy exists. Other failures are
// retried briefly before they are reported.
func ReadFile(path string) ([]byte, error) {
	for i := 0; ; i++ {
		raw, err := readOnce(path)
		if errors.Is(err, os.ErrNotExist) {
			if alt, altErr := readOnce(path + AltSuffix); altErr == nil {
				return alt, nil
			} else if !errors.Is(altErr, os.ErrNotExist) {
				err = altErr
			} else if again, againErr := readOnce(path); againErr == nil {
				return again, nil // the writer finished between our two looks
			} else {
				return nil, err
			}
		}
		if err == nil || i >= retries {
			return raw, err
		}
		time.Sleep(retryWait)
	}
}
