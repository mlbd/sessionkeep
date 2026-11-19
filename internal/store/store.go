// Package store owns the SQLite database. Only the ingest-lease holder opens it for writing
// (ADR-0003); hooks never do. The driver is hidden here so it can be replaced (ADR-0001).
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedded embed.FS

var (
	// ErrMigrationChanged means an already-applied migration no longer matches its recorded checksum.
	ErrMigrationChanged = errors.New("applied migration differs from this binary's copy")
	// ErrSchemaTooNew means the database was written by a newer SessionKeep; this binary must not touch it.
	ErrSchemaTooNew = errors.New("database schema is newer than this binary")
)

// Store is an open, migrated database with a single writer connection.
type Store struct {
	db     *sql.DB
	engine EngineInfo
}

// probeEngine reads the linked engine through SQL. Tests replace it to simulate an old library.
var probeEngine = func(ctx context.Context, db *sql.DB) (EngineInfo, error) {
	var e EngineInfo
	if err := db.QueryRowContext(ctx, "select sqlite_version()").Scan(&e.Version); err != nil {
		return e, err
	}
	rows, err := db.QueryContext(ctx, "pragma compile_options")
	if err != nil {
		return e, err
	}
	defer rows.Close()
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return e, err
		}
		if o == "ENABLE_FTS5" {
			e.FTS5 = true
		}
	}
	return e, rows.Err()
}

// ProbeEngine reports the engine linked into this binary without touching any file.
func ProbeEngine(ctx context.Context) (EngineInfo, error) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		return EngineInfo{}, err
	}
	defer db.Close()
	return probeEngine(ctx, db)
}

// Open opens (creating if needed) the database at path for the writer, checks the engine, switches
// to WAL and applies pending migrations. The engine check happens before WAL is enabled.
func Open(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, embedded, "migrations")
}

func open(ctx context.Context, path string, migrations fs.FS, dir string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; every statement sees the per-connection pragmas from the DSN
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }

	engine, err := probeEngine(ctx, db)
	if err != nil {
		return fail(fmt.Errorf("probe SQLite engine: %w", err))
	}
	if err := CheckEngine(engine); err != nil {
		return fail(err)
	}
	var mode string
	if err := db.QueryRowContext(ctx, "pragma journal_mode=WAL").Scan(&mode); err != nil {
		return fail(fmt.Errorf("enable WAL: %w", err))
	}
	if !strings.EqualFold(mode, "wal") {
		return fail(fmt.Errorf("enable WAL: journal mode is %q (is the database on a network filesystem?)", mode))
	}
	var fk int
	if err := db.QueryRowContext(ctx, "pragma foreign_keys").Scan(&fk); err != nil || fk != 1 {
		return fail(fmt.Errorf("foreign keys are not enforced (value %d, err %v)", fk, err))
	}
	if _, err := migrate(ctx, db, migrations, dir); err != nil {
		return fail(err)
	}
	return &Store{db: db, engine: engine}, nil
}

// Engine returns the engine information read when the store was opened.
func (s *Store) Engine() EngineInfo { return s.engine }

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// dsn builds a SQLite URI. Only %, ? and # are special in the path part of a SQLite URI.
func dsn(path string) string {
	p := filepath.ToSlash(path)
	p = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(p)
	return "file:" + p + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)"
}

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

var migrationName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

func loadMigrations(fsys fs.FS, dir string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migration file %q is not named NNNN_name.sql", e.Name())
		}
		raw, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		// Checksums ignore line-ending differences so a checkout with CRLF conversion still matches.
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		sum := sha256.Sum256([]byte(text))
		v, _ := strconv.Atoi(m[1])
		out = append(out, migration{version: v, name: m[2], sql: text, checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migrations must be numbered 0001, 0002, … without gaps; found %04d at position %d", m.version, i+1)
		}
	}
	return out, nil
}

// migrate applies pending migrations, each in its own transaction together with its bookkeeping
// row, so an interrupted migration leaves no partial schema. It returns how many were applied.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS, dir string) (int, error) {
	known, err := loadMigrations(fsys, dir)
	if err != nil {
		return 0, err
	}
	if _, err := db.ExecContext(ctx, `create table if not exists schema_migrations(
  version integer primary key, name text not null, checksum text not null, applied_at text not null) strict`); err != nil {
		return 0, fmt.Errorf("create schema_migrations: %w", err)
	}
	applied := map[int]string{}
	rows, err := db.QueryContext(ctx, "select version, checksum from schema_migrations")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var v int
		var c string
		if err := rows.Scan(&v, &c); err != nil {
			rows.Close()
			return 0, err
		}
		applied[v] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for v := range applied {
		if v > len(known) {
			return 0, fmt.Errorf("%w: database is at version %d, this binary knows %d", ErrSchemaTooNew, v, len(known))
		}
	}
	n := 0
	for _, m := range known {
		if sum, ok := applied[m.version]; ok {
			if sum != m.checksum {
				return n, fmt.Errorf("%w: %04d_%s", ErrMigrationChanged, m.version, m.name)
			}
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return n, err
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			tx.Rollback()
			return n, fmt.Errorf("migration %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.ExecContext(ctx, "insert into schema_migrations values(?,?,?,?)", m.version, m.name, m.checksum, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return n, err
		}
		if err := tx.Commit(); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
