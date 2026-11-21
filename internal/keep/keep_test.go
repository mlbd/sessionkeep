package keep

import (
	"bytes"
	"testing"
)

func TestWhatIsSealedOpensAgainAndIsNotStoredInTheClear(t *testing.T) {
	secret := []byte(`{"secret_key":"SYNTHETIC-SECRET-VALUE","passphrase":"correct horse"}`)
	sealed, err := Seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if Protected && bytes.Contains(sealed, []byte("SYNTHETIC-SECRET-VALUE")) {
		t.Fatal("the sealed bytes contain the secret in the clear")
	}
	back, err := Open(sealed)
	if err != nil || !bytes.Equal(back, secret) {
		t.Fatalf("open: %q %v", back, err)
	}
	if Protected {
		sealed[len(sealed)/2] ^= 0xff
		if _, err := Open(sealed); err == nil {
			t.Error("altered bytes were opened")
		}
	}
}
