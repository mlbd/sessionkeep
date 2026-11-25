//go:build !windows

package project

// shortName: only Windows has 8.3 short names.
func shortName(string) string { return "" }
