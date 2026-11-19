//go:build !windows

package autostart

// TaskName is unused on this system.
func TaskName(string) string { return "" }

// Enable is not implemented on this system.
func Enable(string, string) (Status, error) { return Status{}, ErrUnsupported }

// Disable is not implemented on this system; there is nothing to remove.
func Disable(string) (Status, error) { return Status{}, nil }

// Query reports that auto-start is unsupported here.
func Query(string) (Status, error) { return Status{Detail: ErrUnsupported.Error()}, nil }

// RunNow is not implemented on this system.
func RunNow(string) error { return ErrUnsupported }
