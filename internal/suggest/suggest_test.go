package suggest

import (
	"fmt"
	"testing"
)

func TestARequestMadeInSeveralSessionsIsSuggested(t *testing.T) {
	var said []Said
	add := func(session, text string) {
		said = append(said, Said{Session: session, Project: "p", At: fmt.Sprintf("2026-11-%02dT10:00:00Z", 20-len(said)), Text: text})
	}
	add("s5", "Fix the login page. Always use pnpm, not npm!")
	add("s4", "Please always use pnpm not npm.\nThen run the tests.")
	add("s3", "always use pnpm, not npm")
	add("s3", "Always use pnpm not npm")            // the same session again counts once
	add("s2", "Never push to main without asking.") // twice only
	add("s1", "never push to main without asking")
	add("s6", "Add a totals row to the invoice table.") // a task, said once
	add("s7", "Don't use any in TypeScript.")
	add("s8", "Do not use any in TypeScript")
	add("s9", "don't use any in typescript please")

	got := Find(said, nil, 3)
	if len(got) != 2 {
		t.Fatalf("want 2 suggestions, got %+v", got)
	}
	if got[0].Text != "Always use pnpm, not npm" || got[0].Sessions != 3 || got[0].LastAt != "2026-11-20T10:00:00Z" || got[0].FirstAt != "2026-11-17T10:00:00Z" {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Sessions != 3 || got[1].Text != "Don't use any in TypeScript" {
		t.Errorf("second: %+v", got[1])
	}
	// What an instruction already says is not suggested again.
	if left := Find(said, []string{"Use pnpm, never npm. Keep answers short."}, 3); len(left) != 1 || left[0].Text != "Don't use any in TypeScript" {
		t.Errorf("with an existing instruction: %+v", left)
	}
	if two := Find(said, nil, 2); len(two) != 3 {
		t.Errorf("with a lower bar: %+v", two)
	}
}
