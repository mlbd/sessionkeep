// Package autostart manages an optional per-user logon entry that stores waiting events when the
// user logs on. SessionKeep has no resident process (ADR-0003); without this task, events captured at the
// very end of a session wait until the next Claude Code session starts. It is opt-in and never
// created by install.
package autostart

import "errors"

// ErrUnsupported means this system has no implementation yet.
var ErrUnsupported = errors.New("auto-start is only implemented for Windows so far")

// Status describes the task.
type Status struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Name      string `json:"task_name,omitempty"`
	Command   string `json:"command,omitempty"`
	Detail    string `json:"detail,omitempty"`
}
