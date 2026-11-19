package redact

import (
	"testing"
	"unicode/utf8"
)

// Redacting already-redacted text must change nothing: stored text is redacted once at capture
// and may pass through the rules again on import or export.
func FuzzTextIsIdempotent(f *testing.F) {
	for _, s := range []string{
		"password=hunter2hunter2", "curl -u admin:s3cretPassw0rd https://example.test", "https://user:pa55word@example.test/x",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", "api_key: 'AKIAABCDEFGHIJKLMNOP'", "plain words only", "token=\n", "a=b=c password==x",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			t.Skip()
		}
		once, _ := Text(in)
		twice, n := Text(once)
		if twice != once || n != 0 {
			t.Fatalf("not idempotent:\n in    %q\n once  %q\n twice %q (%d more)", in, once, twice, n)
		}
	})
}
