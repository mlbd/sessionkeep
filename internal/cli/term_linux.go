package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// isTerminal reports whether f is a terminal the person types into.
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}
