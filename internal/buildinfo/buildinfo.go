// Package buildinfo reports what this binary was built from.
package buildinfo

import (
	"runtime"
	"runtime/debug"
)

// Version is set at release build time with -ldflags "-X sessionkeep/internal/buildinfo.Version=...".
var Version = "0.0.0-dev"

// Info is the machine-readable build description.
type Info struct {
	Version  string `json:"version"`
	Commit   string `json:"commit,omitempty"`
	Modified bool   `json:"modified,omitempty"`
	Go       string `json:"go"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// Get returns build information. Commit is empty when the binary was not built from a Git checkout.
func Get() Info {
	i := Info{Version: Version, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				i.Commit = s.Value
			case "vcs.modified":
				i.Modified = s.Value == "true"
			}
		}
	}
	return i
}
