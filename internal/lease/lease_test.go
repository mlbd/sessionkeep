package lease

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOnlyOneHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest é.lease")
	a, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(path); !errors.Is(err, ErrHeld) {
		t.Fatalf("second holder: want ErrHeld, got %v", err)
	}
	start := time.Now()
	if _, err := AcquireWait(path, 120*time.Millisecond); !errors.Is(err, ErrHeld) || time.Since(start) < 100*time.Millisecond {
		t.Fatalf("AcquireWait should wait then report ErrHeld: %v after %v", err, time.Since(start))
	}
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	if err := a.Release(); err != nil {
		t.Fatalf("double release must be harmless: %v", err)
	}
	b, err := Acquire(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	b.Release()
}

func TestAcquireWaitGetsTheLeaseWhenItIsFreed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lease")
	a, _ := Acquire(path)
	go func() { time.Sleep(60 * time.Millisecond); a.Release() }()
	b, err := AcquireWait(path, 2*time.Second)
	if err != nil {
		t.Fatalf("want the lease once freed, got %v", err)
	}
	b.Release()
}

func TestUnusablePathIsAnErrorNotHeld(t *testing.T) {
	if _, err := Acquire(filepath.Join(t.TempDir(), "missing-dir", "x.lease")); err == nil || errors.Is(err, ErrHeld) {
		t.Fatalf("want a plain error, got %v", err)
	}
}
