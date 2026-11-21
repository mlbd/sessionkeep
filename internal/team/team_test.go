package team

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"sessionkeep/internal/handoff"
)

func sample() handoff.File {
	return handoff.File{Kind: handoff.Kind, Format: handoff.Format, CreatedAt: "2026-11-22T10:00:00Z", MadeBy: "test", SourceDevice: "dev-1", Project: handoff.Project{ID: "p-1", Name: "app"},
		Instructions: []handoff.Instruction{{ID: "i-1", Text: "Use tabs SEALMARK.", Recorded: "2026-11-20T10:00:00Z"}},
		Checkpoints:  []handoff.Checkpoint{}, Sessions: []handoff.Session{}}
}

func TestABundleIsUnreadableWithoutThePassphraseAndRefusedWhenAltered(t *testing.T) {
	sealed, err := SealBundle(sample(), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("SEALMARK")) || bytes.Contains(sealed, []byte("sessionkeep-handoff")) {
		t.Fatal("the bundle carries readable text")
	}
	back, err := OpenBundle(sealed, "correct horse battery")
	if err != nil || len(back.Instructions) != 1 || back.Instructions[0].Text != "Use tabs SEALMARK." {
		t.Fatalf("open: %+v %v", back, err)
	}
	if _, err := OpenBundle(sealed, "another passphrase"); !errors.Is(err, ErrPassphrase) {
		t.Errorf("a wrong passphrase: %v", err)
	}
	for _, at := range []int{len(sealed) - 5, len(sealed) / 2} {
		bad := append([]byte(nil), sealed...)
		bad[at] ^= 0x01
		if _, err := OpenBundle(bad, "correct horse battery"); err == nil {
			t.Errorf("a bundle altered at byte %d was opened", at)
		}
	}
	if _, err := OpenBundle([]byte("not a bundle at all"), "correct horse battery"); !errors.Is(err, handoff.ErrInvalid) {
		t.Errorf("something that is not a bundle: %v", err)
	}
}

func TestBundleNamesHideTheSharedNameAndSayWhoSent(t *testing.T) {
	key := BundleKey("shop-app", "device-A", time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC), "abc123")
	if bytes.Contains([]byte(key), []byte("shop")) || !bytes.HasPrefix([]byte(key), []byte(Prefix("shop-app"))) {
		t.Errorf("key %s", key)
	}
	if !SentBy(key, "device-A") || SentBy(key, "device-B") {
		t.Errorf("sender of %s is told wrongly", key)
	}
	if Prefix("shop-app") == Prefix("shop-api") {
		t.Error("two names share a place")
	}
	for _, bad := range []string{"", "A", "Shop", "shop app", "-shop", "x"} {
		if CheckName(bad) == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
}
