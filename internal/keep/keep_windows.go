//go:build windows

// Package keep protects small secrets (storage keys, a passphrase) before they are written to a
// file, with what the operating system offers for that.
package keep

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protected says whether Seal ties the result to the signed-in user of this computer. On Windows
// it does (the Data Protection API): the sealed bytes cannot be opened by another Windows user
// or on another computer.
const Protected = true

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func take(out *windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

// Seal encrypts plain for the signed-in Windows user.
func Seal(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(plain), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return take(&out), nil
}

// Open reverses Seal.
func Open(sealed []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(sealed), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return take(&out), nil
}
