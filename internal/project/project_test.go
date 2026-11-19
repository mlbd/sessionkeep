package project

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"sessionkeep/internal/config"
)

func TestNormalize(t *testing.T) {
	tests := []struct{ goos, in, want string }{
		{"windows", `C:\Work\App`, `c:\work\app`},
		{"windows", `c:/work/app/`, `c:\work\app`},
		{"windows", `C:\Work\.\App\..\App\\src`, `c:\work\app\src`},
		{"windows", `\\?\C:\Work\App`, `c:\work\app`},
		{"windows", `C:\`, `c:\`},
		{"windows", `C:\..\..`, `c:\`},
		{"windows", `C:\Users\José Müller\Проект`, `c:\users\josé müller\проект`},
		{"windows", `\\Server\Share\Team\Repo\`, `\\server\share\team\repo`},
		{"windows", `\\?\UNC\Server\Share\x`, `\\server\share\x`},
		{"linux", "/home/dev/app/", "/home/dev/app"},
		{"linux", "/home/dev/./app/../App", "/home/dev/App"},
		{"linux", "/", "/"},
		{"darwin", "/Users/dev/My Project", "/Users/dev/My Project"},
	}
	for _, tc := range tests {
		got, err := Normalize(tc.goos, tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Normalize(%s, %q) = %q, %v; want %q", tc.goos, tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []struct{ goos, in string }{
		{"windows", `work\app`}, {"windows", `C:app`}, {"windows", ``}, {"windows", `\\server`}, {"windows", `/unix/style`},
		{"linux", "relative/dir"}, {"linux", ""}, {"linux", "/a\x00b"},
	} {
		if got, err := Normalize(bad.goos, bad.in); err == nil {
			t.Errorf("Normalize(%s, %q) = %q; want an error", bad.goos, bad.in, got)
		}
	}
}

func decide(mode, level string, exclusions []string, roots []Root, cwd string, repoRoot string) Decision {
	s := config.DefaultSettings()
	s.AutoAdd = config.AutoAdd{Mode: mode, Level: level}
	s.Exclusions = exclusions
	env := Env{GOOS: "windows", Home: `C:\Users\dev`, RepoRoot: func(string) string { return repoRoot }}
	return Decide(env, s, roots, cwd)
}

func TestDecide(t *testing.T) {
	roots := []Root{
		{Path: `c:\work\app`, ProjectID: "p-app", Level: "conversation"},
		{Path: `c:\work\app\packages\billing`, ProjectID: "p-billing", Level: "metadata"},
		{Path: `c:\work\paused`, ProjectID: "p-paused", Level: "conversation", Paused: true},
		{Path: `c:\work\off`, ProjectID: "p-off", Level: "off"},
		{Path: `c:\clients\acme\tool`, ProjectID: "p-acme", Level: "conversation"},
	}
	ex := []string{`c:\clients\acme`}
	tests := []struct {
		name, mode, level, cwd, repo string
		want                         Decision
	}{
		{"registered root, different case and slashes", "ask", "", `C:/Work/APP`, "", Decision{Action: ActionCapture, ProjectID: "p-app", Level: "conversation", Root: `c:\work\app`}},
		{"subdirectory of a root", "ask", "", `C:\Work\App\src\deep`, "", Decision{Action: ActionCapture, ProjectID: "p-app", Level: "conversation", Root: `c:\work\app`}},
		{"nested project wins by longest root", "ask", "", `C:\Work\App\packages\billing\src`, "", Decision{Action: ActionCapture, ProjectID: "p-billing", Level: "metadata", Root: `c:\work\app\packages\billing`}},
		{"sibling with a shared name prefix is not inside the root", "manual", "", `C:\Work\App2`, "", Decision{Action: ActionNone}},
		{"paused project captures nothing", "auto", "conversation", `C:\Work\Paused\src`, "", Decision{Action: ActionNone, ProjectID: "p-paused", Root: `c:\work\paused`}},
		{"off project captures nothing and is not re-added by auto", "auto", "conversation", `C:\Work\Off`, "", Decision{Action: ActionNone, ProjectID: "p-off", Root: `c:\work\off`}},
		{"exclusion beats a registered root", "auto", "conversation", `C:\Clients\Acme\Tool\src`, "", Decision{Action: ActionNone}},
		{"exclusion beats auto mode", "auto", "conversation", `C:\Clients\ACME\other`, "", Decision{Action: ActionNone}},
		{"exclusion gives no reminder", "ask", "", `C:\Clients\Acme\new`, "", Decision{Action: ActionNone}},
		{"folder next to an exclusion is not excluded", "ask", "", `C:\Clients\Acme2`, "", Decision{Action: ActionRemind}},
		{"unknown folder, manual", "manual", "", `C:\Work\New`, "", Decision{Action: ActionNone}},
		{"unknown folder, ask", "ask", "", `C:\Work\New`, "", Decision{Action: ActionRemind}},
		{"unknown folder, auto, no repository", "auto", "conversation", `C:\Work\New\sub`, "", Decision{Action: ActionAutoRegister, Level: "conversation", Root: `c:\work\new\sub`}},
		{"unknown folder, auto, registers the repository root", "auto", "metadata", `C:\Work\New\sub\dir`, `C:\Work\New`, Decision{Action: ActionAutoRegister, Level: "metadata", Root: `c:\work\new`}},
		{"auto ignores a repository root that does not contain the folder", "auto", "conversation", `C:\Work\New`, `D:\Elsewhere`, Decision{Action: ActionAutoRegister, Level: "conversation", Root: `c:\work\new`}},
		{"auto refuses a drive root", "auto", "conversation", `C:\`, "", Decision{Action: ActionRemind}},
		{"auto refuses a top-level folder", "auto", "conversation", `C:\Work`, "", Decision{Action: ActionRemind}},
		{"auto refuses the home directory", "auto", "conversation", `C:\Users\Dev`, "", Decision{Action: ActionRemind}},
		{"auto refuses a repository rooted above home", "auto", "conversation", `C:\Users\Dev\code\x`, `C:\Users`, Decision{Action: ActionRemind}},
		{"auto accepts a folder inside home", "auto", "conversation", `C:\Users\Dev\code\x`, "", Decision{Action: ActionAutoRegister, Level: "conversation", Root: `c:\users\dev\code\x`}},
		{"relative path captures nothing", "auto", "conversation", `work\app`, "", Decision{Action: ActionNone}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decide(tc.mode, tc.level, ex, roots, tc.cwd, tc.repo)
			if got.Reason == "" {
				t.Error("every decision must carry a reason")
			}
			got.Reason = ""
			if got != tc.want {
				t.Fatalf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// A root with an unknown level (damaged cache, newer version) must never be treated as capture.
func TestDecideTreatsUnknownLevelAsOff(t *testing.T) {
	got := decide("ask", "", nil, []Root{{Path: `c:\work\app`, ProjectID: "p", Level: "everything"}}, `C:\Work\App`, "")
	if got.Action != ActionNone {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestRootsCacheRoundTripAndFailures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data é")
	p, err := config.Resolve(runtime.GOOS, func(k string) string {
		if k == config.EnvDataDir {
			return dir
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if roots, err := ReadRootsCache(p); err != nil || roots != nil {
		t.Fatalf("missing cache must mean no roots, got %v, %v", roots, err)
	}
	if err := config.Ensure(p); err != nil {
		t.Fatal(err)
	}
	in := []Root{{Path: `c:\b`, ProjectID: "2", Level: "metadata", Paused: true}, {Path: `c:\a`, ProjectID: "1", Level: "conversation"}}
	if err := WriteRootsCache(p, in, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	out, err := ReadRootsCache(p)
	if err != nil || len(out) != 2 || out[0].Path != `c:\a` || !out[1].Paused {
		t.Fatalf("round trip: %+v, %v", out, err)
	}
	left, _ := filepath.Glob(filepath.Join(p.CacheDir, "*.tmp"))
	if len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
	os.WriteFile(p.RootsCachePath(), []byte(`{"version":1,"roots":[{"path":`), 0o600)
	if _, err := ReadRootsCache(p); err == nil {
		t.Fatal("a truncated cache must be an error, not an empty list")
	}
	os.WriteFile(p.RootsCachePath(), []byte(`{"version":7,"roots":[]}`), 0o600)
	if _, err := ReadRootsCache(p); err == nil {
		t.Fatal("an unknown cache version must be an error")
	}
}

func TestFindRepoRoot(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo é")
	deep := filepath.Join(repo, "a", "b")
	os.MkdirAll(deep, 0o700)
	os.MkdirAll(filepath.Join(repo, ".git"), 0o700)
	if got := FindRepoRoot(deep); got != repo {
		t.Fatalf("got %q, want %q", got, repo)
	}
	// A worktree or submodule has a .git file, not a directory.
	wt := filepath.Join(base, "worktree")
	os.MkdirAll(filepath.Join(wt, "src"), 0o700)
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: elsewhere"), 0o600)
	if got := FindRepoRoot(filepath.Join(wt, "src")); got != wt {
		t.Fatalf("got %q, want %q", got, wt)
	}
}
