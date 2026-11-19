package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func asset(t *testing.T, name string) string {
	t.Helper()
	raw, err := fs.ReadFile(uiFS, "ui/"+name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The pages must load nothing from anywhere else and must never build markup from data.
func TestPagesAreSelfContainedAndInjectionSafe(t *testing.T) {
	entries, _ := fs.ReadDir(uiFS, "ui")
	if len(entries) != 3 {
		t.Fatalf("expected exactly index.html, app.css and app.js; found %d files", len(entries))
	}
	remote := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)
	for _, e := range entries {
		body := asset(t, e.Name())
		if m := remote.FindString(body); m != "" {
			t.Errorf("%s refers to a remote address: %s", e.Name(), m)
		}
		if strings.Contains(body, "@import") || strings.Contains(strings.ToLower(body), "url(") {
			t.Errorf("%s pulls in another resource", e.Name())
		}
	}
	html := asset(t, "index.html")
	for _, bad := range []string{"<script>", "onclick=", "onload=", "onerror=", "javascript:", "style=", "<style", "<iframe", "<object", "<embed", "<form action"} {
		if strings.Contains(strings.ToLower(html), bad) {
			t.Errorf("index.html contains %q, which the content security policy forbids or which is unsafe", bad)
		}
	}
	js := asset(t, "app.js")
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "setAttribute('on", "localStorage", "sessionStorage", "document.cookie", "XMLHttpRequest", "WebSocket", "import("} {
		if strings.Contains(js, bad) {
			t.Errorf("app.js uses %s", bad)
		}
	}
	if !strings.Contains(js, "X-SessionKeep-CSRF") || !strings.Contains(js, "credentials: 'same-origin'") {
		t.Error("app.js must send the request token and same-origin credentials")
	}
}

// Accessibility checks that do not need a browser. They cover structure, not appearance: a
// person still has to look at the pages.
func TestAccessibilityBasics(t *testing.T) {
	html := asset(t, "index.html")
	for name, re := range map[string]string{
		"document language":           `<html lang="[a-z-]+">`,
		"viewport meta":               `<meta name="viewport"`,
		"page title":                  `<title>[^<]+</title>`,
		"exactly one h1":              `<h1[ >]`,
		"main landmark":               `<main id="main"`,
		"labelled navigation":         `<nav aria-label="[^"]+">`,
		"skip link":                   `<a class="skip" href="#main">`,
		"live region for status text": `id="notice" role="status" aria-live="polite"`,
	} {
		if !regexp.MustCompile(re).MatchString(html) {
			t.Errorf("index.html lacks: %s", name)
		}
	}
	if strings.Count(html, "<h1") != 1 {
		t.Error("index.html must have exactly one h1")
	}

	js := asset(t, "app.js")
	// Every input, select and textarea the script creates is wrapped by field(...) (a <label>),
	// placed inside a label, or given an aria-label.
	controls := regexp.MustCompile(`(?m)^.*\b(?:el\('(?:input|textarea)'|select\('|projectSelect\().*$`).FindAllString(js, -1)
	if len(controls) < 15 {
		t.Fatalf("found only %d form controls; the check below would be meaningless", len(controls))
	}
	assigned := regexp.MustCompile(`const (\w+) = (?:el\('(?:input|textarea)'|select\(|projectSelect\()`)
	for _, m := range assigned.FindAllStringSubmatch(js, -1) {
		name := m[1]
		labelled := regexp.MustCompile(`field\('[^']*', `+name+`\)`).MatchString(js) ||
			regexp.MustCompile(`el\('label', \{[^}]*\}, `+name+`,`).MatchString(js) ||
			regexp.MustCompile(name+`\.setAttribute\('aria-label'`).MatchString(js)
		if !labelled {
			t.Errorf("form control %q has no label", name)
		}
	}
	// Buttons must have visible text; table headers must be marked as column headers.
	for _, b := range regexp.MustCompile(`el\('button', \{[^}]*\}`).FindAllString(js, -1) {
		if !strings.Contains(b, "text:") {
			t.Errorf("button without text: %s", b)
		}
	}
	if !strings.Contains(js, "scope: 'col'") {
		t.Error("table headers must carry scope=col")
	}
	// Status is never colour alone: every state class is used together with words.
	for _, line := range regexp.MustCompile(`(?m)^.*class: [^,]*state-(?:ok|warn|bad).*$`).FindAllString(js, -1) {
		if !strings.Contains(line, "text:") {
			t.Errorf("state shown by colour only: %s", strings.TrimSpace(line))
		}
	}

	css := asset(t, "app.css")
	for name, want := range map[string]string{"visible keyboard focus": ":focus-visible", "dark theme": "prefers-color-scheme: dark", "reduced motion": "prefers-reduced-motion", "small screens": "max-width: 520px"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks: %s", name)
		}
	}
	if strings.Contains(css, "outline: none") || strings.Contains(css, "outline:none") || strings.Contains(css, "outline: 0") {
		t.Error("app.css removes focus outlines")
	}
}
