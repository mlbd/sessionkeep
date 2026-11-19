package store

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MinSQLite is the lowest SQLite release SessionKeep will run in WAL mode. Releases before 3.51.3
// contain the WAL-reset corruption bug (https://www.sqlite.org/wal.html, section 11). The fix
// was also backported to 3.44.6 and 3.50.7, but SessionKeep ships its own engine, so the simpler and
// stricter rule is used.
var MinSQLite = [3]int{3, 51, 3}

// ErrIncompatibleEngine means the linked SQLite must not be used. Callers report `incompatible`.
var ErrIncompatibleEngine = errors.New("incompatible SQLite engine")

// EngineInfo describes the SQLite library actually linked into this binary, read through SQL.
type EngineInfo struct {
	Version string `json:"sqlite_version"`
	FTS5    bool   `json:"fts5"`
}

// CheckEngine decides whether the engine may be used. It never trusts the Go module version.
func CheckEngine(e EngineInfo) error {
	v, err := parseVersion(e.Version)
	if err != nil {
		return fmt.Errorf("%w: unreadable version %q", ErrIncompatibleEngine, e.Version)
	}
	for i := range v {
		if v[i] != MinSQLite[i] {
			if v[i] < MinSQLite[i] {
				return fmt.Errorf("%w: SQLite %s is older than %d.%d.%d (WAL-reset bug)", ErrIncompatibleEngine, e.Version, MinSQLite[0], MinSQLite[1], MinSQLite[2])
			}
			break
		}
	}
	if !e.FTS5 {
		return fmt.Errorf("%w: SQLite %s was built without FTS5", ErrIncompatibleEngine, e.Version)
	}
	return nil
}

func parseVersion(s string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(s, ".")
	if len(parts) < 3 {
		return out, errors.New("want major.minor.patch")
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, errors.New("non-numeric component")
		}
		out[i] = n
	}
	return out, nil
}
