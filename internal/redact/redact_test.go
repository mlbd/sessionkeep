package redact

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Every value here is synthetic and has no meaning outside this test.
const (
	fakeOpenAI = "sk-proj-Zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2Rr1Qq0Pp"
	fakeGitHub = "ghp_0123456789abcdefghijABCDEFGHIJ012345"
	fakeAWS    = "AKIAIOSFODNN7EXAMPLE"
	fakeJWT    = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzeW50aGV0aWMifQ.c2lnbmF0dXJlLXN5bnRoZXRpYw"
	fakePass   = "Tr0ub4dor&3-synthetic"
	fakePEM    = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAsyntheticsyntheticsynthetic\nc3ludGhldGljIGtleSBib2R5IGxpbmUgdHdv\n-----END RSA PRIVATE KEY-----"
)

func TestSecretsAreRemoved(t *testing.T) {
	tests := []struct {
		name, in string
		secrets  []string // substrings that must not survive
		keep     []string // substrings that must survive
	}{
		{"multiline PEM", "deploy key:\n" + fakePEM + "\nthanks", []string{"MIIEowIBAAKCAQEA", "c3ludGhldGljIGtleSBib2R5"}, []string{"deploy key:", "thanks"}},
		{"truncated PEM", "here\n-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA", []string{"b3BlbnNzaC1rZXktdjEAAAAA"}, []string{"here"}},
		{"bearer header", "curl -H 'Authorization: Bearer " + fakeJWT + "' https://api.example.test/v1", []string{fakeJWT, "eyJhbGci"}, []string{"Authorization", "https://api.example.test/v1"}},
		{"basic header in JSON", `{"headers":{"authorization":"Basic dXNlcjpwYXNzd29yZA=="}}`, []string{"dXNlcjpwYXNzd29yZA"}, []string{"headers"}},
		{"bare bearer", "send Bearer abcdefghijklmnopqrstuvwx1234 to the gateway", []string{"abcdefghijklmnopqrstuvwx1234"}, []string{"gateway"}},
		{"URL credentials", "DATABASE at postgres://admin:" + fakePass + "@db.example.test:5432/app", []string{fakePass, "admin:"}, []string{"postgres://", "db.example.test:5432/app"}},
		{"provider tokens", "keys " + fakeOpenAI + " and " + fakeGitHub + " and " + fakeAWS, []string{fakeOpenAI, fakeGitHub, fakeAWS}, []string{"keys", "and"}},
		{"env assignment", "DB_PASSWORD=" + fakePass + "\nAPI_KEY: \"k-9f8e7d6c5b4a\"\nclient_secret = 'shh-synthetic-1'", []string{fakePass, "k-9f8e7d6c5b4a", "shh-synthetic-1"}, []string{"DB_PASSWORD", "API_KEY", "client_secret"}},
		{"JSON assignment", `{"apiKey": "live-abc123def456", "name": "demo"}`, []string{"live-abc123def456"}, []string{`"name": "demo"`}},
		{"base64-wrapped token", "blob " + base64.StdEncoding.EncodeToString([]byte("token="+fakeGitHub)) + " end", []string{base64.StdEncoding.EncodeToString([]byte("token=" + fakeGitHub))}, []string{"blob", "end"}},
		{"base64-wrapped PEM, URL alphabet", "x " + base64.RawURLEncoding.EncodeToString([]byte(fakePEM)) + " y", []string{base64.RawURLEncoding.EncodeToString([]byte(fakePEM))[:40]}, []string{"x ", " y"}},
		{"curl user flag", "curl -u admin:" + fakePass + " https://api.example.test/v1", []string{fakePass}, []string{"curl -u", "https://api.example.test/v1"}},
		{"long credential flags", "mysql --user=root --password " + fakePass + " && tool --api-key=k-9f8e7d6c5b4a --token abcd1234efgh", []string{fakePass, "k-9f8e7d6c5b4a", "abcd1234efgh"}, []string{"mysql", "--password", "--api-key", "&& tool"}},
		{"private marker", "public <private>my home address is 1 Synthetic Way</private> public", []string{"Synthetic Way"}, []string{"public"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, n := Text(tc.in)
			if n == 0 {
				t.Fatalf("nothing redacted in %q", tc.in)
			}
			for _, s := range tc.secrets {
				if strings.Contains(out, s) {
					t.Errorf("secret %q survived: %q", s, out)
				}
			}
			for _, k := range tc.keep {
				if !strings.Contains(out, k) {
					t.Errorf("context %q was lost: %q", k, out)
				}
			}
			again, n2 := Text(out)
			if again != out || n2 != 0 {
				t.Errorf("not idempotent: second pass changed %q to %q (count %d)", out, again, n2)
			}
		})
	}
}

func TestOrdinaryTextIsLeftAlone(t *testing.T) {
	for _, in := range []string{
		"Please refactor the auth module and run the tests.",
		"max_tokens: 4096, temperature: 0.2",
		"const token = getToken(user)",
		"password = process.env.DB_PASSWORD",
		"if (isSecret) { rotate() }",
		"commit 6a78104787b6f0c2d9e1a3b4c5d6e7f8091a2b3c touched internal/store/store.go",
		"see https://example.test/docs/api?page=2#tokens",
		"The token bucket algorithm limits requests; password rules: 12 characters minimum.",
		"use_token: true",
		"C:\\Users\\dev\\project\\secret-notes\\readme.md",
		"SGVsbG8sIHRoaXMgaXMganVzdCBvcmRpbmFyeSB0ZXh0Lg==", // base64 of plain prose
		"grep -u pattern file.txt && ls -u",
		"pass --password-file ./pw.txt and --token-url https://example.test/t then --user alice",
	} {
		if out, n := Text(in); out != in || n != 0 {
			t.Errorf("changed ordinary text:\n in: %q\nout: %q (count %d)", in, out, n)
		}
	}
}

func TestCountMatchesNumberOfReplacements(t *testing.T) {
	out, n := Text("a " + fakeGitHub + " b " + fakeAWS + " c password=" + fakePass)
	if n != 3 || strings.Count(out, "[REDACTED:") != 3 {
		t.Fatalf("count=%d markers=%d out=%q", n, strings.Count(out, "[REDACTED:"), out)
	}
}

// Shapes this rule set is known NOT to catch. The test documents the limit so nobody reads a
// green suite as "all secrets are caught"; if a rule later covers one, move it to the table above.
func TestKnownGaps(t *testing.T) {
	for name, in := range map[string]string{
		"secret with no label or known shape": "the door code is hunter2hunter2",
		"hex key without a label":             "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"secret split across lines":           "pass" + "word is\n" + fakePass,
	} {
		if _, n := Text(in); n != 0 {
			t.Errorf("%s: now detected — move it out of TestKnownGaps", name)
		}
	}
}
