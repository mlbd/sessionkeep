//go:build !windows

package fsx

import "os"

// replace renames tmp over dst; on POSIX systems rename already replaces atomically under readers.
func replace(tmp, dst string) error { return os.Rename(tmp, dst) }
