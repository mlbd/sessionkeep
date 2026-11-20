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

// slow runs every expression, as the code did before the needs existed.
func slow(s string) (string, int) {
	useNeeds = false
	defer func() { useNeeds = true }()
	return Text(s)
}

var needSeeds = []string{
	"password=hunter2hunter2", "PASSWORD: Hunter2Hunter2", "curl -u admin:s3cretPassw0rd https://example.test", "https://user:pa55word@example.test/x",
	"Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", "AUTHORIZATION=basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==", "BEARER abcdefghijklmnopqrstuvwxyz012345",
	"api_key: 'AKIAABCDEFGHIJKLMNOP'", "ASIAABCDEFGHIJKLMNOP", "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "github_pat_abcdefghijklmnopqrstuvwxyz0123456789",
	"xoxb-1234567890-abcdefghij", "AIzaSyA1234567890abcdefghijklmnopqrstuvw", "sk-ant-abcdefghijklmnopqrstuvwxyz", "rk_live_abcdefghijklmnopqrstuvwxyz",
	"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop", "<PRIVATE>keep out</Private>", "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----",
	"-----BEGIN OPENSSH PRIVATE KEY-----\nabc", "--token abcdef123456", "--PASSWORD=abcdef123456", "mysql --user root:hunter2hunter2",
	"cGFzc3dvcmQ9aHVudGVyMmh1bnRlcjI=", "dG9rZW46IGFiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eHl6", "plain words only", "token=\n", "a=b=c password==x",
	"\u017fecret=hunter2hunter2", "to\u212aen=hunter2hunter2", "client_secret = \"abc def ghi\"", "Private_Key: abcdefghijkl", "x --password=abcd: token=zzzzzzzz",
	"Bearer\tabcdefghijklmnopqrstuvwxyz012345", "bearer\nabcdefghijklmnopqrstuvwxyz012345", "authorization:\tToken abcdefgh",
	"", "e", "ftp://a:b@c --token abcdefgh password=qwertyuiop -u x:yyyyyy",
}

func TestNeedsMatchTheRules(t *testing.T) {
	if len(needs) != len(rules) {
		t.Fatalf("%d needs for %d rules; they are parallel lists", len(needs), len(rules))
	}
}

// The needs are a speed-up only: with them and without them the result is the same.
func TestNeedsNeverChangeTheResult(t *testing.T) {
	for _, s := range needSeeds {
		for _, v := range []string{s, "before " + s + " after", s + "\n" + s} {
			fast, n := Text(v)
			want, wn := slow(v)
			if fast != want || n != wn {
				t.Errorf("needs changed the result for %q:\n with    %q (%d)\n without %q (%d)", v, fast, n, want, wn)
			}
		}
	}
}

func FuzzNeedsNeverChangeTheResult(f *testing.F) {
	for _, s := range needSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			t.Skip()
		}
		fast, n := Text(in)
		want, wn := slow(in)
		if fast != want || n != wn {
			t.Fatalf("needs changed the result for %q:\n with    %q (%d)\n without %q (%d)", in, fast, n, want, wn)
		}
	})
}
