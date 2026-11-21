// Package suggest finds things the user keeps telling the assistant: a request made in nearly
// the same words in several sessions is a candidate for a standing instruction. Nothing here
// uses a model. Sentences are compared by their words, so a rewording that shares few words is
// missed and an unrelated sentence that shares many is found; every result is only a suggestion
// for the user to read.
package suggest

import (
	"regexp"
	"sort"
	"strings"
)

// Said is one thing the user typed.
type Said struct {
	Session, Project, At, Text string
}

// Suggestion is one request that came back in several sessions.
type Suggestion struct {
	Text     string `json:"text"` // the most recent wording
	Sessions int    `json:"sessions"`
	Projects int    `json:"projects"`
	FirstAt  string `json:"first_at"`
	LastAt   string `json:"last_at"`
}

const (
	minRunes, maxRunes = 8, 200
	same               = 0.7 // share of words two sentences must have in common to count as one
	covered            = 0.6 // share of a request's words an existing instruction must have to cover it
	maxSuggestions     = 20
)

var (
	sentence = regexp.MustCompile(`[^.!?\n\r]+`)
	negation = regexp.MustCompile(`\b(do not|don't|dont)\b`)
	word     = regexp.MustCompile(`[a-z0-9][a-z0-9'_-]*`)
	// Words that make a sentence a standing wish and not a one-off task.
	directive = regexp.MustCompile(`(?i)\b(always|never|don'?t|do not|must|make sure|remember|from now on|every time|each time|stop|use|avoid|prefer|no more|instead of|only|keep)\b`)
	filler    = map[string]bool{"the": true, "a": true, "an": true, "to": true, "of": true, "in": true, "on": true, "for": true, "and": true, "or": true,
		"is": true, "it": true, "this": true, "that": true, "please": true, "you": true, "i": true, "me": true, "my": true, "we": true, "be": true,
		"are": true, "with": true, "can": true, "will": true, "should": true, "would": true, "just": true, "so": true, "also": true, "again": true}
)

func words(s string) map[string]bool {
	out := map[string]bool{}
	// "do not", "don't" and "dont" are one word here, so the three spellings compare as equal.
	for _, w := range word.FindAllString(negation.ReplaceAllString(strings.ToLower(s), "dont"), -1) {
		if !filler[w] {
			out[w] = true
		}
	}
	return out
}

func overlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	both := 0
	for w := range a {
		if b[w] {
			both++
		}
	}
	return float64(both) / float64(len(a)+len(b)-both)
}

// strength words say how firmly something is wanted, not what; an instruction that says the
// same thing with another of them still covers the request.
var strength = map[string]bool{"always": true, "never": true, "not": true, "dont": true, "must": true, "only": true, "no": true}

// within is the share of a's words, strength words aside, that b also has.
func within(a, b map[string]bool) float64 {
	n, both := 0, 0
	for w := range a {
		if strength[w] {
			continue
		}
		n++
		if b[w] {
			both++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(both) / float64(n)
}

type cluster struct {
	text            string
	words           map[string]bool
	sessions, projs map[string]bool
	first, last     string
}

// Find returns the requests that appear in at least minSessions different sessions and are not
// already covered by one of the existing instructions. said must be newest first.
func Find(said []Said, existing []string, minSessions int) []Suggestion {
	var have []map[string]bool
	for _, e := range existing {
		have = append(have, words(e))
		for _, part := range sentence.FindAllString(e, -1) {
			have = append(have, words(part))
		}
	}
	var clusters []*cluster
	for _, s := range said {
		for _, part := range sentence.FindAllString(s.Text, -1) {
			part = strings.Join(strings.Fields(part), " ")
			if n := len([]rune(part)); n < minRunes || n > maxRunes || !directive.MatchString(part) {
				continue
			}
			w := words(part)
			if len(w) < 2 {
				continue
			}
			var c *cluster
			for _, x := range clusters {
				if overlap(w, x.words) >= same {
					c = x
					break
				}
			}
			if c == nil {
				c = &cluster{text: part, words: w, sessions: map[string]bool{}, projs: map[string]bool{}, last: s.At}
				clusters = append(clusters, c)
			}
			c.sessions[s.Session], c.projs[s.Project], c.first = true, true, s.At
		}
	}
	out := []Suggestion{}
	for _, c := range clusters {
		if len(c.sessions) < minSessions {
			continue
		}
		known := false
		for _, h := range have {
			known = known || within(c.words, h) >= covered
		}
		if !known {
			out = append(out, Suggestion{Text: c.text, Sessions: len(c.sessions), Projects: len(c.projs), FirstAt: c.first, LastAt: c.last})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sessions > out[j].Sessions })
	if len(out) > maxSuggestions {
		out = out[:maxSuggestions]
	}
	return out
}
