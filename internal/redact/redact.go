// Package redact removes known secret shapes from text before SessionKeep writes it anywhere
// (spool, database, cache, logs).
//
// Pattern matching cannot find every secret. It is one layer: the others are the per-project
// capture level, exclusions, and the field allowlist in the hook adapter. Never describe this
// package as a guarantee.
package redact

import (
	"encoding/base64"
	"regexp"
	"strings"
	"unicode"
)

// Version identifies the rule set; it is stored with redacted content so old captures can be found
// and re-examined when rules improve.
//
//	1  the first rule set
//	2  more provider tokens and webhook URLs, Azure keys and SAS signatures, PGP private key
//	   blocks, cookies, mysql/docker passwords, attached or quoted curl -u, URLs with an empty
//	   user or "@" in the password, card numbers, .env-style names such as SMTP_PASS, and short
//	   values under a password name
const Version = 2

type rule struct {
	kind string
	re   *regexp.Regexp
	repl string
}

const assignKeys = `password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret|credential`

var (
	rules = []rule{
		{"private", regexp.MustCompile(`(?is)<private>.*?</private>`), "[REDACTED:private]"},
		{"private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----.*?-----END [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----`), "[REDACTED:private-key]"},
		// A key block whose END line was cut off: drop everything after BEGIN.
		{"private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY(?: BLOCK)?-----.*`), "[REDACTED:private-key]"},
		{"auth-header", regexp.MustCompile(`(?i)\b((?:proxy-)?authorization)(["']?\s*[:=]\s*["']?)(?:bearer|basic|token|digest)\s+[A-Za-z0-9._~+/=-]{4,}`), "$1$2[REDACTED:auth-header]"},
		{"auth-header", regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{16,}`), "[REDACTED:auth-header]"},
		// The user may be empty (redis://:pass@host) and the password may hold "@": the match runs
		// to the last "@" before the path.
		{"url-credentials", regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/:@"'\[\]]*:[^\s/"'\[\]]+@`), "$1[REDACTED:url-credentials]@"},
		// Command-line credentials: curl/wget -u user:pass (also -uuser:pass and quoted), and
		// --password VALUE style flags.
		{"cli-credentials", regexp.MustCompile(`(^|\s)(-u|--user)(\s*|=)(["']?)[^\s:"'\[\]]+:[^\s"'\[\]]+`), "$1$2$3$4[REDACTED:cli-credentials]"},
		{"cli-credentials", regexp.MustCompile(`(?i)(--(?:password|passwd|pass|token|api-?key|secret|access-?key|auth)(?:\s+|=))[^\s"'\[\]-][^\s"'\[\]]{3,}`), "$1[REDACTED:cli-credentials]"},
		// mysql -pPASS (the password attached to a lower-case -p) and docker login -p PASS.
		{"cli-credentials", regexp.MustCompile(`\b((?:mysql|mysqldump|mysqladmin|mariadb)\b[^\n]*?\s-p)[^\s"'\[-][^\s"']{3,}`), "$1[REDACTED:cli-credentials]"},
		{"cli-credentials", regexp.MustCompile(`\b((?:docker|podman|nerdctl)\s+login\b[^\n]*?\s-p\s+)[^\s"'\[-]\S*`), "$1[REDACTED:cli-credentials]"},
		{"cookie", regexp.MustCompile(`(?i)\b((?:set-)?cookie)(["']?\s*:\s*["']?)[^\s=;"']+=[^\n"']{8,}`), "$1$2[REDACTED:cookie]"},
		{"webhook", regexp.MustCompile(`(https://hooks\.slack\.com/(?:services|workflows|triggers)/)[A-Za-z0-9/_-]{10,}`), "$1[REDACTED:webhook]"},
		{"webhook", regexp.MustCompile(`(https://(?:ptb\.|canary\.)?discord(?:app)?\.com/api/webhooks/)[0-9]+/[A-Za-z0-9_-]{20,}`), "$1[REDACTED:webhook]"},
		{"azure-key", regexp.MustCompile(`(?i)\b((?:Account|SharedAccess)Key\s*=\s*)[A-Za-z0-9+/]{20,}={0,2}`), "$1[REDACTED:azure-key]"},
		{"azure-key", regexp.MustCompile(`([?&]sig=)[A-Za-z0-9%+/=]{16,}`), "$1[REDACTED:azure-key]"},
		{"token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\b(?:sk|rk|pk)[-_](?:live|test|proj|ant|or)?[-_]?[A-Za-z0-9_-]{20,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bwhsec_[A-Za-z0-9+/=]{24,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bhf_[A-Za-z0-9]{30,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bGOCSPX-[A-Za-z0-9_-]{20,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}`), "[REDACTED:token]"},
	}
	// Card numbers: a known first digits, 13 to 19 digits with optional single spaces or dashes;
	// each candidate must also pass the Luhn check.
	card = regexp.MustCompile(`\b(?:4\d{3}|5[1-5]\d{2}|2[2-7]\d{2}|3[47]\d{2}|6(?:011|5\d{2}))(?:[ -]?\d){9,15}\b`)
	// .env-style names that do not contain one of assignKeys: upper case, ending in a part that
	// marks a secret, such as SMTP_PASS, APP_SALT or BASIC_AUTH. Groups as in assign.
	envAssign = regexp.MustCompile(`\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*_(?:PASS|PASSPHRASE|SALT|AUTH|DSN|WEBHOOK|WEBHOOK_URL|CONNECTION_STRING|CONN_STR|SIGNING_KEY|ENCRYPTION_KEY|MASTER_KEY))(\s*=\s*)("[^"\n]+"|'[^'\n]+'|[^\s"',;)}\]]+)`)
	// name = value / name: value where the name looks secret. Group 1 name, 2 separator, 3 value.
	assign = regexp.MustCompile(`(?i)\b([A-Za-z0-9_.-]*(?:` + assignKeys + `)[A-Za-z0-9_.-]*["']?)(\s*[:=]\s*)("[^"\n]+"|'[^'\n]+'|[^\s"',;)}\]]+)`)
	b64    = regexp.MustCompile(`[A-Za-z0-9+/_-]{24,}={0,2}`)
)

// Text returns s with known secret shapes replaced by [REDACTED:kind] markers, and how many
// replacements were made. Text(Text(s)) equals Text(s).
func Text(s string) (string, int) { return text(s, true) }

// A need is a cheap test that must hold before a rule can possibly match: the text, in lower
// case, contains one of `any` and all of `all`. Running seventeen expressions over every piece
// of text cost 0.4 ms per 700 bytes; most text contains none of these strings, so most
// expressions are skipped. A need only ever skips an expression that could not have matched:
// TestNeedsNeverChangeTheResult and FuzzNeedsNeverChangeTheResult compare against running
// everything.
type need struct{ any, all []string }

func (n need) possible(lower string) bool {
	for _, x := range n.all {
		if !strings.Contains(lower, x) {
			return false
		}
	}
	for _, x := range n.any {
		if strings.Contains(lower, x) {
			return true
		}
	}
	return len(n.any) == 0
}

// needs is parallel to rules.
var needs = []need{
	{any: []string{"<private>"}},
	{any: []string{"private key"}},
	{any: []string{"private key"}},
	{any: []string{"authorization"}},
	{any: []string{"bearer"}},
	{all: []string{"://", "@"}},
	{any: []string{"-u"}},
	{any: []string{"--"}},
	{any: []string{"mysql", "mariadb"}, all: []string{"-p"}},
	{all: []string{"login", "-p"}},
	{any: []string{"cookie"}},
	{any: []string{"hooks.slack.com"}},
	{any: []string{"/api/webhooks/"}},
	{any: []string{"accountkey", "sharedaccesskey"}},
	{any: []string{"sig="}},
	{any: []string{"eyj"}},
	{any: []string{"sk-", "sk_", "rk-", "rk_", "pk-", "pk_"}},
	{any: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}},
	{any: []string{"github_pat_"}},
	{any: []string{"akia", "asia"}},
	{any: []string{"xox"}},
	{any: []string{"aiza"}},
	{any: []string{"npm_"}},
	{any: []string{"whsec_"}},
	{any: []string{"hf_"}},
	{any: []string{"glpat-"}},
	{any: []string{"gocspx-"}},
	{any: []string{"sg."}},
}

var assignNeed = need{any: []string{"password", "passwd", "pwd", "secret", "token", "key", "credential",
	"_pass", "_salt", "_auth", "_dsn", "_webhook", "_conn"}}

// useNeeds is switched off only by tests, to compare results.
var useNeeds = true

// lowerFor returns the text in lower case for the needs, or ok=false when the needs must not be
// used: case-insensitive expressions also match two non-ASCII letters (the long s, U+017F, and
// the Kelvin sign, U+212A) that lower-casing does not reliably turn into "s" and "k".
func lowerFor(s string) (string, bool) {
	if !useNeeds || strings.ContainsRune(s, 0x017f) || strings.ContainsRune(s, 0x212a) {
		return "", false
	}
	return strings.ToLower(s), true
}

// hasRun reports whether s holds at least min consecutive base64 characters.
func hasRun(s string, min int) bool {
	run := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '_' || c == '-' {
			if run++; run >= min {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

func text(s string, decode bool) (string, int) {
	n := 0
	lower, fast := lowerFor(s)
	for i, r := range rules {
		if fast && !needs[i].possible(lower) {
			continue
		}
		before := n
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			n++
			return r.re.ReplaceAllString(m, r.repl)
		})
		if n != before { // the text changed: the needs must look at the new text
			lower, fast = lowerFor(s)
		}
	}
	if digits(s, 13) {
		s = card.ReplaceAllStringFunc(s, func(m string) string {
			if !luhn(m) {
				return m
			}
			n++
			return "[REDACTED:card]"
		})
	}
	if fast && !assignNeed.possible(lower) && (!decode || !hasRun(s, 24)) {
		return s, n
	}
	for _, re := range []*regexp.Regexp{assign, envAssign} {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			g := re.FindStringSubmatch(m)
			if !secretValue(g[1], g[3]) {
				return m
			}
			n++
			return g[1] + g[2] + "[REDACTED:value]"
		})
	}
	if decode {
		s = b64.ReplaceAllStringFunc(s, func(m string) string {
			if dec, ok := decodeText(m); ok {
				if _, c := text(dec, false); c > 0 {
					n++
					return "[REDACTED:encoded-secret]"
				}
			}
			return m
		})
	}
	return s, n
}

// digits reports whether s holds at least min ASCII digits.
func digits(s string, min int) bool {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			if n++; n >= min {
				return true
			}
		}
	}
	return false
}

// luhn reports whether the digits of a card-number candidate (spaces and dashes ignored) are 13
// to 19 long and pass the Luhn check, which a random number passes one time in ten.
func luhn(m string) bool {
	var d []int
	for i := 0; i < len(m); i++ {
		if m[i] >= '0' && m[i] <= '9' {
			d = append(d, int(m[i]-'0'))
		}
	}
	if len(d) < 13 || len(d) > 19 {
		return false
	}
	sum := 0
	for i := range d {
		x := d[len(d)-1-i]
		if i%2 == 1 {
			if x *= 2; x > 9 {
				x -= 9
			}
		}
		sum += x
	}
	return sum%10 == 0
}

// secretValue filters out assignments that are plainly not secrets, such as `max_tokens: 4096`,
// `token = true`, a call like `token = getToken()` or an already redacted value. Under a name that
// says password or passphrase, a value of 4 characters counts; otherwise 6.
func secretValue(name, v string) bool {
	v = strings.Trim(v, `"'`)
	min := 6
	if l := strings.ToLower(name); strings.Contains(l, "pass") || strings.Contains(l, "pwd") {
		min = 4
	}
	if len(v) < min || strings.HasPrefix(v, "[REDACTED:") {
		return false
	}
	switch strings.ToLower(v) {
	case "true", "false", "null", "none", "undefined", "string", "number", "boolean":
		return false
	}
	digits := true
	for _, r := range v {
		if !unicode.IsDigit(r) && r != '.' && r != '_' {
			digits = false
			break
		}
	}
	if digits {
		return false
	}
	// Code, not a literal: a call, a member access chain, an env lookup, a placeholder.
	if strings.ContainsAny(v, "(<{$") || strings.HasPrefix(v, "process.env") || strings.HasPrefix(v, "os.") {
		return false
	}
	return true
}

func decodeText(m string) (string, bool) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		b, err := enc.DecodeString(m)
		if err != nil || len(b) < 12 {
			continue
		}
		printable := 0
		for _, c := range b {
			if c == '\n' || c == '\r' || c == '\t' || (c >= 0x20 && c < 0x7f) {
				printable++
			}
		}
		if printable*10 >= len(b)*9 {
			return string(b), true
		}
	}
	return "", false
}
