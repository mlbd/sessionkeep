package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolve(t *testing.T) {
	tests := []struct {
		name, goos string
		env        map[string]string
		wantDir    string
		wantSource string
		wantDB     string
	}{
		{"windows default", "windows", map[string]string{"LOCALAPPDATA": `C:\Users\dev\AppData\Local`},
			`C:\Users\dev\AppData\Local\SessionKeep`, "localappdata", `C:\Users\dev\AppData\Local\SessionKeep\sessionkeep.db`},
		{"windows profile with spaces and non-ASCII", "windows", map[string]string{"LOCALAPPDATA": `C:\Users\José Müller 開発\AppData\Local`},
			`C:\Users\José Müller 開発\AppData\Local\SessionKeep`, "localappdata", `C:\Users\José Müller 開発\AppData\Local\SessionKeep\sessionkeep.db`},
		{"windows trailing separator", "windows", map[string]string{"LOCALAPPDATA": `D:\Data\`},
			`D:\Data\SessionKeep`, "localappdata", `D:\Data\SessionKeep\sessionkeep.db`},
		{"windows UNC", "windows", map[string]string{"LOCALAPPDATA": `\\server\share\Local`},
			`\\server\share\Local\SessionKeep`, "localappdata", `\\server\share\Local\SessionKeep\sessionkeep.db`},
		{"windows override wins", "windows", map[string]string{"LOCALAPPDATA": `C:\L`, EnvDataDir: `E:\sand box\é\`},
			`E:\sand box\é`, "override", `E:\sand box\é\sessionkeep.db`},
		{"darwin", "darwin", map[string]string{"HOME": "/Users/dev"},
			"/Users/dev/Library/Application Support/SessionKeep", "application-support", "/Users/dev/Library/Application Support/SessionKeep/sessionkeep.db"},
		{"linux xdg", "linux", map[string]string{"HOME": "/home/dev", "XDG_DATA_HOME": "/data/xdg"},
			"/data/xdg/sessionkeep", "xdg", "/data/xdg/sessionkeep/sessionkeep.db"},
		{"linux relative xdg is ignored", "linux", map[string]string{"HOME": "/home/dev", "XDG_DATA_HOME": "relative/dir"},
			"/home/dev/.local/share/sessionkeep", "home", "/home/dev/.local/share/sessionkeep/sessionkeep.db"},
		{"linux home with space", "linux", map[string]string{"HOME": "/home/dev user"},
			"/home/dev user/.local/share/sessionkeep", "home", "/home/dev user/.local/share/sessionkeep/sessionkeep.db"},
		{"linux root home", "linux", map[string]string{"HOME": "/"},
			"/.local/share/sessionkeep", "home", "/.local/share/sessionkeep/sessionkeep.db"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Resolve(tc.goos, env(tc.env))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.DataDir != tc.wantDir || p.Source != tc.wantSource || p.DBPath != tc.wantDB {
				t.Fatalf("got dir=%q source=%q db=%q; want dir=%q source=%q db=%q", p.DataDir, p.Source, p.DBPath, tc.wantDir, tc.wantSource, tc.wantDB)
			}
		})
	}
}

func TestResolveFailsClosedWithoutUsableEnvironment(t *testing.T) {
	tests := []struct {
		name, goos string
		env        map[string]string
	}{
		{"windows no LOCALAPPDATA", "windows", nil},
		{"windows relative LOCALAPPDATA", "windows", map[string]string{"LOCALAPPDATA": `AppData\Local`}},
		{"windows drive-relative", "windows", map[string]string{"LOCALAPPDATA": `C:Local`}},
		{"windows relative override is rejected, not silently replaced", "windows", map[string]string{"LOCALAPPDATA": `C:\L`, EnvDataDir: `.\data`}},
		{"darwin no HOME", "darwin", nil},
		{"linux nothing", "linux", map[string]string{"XDG_DATA_HOME": "relative"}},
		{"linux relative override", "linux", map[string]string{"HOME": "/home/dev", EnvDataDir: "data"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Resolve(tc.goos, env(tc.env))
			if !errors.Is(err, ErrNoDataDir) {
				t.Fatalf("want ErrNoDataDir, got err=%v paths=%+v", err, p)
			}
			if p != (Paths{}) {
				t.Fatalf("paths must be empty on error, got %+v", p)
			}
		})
	}
}

// Real filesystem: a directory whose name has a space and non-ASCII characters, on the host OS.
func TestEnsureCreatesLayoutInAwkwardPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profil é 開発", "data dir")
	p, err := Resolve(runtime.GOOS, env(map[string]string{EnvDataDir: root}))
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(p); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(p); err != nil {
		t.Fatalf("second Ensure must be a no-op: %v", err)
	}
	for _, d := range []string{p.DataDir, p.SpoolDir, p.CacheDir} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("%s not created: %v", d, err)
		}
	}
	left, _ := filepath.Glob(filepath.Join(p.DataDir, ".write-check-*"))
	if len(left) != 0 {
		t.Fatalf("write-check file left behind: %v", left)
	}
}

func TestEnsureReportsUnusableLocation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Resolve(runtime.GOOS, env(map[string]string{EnvDataDir: filepath.Join(file, "data")}))
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(p); err == nil {
		t.Fatal("Ensure must fail when the data directory cannot be created")
	}
}
