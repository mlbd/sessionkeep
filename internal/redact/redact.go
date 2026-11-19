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

func text(s string, decode bool) (string, int) {
	n := 0
	for _, r := range rules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			n++
			return r.re.ReplaceAllString(m, r.repl)
		})
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
