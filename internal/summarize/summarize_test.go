package summarize

import (
	"strings"
	"testing"
)

func TestBuildInputKeepsEverythingThatFits(t *testing.T) {
	in, n := BuildInput([]Piece{{"user", "2026-11-01T10:00:00.000000000Z", "Fix the login bug."}, {"assistant", "2026-11-01T10:00:05.000000000Z", "Fixed in auth.go."}})
	if n != 2 || !strings.HasPrefix(in, "BEGIN SESSION\n") || !strings.HasSuffix(in, "END SESSION\n") {
		t.Fatalf("n=%d\n%s", n, in)
	}
	if !strings.Contains(in, "[user, 2026-11-01 10:00:00]\nFix the login bug.") || strings.Index(in, "Fix the login") > strings.Index(in, "Fixed in auth.go") {
		t.Errorf("pieces missing or out of order:\n%s", in)
	}
	if strings.Contains(in, "left out") {
		t.Errorf("nothing was left out, but the input says so:\n%s", in)
	}
}

func TestBuildInputLeavesOutTheMiddleAndSaysSo(t *testing.T) {
	var pieces []Piece
	for i := 0; i < 60; i++ {
		mark := "MIDDLE"
		if i == 0 {
			mark = "FIRSTPIECE"
		} else if i == 59 {
			mark = "LASTPIECE"
		}
		pieces = append(pieces, Piece{"user", "2026-11-01T10:00:00Z", mark + strings.Repeat(" é", MaxPieceBytes)}) // each piece is over the per-piece limit
	}
	in, n := BuildInput(pieces)
	if len(in) > MaxInputBytes+2_000 {
		t.Fatalf("input is %d bytes, over the limit of %d", len(in), MaxInputBytes)
	}
	if n >= 60 || n < 10 {
		t.Fatalf("kept %d of 60 pieces", n)
	}
	if !strings.Contains(in, "FIRSTPIECE") || !strings.Contains(in, "LASTPIECE") {
		t.Error("the start or the end of the session is missing")
	}
	if !strings.Contains(in, "piece(s) left out here") || !strings.Contains(in, "[cut: this piece was longer]") {
		t.Error("the input does not say that text was left out or cut")
	}
	if strings.Count(in, "MIDDLE")+2 != n {
		t.Errorf("reported %d pieces but the input holds %d", n, strings.Count(in, "MIDDLE")+2)
	}
	if !strings.Contains(in, "é") || strings.ContainsRune(in, '�') {
		t.Error("a cut split a character")
	}
}

func TestSystemPromptAndArgsAreSafeForACmdFile(t *testing.T) {
	if strings.ContainsAny(System, "\r\n\"%^&|<>!`$") {
		t.Errorf("the system prompt holds a character that is unsafe in a .cmd argument: %q", System)
	}
	a := strings.Join(Args("opus"), "\x00")
	for _, want := range []string{"-p", "--model=opus", "--safe-mode", "--strict-mcp-config", "--tools\x00\x00", "--no-session-persistence", "--output-format\x00json"} {
		if !strings.Contains(a, want) {
			t.Errorf("arguments lack %q: %q", want, Args("opus"))
		}
	}
	for _, never := range []string{"dangerously", "bypass", "--add-dir", "--continue", "--resume"} {
		if strings.Contains(a, never) {
			t.Errorf("arguments contain %q", never)
		}
	}
}

func TestParseResult(t *testing.T) {
	text, model, err := ParseResult([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"  Goal. Fix login.  ","modelUsage":{"claude-sonnet-5-5":{"inputTokens":3}}}` + "\n"))
	if err != nil || text != "Goal. Fix login." || model != "claude-sonnet-5-5" {
		t.Fatalf("%q %q %v", text, model, err)
	}
	for name, raw := range map[string]string{
		"not json":    "Welcome to Claude",
		"error":       `{"is_error":true,"subtype":"error_during_execution","result":"usage limit reached"}`,
		"empty":       `{"is_error":false,"result":"   "}`,
		"wrong shape": `["a"]`,
	} {
		if _, _, err := ParseResult([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	long, _, err := ParseResult([]byte(`{"result":"` + strings.Repeat("é", MaxOutputBytes) + `"}`))
	if err != nil || len(long) > MaxOutputBytes || strings.ContainsRune(long, '�') {
		t.Errorf("long answer: %d bytes, %v", len(long), err)
	}
}
