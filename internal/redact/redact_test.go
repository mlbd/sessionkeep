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
		// Rule set 2 (security review M1). All values are made up.
		{"npm token", "use npm_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789 here", []string{"npm_AbCdEfGh"}, []string{"use", "here"}},
		{"webhook secret", "whsec_c3ludGhldGljc3ludGhldGljc3ludGg=", []string{"c3ludGhldGljc3lu"}, nil},
		{"hugging face, gitlab, google, sendgrid", "hf_AbCdEfGhIjKlMnOpQrStUvWxYz01234 glpat-AbCdEfGhIjKlMnOpQrSt GOCSPX-AbCdEfGhIjKlMnOpQrStUv SG.AbCdEfGhIjKlMnOp.QrStUvWxYz0123456789ab",
			[]string{"hf_AbCd", "glpat-AbCd", "GOCSPX-AbCd", "SG.AbCd"}, nil},
		{"slack and discord webhooks", "post to https://hooks.slack.com/services/T0000/B0000/XXXXsyntheticXXXX and https://discord.com/api/webhooks/123456789/AbCdEfGhIjKlMnOpQrStUvWx",
			[]string{"T0000/B0000", "XXXXsynthetic", "AbCdEfGhIjKlMnOpQrStUvWx"}, []string{"https://hooks.slack.com/services/", "https://discord.com/api/webhooks/"}},
		{"azure connection string and SAS", "DefaultEndpointsProtocol=https;AccountName=demo;AccountKey=c3ludGhldGljc3ludGhldGljc3ludGhldGlj==;EndpointSuffix=core.windows.net and https://demo.blob.core.windows.net/c?sv=2024&sig=AbCdEf%2BGhIjKlMnOpQr%3D",
			[]string{"c3ludGhldGljc3ludGhl", "AbCdEf%2BGh"}, []string{"AccountName=demo", "sv=2024"}},
		{"PGP private key block", "-----BEGIN PGP PRIVATE KEY BLOCK-----\nlQOYBGsyntheticPGPbody\n-----END PGP PRIVATE KEY BLOCK-----", []string{"lQOYBGsyntheticPGPbody"}, nil},
		{"cookie header", "curl -H 'Cookie: session=AbCdEf0123456789; theme=dark' https://example.test", []string{"AbCdEf0123456789"}, []string{"curl -H", "https://example.test"}},
		{"mysql and docker passwords", "mysql -u root -p" + fakePass + " app && docker login -u me -p dckr_synthetic_pw registry.example.test",
			[]string{fakePass, "dckr_synthetic_pw"}, []string{"mysql -u root -p", "registry.example.test"}},
		{"curl user, attached and quoted", "curl -uadmin:" + fakePass + " x; curl -u 'bob:" + fakePass + "2' y", []string{fakePass}, []string{"curl", " x;", " y"}},
		{"URL with an empty user or @ in the password", "redis://:" + fakePass + "@cache.example.test:6379 and https://user:p@ss-synth-1@host.example.test/x",
			[]string{fakePass, "p@ss-synth-1", "ss-synth-1"}, []string{"cache.example.test:6379", "host.example.test/x"}},
		{"card number", "card 4111 1111 1111 1111 exp 12/30 and 5555555555554444", []string{"4111 1111", "5555555555554444"}, []string{"card", "exp 12/30"}},
		{".env names", "SMTP_PASS=s3cr3t-mail\nAPP_SALT='pepper-and-salt'\nBASIC_AUTH=user:hunter22\nSENTRY_DSN=https://abc123@o1.ingest.example.test/1",
			[]string{"s3cr3t-mail", "pepper-and-salt", "user:hunter22", "abc123@"}, []string{"SMTP_PASS", "APP_SALT", "BASIC_AUTH", "SENTRY_DSN"}},
		{"short password", `password: "hx7q"`, []string{"hx7q"}, []string{"password"}},
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
		// Near misses for rule set 2.
		"mysql -u root -P 3306 -h db.example.test",
		"docker login registry.example.test",
		"Cookie: none set yet",
		"the npm_config_cache folder and hf_hub download",
		"passenger_count = 4; bypass = true; compass: north",
		"order 4111 shipped; timestamp 1795599784232497600; id 4111111111111112",
		"NEXT_PUBLIC_AUTH=true and DISABLE_AUTH=1",
		"token: abc",
		"see https://user@example.test/repo.git",
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
