// Package plugin generates the Claude Code plugin directory for this installation.
//
// The hook definitions are generated rather than shipped as static files because every hook must
// be an exec-form command holding the absolute path of the native binary (ADR-0002): on Windows a
// shell-form hook costs 70–190 ms before it does anything, and .cmd shims cannot be exec'd.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Name is the plugin identifier; Claude Code namespaces the plugin's components under it.
const Name = "sessionkeep"

// CaptureEvents are the hook events SessionKeep listens to, in the order they are written.
var CaptureEvents = []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "PostToolUseFailure", "Stop", "StopFailure", "PostCompact", "SessionEnd"}

// DrainEvents are the events after which a background drain is started. The drain is a second,
// asynchronous hook entry: the host runs it in the background, it holds the ingest lease, stores
// what the capture hooks spooled, and exits. It is not a daemon and is not started by SessionKeep.
var DrainEvents = []string{"SessionStart", "Stop"}

type hookCmd struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout,omitempty"`
	Async   bool     `json:"async,omitempty"`
}

type matcher struct {
	Hooks []hookCmd `json:"hooks"`
}

// Hooks returns the content of hooks/hooks.json for a binary at exe.
func Hooks(exe string) map[string]any {
	events := map[string][]matcher{}
	for _, e := range CaptureEvents {
		events[e] = []matcher{{Hooks: []hookCmd{{Type: "command", Command: exe, Args: []string{"hook", e}, Timeout: 10}}}}
	}
	for _, e := range DrainEvents {
		events[e][0].Hooks = append(events[e][0].Hooks, hookCmd{Type: "command", Command: exe, Args: []string{"ingest", "--quiet"}, Async: true})
	}
	return map[string]any{"hooks": events}
}

// Manifest returns the content of .claude-plugin/plugin.json.
func Manifest(version string) map[string]any {
	return map[string]any{
		"name":        Name,
		"displayName": "SessionKeep",
		"version":     version,
		"description": "Local-first developer memory: captures permitted conversation events for projects you registered.",
		"author":      map[string]string{"name": "SessionKeep"},
		"license":     "Apache-2.0",
	}
}

// ErrNotOurs means the target directory exists, is not empty and is not a SessionKeep plugin.
var ErrNotOurs = errors.New("directory exists and is not a SessionKeep plugin directory")

// Write creates or refreshes the plugin directory at dir for the binary at exe (absolute path).
// It only ever writes its own two files, and refuses a non-empty directory it did not create.
func Write(dir, exe, version string, withMCP bool) error {
	if !filepath.IsAbs(exe) {
		return fmt.Errorf("binary path must be absolute, got %q", exe)
	}
	if fi, err := os.Stat(exe); err != nil || fi.IsDir() {
		return fmt.Errorf("binary not found at %s", exe)
	}
	manifestPath := filepath.Join(dir, ".claude-plugin", "plugin.json")
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		var m struct{ Name string }
		raw, err := os.ReadFile(manifestPath)
		if err != nil || json.Unmarshal(raw, &m) != nil || m.Name != Name {
			return fmt.Errorf("%w: %s", ErrNotOurs, dir)
		}
	}
	files := map[string]any{
		manifestPath: Manifest(version),
		filepath.Join(dir, "hooks", "hooks.json"): Hooks(exe),
	}
	for path, content := range files {
		raw, err := json.MarshalIndent(content, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	// The MCP server is optional: a resident process per session is only added when asked for.
	mcpPath := filepath.Join(dir, ".mcp.json")
	if withMCP {
		raw, err := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{Name: map[string]any{"command": exe, "args": []string{"mcp"}}}}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(mcpPath, append(raw, 10), 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(mcpPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	for name, body := range Skills(exe) {
		path := filepath.Join(dir, "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}
