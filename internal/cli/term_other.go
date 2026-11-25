//go:build !windows && !linux && !darwin

package cli

import "os"

// isTerminal reports whether f looks like a terminal. Without a precise check on this system, a
// character device counts as one.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
