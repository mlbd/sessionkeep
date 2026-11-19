// Package archive reads and writes SessionKeep export files: one zip holding a database copy, the
// settings file and a manifest that says exactly what is inside.
package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Format is the export file format version.
const Format = 1

// Names of the entries in an export file. Nothing else is ever written or accepted.
const (
	ManifestName = "manifest.json"
	DBName       = "sessionkeep.db"
	SettingsName = "config.json"
)

// MaxEntryBytes bounds what Open will extract for one entry (64 GiB), so a crafted file cannot
// expand without limit.
const MaxEntryBytes = 64 << 30

// File describes one entry.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is the description stored inside an export.
type Manifest struct {
	Format        int             `json:"format"`
	CreatedAt     string          `json:"created_at"`
	Version       string          `json:"sessionkeep_version"`
	SchemaVersion int             `json:"schema_version"`
	SQLite        string          `json:"sqlite_version"`
	Totals        json.RawMessage `json:"totals"`
	// NotIncluded lists, in words, what this export deliberately or unavoidably leaves out.
	NotIncluded []string `json:"not_included"`
	Files       []File   `json:"files"`
}

func digest(path string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return File{Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, err
}

// Write creates the export at dst (which must not exist) from a database copy and, if present,
// a settings file. The manifest's Files are filled in here.
func Write(dst, dbCopy, settings string, m Manifest) (Manifest, error) {
	m.Format = Format
	m.Files = nil
	sources := map[string]string{DBName: dbCopy}
	if _, err := os.Stat(settings); err == nil {
		sources[SettingsName] = settings
	}
	for _, name := range []string{DBName, SettingsName} {
		src, ok := sources[name]
		if !ok {
			continue
		}
		f, err := digest(src)
		if err != nil {
			return m, err
		}
		f.Name = name
		m.Files = append(m.Files, f)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return m, err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(dst)
		}
	}()
	zw := zip.NewWriter(out)
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	w, err := zw.Create(ManifestName)
	if err != nil {
		return m, err
	}
	if _, err := w.Write(raw); err != nil {
		return m, err
	}
	for _, f := range m.Files {
		// The database is stored, not deflated: it is large, and storing keeps export fast.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store})
		if err != nil {
			return m, err
		}
		src, err := os.Open(sources[f.Name])
		if err != nil {
			return m, err
		}
		_, err = io.Copy(w, src)
		src.Close()
		if err != nil {
			return m, err
		}
	}
	if err := zw.Close(); err != nil {
		return m, err
	}
	if err := out.Sync(); err != nil {
		return m, err
	}
	ok = true
	return m, nil
}

// ErrInvalid means the file is not a usable SessionKeep export.
var ErrInvalid = errors.New("not a valid SessionKeep export")

// Extract verifies an export and extracts its entries into dir (which must exist and should be
// empty). Every entry is checked against the manifest's size and digest. Entry names are matched
// against the fixed list, so nothing can be written outside dir.
func Extract(src, dir string) (Manifest, error) {
	var m Manifest
	zr, err := zip.OpenReader(src)
	if err != nil {
		return m, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		if f.Name != ManifestName && f.Name != DBName && f.Name != SettingsName {
			return m, fmt.Errorf("%w: unexpected entry %q", ErrInvalid, f.Name)
		}
		if _, dup := entries[f.Name]; dup {
			return m, fmt.Errorf("%w: entry %q appears twice", ErrInvalid, f.Name)
		}
		entries[f.Name] = f
	}
	mf, ok := entries[ManifestName]
	if !ok || mf.UncompressedSize64 > 1<<20 {
		return m, fmt.Errorf("%w: manifest missing or too large", ErrInvalid)
	}
	rc, err := mf.Open()
	if err != nil {
		return m, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	raw, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	rc.Close()
	if err != nil || json.Unmarshal(raw, &m) != nil {
		return m, fmt.Errorf("%w: manifest unreadable", ErrInvalid)
	}
	if m.Format != Format {
		return m, fmt.Errorf("%w: export format %d is not supported by this version (expects %d)", ErrInvalid, m.Format, Format)
	}
	listed := map[string]File{}
	for _, f := range m.Files {
		listed[f.Name] = f
	}
	if _, ok := listed[DBName]; !ok || len(listed) != len(entries)-1 {
		return m, fmt.Errorf("%w: manifest and contents disagree", ErrInvalid)
	}
	for name, want := range listed {
		zf, ok := entries[name]
		if !ok || name == ManifestName {
			return m, fmt.Errorf("%w: %s listed but missing", ErrInvalid, name)
		}
		if want.Size < 0 || want.Size > MaxEntryBytes || zf.UncompressedSize64 != uint64(want.Size) {
			return m, fmt.Errorf("%w: %s has the wrong size", ErrInvalid, name)
		}
		rc, err := zf.Open()
		if err != nil {
			return m, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			rc.Close()
			return m, err
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(rc, want.Size+1))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return m, err
		}
		if n != want.Size || hex.EncodeToString(h.Sum(nil)) != want.SHA256 {
			return m, fmt.Errorf("%w: %s does not match its recorded digest (the file is damaged or was altered)", ErrInvalid, name)
		}
	}
	return m, nil
}
