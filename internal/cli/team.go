package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"sessionkeep/internal/config"
	"sessionkeep/internal/handoff"
	"sessionkeep/internal/ingest"
	"sessionkeep/internal/instruct"
	"sessionkeep/internal/keep"
	"sessionkeep/internal/s3lite"
	"sessionkeep/internal/store"
	"sessionkeep/internal/team"
)

const teamUsage = `usage: sessionkeep team connect --name SHARED-NAME --endpoint URL --bucket BUCKET [--region REGION] [--from YOUR-NAME] [--project ID_OR_PATH]
           (the storage keys and the team passphrase are read from the environment, never from the command line:
            SESSIONKEEP_TEAM_ACCESS_KEY, SESSIONKEEP_TEAM_SECRET_KEY, SESSIONKEEP_TEAM_PASSPHRASE)
       sessionkeep team status [--project ID_OR_PATH]
       sessionkeep team push [--project ID_OR_PATH] [--no-sessions] [--yes]
       sessionkeep team pull [--project ID_OR_PATH] [--yes]
       sessionkeep team disconnect [--project ID_OR_PATH] [--yes]`

// Environment variables that carry secrets into `team connect`.
const (
	envTeamAccess     = "SESSIONKEEP_TEAM_ACCESS_KEY"
	envTeamSecret     = "SESSIONKEEP_TEAM_SECRET_KEY"
	envTeamPassphrase = "SESSIONKEEP_TEAM_PASSPHRASE"
)

var bucketForm = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

const teamTimeout = 3 * time.Minute

func teamClient(conn team.Connection, sec team.Secrets) *s3lite.Client {
	return &s3lite.Client{Endpoint: conn.Endpoint, Bucket: conn.Bucket, Region: conn.Region, AccessKey: sec.AccessKey, SecretKey: sec.SecretKey}
}

func teamErr(err error) error {
	var se *s3lite.StatusError
	switch {
	case errors.As(err, &se):
		return failErr("storage_refused", "%v", err)
	case errors.Is(err, team.ErrPassphrase):
		return failErr("wrong_passphrase", "the team passphrase stored here does not open what is in the bucket; connect again with the passphrase the team uses")
	}
	return failErr("storage_unreachable", "%v", err)
}

// teamCmd keeps a project's memory in a bucket the owner chose and exchanges it with colleagues.
// Nothing is sent or taken in without --yes, and everything sent is encrypted here first.
func teamCmd(inv invocation, rest []string) error {
	if len(rest) != 1 {
		return usageErr("%s", teamUsage)
	}
	switch rest[0] {
	case "connect":
		if err := inv.only("name", "endpoint", "bucket", "region", "from", "project"); err != nil {
			return err
		}
		return teamConnect(inv)
	case "status":
		if err := inv.only("project"); err != nil {
			return err
		}
		return teamStatus(inv)
	case "push":
		if err := inv.only("project", "no-sessions", "yes"); err != nil {
			return err
		}
		return teamPush(inv)
	case "pull":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
		return teamPull(inv)
	case "disconnect":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
		return teamDisconnect(inv)
	}
	return usageErr("%s", teamUsage)
}

// teamProject resolves the project and its connection.
func teamProject(inv invocation, st state) (store.Project, team.Connection, team.Secrets, error) {
	var proj store.Project
	err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
		proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"])
		return err
	})
	if err != nil {
		return proj, team.Connection{}, team.Secrets{}, err
	}
	conn, sec, ok, err := team.Get(st.paths, proj.ID)
	if err != nil {
		return proj, conn, sec, failErr("team_settings", "%v", err)
	}
	if !ok {
		return proj, conn, sec, failErr("not_connected", "project %q is not connected to a team bucket; run `sessionkeep team connect` first", proj.Name)
	}
	return proj, conn, sec, nil
}

func teamConnect(inv invocation) error {
	name, bucket, region := inv.flags["name"], inv.flags["bucket"], inv.flags["region"]
	if name == "" || bucket == "" || inv.flags["endpoint"] == "" {
		return usageErr("%s", teamUsage)
	}
	if err := team.CheckName(name); err != nil {
		return usageErr("%v", err)
	}
	if !bucketForm.MatchString(bucket) {
		return usageErr("%q is not a bucket name (lower-case letters, digits, dots and dashes)", bucket)
	}
	endpoint, err := s3lite.CheckEndpoint(inv.flags["endpoint"])
	if err != nil {
		return usageErr("%v", err)
	}
	if region == "" {
		region = "auto"
	}
	if len(inv.flags["from"]) > 100 {
		return usageErr("--from is a short name, at most 100 characters")
	}
	sec := team.Secrets{AccessKey: strings.TrimSpace(inv.getenv(envTeamAccess)), SecretKey: strings.TrimSpace(inv.getenv(envTeamSecret)), Passphrase: inv.getenv(envTeamPassphrase)}
	if sec.AccessKey == "" || sec.SecretKey == "" || sec.Passphrase == "" {
		return &cmdError{ExitNeedsConsent, "keys_required", fmt.Sprintf("set %s, %s and %s in the environment for this one command; they are stored protected afterwards and are never taken from the command line", envTeamAccess, envTeamSecret, envTeamPassphrase)}
	}
	if len([]rune(sec.Passphrase)) < team.MinPassphrase {
		return usageErr("the team passphrase must be at least %d characters; it is all that keeps the stored sessions unreadable", team.MinPassphrase)
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var proj store.Project
	if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
		proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"])
		return err
	}); err != nil {
		return err
	}
	conn := team.Connection{ProjectID: proj.ID, Name: name, Endpoint: endpoint, Bucket: bucket, Region: region, As: strings.TrimSpace(inv.flags["from"]), ConnectedAt: time.Now().UTC().Format(time.RFC3339)}
	ctx, cancel := context.WithTimeout(context.Background(), teamTimeout)
	defer cancel()
	client := teamClient(conn, sec)
	// Reading the listing proves the keys work; opening one bundle proves the passphrase is the
	// team's. Nothing is written to the bucket here.
	there, err := client.List(ctx, team.Prefix(name))
	if err != nil {
		return teamErr(err)
	}
	if len(there) > 0 {
		raw, err := client.Get(ctx, there[len(there)-1].Key, team.MaxBundleBytes)
		if err != nil {
			return teamErr(err)
		}
		if _, err := team.OpenBundle(raw, sec.Passphrase); errors.Is(err, team.ErrPassphrase) {
			return failErr("wrong_passphrase", "the bucket already holds %d bundle(s) under the shared name %q, and this passphrase does not open them; use the passphrase the team uses, or another shared name", len(there), name)
		}
	}
	if err := team.Put(st.paths, conn, sec); err != nil {
		return err
	}
	guard := "The keys and the passphrase are stored protected for your Windows user on this computer."
	if !keep.Protected {
		guard = "The keys and the passphrase are stored in a file only you can read; this system has no stronger protection built in yet."
	}
	return inv.emit(map[string]any{"connected": true, "project_id": proj.ID, "name": name, "endpoint": endpoint, "bucket": bucket, "bundles_there": len(there), "keys_protected_by_os": keep.Protected},
		fmt.Sprintf("project %q is connected to bucket %s as %q; %d bundle(s) are there already.\n%s\nNothing was sent. Send with `sessionkeep team push`, get the others' work with `sessionkeep team pull`.\n", proj.Name, bucket, name, len(there), guard))
}

// teamNew gathers what this computer has not sent yet.
func teamNew(inv invocation, st state, proj store.Project, conn team.Connection, withSessions bool) (handoff.File, *handoff.Scrubber, int, team.State, error) {
	state, err := team.LoadState(st.paths, proj.ID)
	if err != nil {
		return handoff.File{}, nil, 0, state, failErr("team_settings", "%v", err)
	}
	skip := make(map[string]bool, len(state.Pushed))
	for _, id := range state.Pushed {
		skip[id] = true
	}
	f := newHandoff(st, proj, conn.As)
	var scrub *handoff.Scrubber
	secrets := 0
	err = withStore(st, func(ctx context.Context, s *store.Store) error {
		if _, err := ingest.Drain(ctx, ingest.Deps{Paths: st.paths, Settings: st.settings, Store: s, GOOS: runtime.GOOS, Now: time.Now, NewID: config.NewID}); err != nil {
			return fmt.Errorf("store waiting events before sending: %w", err)
		}
		var err error
		scrub, secrets, err = gatherHandoff(ctx, s, proj, homeOf(inv), &f, handoffPick{withSessions: withSessions, skip: skip, ownOnly: true})
		return err
	})
	return f, scrub, secrets, state, err
}

func teamPush(inv invocation) error {
	apply := inv.flags["yes"] != ""
	withSessions := inv.flags["no-sessions"] == ""
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	f, scrub, secrets, state, err := teamNew(inv, st, proj, conn, withSessions)
	if err != nil {
		return err
	}
	c := f.Count()
	out := map[string]any{"sent": false, "project_id": proj.ID, "bucket": conn.Bucket, "counts": c, "paths_shortened": scrub.Count, "secrets_removed": secrets}
	if c.Instructions+c.Checkpoints+c.Turns == 0 {
		return inv.emit(out, fmt.Sprintf("Nothing new to send for project %q: everything of yours is already in the bucket.\n", proj.Name))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "New since your last push, for project %q to bucket %s:\n  %d instruction(s), %d checkpoint(s), %d session(s) with %d piece(s) of text\n", proj.Name, conn.Bucket, c.Instructions, c.Checkpoints, c.Sessions, c.Turns)
	if !withSessions {
		b.WriteString("  Sessions are left out (--no-sessions).\n")
	}
	fmt.Fprintf(&b, "  Your project and home folders were shortened in %d place(s); %d more secret(s) were removed.\n", scrub.Count, secrets)
	b.WriteString("It is encrypted on this computer with the team passphrase before it leaves; the storage service cannot read it. It can see that a bundle exists, its size, the time and which computer sent it.\n")
	b.WriteString("Not removed: other names or paths you typed in a conversation. Not sent: " + strings.Join(handoffLeftOut, "; ") + ".\n")
	if !apply {
		b.WriteString("NOTHING SENT YET. Run the same command with --yes to send it.\n")
		inv.emit(out, b.String())
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was sent; add --yes to confirm"}
	}
	sealed, err := team.SealBundle(f, sec.Passphrase)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), teamTimeout)
	defer cancel()
	key := team.BundleKey(conn.Name, st.settings.DeviceID, time.Now(), strings.ReplaceAll(config.NewID(), "-", "")[:10])
	if err := teamClient(conn, sec).Put(ctx, key, sealed); err != nil {
		return teamErr(err)
	}
	for _, x := range f.Instructions {
		state.Pushed = append(state.Pushed, x.ID)
	}
	for _, x := range f.Checkpoints {
		state.Pushed = append(state.Pushed, x.ID)
	}
	for _, s := range f.Sessions {
		for _, t := range s.Turns {
			state.Pushed = append(state.Pushed, t.ID)
		}
	}
	state.Pulled = append(state.Pulled, key)
	if err := team.SaveState(st.paths, proj.ID, state); err != nil {
		return fmt.Errorf("the bundle was sent, but the record of what was sent could not be saved (the next push will send it again, which is harmless): %w", err)
	}
	out["sent"], out["bytes"] = true, len(sealed)
	fmt.Fprintf(&b, "SENT: one encrypted bundle of %d bytes.\n", len(sealed))
	return inv.emit(out, b.String())
}

// teamWaiting downloads and opens the bundles this computer has not received. It returns the
// opened files with their names, every name seen, and the names that could not be read.
func teamWaiting(st state, proj store.Project, conn team.Connection, sec team.Secrets, known team.State) (files []handoff.File, keys, all, unreadable []string, err error) {
	have := make(map[string]bool, len(known.Pulled))
	for _, k := range known.Pulled {
		have[k] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), teamTimeout)
	defer cancel()
	client := teamClient(conn, sec)
	there, err := client.List(ctx, team.Prefix(conn.Name))
	if err != nil {
		return nil, nil, nil, nil, teamErr(err)
	}
	for _, o := range there {
		all = append(all, o.Key)
		if have[o.Key] || team.SentBy(o.Key, st.settings.DeviceID) {
			continue
		}
		raw, err := client.Get(ctx, o.Key, team.MaxBundleBytes)
		if err != nil {
			return nil, nil, nil, nil, teamErr(err)
		}
		f, err := team.OpenBundle(raw, sec.Passphrase)
		switch {
		case errors.Is(err, team.ErrPassphrase):
			return nil, nil, nil, nil, teamErr(err)
		case err != nil:
			unreadable = append(unreadable, o.Key[strings.LastIndex(o.Key, "/")+1:])
			continue
		case f.SourceDevice == st.settings.DeviceID:
			continue
		}
		files, keys = append(files, f), append(keys, o.Key)
	}
	return files, keys, all, unreadable, nil
}

func teamPull(inv invocation) error {
	apply := inv.flags["yes"] != ""
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	known, err := team.LoadState(st.paths, proj.ID)
	if err != nil {
		return failErr("team_settings", "%v", err)
	}
	files, keys, all, unreadable, err := teamWaiting(st, proj, conn, sec, known)
	if err != nil {
		return err
	}
	var total handoff.Counts
	var instructions []handoff.Instruction
	senders := map[string]bool{}
	for _, f := range files {
		c := f.Count()
		total.Instructions += c.Instructions
		total.Checkpoints += c.Checkpoints
		total.Sessions += c.Sessions
		total.Turns += c.Turns
		instructions = append(instructions, f.Instructions...)
		senders[senderName(f)] = true
	}
	names := make([]string, 0, len(senders))
	for n := range senders {
		names = append(names, n)
	}
	sort.Strings(names)
	out := map[string]any{"received": false, "project_id": proj.ID, "bucket": conn.Bucket, "bundles": len(files), "counts": total, "from": names,
		"instructions": instructions, "unreadable_bundles": unreadable, "sessions_taken": takesSessions(proj)}
	var b strings.Builder
	for _, u := range unreadable {
		fmt.Fprintf(&b, "Skipped: bundle %s is damaged or was altered, and nothing was taken from it.\n", u)
	}
	if len(files) == 0 {
		b.WriteString(fmt.Sprintf("Nothing new from the team for project %q.\n", proj.Name))
		return inv.emit(out, b.String())
	}
	fmt.Fprintf(&b, "New from the team (%s), for your project %q: %d bundle(s) with\n  %d instruction(s), %d checkpoint(s), %d session(s) with %d piece(s) of text\n", strings.Join(names, ", "), proj.Name, len(files), total.Instructions, total.Checkpoints, total.Sessions, total.Turns)
	if total.Sessions > 0 && !takesSessions(proj) {
		b.WriteString("The sessions would be left out: this project is paused or does not save conversation text.\n")
	}
	b.WriteString(instructionsToRead(instructions))
	if !apply {
		b.WriteString("NOTHING RECEIVED YET. Run the same command with --yes to take it in.\n")
		inv.emit(out, b.String())
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was received; add --yes to confirm"}
	}
	var res handoffResult
	if err := withStore(st, func(ctx context.Context, s *store.Store) error {
		for _, f := range files {
			one, err := applyHandoff(ctx, s, st, proj, f)
			if err != nil {
				return err
			}
			res.add(one)
		}
		return instruct.RewriteCaches(ctx, s, st.paths, st.settings.DeviceID, time.Now())
	}); err != nil {
		return err
	}
	// Bundles whose sessions had to be left out are not marked as received, so that they are
	// taken in once the project saves text.
	if total.Sessions == 0 || takesSessions(proj) {
		known.Pulled = append(known.Pulled, keys...)
	}
	for _, k := range all {
		if team.SentBy(k, st.settings.DeviceID) {
			known.Pulled = append(known.Pulled, k)
		}
	}
	known.Pulled = uniqueSorted(known.Pulled)
	if err := team.SaveState(st.paths, proj.ID, known); err != nil {
		return fmt.Errorf("everything was stored, but the record of what was received could not be saved (the next pull will look at it again, which is harmless): %w", err)
	}
	out["received"], out["stored"] = true, res
	b.WriteString(receivedLine(res))
	return inv.emit(out, b.String())
}

func uniqueSorted(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for i, v := range in {
		if i == 0 || v != in[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func teamStatus(inv invocation) error {
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	f, _, _, known, err := teamNew(inv, st, proj, conn, true)
	if err != nil {
		return err
	}
	files, _, all, unreadable, err := teamWaiting(st, proj, conn, sec, known)
	if err != nil {
		return err
	}
	c := f.Count()
	return inv.emit(map[string]any{"connected": true, "project_id": proj.ID, "name": conn.Name, "endpoint": conn.Endpoint, "bucket": conn.Bucket, "as": conn.As,
		"keys_protected_by_os": keep.Protected, "to_send": c, "bundles_to_receive": len(files), "bundles_in_bucket": len(all), "unreadable_bundles": unreadable},
		fmt.Sprintf("project %q is connected to bucket %s at %s as %q (connected %.10s).\nTo send: %d instruction(s), %d checkpoint(s), %d piece(s) of text in %d session(s).\nTo receive: %d bundle(s) from others; the bucket holds %d for this project in all.\n",
			proj.Name, conn.Bucket, conn.Endpoint, conn.Name, conn.ConnectedAt, c.Instructions, c.Checkpoints, c.Turns, c.Sessions, len(files), len(all)))
}

func teamDisconnect(inv invocation) error {
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, _, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	const stays = "What is already in the bucket stays there (delete it in the storage service if you want it gone), and so does everything already received on this and other computers.\n"
	if inv.flags["yes"] == "" {
		inv.emit(map[string]any{"disconnected": false, "project_id": proj.ID},
			fmt.Sprintf("NOT DISCONNECTED YET. This would remove the stored keys, the passphrase and the record of what was sent and received for project %q (bucket %s).\n%sRun the same command with --yes.\n", proj.Name, conn.Bucket, stays))
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was changed; add --yes to confirm"}
	}
	if err := team.Remove(st.paths, proj.ID); err != nil {
		return err
	}
	return inv.emit(map[string]any{"disconnected": true, "project_id": proj.ID}, fmt.Sprintf("project %q is no longer connected. %s", proj.Name, stays))
}
