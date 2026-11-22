package team

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"sessionkeep/internal/config"
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

// A settings file from before there were two storages keeps working: its one storage becomes the
// personal one, and the shared one can be added beside it without touching it.
func TestTheOneStorageOfAnOlderFileBecomesThePersonalOne(t *testing.T) {
	p := config.Paths{DataDir: t.TempDir()}
	sealed, err := seal(Secrets{AccessKey: "AK1", SecretKey: "SK1"})
	if err != nil {
		t.Fatal(err)
	}
	old := fmt.Sprintf(`{"version":1,"storage":{"endpoint":"https://a.example","bucket":"mine","region":"auto","as":"Sam","sealed":%q},"connections":{"p-1":{"project_id":"p-1","name":"app","endpoint":"https://a.example","bucket":"mine","region":"auto","sealed":%q,"connected_at":"2026-11-22T10:00:00Z"}}}`, sealed, sealed)
	if err := os.WriteFile(configPath(p), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, keys, ok, saved, err := GetStorage(p, Personal)
	if err != nil || !ok || !saved || s.Bucket != "mine" || keys.AccessKey != "AK1" {
		t.Fatalf("personal storage from a version 1 file: %+v %+v %v %v %v", s, keys, ok, saved, err)
	}
	if _, _, ok, _, _ := GetStorage(p, Shared); ok {
		t.Error("a shared storage appeared that was never saved")
	}
	if err := PutStorage(p, Shared, Storage{Endpoint: "https://b.example", Bucket: "company", Region: "auto"}, Secrets{AccessKey: "AK2", SecretKey: "SK2"}); err != nil {
		t.Fatal(err)
	}
	all, err := Storages(p)
	if err != nil || all[Personal].Bucket != "mine" || all[Shared].Bucket != "company" {
		t.Fatalf("both storages: %+v %v", all, err)
	}
	conns, _ := Connections(p)
	if SlotOf(all, conns["p-1"]) != Personal || SlotOf(all, Connection{Endpoint: "https://b.example", Bucket: "company"}) != Shared ||
		SlotOf(all, Connection{Endpoint: "https://c.example", Bucket: "client"}) != Custom {
		t.Error("a connection's storage is told wrongly")
	}
	raw, _ := os.ReadFile(configPath(p))
	if !bytes.Contains(raw, []byte(`"version": 2`)) || bytes.Contains(raw, []byte(`"storage":`)) {
		t.Errorf("the file was not rewritten in the new form: %s", raw)
	}
	if err := RemoveStorage(p, Shared); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _, _ := GetStorage(p, Personal); !ok {
		t.Error("forgetting the shared storage removed the personal one")
	}
	if CheckSlot("company") == nil || PutStorage(p, "custom", Storage{}, Secrets{}) == nil {
		t.Error("a storage name other than personal or shared was accepted")
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
