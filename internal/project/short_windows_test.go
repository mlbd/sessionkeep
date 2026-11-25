//go:build windows

package project

import "golang.org/x/sys/windows"

// shortName returns the 8.3 short form of an existing path, or "" when the volume has none.
func shortName(p string) string {
	in, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(in, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) >= len(buf) {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
