// Package fsx holds small filesystem helpers shared by packages that persist state.
package fsx

import (
	"os"
	"path/filepath"
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
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
