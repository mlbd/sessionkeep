package history

import (
	"strings"
	"testing"
)

// Synthetic lines in the observed shape. No real session content is used in tests.
const sample = `{"type":"mode","mode":"default","sessionId":"S1"}
{"type":"user","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u1","timestamp":"2026-11-01T10:00:00.5Z","message":{"role":"user","content":"Fix the login bug."}}
{"type":"assistant","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"a1","timestamp":"2026-11-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"PRIVATE-THOUGHT","signature":"x"},{"type":"text","text":"Found it."},{"type":"tool_use","id":"t1","name":"Edit","input":{"secret":"TOOL-INPUT"}},{"type":"text","text":"Fixed."}]}}
{"type":"user","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u2","timestamp":"2026-11-01T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"TOOL-OUTPUT"}]}}
{"type":"user","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u3","timestamp":"2026-11-01T10:00:04Z","message":{"role":"user","content":"<command-name>/mcp</command-name>"}}
{"type":"user","isMeta":true,"isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u4","timestamp":"2026-11-01T10:00:05Z","message":{"role":"user","content":"META-TEXT"}}
{"type":"user","isSidechain":true,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u5","timestamp":"2026-11-01T10:00:06Z","message":{"role":"user","content":"SUBAGENT-PROMPT"}}
{"type":"user","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u6","timestamp":"2026-11-01T10:00:07Z","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>INJECTED</system-reminder>"},{"type":"text","text":"Thanks, now add a test."},{"type":"image","source":{"data":"IMAGE-DATA"}}]}}
{"type":"api-request-blob","hash":"h","sessionId":"S1","message":{"role":"user","content":[{"type":"text","text":"BLOB-COPY"}]}}
{"type":"user","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"u7","timestamp":"not a time","message":{"role":"user","content":"BAD-TIME"}}
{"type":"user","message":{"role":"user","content":"TRUNCATED-LINE
{"type":"assistant","isSidechain":false,"cwd":"C:\\work\\app","sessionId":"S1","uuid":"a2","timestamp":"2026-11-01T10:00:09Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{}}]}}`

func TestParseKeepsOnlyWhatWasSaid(t *testing.T) {
	var got []Turn
	st, err := Parse(strings.NewReader(sample), func(x Turn) { got = append(got, x) })
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ who, uuid, text string }{
		{"user", "u1", "Fix the login bug."},
		{"assistant", "a1", "Found it.\n\nFixed."},
		{"user", "u6", "Thanks, now add a test."},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d turns, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Who != w.who || got[i].UUID != w.uuid || got[i].Text != w.text || got[i].SessionID != "S1" || got[i].Cwd != `C:\work\app` {
			t.Errorf("turn %d = %+v, want %+v", i, got[i], w)
		}
	}
	if got[0].At.Nanosecond() != 500_000_000 || got[0].At.Location().String() != "UTC" {
		t.Errorf("time not parsed as UTC with fraction: %v", got[0].At)
	}
	for _, x := range got {
		for _, never := range []string{"PRIVATE-THOUGHT", "TOOL-INPUT", "TOOL-OUTPUT", "META-TEXT", "SUBAGENT-PROMPT", "INJECTED", "IMAGE-DATA", "BLOB-COPY", "BAD-TIME", "TRUNCATED-LINE", "command-name"} {
			if strings.Contains(x.Text, never) {
				t.Errorf("turn %s contains %q, which must never be imported", x.UUID, never)
			}
		}
	}
	if st.Lines != 12 || st.Turns != 3 || st.BadLines != 1 || st.Sidechain != 1 || st.Injected != 2 || st.NoText != 2 || st.MissingField != 1 {
		t.Errorf("stats = %+v", st)
	}
}

func TestParseSkipsOversizeLinesAndKeepsGoing(t *testing.T) {
	big := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", MaxLineBytes) + `"}}`
	ok := `{"type":"user","cwd":"/w","sessionId":"S","uuid":"u","timestamp":"2026-11-01T10:00:00Z","message":{"role":"user","content":"after the big line"}}`
	n := 0
	st, err := Parse(strings.NewReader(big+"\n"+ok+"\n"), func(x Turn) {
		n++
		if x.Text != "after the big line" {
			t.Errorf("unexpected turn %q", x.Text[:20])
		}
	})
	if err != nil || n != 1 || st.Oversize != 1 || st.Lines != 2 {
		t.Fatalf("err=%v turns=%d stats=%+v", err, n, st)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(sample)
	f.Add(`{"type":"user","message":{"role":"user","content":[{"type":"text"}]}}`)
	f.Add("\n\n{}\n[]\nnull\n\"x\"")
	f.Fuzz(func(t *testing.T, in string) {
		st, err := Parse(strings.NewReader(in), func(x Turn) {
			if x.SessionID == "" || x.UUID == "" || x.Cwd == "" || strings.TrimSpace(x.Text) == "" || (x.Who != "user" && x.Who != "assistant") || x.At.IsZero() {
				t.Fatalf("incomplete turn emitted: %+v", x)
			}
		})
		if err != nil {
			t.Fatalf("reading from memory failed: %v", err)
		}
		if st.Turns > st.Lines {
			t.Fatalf("more turns than lines: %+v", st)
		}
	})
}
