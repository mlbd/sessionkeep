// Phase 0 spike P0-05. Synthetic data only. Throwaway code, not product code.
// Modes: hook <dir> | consume <dir> | verify <dir>
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

type event struct {
	EventID string          `json:"event_id"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func hook(dir string) int {
	// Fail open: any error returns 0 with nothing on stdout.
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return 0
	}
	var e event
	if json.Unmarshal(raw, &e) != nil || e.EventID == "" {
		return 0
	}
	spool := filepath.Join(dir, "spool")
	tmp := filepath.Join(spool, "."+e.EventID+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync() // FlushFileBuffers on Windows
	}
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return 0
	}
	if os.Rename(tmp, filepath.Join(spool, e.EventID+".evt")) != nil {
		os.Remove(tmp)
	}
	return 0
}

func lease(dir string) (windows.Handle, bool) {
	p, _ := windows.UTF16PtrFromString(filepath.Join(dir, "ingest.lease"))
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return 0, false
	}
	ol := new(windows.Overlapped)
	if windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol) != nil {
		windows.CloseHandle(h)
		return 0, false
	}
	return h, true // released by the OS when the process dies
}

func open(dir string) *sql.DB {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "spool.db"))+
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`create table if not exists events(event_key text primary key, kind text, payload text, ingested_at text)`); err != nil {
		panic(err)
	}
	return db
}

func consume(dir string) int {
	if _, ok := lease(dir); !ok {
		fmt.Println("lease held by another process")
		return 4
	}
	db := open(dir)
	crashAfter, _ := strconv.Atoi(os.Getenv("CRASH_AFTER")) // crash at the Nth event; 0 = never
	point := os.Getenv("CRASH_POINT")                      // before_commit | after_commit
	slow, _ := strconv.Atoi(os.Getenv("SLOW_MS"))
	files, _ := filepath.Glob(filepath.Join(dir, "spool", "*.evt"))
	rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
	inserted, replayed := 0, 0
	for i, fp := range files {
		raw, err := os.ReadFile(fp)
		if err != nil {
			continue
		}
		var e event
		if json.Unmarshal(raw, &e) != nil {
			os.Rename(fp, strings.TrimSuffix(fp, ".evt")+".bad")
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return 1
		}
		res, err := tx.Exec(`insert or ignore into events values(?,?,?,?)`, e.EventID, e.Kind, string(e.Payload), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			tx.Rollback()
			return 1
		}
		if crashAfter > 0 && i+1 == crashAfter && point == "before_commit" {
			os.Exit(3)
		}
		if tx.Commit() != nil {
			return 1
		}
		if n, _ := res.RowsAffected(); n == 1 {
			inserted++
		} else {
			replayed++
		}
		if crashAfter > 0 && i+1 == crashAfter && point == "after_commit" {
			os.Exit(3)
		}
		os.Remove(fp)
		if slow > 0 {
			time.Sleep(time.Duration(slow) * time.Millisecond)
		}
	}
	fmt.Printf("drained files=%d inserted=%d replayed_ignored=%d\n", len(files), inserted, replayed)
	db.Close()
	return 0
}

func verify(dir string) int {
	db := open(dir)
	var n, d int
	var ic string
	db.QueryRow("select count(*), count(distinct event_key) from events").Scan(&n, &d)
	db.QueryRow("pragma integrity_check").Scan(&ic)
	left, _ := filepath.Glob(filepath.Join(dir, "spool", "*"))
	fmt.Printf("rows=%d distinct=%d integrity=%s spool_left=%d\n", n, d, ic, len(left))
	return 0
}

func main() {
	switch os.Args[1] {
	case "hook":
		os.Exit(hook(os.Args[2]))
	case "consume":
		os.Exit(consume(os.Args[2]))
	case "verify":
		os.Exit(verify(os.Args[2]))
	}
}
