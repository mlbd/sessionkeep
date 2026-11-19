//go:build !windows

package fsx

import "os"

func readOnce(path string) ([]byte, error) { return os.ReadFile(path) }
