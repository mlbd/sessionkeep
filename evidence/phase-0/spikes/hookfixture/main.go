// Phase 0 spike P0-02: records real Claude Code hook stdin payloads. Synthetic session only. Throwaway code.
// Usage (exec-form hook): recorder.exe <outdir> <EventName>
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	out, ev := os.Args[1], os.Args[2]
	start := time.Now()
	raw, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	name := fmt.Sprintf("%d-%s.json", start.UnixNano(), ev)
	os.WriteFile(filepath.Join(out, name), raw, 0o600)

	var in map[string]any
	json.Unmarshal(raw, &in)
	note := map[string]any{"event": ev, "stdin_bytes": len(raw), "env_plugin_root_set": os.Getenv("CLAUDE_PLUGIN_ROOT") != "",
		"env_project_dir_set": os.Getenv("CLAUDE_PROJECT_DIR") != "", "arg0": os.Args[0]}
	if tp, _ := in["transcript_path"].(string); tp != "" {
		b, err := os.ReadFile(tp)
		note["transcript_exists"] = err == nil
		if lam, _ := in["last_assistant_message"].(string); lam != "" && err == nil {
			probe := lam
			if len(probe) > 40 {
				probe = probe[:40]
			}
			note["transcript_contains_last_assistant_message"] = strings.Contains(string(b), probe)
		}
	}
	nb, _ := json.Marshal(note)
	os.WriteFile(filepath.Join(out, name+".note"), nb, 0o600)

	ctx := ""
	switch ev {
	case "SessionStart":
		ctx = "SessionKeep fixture record (synthetic test data): the user recorded that the first project codeword is ZEBRA-4471."
	case "UserPromptSubmit":
		var b strings.Builder
		b.WriteString("SessionKeep fixture record (synthetic test data). Recorded user notes follow, 1 per line.\n")
		for i := 1; b.Len() < 8800; i++ {
			fmt.Fprintf(&b, "Note %03d: synthetic filler entry with no meaning, kept only to occupy space in this record.\n", i)
		}
		b.WriteString("Final note: the user recorded that the second project codeword is HERON-9026.")
		ctx = b.String()
	}
	if ctx != "" {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": ev, "additionalContext": ctx}})
		os.WriteFile(filepath.Join(out, name+".ctxlen"), []byte(fmt.Sprint(len(ctx))), 0o600)
	}
}
