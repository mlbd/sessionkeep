package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A claim's "applies to" is compared with what the project actually uses only when it names a
// package and a version ("react@18", "next 14.2", "go 1.22"). The installed version is read from
// the lock files in the project's top folder. Nothing is run and nothing is fetched.

// maxLockFile is the largest lock file read.
const maxLockFile = 64 << 20

// LockFiles are the files looked at, in this order. Others are not read.
var LockFiles = []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "Cargo.lock", "uv.lock", "poetry.lock", "requirements.txt"}

// ParseAppliesTo splits "name@version" or "name version" into its parts. ok is false when the
// text is not in one of those two forms or the version does not start with a number.
func ParseAppliesTo(s string) (name, version string, ok bool) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "@"); i > 0 && !strings.ContainsAny(s, " \t") {
		name, version = s[:i], s[i+1:]
	} else if f := strings.Fields(s); len(f) == 2 {
		name, version = f[0], f[1]
	} else {
		return "", "", false
	}
	if len(numbers(version)) == 0 {
		return "", "", false
	}
	return name, version, true
}

// numbers returns the leading numeric components of a version: "v18.3.1-rc.1" gives 18, 3, 1 and
// "14.x" gives 14. Text that does not start with a number (after an optional "v") gives none.
func numbers(v string) []int {
	v = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(v), "v"), "V")
	var out []int
	for _, part := range strings.Split(v, ".") {
		end := 0
		for end < len(part) && part[end] >= '0' && part[end] <= '9' {
			end++
		}
		if end == 0 || end > 9 {
			break
		}
		n, _ := strconv.Atoi(part[:end])
		out = append(out, n)
		if end != len(part) {
			break
		}
	}
	return out
}

// CompareVersions says how the version a claim is about relates to an installed one: "match"
// when every stated number is the same, "other_major" when the first differs, "other_version"
// when a later one does. ok is false when either cannot be read as a version.
func CompareVersions(claimed, installed string) (result string, ok bool) {
	c, i := numbers(claimed), numbers(installed)
	if len(c) == 0 || len(i) == 0 {
		return "", false
	}
	if c[0] != i[0] {
		return "other_major", true
	}
	for k := 1; k < len(c); k++ {
		if k >= len(i) || c[k] != i[k] {
			return "other_version", true
		}
	}
	return "match", true
}

// Installed looks a package up in the lock files of one folder and returns the first file that
// lists it, with every distinct version found there. An unreadable or oversized file is skipped.
func Installed(root, name string) (file string, versions []string) {
	for _, f := range LockFiles {
		path := filepath.Join(root, f)
		fi, err := os.Stat(path)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxLockFile {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var found []string
		switch f {
		case "package-lock.json":
			found = fromPackageLock(raw, name)
		case "pnpm-lock.yaml":
			found = fromPnpmLock(string(raw), name)
		case "yarn.lock":
			found = fromYarnLock(string(raw), name)
		case "go.mod":
			found = fromGoMod(string(raw), name)
		case "requirements.txt":
			found = fromRequirements(string(raw), name)
		default:
			found = fromTOMLLock(string(raw), name, f != "Cargo.lock")
		}
		if found = distinct(found); len(found) > 0 {
			return f, found
		}
	}
	return "", nil
}

func distinct(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" && !seen[v] && len(numbers(v)) > 0 {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// lines splits text and drops carriage returns.
func lines(s string) []string { return strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") }

func unquote(s string) string { return strings.Trim(strings.TrimSpace(s), `"'`) }

// fromPackageLock reads npm's lock file: the copy at the top of node_modules is the one the
// project itself uses.
func fromPackageLock(raw []byte, name string) []string {
	var lock struct {
		Packages     map[string]struct{ Version string } `json:"packages"`
		Dependencies map[string]struct{ Version string } `json:"dependencies"`
	}
	if json.Unmarshal(raw, &lock) != nil {
		return nil
	}
	if p, ok := lock.Packages["node_modules/"+name]; ok {
		return []string{p.Version}
	}
	if p, ok := lock.Dependencies[name]; ok {
		return []string{p.Version}
	}
	return nil
}

// fromPnpmLock reads what the project's own packages depend on. Newer files give a
// "specifier" and a "version" under the package name; older ones give "name: version" in the
// top-level dependency lists.
func fromPnpmLock(text, name string) []string {
	var out []string
	ls := lines(text)
	section := ""
	for i, l := range ls {
		trimmed := strings.TrimSpace(l)
		if l != "" && l[0] != ' ' && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok || unquote(key) != name {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			for _, next := range ls[i+1 : min(i+3, len(ls))] {
				if k, v, ok := strings.Cut(strings.TrimSpace(next), ":"); ok && k == "version" {
					out = append(out, pnpmVersion(v))
				}
			}
		} else if indent := len(l) - len(strings.TrimLeft(l, " ")); indent == 2 && (section == "dependencies" || section == "devDependencies" || section == "optionalDependencies") {
			out = append(out, pnpmVersion(value))
		}
	}
	return out
}

// pnpmVersion drops what pnpm appends to a version: "18.2.0(react@18.2.0)" and "18.2.0_abc".
func pnpmVersion(v string) string {
	v = unquote(v)
	if i := strings.IndexAny(v, "(_"); i >= 0 {
		v = v[:i]
	}
	return v
}

// fromYarnLock reads both yarn formats: an entry starts with the requests it answers
// ("react@^18.0.0, react@^18.2.0:") and its version is on a line below.
func fromYarnLock(text, name string) []string {
	var out []string
	ls := lines(text)
	for i, l := range ls {
		if l == "" || l[0] == ' ' || l[0] == '#' || !strings.HasSuffix(l, ":") {
			continue
		}
		hit := false
		for _, spec := range strings.Split(strings.TrimSuffix(l, ":"), ",") {
			spec = unquote(spec)
			if at := strings.LastIndex(spec, "@"); at > 0 && spec[:at] == name {
				hit = true
			}
		}
		if !hit {
			continue
		}
		for _, next := range ls[i+1:] {
			if next == "" || next[0] != ' ' {
				break
			}
			if rest, ok := strings.CutPrefix(strings.TrimSpace(next), "version"); ok {
				out = append(out, unquote(strings.TrimPrefix(strings.TrimSpace(rest), ":")))
				break
			}
		}
	}
	return out
}

// fromGoMod reads a module's required version, or the Go version itself for the name "go".
func fromGoMod(text, name string) []string {
	var out []string
	for _, l := range lines(text) {
		if i := strings.Index(l, "//"); i >= 0 {
			l = l[:i]
		}
		f := strings.Fields(l)
		if len(f) > 0 && f[0] == "require" {
			f = f[1:]
		}
		if len(f) == 2 && f[0] == name && (name == "go" || strings.HasPrefix(f[1], "v")) {
			out = append(out, f[1])
		}
	}
	return out
}

// pythonName is how Python package names are compared: case and the characters - _ . do not matter.
func pythonName(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(s))
}

// fromTOMLLock reads the [[package]] entries of Cargo.lock, uv.lock and poetry.lock.
func fromTOMLLock(text, name string, python bool) []string {
	var out []string
	same := func(a string) bool {
		if python {
			return pythonName(a) == pythonName(name)
		}
		return a == name
	}
	inPackage, hit := false, false
	for _, l := range lines(text) {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			inPackage, hit = l == "[[package]]", false
			continue
		}
		if !inPackage {
			continue
		}
		key, value, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			hit = same(unquote(value))
		case "version":
			if hit {
				out = append(out, unquote(value))
			}
		}
	}
	return out
}

// fromRequirements reads exact pins ("name==1.2.3"); ranges say nothing about what is installed.
func fromRequirements(text, name string) []string {
	var out []string
	for _, l := range lines(text) {
		if i := strings.IndexAny(l, "#;"); i >= 0 {
			l = l[:i]
		}
		pkg, version, ok := strings.Cut(l, "==")
		if !ok {
			continue
		}
		if i := strings.Index(pkg, "["); i >= 0 {
			pkg = pkg[:i]
		}
		if pythonName(strings.TrimSpace(pkg)) == pythonName(name) {
			out = append(out, strings.TrimSpace(version))
		}
	}
	return out
}
