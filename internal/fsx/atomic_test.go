package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Readers hammer a file while a writer replaces it: every read must succeed and see one complete
// version, never a mixture and never an error.
func TestReplaceWhileReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state é.json")
	a, b := strings.Repeat("A", 4096), strings.Repeat("B", 8192)
	if err := WriteAtomic(path, []byte(a)); err != nil {
		t.Fatal(err)
	}
	var stop atomic.Bool
	var bad atomic.Int64
	var first atomic.Value
	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				raw, err := ReadFile(path)
				if err != nil || (string(raw) != a && string(raw) != b) {
					bad.Add(1)
					first.CompareAndSwap(nil, errors.Join(err, errors.New("len="+string(rune('0'+len(raw)%10)))).Error())
				}
			}
		}()
	}
	for i := 0; i < 400; i++ {
		content := a
		if i%2 == 1 {
			content = b
		}
		if err := WriteAtomic(path, []byte(content)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	stop.Store(true)
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d reads failed or saw a partial file; first: %v", bad.Load(), first.Load())
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
	if len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

// If the primary copy is gone (the moment of a replace, or a user deleting it), the second copy
// answers; "not found" is reported only when neither exists.
func TestSecondCopyAnswersWhenThePrimaryIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteAtomic(path, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path + AltSuffix); string(raw) != "v1" {
		t.Fatalf("second copy holds %q", raw)
	}
	os.Remove(path)
	if raw, err := ReadFile(path); err != nil || string(raw) != "v1" {
		t.Fatalf("with the primary missing: %q, %v", raw, err)
	}
	os.Remove(path + AltSuffix)
	if _, err := ReadFile(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("with both missing: want not-exist, got %v", err)
	}
	// A damaged primary is returned as is: falling back would hide the damage.
	WriteAtomic(path, []byte("good"))
	os.WriteFile(path, []byte("damaged"), 0o600)
	if raw, _ := ReadFile(path); string(raw) != "damaged" {
		t.Fatalf("got %q; a present primary must always win", raw)
	}
}

func TestReadFileMissingIsImmediate(t *testing.T) {
	if _, err := ReadFile(filepath.Join(t.TempDir(), "nope")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want not-exist, got %v", err)
	}
}

func TestWriteAtomicIntoMissingDirectoryFails(t *testing.T) {
	if err := WriteAtomic(filepath.Join(t.TempDir(), "missing", "x"), []byte("x")); err == nil {
		t.Fatal("want an error")
	}
}
