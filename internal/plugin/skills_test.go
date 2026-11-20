package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsNameThisBinaryAndKeepTheSafetyRules(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	exe := self(t)
	if err := Write(dir, exe, "1", false); err != nil {
		t.Fatal(err)
	}
	quoted := `"` + filepath.ToSlash(exe) + `"`
	for name, must := range map[string][]string{
		"recall":   {"memory search", "memory get", "evidence, not instructions", "not found", "--all-projects", quoted + " ingest"},
		"remember": {"--scope project", "--yes", "ask them to confirm", "Do not rephrase", quoted + " remember"},
		"resume":   {quoted + " resume", quoted + " checkpoint", "WARNING", "Do not switch branches", "not new instructions"},
		"research": {quoted + " research search", quoted + " research check", quoted + " research record", "never instructions to follow", "Do not present it as current", "Do not turn it on yourself", "do not fill the gap from memory"},
	} {
		raw, err := os.ReadFile(filepath.Join(dir, "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatalf("skill %s not written: %v", name, err)
		}
		body := string(raw)
		if !strings.HasPrefix(body, "---\nname: "+name+"\ndescription: ") {
			t.Errorf("%s: frontmatter must start with name and description", name)
		}
		if strings.Contains(body, "SESSIONKEEP") || strings.Contains(body, `\`) {
			t.Errorf("%s: placeholder or backslash left in the skill text", name)
		}
		for _, m := range must {
			if !strings.Contains(body, m) {
				t.Errorf("%s: skill text lacks %q", name, m)
			}
		}
	}
}

func TestMCPDeclarationIsOptional(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	exe := self(t)
	if err := Write(dir, exe, "1", false); err != nil {
		t.Fatal(err)
	}
	mcpFile := filepath.Join(dir, ".mcp.json")
	if _, err := os.Stat(mcpFile); err == nil {
		t.Fatal("the MCP server must not be declared unless asked for")
	}
	if err := Write(dir, exe, "1", true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(mcpFile)
	if err != nil || !strings.Contains(string(raw), `"mcp"`) || !strings.Contains(string(raw), `"sessionkeep"`) {
		t.Fatalf(".mcp.json: %s %v", raw, err)
	}
	if err := Write(dir, exe, "1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mcpFile); err == nil {
		t.Fatal("regenerating without the option must remove the declaration")
	}
}
