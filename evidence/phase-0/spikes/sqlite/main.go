// Phase 0 spike P0-04. Synthetic data only. Throwaway code, not product code.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"modernc.org/sqlite"
)

func must(err error) {
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
}

func pct(d []time.Duration, p float64) float64 {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return float64(d[int(p/100*float64(len(d)-1)+0.5)].Microseconds()) / 1000
}

func summ(name string, d []time.Duration) {
	fmt.Printf("%s: n=%d p50=%.2fms p95=%.2fms p99=%.2fms\n", name, len(d), pct(d, 50), pct(d, 95), pct(d, 99))
}

type backuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

func main() {
	dir := os.Args[1]
	dbp := filepath.Join(dir, "spike.db")
	rw := "file:" + filepath.ToSlash(dbp) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", rw)
	must(err)
	db.SetMaxOpenConns(1) // single writer connection

	var ver, jm string
	must(db.QueryRow("select sqlite_version()").Scan(&ver))
	must(db.QueryRow("pragma journal_mode").Scan(&jm))
	fmt.Println("sqlite_version:", ver, "journal_mode:", jm)
	rows, err := db.Query("pragma compile_options")
	must(err)
	var opts []string
	for rows.Next() {
		var o string
		must(rows.Scan(&o))
		if strings.Contains(o, "FTS") || strings.Contains(o, "THREADSAFE") || strings.Contains(o, "JSON") {
			opts = append(opts, o)
		}
	}
	rows.Close()
	fmt.Println("compile_options (filtered):", opts)

	_, err = db.Exec(`create table m(id integer primary key, project text not null, body text not null);
create virtual table f using fts5(body, content='m', content_rowid='id');
create index m_project on m(project, id);`)
	must(err)

	rnd := rand.New(rand.NewSource(42))
	words := make([]string, 0, 4010)
	for i := 0; i < 4000; i++ {
		words = append(words, fmt.Sprintf("w%04d", i))
	}
	words = append(words, "oauth", "token", "migration", "ECONNRESET", "tenant", "isolation")
	body := func() string {
		var b strings.Builder
		for i := 0; i < 60; i++ {
			b.WriteString(words[rnd.Intn(len(words))])
			b.WriteByte(' ')
		}
		return b.String()
	}
	t0 := time.Now()
	tx, err := db.Begin()
	must(err)
	im, _ := tx.Prepare("insert into m(id,project,body) values(?,?,?)")
	ifts, _ := tx.Prepare("insert into f(rowid,body) values(?,?)")
	for i := 1; i <= 100000; i++ {
		b := body()
		_, err = im.Exec(i, fmt.Sprintf("p%d", i%5), b)
		must(err)
		_, err = ifts.Exec(i, b)
		must(err)
	}
	must(tx.Commit())
	st, _ := os.Stat(dbp)
	fmt.Printf("bulk insert 100k rows: %.1fs db=%dMB\n", time.Since(t0).Seconds(), st.Size()>>20)
	_, err = db.Exec("pragma wal_checkpoint(TRUNCATE)")
	must(err)

	// concurrent: 1 writer (durable single-row commits) + 4 read-only connections running FTS queries, 4 seconds
	ro := "file:" + filepath.ToSlash(dbp) + "?mode=ro&_pragma=busy_timeout(5000)"
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var werr, rerr atomic.Int64
	var wlat []time.Duration
	rlat := make([][]time.Duration, 4)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			t := time.Now()
			if _, e := db.Exec("insert into m(project,body) values('p0','single durable insert oauth')"); e != nil {
				werr.Add(1)
			}
			wlat = append(wlat, time.Since(t))
		}
	}()
	terms := []string{"oauth token", "ECONNRESET", "tenant AND isolation", `"oauth token"`, "w0042"}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			rdb, e := sql.Open("sqlite", ro)
			must(e)
			defer rdb.Close()
			rdb.SetMaxOpenConns(1)
			for i := 0; ctx.Err() == nil; i++ {
				t := time.Now()
				q, e := rdb.Query(`select m.id, snippet(f,0,'[',']','..',8) from f join m on m.id=f.rowid
where f match ? and m.project=? order by bm25(f) limit 20`, terms[(i+r)%len(terms)], fmt.Sprintf("p%d", i%5))
				if e != nil {
					rerr.Add(1)
					continue
				}
				for q.Next() {
				}
				if q.Err() != nil {
					rerr.Add(1)
				}
				q.Close()
				rlat[r] = append(rlat[r], time.Since(t))
			}
		}(r)
	}

	// online backup while the writer and readers are running
	time.Sleep(500 * time.Millisecond)
	bak := filepath.Join(dir, "backup.db")
	bdb, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbp)+"?_pragma=busy_timeout(5000)")
	must(err)
	c, err := bdb.Conn(context.Background())
	must(err)
	tb := time.Now()
	must(c.Raw(func(dc any) error {
		b, ok := dc.(backuper)
		if !ok {
			return fmt.Errorf("driver conn has no NewBackup")
		}
		bk, e := b.NewBackup(bak)
		if e != nil {
			return e
		}
		for more := true; more; {
			if more, e = bk.Step(2000); e != nil {
				return e
			}
		}
		return bk.Finish()
	}))
	fmt.Printf("online backup during load: %.2fs\n", time.Since(tb).Seconds())
	c.Close()
	bdb.Close()
	wg.Wait()

	summ("writer durable commit under read load", wlat)
	var all []time.Duration
	for _, l := range rlat {
		all = append(all, l...)
	}
	summ("FTS5 bm25 scoped top-20, 4 ro conns, during writes", all)
	fmt.Println("writer errors:", werr.Load(), "reader errors:", rerr.Load())

	vdb, err := sql.Open("sqlite", "file:"+filepath.ToSlash(bak)+"?mode=ro")
	must(err)
	var ic string
	var n, nf int
	must(vdb.QueryRow("pragma integrity_check").Scan(&ic))
	must(vdb.QueryRow("select count(*) from m").Scan(&n))
	must(vdb.QueryRow("select count(*) from f where f match 'oauth'").Scan(&nf))
	fmt.Println("backup integrity_check:", ic, "rows:", n, "fts hits 'oauth':", nf)
	vdb.Close()
	must(db.QueryRow("pragma integrity_check").Scan(&ic))
	fmt.Println("source integrity_check:", ic)
	db.Close()
}
