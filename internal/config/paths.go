// Package config resolves where SessionKeep keeps its data.
//
// User data never lives in a Claude Code plugin directory (ADR-0002): the plugin root changes on
// every update and the plugin data directory is deleted on uninstall.
package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// EnvDataDir overrides the data directory. It exists for test sandboxes and controlled setups;
// it is not an authorization mechanism.
const EnvDataDir = "SESSIONKEEP_DATA_DIR"

// Paths is the resolved on-disk layout. All values are absolute.
type Paths struct {
	DataDir   string `json:"data_dir"`
	DBPath    string `json:"db_path"`
	SpoolDir  string `json:"spool_dir"`
	CacheDir  string `json:"cache_dir"`
	LeasePath string `json:"lease_path"`
	// Source says which rule produced DataDir: "override", "localappdata", "application-support", "xdg" or "home".
	Source string `json:"source"`
}

// ErrNoDataDir means the environment gives no usable location. Callers on the hook path must
// treat it as "capture unavailable" and still exit 0.
var ErrNoDataDir = errors.New("cannot determine a data directory")

// Resolve computes the layout for goos from environment lookups only. It touches no files, so it
// is safe on the hook path and testable for every OS from any host.
func Resolve(goos string, getenv func(string) string) (Paths, error) {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	join := func(parts ...string) string {
		out := strings.TrimRight(parts[0], `/\`)
		if out == "" || (goos == "windows" && len(out) == 2 && out[1] == ':') {
			out = parts[0][:len(out)] // keep "/" or "C:" roots intact
		}
		return out + sep + strings.Join(parts[1:], sep)
	}

	var dir, source string
	switch {
	case getenv(EnvDataDir) != "":
		dir, source = getenv(EnvDataDir), "override"
		if !isAbs(goos, dir) {
			return Paths{}, fmt.Errorf("%w: %s must be an absolute path", ErrNoDataDir, EnvDataDir)
		}
		dir = strings.TrimRight(dir, `/\`)
	case goos == "windows":
		base := getenv("LOCALAPPDATA")
		if !isAbs(goos, base) {
			return Paths{}, fmt.Errorf("%w: LOCALAPPDATA is not set to an absolute path", ErrNoDataDir)
		}
		dir, source = join(base, "SessionKeep"), "localappdata"
	case goos == "darwin":
		home := getenv("HOME")
		if !isAbs(goos, home) {
			return Paths{}, fmt.Errorf("%w: HOME is not set to an absolute path", ErrNoDataDir)
		}
		dir, source = join(home, "Library", "Application Support", "SessionKeep"), "application-support"
	default:
		if xdg := getenv("XDG_DATA_HOME"); isAbs(goos, xdg) { // the XDG spec says to ignore relative values
			dir, source = join(xdg, "sessionkeep"), "xdg"
			break
		}
		home := getenv("HOME")
		if !isAbs(goos, home) {
			return Paths{}, fmt.Errorf("%w: neither XDG_DATA_HOME nor HOME is an absolute path", ErrNoDataDir)
		}
		dir, source = join(home, ".local", "share", "sessionkeep"), "home"
	}

	return Paths{
		DataDir:   dir,
		DBPath:    join(dir, "sessionkeep.db"),
		SpoolDir:  join(dir, "spool"),
		CacheDir:  join(dir, "cache"),
		LeasePath: join(dir, "ingest.lease"),
		Source:    source,
	}, nil
}

func isAbs(goos, p string) bool {
	if goos != "windows" {
		return strings.HasPrefix(p, "/")
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		c := p[0] | 0x20
		return c >= 'a' && c <= 'z'
	}
	return strings.HasPrefix(p, `\\`) && len(p) > 2 // UNC
}

// Ensure creates the data, spool and cache directories for the current user and proves the data
// directory is writable. It is for setup and the lease holder, never for hooks.
//
// The 0700 mode restricts access on Unix. MkdirAll leaves a folder that already exists as it is
// (on macOS the installer may create the data folder first, with the usual 0755), so on Unix the
// mode is set as well (L4). On Windows, Go does not translate it into an ACL; the default location
// inherits the per-user ACL of the profile, and a custom location keeps whatever ACL its parent has.
func Ensure(p Paths) error {
	for _, d := range []string{p.DataDir, p.SpoolDir, p.CacheDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(d, 0o700); err != nil {
				return fmt.Errorf("make %s private: %w", d, err)
			}
		}
	}
	f, err := os.CreateTemp(p.DataDir, ".write-check-*")
	if err != nil {
		return fmt.Errorf("data directory is not writable: %w", err)
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
