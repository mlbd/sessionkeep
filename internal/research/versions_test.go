package research

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAppliesToIsComparedOnlyWhenItNamesAPackageAndAVersion(t *testing.T) {
	for in, want := range map[string][2]string{
		"react@18":            {"react", "18"},
		"@scope/pkg@2.1":      {"@scope/pkg", "2.1"},
		"next 14.2":           {"next", "14.2"},
		"go 1.22":             {"go", "1.22"},
		"  serde@v1.0.200  ":  {"serde", "v1.0.200"},
		"golang.org/x/net@v0": {"golang.org/x/net", "v0"},
	} {
		name, version, ok := ParseAppliesTo(in)
		if !ok || name != want[0] || version != want[1] {
			t.Errorf("%q: got %q %q %v", in, name, version, ok)
		}
	}
	for _, in := range []string{"", "API version 2026-10", "the Pro plan", "react", "react@latest", "@18", "react 18 and later", "a@b c"} {
		if name, version, ok := ParseAppliesTo(in); ok {
			t.Errorf("%q must not be compared, got %q %q", in, name, version)
		}
	}
}

func TestVersionsAreComparedByTheNumbersTheClaimStates(t *testing.T) {
	for _, c := range []struct{ claimed, installed, want string }{
		{"18", "18.3.1", "match"},
		{"18.x", "18.3.1", "match"},
		{"v18.3", "18.3.1", "match"},
		{"18.3.1", "18.3.1-rc.1", "match"},
		{"17", "18.3.1", "other_major"},
		{"2", "v1.9.0", "other_major"},
		{"14.2", "14.1.0", "other_version"},
		{"14.2.3", "14.2", "other_version"},
		{"0.9", "0.10.2", "other_version"},
		{"1.22", "1.22.4", "match"},
	} {
		if got, ok := CompareVersions(c.claimed, c.installed); !ok || got != c.want {
			t.Errorf("claim %s, installed %s: got %q %v, want %q", c.claimed, c.installed, got, ok, c.want)
		}
	}
	for _, c := range [][2]string{{"latest", "1.0"}, {"1.0", "workspace:*"}, {"", "1"}} {
		if got, ok := CompareVersions(c[0], c[1]); ok {
			t.Errorf("%v cannot be compared, got %q", c, got)
		}
	}
}

func TestInstalledVersionIsReadFromEachKindOfLockFile(t *testing.T) {
	files := map[string]string{
		"package-lock.json": `{"lockfileVersion":3,"packages":{"":{"name":"app"},"node_modules/react":{"version":"18.3.1"},
			"node_modules/@scope/pkg":{"version":"2.1.0"},"node_modules/other/node_modules/react":{"version":"16.0.0"}}}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n      react:\n        specifier: ^18.2.0\n        version: 18.2.0\n" +
			"      '@scope/pkg':\n        specifier: ^2.0.0\n        version: 2.1.0(react@18.2.0)\npackages:\n  react@18.2.0:\n    resolution: {integrity: x}\n" +
			"snapshots:\n  thing@1.0.0:\n    dependencies:\n      react: 16.0.0\n",
		"yarn.lock": "# yarn lockfile v1\n\n\"@scope/pkg@^2.0.0\":\n  version \"2.1.0\"\n  resolved \"x\"\n\nreact@^18.0.0, react@^18.2.0:\n  version \"18.3.1\"\n\n" +
			"react-dom@^18.0.0:\n  version \"18.3.0\"\n\n\"left@npm:^1.0.0\":\n  version: 1.4.0\n",
		"go.mod": "module example.com/app\n\ngo 1.22.4\n\nrequire example.com/one v0.3.0 // indirect\n\nrequire (\n\tgolang.org/x/net v0.30.0\n\texample.com/two/v2 v2.5.1 // indirect\n)\n",
		"Cargo.lock": "version = 3\n\n[[package]]\nname = \"serde\"\nversion = \"1.0.200\"\nsource = \"x\"\n\n[[package]]\nname = \"syn\"\nversion = \"1.0.109\"\n\n" +
			"[[package]]\nname = \"syn\"\nversion = \"2.0.60\"\n\n[metadata]\nname = \"serde\"\nversion = \"9.9.9\"\n",
		"uv.lock":          "version = 1\n\n[[package]]\nname = \"typing-extensions\"\nversion = \"4.12.2\"\n",
		"poetry.lock":      "[[package]]\nname = \"Django\"\nversion = \"5.1.3\"\n",
		"requirements.txt": "# pins\nrequests==2.32.3 ; python_version >= '3.8'\nuvicorn[standard]==0.32.0\nflask>=3.0\n",
	}
	for _, c := range []struct {
		file, name string
		want       []string
	}{
		{"package-lock.json", "react", []string{"18.3.1"}},
		{"package-lock.json", "@scope/pkg", []string{"2.1.0"}},
		{"package-lock.json", "vue", nil},
		{"pnpm-lock.yaml", "react", []string{"18.2.0"}},
		{"pnpm-lock.yaml", "@scope/pkg", []string{"2.1.0"}},
		{"yarn.lock", "react", []string{"18.3.1"}},
		{"yarn.lock", "@scope/pkg", []string{"2.1.0"}},
		{"yarn.lock", "left", []string{"1.4.0"}},
		{"go.mod", "go", []string{"1.22.4"}},
		{"go.mod", "golang.org/x/net", []string{"v0.30.0"}},
		{"go.mod", "example.com/one", []string{"v0.3.0"}},
		{"go.mod", "example.com/two/v2", []string{"v2.5.1"}},
		{"go.mod", "example.com/app", nil},
		{"Cargo.lock", "serde", []string{"1.0.200"}},
		{"Cargo.lock", "syn", []string{"1.0.109", "2.0.60"}},
		{"uv.lock", "typing_extensions", []string{"4.12.2"}},
		{"poetry.lock", "django", []string{"5.1.3"}},
		{"requirements.txt", "Requests", []string{"2.32.3"}},
		{"requirements.txt", "uvicorn", []string{"0.32.0"}},
		{"requirements.txt", "flask", nil},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, c.file), []byte(files[c.file]), 0o600); err != nil {
			t.Fatal(err)
		}
		file, got := Installed(dir, c.name)
		if !reflect.DeepEqual(got, c.want) || (len(c.want) > 0 && file != c.file) {
			t.Errorf("%s in %s: got %v from %q, want %v", c.name, c.file, got, file, c.want)
		}
	}
}

// A damaged, missing or enormous lock file means "not found", never a guess and never a crash.
func TestUnreadableLockFilesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	if file, got := Installed(dir, "react"); file != "" || got != nil {
		t.Fatalf("empty folder: %q %v", file, got)
	}
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages": [`), 0o600)
	os.Mkdir(filepath.Join(dir, "yarn.lock"), 0o700)
	os.WriteFile(filepath.Join(dir, "Cargo.lock"), []byte("[[package]]\nname = \"react\"\nversion = \"workspace\"\n"), 0o600)
	if file, got := Installed(dir, "react"); file != "" || got != nil {
		t.Fatalf("damaged files: %q %v", file, got)
	}
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(strings.Repeat("x==1\n", 10)+"react==3.1\n"), 0o600)
	if file, got := Installed(dir, "react"); file != "requirements.txt" || !reflect.DeepEqual(got, []string{"3.1"}) {
		t.Fatalf("a later readable file is still used: %q %v", file, got)
	}
}
