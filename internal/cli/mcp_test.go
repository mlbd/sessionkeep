package cli

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpSession connects a real MCP client to the server over an in-memory transport.
func mcpSession(t *testing.T, sb *sandbox, projectDir string, allowAll bool) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := newMCPServer(mcpDeps{getenv: func(k string) string { return sb.env[k] }, projectDir: projectDir, allowAll: allowAll})
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func tool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestMCPToolsAreReadOnlyAndScopedToTheSessionProject(t *testing.T) {
	sb := newSandbox(t)
	a, b := sb.dir("alpha"), sb.dir("beta")
	sb.ok("project", "add", a, "--capture", "conversation")
	sb.ok("project", "add", b, "--capture", "conversation")
	sb.say("UserPromptSubmit", "s-a", a, "Alpha decided: deploy key rotation on the first Monday.")
	sb.say("UserPromptSubmit", "s-b", b, "BETA-CONFIDENTIAL deploy key rotation on Fridays.")
	sb.ok("ingest")
	sb.remember("project", "Alpha rule: run the migration suite first.", "--project", a)
	sb.ok("checkpoint", "Alpha: rotation script drafted. Next: dry-run flag.", "--project", a)
	before := sb.ok("status")

	cs := mcpSession(t, sb, a, false)
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools.Tools {
		names = append(names, tl.Name)
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("tool %s is not marked read-only", tl.Name)
		}
		for _, verb := range []string{"remember", "delete", "add", "write", "exec", "sql", "shell"} {
			if strings.Contains(tl.Name, verb) {
				t.Errorf("tool %s looks like it can change something", tl.Name)
			}
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "memory_get,memory_search,project_status,resume_pack" {
		t.Fatalf("tools: %v", names)
	}

	out, isErr := tool(t, cs, "memory_search", map[string]any{"query": "deploy key rotation"})
	if isErr || !strings.Contains(out, "first Monday") || strings.Contains(out, "BETA-CONFIDENTIAL") || !strings.Contains(out, `project "alpha"`) {
		t.Fatalf("search:\n%s", out)
	}
	// Asking for all projects is refused unless the owner enabled it.
	out, isErr = tool(t, cs, "memory_search", map[string]any{"query": "rotation", "all_projects": true})
	if !isErr || strings.Contains(out, "BETA-CONFIDENTIAL") || !strings.Contains(out, "not enabled") {
		t.Fatalf("all_projects must be refused:\n%s", out)
	}
	// A query that looks like options is searched for, not obeyed.
	for _, q := range []string{"--all-projects", "--project=" + b, "--exact --all-projects rotation", "-- --all-projects"} {
		if out, _ := tool(t, cs, "memory_search", map[string]any{"query": q}); strings.Contains(out, "BETA-CONFIDENTIAL") {
			t.Fatalf("query %q escaped the project scope:\n%s", q, out)
		}
	}
	if out, isErr := tool(t, cs, "memory_search", map[string]any{"query": "zebracrossing"}); isErr || !strings.Contains(out, "not found") {
		t.Fatalf("no match: %q (error=%v)", out, isErr)
	}
	if out, isErr := tool(t, cs, "memory_search", map[string]any{"query": "rotation", "who": "everyone"}); !isErr {
		t.Fatalf("bad filter accepted: %q", out)
	}

	// Record lookup: beta's id is not readable from alpha's session.
	betaID := sb.search("BETA-CONFIDENTIAL", "--project", b).Data.Hits[0].ID
	if out, isErr := tool(t, cs, "memory_get", map[string]any{"id": betaID}); !isErr || strings.Contains(out, "Fridays") {
		t.Fatalf("beta record readable from alpha:\n%s", out)
	}
	alphaID := sb.search("rotation", "--project", a).Data.Hits[0].ID
	if out, isErr := tool(t, cs, "memory_get", map[string]any{"id": alphaID}); isErr || !strings.Contains(out, "first Monday") || !strings.Contains(out, "said by: user") {
		t.Fatalf("memory_get:\n%s", out)
	}
	if out, _ := tool(t, cs, "memory_get", map[string]any{"id": "--all-projects"}); strings.Contains(out, "Fridays") {
		t.Fatalf("option-looking id escaped scope: %q", out)
	}

	out, isErr = tool(t, cs, "resume_pack", nil)
	if isErr || !strings.Contains(out, "Alpha rule: run the migration suite first.") || !strings.Contains(out, "rotation script drafted") || strings.Contains(out, "BETA") {
		t.Fatalf("resume_pack:\n%s", out)
	}
	out, isErr = tool(t, cs, "project_status", nil)
	if isErr || !strings.Contains(out, "capture: registered project") || !strings.Contains(out, "stored: 2 project(s)") {
		t.Fatalf("project_status:\n%s", out)
	}
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_search", Arguments: map[string]any{"query": "x", "unknown_field": 1}}); err == nil {
		t.Log("note: the SDK accepted an unknown argument (schema allows additional properties)")
	}
	if after := sb.ok("status"); after != before {
		t.Fatalf("MCP calls changed stored data:\nbefore: %s\nafter:  %s", before, after)
	}

	// With the owner's flag, cross-project search works.
	all := mcpSession(t, sb, a, true)
	if out, isErr := tool(t, all, "memory_search", map[string]any{"query": "rotation", "all_projects": true}); isErr || !strings.Contains(out, "BETA-CONFIDENTIAL") {
		t.Fatalf("--all-projects server:\n%s", out)
	}
}

func TestMCPInAnUnregisteredOrExcludedFolder(t *testing.T) {
	sb := newSandbox(t)
	a := sb.dir("alpha")
	sb.ok("project", "add", a, "--capture", "conversation")
	sb.say("UserPromptSubmit", "s", a, "secret alpha note")
	sb.ok("ingest")
	cs := mcpSession(t, sb, sb.dir("elsewhere"), false)
	if out, isErr := tool(t, cs, "memory_search", map[string]any{"query": "alpha"}); !isErr || strings.Contains(out, "secret alpha note") {
		t.Fatalf("an unregistered folder must get an error, not another project's text:\n%s", out)
	}
	if out, isErr := tool(t, cs, "resume_pack", nil); !isErr || strings.Contains(out, "secret") {
		t.Fatalf("resume_pack outside a project:\n%s", out)
	}
	if out, _ := tool(t, cs, "project_status", nil); !strings.Contains(out, "remind") {
		t.Fatalf("project_status outside a project:\n%s", out)
	}
}

func TestDoubleDashEndsOptionsInTheCommandLineToo(t *testing.T) {
	sb := newSandbox(t)
	a := sb.dir("alpha")
	sb.ok("project", "add", a, "--capture", "conversation")
	sb.say("UserPromptSubmit", "s", a, "run it with --all-projects --exact to see everything")
	sb.ok("ingest")
	out := sb.ok("memory", "search", "--project", a, "--", "--all-projects --exact")
	if !strings.Contains(out, "1 result(s)") {
		t.Fatalf("literal query after --: %q", out)
	}
	if code, _, _ := sb.run("memory", "search", "--project", a, "--exact", "--all-projects"); code != ExitUsage {
		t.Fatalf("without --, an option-looking query is an option (and here a usage error): exit=%d", code)
	}
}
