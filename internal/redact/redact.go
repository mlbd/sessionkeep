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
const Version = 1

type rule struct {
	kind string
	re   *regexp.Regexp
	repl string
}

const assignKeys = `password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret|credential`

var (
	rules = []rule{
		{"private", regexp.MustCompile(`(?is)<private>.*?</private>`), "[REDACTED:private]"},
		{"private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), "[REDACTED:private-key]"},
		// A key block whose END line was cut off: drop everything after BEGIN.
		{"private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*`), "[REDACTED:private-key]"},
		{"auth-header", regexp.MustCompile(`(?i)\b((?:proxy-)?authorization)(["']?\s*[:=]\s*["']?)(?:bearer|basic|token|digest)\s+[A-Za-z0-9._~+/=-]{4,}`), "$1$2[REDACTED:auth-header]"},
		{"auth-header", regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{16,}`), "[REDACTED:auth-header]"},
		{"url-credentials", regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/:@"'\[\]]+:[^\s/@"'\[\]]+@`), "$1[REDACTED:url-credentials]@"},
		// Command-line credentials: curl/wget -u user:pass, and --password VALUE style flags.
		{"cli-credentials", regexp.MustCompile(`(^|\s)(-u|--user)(\s+|=)[^\s:"'\[\]]+:[^\s"'\[\]]+`), "$1$2$3[REDACTED:cli-credentials]"},
		{"cli-credentials", regexp.MustCompile(`(?i)(--(?:password|passwd|pass|token|api-?key|secret|access-?key|auth)(?:\s+|=))[^\s"'\[\]-][^\s"'\[\]]{3,}`), "$1[REDACTED:cli-credentials]"},
		{"token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\b(?:sk|rk|pk)[-_](?:live|test|proj|ant|or)?[-_]?[A-Za-z0-9_-]{20,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), "[REDACTED:token]"},
		{"token", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`), "[REDACTED:token]"},
	}
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
	{any: []string{"private key-----"}},
	{any: []string{"private key-----"}},
	{any: []string{"authorization"}},
	{any: []string{"bearer"}},
	{all: []string{"://", "@"}},
	{any: []string{"-u"}},
	{any: []string{"--"}},
	{any: []string{"eyj"}},
	{any: []string{"sk-", "sk_", "rk-", "rk_", "pk-", "pk_"}},
	{any: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}},
	{any: []string{"github_pat_"}},
	{any: []string{"akia", "asia"}},
	{any: []string{"xox"}},
	{any: []string{"aiza"}},
}

var assignNeed = need{any: []string{"password", "passwd", "pwd", "secret", "token", "key", "credential"}}

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
	if fast && !assignNeed.possible(lower) && (!decode || !hasRun(s, 24)) {
		return s, n
	}
	s = assign.ReplaceAllStringFunc(s, func(m string) string {
		g := assign.FindStringSubmatch(m)
		if !secretValue(g[3]) {
			return m
		}
		n++
		return g[1] + g[2] + "[REDACTED:value]"
	})
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

// secretValue filters out assignments that are plainly not secrets, such as `max_tokens: 4096`,
// `token = true`, a call like `token = getToken()` or an already redacted value.
func secretValue(v string) bool {
	v = strings.Trim(v, `"'`)
	if len(v) < 6 || strings.HasPrefix(v, "[REDACTED:") {
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
