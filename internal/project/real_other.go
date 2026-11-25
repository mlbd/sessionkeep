//go:build !windows

package project

import "path/filepath"

// realPath resolves the symbolic links in an existing path.
func realPath(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}
