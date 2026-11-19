//go:build !windows

package ingest

import "errors"

func lockExclusive(string) (func(), error) { return nil, errors.New("not supported on this system") }
