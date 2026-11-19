// Package lease provides the single-writer lock from ADR-0003: whoever holds it is the only
// process allowed to write the database. The operating system releases it when the holder dies,
// so a crash can never leave it stuck.
package lease

import (
	"errors"
	"os"
	"time"
)

// ErrHeld means another process holds the lease.
var ErrHeld = errors.New("the ingest lease is held by another SessionKeep process")

// Lease is a held lock. Release it when done; process exit releases it too.
type Lease struct{ f *os.File }

// Acquire takes the lease at path without waiting. The file is created if needed and is never
// deleted: its content is irrelevant, only the OS lock on it matters.
func Acquire(path string) (*Lease, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lease{f: f}, nil
}

// AcquireWait retries Acquire until it succeeds or wait has passed.
func AcquireWait(path string, wait time.Duration) (*Lease, error) {
	deadline := time.Now().Add(wait)
	for {
		l, err := Acquire(path)
		if !errors.Is(err, ErrHeld) || time.Now().After(deadline) {
			return l, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Release gives the lease up.
func (l *Lease) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlock(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}
