package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"sessionkeep/internal/gitstate"
	"sessionkeep/internal/store"
)

// fileSession is what one session did to a file, or to the files of a commit.
type fileSession struct {
	ID      string   `json:"session_id"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Changes int      `json:"changes"`
	Reads   int      `json:"reads"`
	Files   []string `json:"files,omitempty"`
	Asked   string   `json:"asked,omitempty"`
	Reply   string   `json:"reply,omitempty"`

	first, last int64
}

const storyTextRunes = 400

func storyLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > storyTextRunes {
		return string(r[:storyTextRunes]) + " ..."
	}
	return s
}

// groupTouches turns tool calls into one entry per session that changed something, oldest
// first, each with the request that led to its first change and the reply after its last.
// It also returns how many sessions only read.
func groupTouches(ctx context.Context, s *store.Store, projectID string, touches []store.Touch) ([]fileSession, int, error) {
	by := map[string]*fileSession{}
	var order []string
	for _, t := range touches {
		x := by[t.SessionID]
		if x == nil {
			x = &fileSession{ID: t.SessionID, From: t.At}
			by[t.SessionID] = x
			order = append(order, t.SessionID)
		}
		x.To = t.At
		if !store.ChangeTools[t.Tool] {
			x.Reads++
			continue
		}
		x.Changes++
		if x.first == 0 {
			x.first = t.Seq
		}
		x.last = t.Seq
		seen := false
		for _, f := range x.Files {
			seen = seen || f == t.File
		}
		if !seen {
			x.Files = append(x.Files, t.File)
		}
	}
	out, readers := []fileSession{}, 0
	for _, id := range order {
		x := by[id]
		if x.Changes == 0 {
			readers++
			continue
		}
		asked, err := s.RequestBefore(ctx, projectID, id, x.first)
		if err != nil {
			return nil, 0, err
		}
		reply, err := s.ReplyAfter(ctx, projectID, store.Failure{SessionID: id, Seq: x.last})
		if err != nil {
			return nil, 0, err
		}
		x.Asked, x.Reply = storyLine(asked), storyLine(reply)
		out = append(out, *x)
	}
	return out, readers, nil
}

func printFileSessions(b *strings.Builder, list []fileSession, loc *time.Location) {
	for _, x := range list {
		at, _ := time.Parse(time.RFC3339Nano, x.From)
		fmt.Fprintf(b, "\n%s  session %.8s  %d change(s)", at.In(loc).Format("2006-01-02 15:04"), x.ID, x.Changes)
		if len(x.Files) > 1 {
			fmt.Fprintf(b, " in %d files", len(x.Files))
		}
		b.WriteString("\n")
		if x.Asked != "" {
			fmt.Fprintf(b, "    asked: %s\n", x.Asked)
		}
		if x.Reply != "" {
			fmt.Fprintf(b, "    reply: %s\n", x.Reply)
		}
		fmt.Fprintf(b, "    all of it: sessionkeep session show %.8s\n", x.ID)
	}
}

// insideRoot returns path relative to the project root it lies in, or "".
func insideRoot(p store.Project, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	key := store.FileKey(abs)
	for _, r := range p.Roots {
		if root := store.FileKey(r); strings.HasPrefix(key, root+"/") {
			return key[len(root)+1:]
		}
	}
	return ""
}

func openForRead(inv invocation) (state, *store.Store, error) {
	st, err := load(inv, false)
	if err != nil {
		return st, nil, err
	}
	s, err := store.OpenReadOnly(context.Background(), st.paths.DBPath)
	if err != nil {
		if errors.Is(err, store.ErrNeedsSetup) || errors.Is(err, store.ErrSchemaTooNew) {
			return st, nil, &cmdError{ExitUnavailable, "database_unavailable", err.Error()}
		}
		return st, nil, err
	}
	return st, s, nil
}

const noToolDetail = "Changes are known only where the project saves conversation text (file names are part of that), and only for sessions SessionKeep captured as they happened; imported sessions and changes made by hand leave no trace here.\n"

// fileCmd tells the story of one file: the sessions that changed it, what was asked and what
// was answered. It reads only.
func fileCmd(inv invocation, rest []string) error {
	if err := inv.only("project"); err != nil {
		return err
	}
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		return usageErr("usage: sessionkeep file PATH [--project ID_OR_PATH]")
	}
	st, s, err := openForRead(inv)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx := context.Background()
	proj, err := resolveProject(ctx, inv, st, s, inv.flags["project"])
	if err != nil {
		return err
	}
	tail := insideRoot(proj, rest[0])
	if tail == "" {
		tail = store.FileKey(rest[0])
		if i := strings.Index(tail, ":/"); i >= 0 { // an absolute path outside the project: compare by what follows the drive
			tail = tail[i+2:]
		}
	}
	touches, err := s.Touches(ctx, proj.ID, tail, "", "", 5000)
	if err != nil {
		return err
	}
	list, readers, err := groupTouches(ctx, s, proj.ID, touches)
	if err != nil {
		return err
	}
	changes := 0
	for _, x := range list {
		changes += x.Changes
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s in project %q: changed in %d session(s) (%d change(s)); only read in %d more.\n", tail, proj.Name, len(list), changes, readers)
	if len(list) == 0 {
		b.WriteString(noToolDetail)
	}
	printFileSessions(&b, list, time.Local)
	return inv.emit(map[string]any{"file": tail, "project_id": proj.ID, "sessions": list, "sessions_that_only_read": readers}, b.String())
}

var (
	revForm  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./~^@{}-]{0,199}$`)
	lineForm = regexp.MustCompile(`^(.+):([0-9]{1,7})$`)
)

// whyCmd finds the conversation behind a commit, or behind the commit that last changed a line:
// the sessions that changed the commit's files in the week before it. It reads only.
func whyCmd(inv invocation, rest []string) error {
	const usageText = "usage: sessionkeep why COMMIT [--project ID_OR_PATH]\n       sessionkeep why FILE:LINE [--project ID_OR_PATH]"
	if err := inv.only("project"); err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErr("%s", usageText)
	}
	st, s, err := openForRead(inv)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx := context.Background()
	proj, err := resolveProject(ctx, inv, st, s, inv.flags["project"])
	if err != nil {
		return err
	}
	dir := workDir(inv, st, proj, inv.flags["project"])
	rev, about := rest[0], ""
	if m := lineForm.FindStringSubmatch(rest[0]); m != nil {
		line, _ := strconv.Atoi(m[2])
		file := m[1]
		if rel := insideRoot(proj, file); rel != "" && !filepath.IsAbs(file) {
			file = filepath.ToSlash(file)
		}
		found, err := gitstate.BlameLine(dir, file, line)
		if err != nil {
			return failErr("not_found", "git could not say which commit last changed line %d of %s (%v)", line, m[1], err)
		}
		rev, about = found, fmt.Sprintf("Line %d of %s was last changed by this commit.\n", line, m[1])
	} else if !revForm.MatchString(rev) {
		return usageErr("%q is not a commit or FILE:LINE; %s", rev, usageText)
	}
	c, files, err := gitstate.CommitInfo(dir, rev)
	if err != nil {
		return failErr("not_found", "git knows no commit %q in %s (%v)", rev, dir, err)
	}
	at, _ := time.Parse(time.RFC3339, c.At)
	since, until := at.Add(-7*24*time.Hour).Format(time.RFC3339), at.Add(time.Minute).Format(time.RFC3339)
	var touches []store.Touch
	for _, f := range files {
		t, err := s.Touches(ctx, proj.ID, store.FileKey(f), since, until, 2000)
		if err != nil {
			return err
		}
		touches = append(touches, t...)
	}
	sort.Slice(touches, func(i, j int) bool { return touches[i].Seq < touches[j].Seq })
	list, _, err := groupTouches(ctx, s, proj.ID, touches)
	if err != nil {
		return err
	}
	// The session that changed most of the commit's files comes first.
	sort.SliceStable(list, func(i, j int) bool { return len(list[i].Files) > len(list[j].Files) })

	var b strings.Builder
	b.WriteString(about)
	fmt.Fprintf(&b, "commit %s  %s  %q, %d file(s) in project %q.\n", c.ID, at.In(time.Local).Format("2006-01-02 15:04"), c.Subject, len(files), proj.Name)
	if len(list) == 0 {
		b.WriteString("No saved session changed these files in the 7 days before it.\n" + noToolDetail)
	} else {
		fmt.Fprintf(&b, "%d saved session(s) changed its files in the 7 days before it; the one that changed most of them is first. A session is tied to the commit by file and time only.\n", len(list))
	}
	printFileSessions(&b, list, time.Local)
	return inv.emit(map[string]any{"commit": c, "files": files, "project_id": proj.ID, "sessions": list}, b.String())
}
