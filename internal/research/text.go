package research

import (
	"html"
	"strings"
	"time"
	"unicode"
)

// Quote limits, in characters.
const (
	MinQuote = 12
	MaxQuote = 600
)

// categories maps what a claim is about to how long a successful check stays good
// (docs/03-RESEARCH.md section 3). The time runs from the last successful check, never from the
// last time the claim was read.
var categories = []struct {
	Name   string
	Days   int
	Places string
}{
	{"security", 1, "security advisories and fixes"},
	{"price", 1, "prices"},
	{"quota", 3, "quotas, rate limits, availability"},
	{"policy", 7, "sign-in, permissions and platform rules"},
	{"release", 7, "what the newest version of a library or tool is"},
	{"api", 14, "how an API or endpoint behaves"},
	{"framework", 30, "settled ways of using a framework"},
	{"concept", 180, "general concepts"},
}

// Window returns how long a check of a claim in this category stays good.
func Window(category string) (time.Duration, bool) {
	for _, c := range categories {
		if c.Name == category {
			return time.Duration(c.Days) * 24 * time.Hour, true
		}
	}
	return 0, false
}

// CategoryHelp lists the categories, one per line.
func CategoryHelp() string {
	var b strings.Builder
	for _, c := range categories {
		unit := "days"
		if c.Days == 1 {
			unit = "day"
		}
		b.WriteString("  " + c.Name + strings.Repeat(" ", 10-len(c.Name)) + " " + c.Places + " (re-check after " + itoa(c.Days) + " " + unit + ")\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}

// Squash turns every run of white space into one space and trims the ends.
func Squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// QuoteIn reports whether the page text contains the quote, comparing letter for letter but
// ignoring how the white space is laid out.
func QuoteIn(text, quote string) bool {
	q := Squash(quote)
	return q != "" && strings.Contains(Squash(text), q)
}

// Passages returns the lines of text that contain find (ignoring case and white-space layout),
// or, if the phrase is nowhere, the lines that contain all of its words; each with the line
// before and after it. At most max characters are returned.
func Passages(text, find string, max int) string {
	lines := strings.Split(text, "\n")
	phrase := strings.ToLower(Squash(find))
	words := strings.Fields(phrase)
	if len(words) == 0 {
		return ""
	}
	pick := func(match func(string) bool) []bool {
		keep, any := make([]bool, len(lines)), false
		for i, l := range lines {
			if match(strings.ToLower(Squash(l))) {
				any = true
				for j := max0(i - 1); j <= i+1 && j < len(lines); j++ {
					keep[j] = true
				}
			}
		}
		if !any {
			return nil
		}
		return keep
	}
	keep := pick(func(l string) bool { return strings.Contains(l, phrase) })
	if keep == nil {
		keep = pick(func(l string) bool {
			for _, w := range words {
				if !strings.Contains(l, w) {
					return false
				}
			}
			return true
		})
	}
	var b strings.Builder
	gap := false
	for i, l := range lines {
		if keep == nil || !keep[i] {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteString("[...]\n")
		}
		gap = false
		if b.Len()+len(l) > max {
			b.WriteString("[more matches were cut]\n")
			break
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// Extract turns a response body into a title and plain text. HTML loses its tags, scripts and
// styles; anything else is taken as text. The result holds no control characters.
func Extract(contentType, body string) (title, text string) {
	body = strings.ToValidUTF8(body, "�")
	ct := strings.ToLower(contentType)
	head := strings.ToLower(body[:min(len(body), 512)])
	if strings.Contains(ct, "html") || (!strings.Contains(ct, "json") && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "markdown") &&
		(strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html"))) {
		title, body = htmlText(body)
		// Tags end lines generously; empty lines carry no meaning in the result.
		return Squash(clean(title)), strings.Join(strings.FieldsFunc(tidy(clean(body)), func(r rune) bool { return r == '\n' }), "\n")
	}
	return Squash(clean(title)), tidy(clean(body))
}

// clean drops control characters other than line breaks and tabs.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r == '\r' {
			return '\n'
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, s)
}

// tidy squashes the white space inside each line and keeps at most one empty line in a row.
func tidy(s string) string {
	var b strings.Builder
	blank := true // also drops leading empty lines
	for _, l := range strings.Split(s, "\n") {
		l = Squash(l)
		if l == "" {
			if !blank {
				b.WriteByte('\n')
			}
			blank = true
			continue
		}
		b.WriteString(l + "\n")
		blank = false
	}
	return strings.TrimRight(b.String(), "\n")
}

// Elements whose content is not page text.
var skipped = map[string]bool{"script": true, "style": true, "noscript": true, "template": true, "svg": true, "iframe": true, "object": true, "select": true, "textarea": true}

// Elements that start a new line.
var breaks = map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"pre": true, "section": true, "article": true, "table": true, "ul": true, "ol": true, "blockquote": true, "hr": true, "dt": true, "dd": true,
	"header": true, "footer": true, "nav": true, "main": true, "aside": true, "figure": true, "figcaption": true, "details": true, "summary": true, "title": true, "head": true, "body": true}

// indexFold finds sub (lower-case ASCII) in s from position from, ignoring ASCII case.
func indexFold(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		j := 0
		for j < len(sub) {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != sub[j] {
				break
			}
			j++
		}
		if j == len(sub) {
			return i
		}
	}
	return -1
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// tagEnd returns the position just after the '>' that closes the tag starting at i, skipping
// quoted attribute values, or -1 if the tag never closes.
func tagEnd(s string, i int) int {
	var quote byte
	for ; i < len(s); i++ {
		switch c := s[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i + 1
		}
	}
	return -1
}

// htmlText is a small, deterministic tag remover. It does not build a document tree and makes
// no attempt to run or interpret anything; what it returns is only ever shown as plain text.
func htmlText(s string) (title, text string) {
	var b strings.Builder
	flush := func(raw string) { b.WriteString(html.UnescapeString(raw)) }
	i := 0
	for i < len(s) {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			flush(s[i:])
			break
		}
		flush(s[i : i+lt])
		i += lt
		if strings.HasPrefix(s[i:], "<!--") {
			end := strings.Index(s[i+4:], "-->")
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}
		j := i + 1
		closing := j < len(s) && s[j] == '/'
		if closing {
			j++
		}
		if j >= len(s) || !(isLetter(s[j]) || (!closing && (s[j] == '!' || s[j] == '?'))) {
			b.WriteByte('<') // a bare "<" in text, as in "a < b"
			i++
			continue
		}
		k := j
		for k < len(s) && (isLetter(s[k]) || s[k] >= '0' && s[k] <= '9') {
			k++
		}
		name := strings.ToLower(s[j:k])
		end := tagEnd(s, k)
		if end < 0 {
			break // an unfinished tag: nothing after it is text
		}
		i = end
		if closing {
			if breaks[name] {
				b.WriteByte('\n')
			}
			continue
		}
		if name == "title" || skipped[name] {
			stop := indexFold(s, "</"+name, i)
			if stop < 0 {
				break
			}
			if name == "title" && title == "" {
				title = html.UnescapeString(s[i:stop])
			}
			if i = tagEnd(s, stop); i < 0 {
				break
			}
			continue
		}
		if breaks[name] {
			b.WriteByte('\n')
		} else if name == "td" || name == "th" {
			b.WriteByte(' ')
		}
	}
	return title, b.String()
}
