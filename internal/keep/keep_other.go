//go:build !windows

// Package keep protects small secrets (storage keys, a passphrase) before they are written to a
// file, with what the operating system offers for that.
package keep

// Protected says whether Seal ties the result to the signed-in user of this computer. On this
// system it does not yet: the bytes are kept as they are, in a file only the user can read.
const Protected = false

// Seal returns plain unchanged; see Protected.
func Seal(plain []byte) ([]byte, error) { return append([]byte(nil), plain...), nil }

// Open returns sealed unchanged; see Protected.
func Open(sealed []byte) ([]byte, error) { return append([]byte(nil), sealed...), nil }
