package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Until version 0.15.0 the program was called DevWeft and kept its data in a folder and a
// database file of that name.
const (
	oldDirTitle = "DevWeft" // Windows and macOS
	oldDirLower = "devweft" // elsewhere
	oldDBName   = "devweft.db"
)

// AdoptOld moves the data kept under the program's earlier name to where this version looks, so
// that nothing saved before the rename is left behind. It returns the folder it moved, or "" when
// there was nothing to do: the data directory was chosen by hand, nothing exists under the old
// name, or the new place is already in use (then both are left alone and the error says so).
// A move that cannot be finished is undone, and the error says what stood in the way.
func AdoptOld(p Paths) (string, error) {
	if p.Source == "override" || p.DataDir == "" {
		return "", nil
	}
	name := oldDirLower
	if base := filepath.Base(p.DataDir); base != "" && base[0] >= 'A' && base[0] <= 'Z' {
		name = oldDirTitle
	}
	old := filepath.Join(filepath.Dir(p.DataDir), name)
	if st, err := os.Stat(old); err != nil || !st.IsDir() {
		return "", nil
	}
	if entries, err := os.ReadDir(p.DataDir); err == nil {
		if len(entries) > 0 {
			return "", fmt.Errorf("data saved under the earlier name is still in %s and was not moved, because %s already holds data", old, p.DataDir)
		}
		if err := os.Remove(p.DataDir); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(old, p.DataDir); err != nil {
		return "", fmt.Errorf("the data saved under the earlier name in %s could not be moved to %s (close every program that uses it and run this again): %w", old, p.DataDir, err)
	}
	// The database and the two files SQLite may keep beside it change name together.
	var done [][2]string
	for _, suffix := range []string{"", "-wal", "-shm"} {
		from, to := filepath.Join(p.DataDir, oldDBName+suffix), p.DBPath+suffix
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			for _, d := range done {
				os.Rename(d[1], d[0])
			}
			os.Rename(p.DataDir, old)
			return "", fmt.Errorf("the database in %s could not be renamed (close every program that uses it and run this again): %w", old, err)
		}
		done = append(done, [2]string{from, to})
	}
	return old, nil
}
