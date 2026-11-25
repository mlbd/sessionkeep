package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const teamUsage = `usage: sessionkeep team storage [--storage personal|shared] --endpoint URL --bucket BUCKET [--region REGION] [--from YOUR-NAME]
           (saves a bucket once: "personal" is your own and the default, "shared" one you share with others;
            keys from SESSIONKEEP_TEAM_ACCESS_KEY, SESSIONKEEP_TEAM_SECRET_KEY)
       sessionkeep team connect --name SHARED-NAME [--storage personal|shared | --endpoint URL --bucket BUCKET] [--region REGION] [--from YOUR-NAME] [--project ID_OR_PATH]
           (uses the personal storage unless --storage shared or a bucket of its own (--endpoint, --bucket) is given;
            the passphrase is read from SESSIONKEEP_TEAM_PASSPHRASE. Keys and passphrases are never taken from the command line)
       sessionkeep team list
       sessionkeep team status [--project ID_OR_PATH]
       sessionkeep team sync [--project ID_OR_PATH] [--yes]      receive, then send
       sessionkeep team push [--project ID_OR_PATH] [--no-sessions] [--yes]
       sessionkeep team pull [--project ID_OR_PATH] [--yes]
       sessionkeep team disconnect [--project ID_OR_PATH] [--yes]
       sessionkeep team passphrase [--project ID_OR_PATH] [--yes]   show the passphrase stored here for a shared project
       sessionkeep team storage-forget [--storage personal|shared] [--yes]`

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
		if err := inv.only("name", "storage", "endpoint", "bucket", "region", "from", "project", "secrets-stdin"); err != nil {
			return err
		}
		return teamConnect(inv)
	case "storage":
		if err := inv.only("storage", "endpoint", "bucket", "region", "from", "secrets-stdin"); err != nil {
			return err
		}
		return teamStorage(inv)
	case "storage-forget":
		if err := inv.only("storage", "yes"); err != nil {
			return err
		}
		return teamStorageForget(inv)
	case "list":
		if err := inv.only(); err != nil {
			return err
		}
		return teamList(inv)
	case "sync":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
		return teamSync(inv)
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
	case "passphrase":
		if err := inv.only("project", "yes"); err != nil {
			return err
		}
		return teamPassphrase(inv)
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
		return proj, conn, sec, failErr("not_connected", "project %q is not shared through the cloud; turn sharing on for it first (dashboard, Projects page, or `sessionkeep team connect`)", proj.Name)
	}
	return proj, conn, sec, nil
}

// teamSecrets reads the keys and the passphrase: from the environment, or, with --secrets-stdin,
// as one JSON object on standard input (how the dashboard hands them over without putting them
// on a command line).
func teamSecrets(inv invocation) (team.Secrets, error) {
	var sec team.Secrets
	if inv.flags["secrets-stdin"] != "" {
		raw, err := io.ReadAll(io.LimitReader(inv.stdin, 1<<16))
		if err != nil || json.Unmarshal(raw, &sec) != nil {
			return sec, usageErr("--secrets-stdin expects one JSON object with access_key, secret_key and passphrase")
		}
	} else {
		sec = team.Secrets{AccessKey: inv.getenv(envTeamAccess), SecretKey: inv.getenv(envTeamSecret), Passphrase: inv.getenv(envTeamPassphrase)}
	}
	sec.AccessKey, sec.SecretKey = strings.TrimSpace(sec.AccessKey), strings.TrimSpace(sec.SecretKey)
	return sec, nil
}

// teamPlace checks where the bucket is, as given on the command line.
func teamPlace(inv invocation) (endpoint, bucket, region string, err error) {
	bucket, region = inv.flags["bucket"], inv.flags["region"]
	if !bucketForm.MatchString(bucket) {
		return "", "", "", usageErr("%q is not a bucket name (lower-case letters, digits, dots and dashes)", bucket)
	}
	if endpoint, err = s3lite.CheckEndpoint(inv.flags["endpoint"]); err != nil {
		return "", "", "", usageErr("%v", err)
	}
	if region == "" {
		region = "auto"
	}
	if len(inv.flags["from"]) > 100 {
		return "", "", "", usageErr("--from is a short name, at most 100 characters")
	}
	return endpoint, bucket, region, nil
}

// teamSlot reads --storage: personal (the default) or shared.
func teamSlot(inv invocation) (string, error) {
	slot := inv.flags["storage"]
	if slot == "" {
		return team.Personal, nil
	}
	if err := team.CheckSlot(slot); err != nil {
		return "", usageErr("%v", err)
	}
	return slot, nil
}

func keysGuard() string {
	if keep.Protected {
		return "They are stored protected for your Windows user on this computer."
	}
	return "They are stored in a file only you can read; this system has no stronger protection built in yet."
}

// teamStorage saves a bucket and its keys once, as the personal or the shared storage of this
// computer. It reads the bucket's listing to prove the keys work; nothing is written there.
func teamStorage(inv invocation) error {
	if inv.flags["bucket"] == "" || inv.flags["endpoint"] == "" {
		return usageErr("%s", teamUsage)
	}
	slot, err := teamSlot(inv)
	if err != nil {
		return err
	}
	endpoint, bucket, region, err := teamPlace(inv)
	if err != nil {
		return err
	}
	sec, err := teamSecrets(inv)
	if err != nil {
		return err
	}
	if sec.AccessKey == "" || sec.SecretKey == "" {
		return &cmdError{ExitNeedsConsent, "keys_required", fmt.Sprintf("the storage keys are needed: set %s and %s in the environment for this one command; they are stored protected afterwards and are never taken from the command line", envTeamAccess, envTeamSecret)}
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	s := team.Storage{Endpoint: endpoint, Bucket: bucket, Region: region, As: strings.TrimSpace(inv.flags["from"]), SavedAt: time.Now().UTC().Format(time.RFC3339)}
	ctx, cancel := context.WithTimeout(context.Background(), teamTimeout)
	defer cancel()
	if _, err := teamClient(team.Connection{Endpoint: endpoint, Bucket: bucket, Region: region}, sec).List(ctx, "sessionkeep/v1/"); err != nil {
		return teamErr(err)
	}
	if err := team.PutStorage(st.paths, slot, s, sec); err != nil {
		return err
	}
	how := "`sessionkeep team connect --name SHARED-NAME`"
	if slot == team.Shared {
		how = "`sessionkeep team connect --name SHARED-NAME --storage shared`"
	}
	return inv.emit(map[string]any{"saved": true, "storage": slot, "endpoint": endpoint, "bucket": bucket, "as": s.As, "keys_protected_by_os": keep.Protected},
		fmt.Sprintf("The bucket %s answered with these keys, and it is saved as your %s storage. %s\nNothing was sent. Share a project through it with %s.\n", bucket, slot, keysGuard(), how))
}

func teamStorageForget(inv invocation) error {
	slot, err := teamSlot(inv)
	if err != nil {
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	const stays = "Projects already shared keep working with their own copy of the keys until you stop sharing them. Nothing in the bucket is touched.\n"
	if inv.flags["yes"] == "" {
		inv.emit(map[string]any{"forgotten": false, "storage": slot}, fmt.Sprintf("NOT FORGOTTEN YET. This would remove the %s storage and its keys from this computer.\n", slot)+stays+"Run the same command with --yes.\n")
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was changed; add --yes to confirm"}
	}
	if err := team.RemoveStorage(st.paths, slot); err != nil {
		return err
	}
	return inv.emit(map[string]any{"forgotten": true, "storage": slot}, fmt.Sprintf("The %s storage is forgotten. ", slot)+stays)
}

// teamList says, without contacting any storage, which storages are saved and which projects are
// shared, and through which storage. It never shows a key or a passphrase.
func teamList(inv invocation) error {
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	conns, err := team.Connections(st.paths)
	if err != nil {
		return failErr("team_settings", "%v", err)
	}
	saved, err := team.Storages(st.paths)
	if err != nil {
		return failErr("team_settings", "%v", err)
	}
	var b strings.Builder
	storages := map[string]any{}
	// Projects are counted against the storages in use, so the personal storage taken from a
	// project connected earlier counts that project as its own rather than as a separate bucket.
	inUse := map[string]team.Storage{}
	for slot, s := range saved {
		inUse[slot] = s
	}
	for _, slot := range []string{team.Personal, team.Shared} {
		s, _, ok, isSaved, keyErr := team.GetStorage(st.paths, slot)
		if ok {
			inUse[slot] = s
		}
		storages[slot] = map[string]any{"set": ok, "saved": isSaved, "endpoint": s.Endpoint, "bucket": s.Bucket, "as": s.As, "keys_readable": keyErr == nil}
		switch {
		case !ok:
			fmt.Fprintf(&b, "%-9s storage: not set up.\n", slot)
		case isSaved:
			fmt.Fprintf(&b, "%-9s storage: bucket %s at %s.\n", slot, s.Bucket, s.Endpoint)
		default:
			fmt.Fprintf(&b, "%-9s storage: bucket %s at %s (taken from a project connected earlier).\n", slot, s.Bucket, s.Endpoint)
		}
	}
	shared := map[string]any{}
	ids := make([]string, 0, len(conns))
	for id := range conns {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := conns[id]
		slot := team.SlotOf(inUse, c)
		shared[id] = map[string]any{"name": c.Name, "storage": slot, "bucket": c.Bucket, "endpoint": c.Endpoint, "as": c.As, "connected_at": c.ConnectedAt}
		fmt.Fprintf(&b, "  project %s is shared as %q in bucket %s (%s storage, since %.10s)\n", id, c.Name, c.Bucket, slot, c.ConnectedAt)
	}
	if len(ids) == 0 {
		b.WriteString("No project is shared. Projects stay on this computer until you connect them.\n")
	}
	// "storage" is the personal storage, as answered before there were two.
	return inv.emit(map[string]any{"storage": storages[team.Personal], "storages": storages, "projects": shared, "keys_protected_by_os": keep.Protected, "min_passphrase": team.MinPassphrase}, b.String())
}

func teamConnect(inv invocation) error {
	name := inv.flags["name"]
	if name == "" || (inv.flags["bucket"] == "") != (inv.flags["endpoint"] == "") {
		return usageErr("%s", teamUsage)
	}
	if inv.flags["storage"] != "" && inv.flags["endpoint"] != "" {
		return usageErr("give either --storage or a bucket of its own (--endpoint and --bucket), not both")
	}
	if err := team.CheckName(name); err != nil {
		return usageErr("%v", err)
	}
	slot, err := teamSlot(inv)
	if err != nil {
		return err
	}
	sec, err := teamSecrets(inv)
	if err != nil {
		return err
	}
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	var endpoint, bucket, region string
	as := strings.TrimSpace(inv.flags["from"])
	if inv.flags["endpoint"] != "" {
		// A bucket of the project's own, kept only in its connection.
		slot = team.Custom
		if endpoint, bucket, region, err = teamPlace(inv); err != nil {
			return err
		}
		if sec.AccessKey == "" || sec.SecretKey == "" {
			return &cmdError{ExitNeedsConsent, "keys_required", fmt.Sprintf("the storage keys for this bucket are needed: set %s, %s and %s in the environment for this one command; they are stored protected afterwards and are never taken from the command line", envTeamAccess, envTeamSecret, envTeamPassphrase)}
		}
		if as == "" {
			if s, _, ok, _, _ := team.GetStorage(st.paths, team.Personal); ok {
				as = s.As
			}
		}
	} else {
		// A storage saved once on this computer, with its keys.
		s, keys, ok, _, err := team.GetStorage(st.paths, slot)
		if err != nil {
			return failErr("team_settings", "%v", err)
		}
		if !ok {
			return failErr("storage_required", "no %s storage is saved on this computer yet; save it first (in the dashboard under Settings, or with `sessionkeep team storage --storage %s`), or give this project a bucket of its own", slot, slot)
		}
		if len(as) > 100 {
			return usageErr("--from is a short name, at most 100 characters")
		}
		endpoint, bucket, region = s.Endpoint, s.Bucket, s.Region
		sec.AccessKey, sec.SecretKey = keys.AccessKey, keys.SecretKey
		if as == "" {
			as = s.As
		}
	}
	if sec.Passphrase == "" {
		return &cmdError{ExitNeedsConsent, "keys_required", fmt.Sprintf("the passphrase for this shared project is needed: set %s in the environment for this one command; it is never taken from the command line", envTeamPassphrase)}
	}
	if len([]rune(sec.Passphrase)) < team.MinPassphrase {
		return usageErr("the passphrase must be at least %d characters; it is all that keeps the stored sessions unreadable", team.MinPassphrase)
	}
	var proj store.Project
	if err := withStore(st, func(ctx context.Context, s *store.Store) (err error) {
		proj, err = resolveProject(ctx, inv, st, s, inv.flags["project"])
		return err
	}); err != nil {
		return err
	}
	conn := team.Connection{ProjectID: proj.ID, Name: name, Endpoint: endpoint, Bucket: bucket, Region: region, As: as, ConnectedAt: time.Now().UTC().Format(time.RFC3339)}
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
	// A project that was shared somewhere else before starts over: what it sent there says
	// nothing about what this place holds.
	if old, _, was, _ := team.Get(st.paths, proj.ID); was && (old.Name != name || old.Endpoint != endpoint || old.Bucket != bucket) {
		if err := team.Remove(st.paths, proj.ID); err != nil {
			return err
		}
	}
	if err := team.Put(st.paths, conn, sec); err != nil {
		return err
	}
	return inv.emit(map[string]any{"connected": true, "project_id": proj.ID, "name": name, "storage": slot, "endpoint": endpoint, "bucket": bucket, "bundles_there": len(there), "keys_protected_by_os": keep.Protected},
		fmt.Sprintf("project %q is connected to bucket %s (%s storage) as %q; %d bundle(s) are there already.\nThe keys and the passphrase are kept on this computer. %s\nNothing was sent. Receive and send with `sessionkeep team sync`.\n", proj.Name, bucket, slot, name, len(there), keysGuard()))
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
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	out, text, pending, err := teamPushStep(inv, st, proj, conn, sec, inv.flags["no-sessions"] == "", inv.flags["yes"] != "")
	if err != nil {
		return err
	}
	if pending {
		inv.emit(out, text+"NOTHING SENT YET. Run the same command with --yes to send it.\n")
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was sent; add --yes to confirm"}
	}
	return inv.emit(out, text)
}

// teamPushStep sends what this computer has not sent, or, without apply, says what it would
// send. pending is true when there is something and it was not sent.
func teamPushStep(inv invocation, st state, proj store.Project, conn team.Connection, sec team.Secrets, withSessions, apply bool) (out map[string]any, text string, pending bool, err error) {
	f, scrub, secrets, state, err := teamNew(inv, st, proj, conn, withSessions)
	if err != nil {
		return nil, "", false, err
	}
	c := f.Count()
	out = map[string]any{"sent": false, "project_id": proj.ID, "bucket": conn.Bucket, "counts": c, "paths_shortened": scrub.Count, "secrets_removed": secrets}
	if c.Instructions+c.Checkpoints+c.Turns == 0 {
		return out, fmt.Sprintf("Nothing new to send for project %q: everything of yours is already in the bucket.\n", proj.Name), false, nil
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
		return out, b.String(), true, nil
	}
	sealed, err := team.SealBundle(f, sec.Passphrase)
	if err != nil {
		return nil, "", false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), teamTimeout)
	defer cancel()
	key := team.BundleKey(conn.Name, st.settings.DeviceID, time.Now(), strings.ReplaceAll(config.NewID(), "-", "")[:10])
	if err := teamClient(conn, sec).Put(ctx, key, sealed); err != nil {
		return nil, "", false, teamErr(err)
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
		return nil, "", false, fmt.Errorf("the bundle was sent, but the record of what was sent could not be saved (the next push will send it again, which is harmless): %w", err)
	}
	out["sent"], out["bytes"] = true, len(sealed)
	fmt.Fprintf(&b, "SENT: one encrypted bundle of %d bytes.\n", len(sealed))
	return out, b.String(), false, nil
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
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	out, text, pending, err := teamPullStep(st, proj, conn, sec, inv.flags["yes"] != "")
	if err != nil {
		return err
	}
	if pending {
		inv.emit(out, text+"NOTHING RECEIVED YET. Run the same command with --yes to take it in.\n")
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was received; add --yes to confirm"}
	}
	return inv.emit(out, text)
}

// teamSync receives what colleagues sent and then sends what is new here: the two halves of
// keeping a shared project in step, shown together and confirmed once.
func teamSync(inv invocation) error {
	apply := inv.flags["yes"] != ""
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	in, inText, inPending, err := teamPullStep(st, proj, conn, sec, apply)
	if err != nil {
		return err
	}
	send, sendText, sendPending, err := teamPushStep(inv, st, proj, conn, sec, true, apply)
	if err != nil {
		if apply && in["received"] == true {
			return fmt.Errorf("what colleagues sent was received, but sending failed: %w", err)
		}
		return err
	}
	out := map[string]any{"synced": apply || !(inPending || sendPending), "project_id": proj.ID, "name": conn.Name, "bucket": conn.Bucket, "receive": in, "send": send}
	if inPending || sendPending {
		inv.emit(out, inText+sendText+"NOTHING RECEIVED OR SENT YET. Run the same command with --yes to do both.\n")
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was received or sent; add --yes to confirm"}
	}
	return inv.emit(out, inText+sendText)
}

// teamPullStep takes in what colleagues sent, or, without apply, says what it would take in.
// pending is true when there is something and it was not taken in.
func teamPullStep(st state, proj store.Project, conn team.Connection, sec team.Secrets, apply bool) (out map[string]any, text string, pending bool, err error) {
	known, err := team.LoadState(st.paths, proj.ID)
	if err != nil {
		return nil, "", false, failErr("team_settings", "%v", err)
	}
	files, keys, all, unreadable, err := teamWaiting(st, proj, conn, sec, known)
	if err != nil {
		return nil, "", false, err
	}
	var total handoff.Counts
	var instructions []handoff.Instruction
	var checkpoints []handoff.Checkpoint
	senders := map[string]bool{}
	for _, f := range files {
		c := f.Count()
		total.Instructions += c.Instructions
		total.Checkpoints += c.Checkpoints
		total.Sessions += c.Sessions
		total.Turns += c.Turns
		instructions = append(instructions, f.Instructions...)
		checkpoints = append(checkpoints, f.Checkpoints...)
		senders[senderName(f)] = true
	}
	names := make([]string, 0, len(senders))
	for n := range senders {
		names = append(names, n)
	}
	sort.Strings(names)
	out = map[string]any{"received": false, "project_id": proj.ID, "bucket": conn.Bucket, "bundles": len(files), "counts": total, "from": names,
		"instructions": instructions, "checkpoints": checkpoints, "unreadable_bundles": unreadable, "sessions_taken": takesSessions(proj)}
	var b strings.Builder
	for _, u := range unreadable {
		fmt.Fprintf(&b, "Skipped: bundle %s is damaged or was altered, and nothing was taken from it.\n", u)
	}
	if len(files) == 0 {
		b.WriteString(fmt.Sprintf("Nothing new from the team for project %q.\n", proj.Name))
		return out, b.String(), false, nil
	}
	fmt.Fprintf(&b, "New from the team (%s), for your project %q: %d bundle(s) with\n  %d instruction(s), %d checkpoint(s), %d session(s) with %d piece(s) of text\n", strings.Join(names, ", "), proj.Name, len(files), total.Instructions, total.Checkpoints, total.Sessions, total.Turns)
	if total.Sessions > 0 && !takesSessions(proj) {
		b.WriteString("The sessions would be left out: this project is paused or does not save conversation text.\n")
	}
	b.WriteString(instructionsToRead(instructions))
	b.WriteString(checkpointsToRead(checkpoints))
	if !apply {
		return out, b.String(), true, nil
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
		return nil, "", false, err
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
		return nil, "", false, fmt.Errorf("everything was stored, but the record of what was received could not be saved (the next pull will look at it again, which is harmless): %w", err)
	}
	out["received"], out["stored"] = true, res
	b.WriteString(receivedLine(res))
	return out, b.String(), false, nil
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

// teamPassphrase shows the passphrase stored for a shared project, so that someone who forgot it
// can read it back on a computer that is still connected (to connect another computer, or to
// give it to a colleague). Nothing is shown without --yes (D131).
func teamPassphrase(inv invocation) error {
	st, err := load(inv, true)
	if err != nil {
		return err
	}
	proj, conn, sec, err := teamProject(inv, st)
	if err != nil {
		return err
	}
	if inv.flags["yes"] == "" {
		inv.emit(map[string]any{"shown": false, "project_id": proj.ID, "name": conn.Name},
			fmt.Sprintf("NOT SHOWN YET. This would show the passphrase of project %q (shared as %q) on the screen.\nAnyone who sees it and can reach the bucket can read what is sent for this project.\nRun the same command with --yes.\n", proj.Name, conn.Name))
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was shown; add --yes to confirm"}
	}
	return inv.emit(map[string]any{"shown": true, "project_id": proj.ID, "name": conn.Name, "passphrase": sec.Passphrase},
		fmt.Sprintf("The passphrase of project %q (shared as %q):\n%s\n", proj.Name, conn.Name, sec.Passphrase))
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
			fmt.Sprintf("NOT DISCONNECTED YET. This would remove the passphrase, this project's copy of the storage keys and the record of what was sent and received for project %q (bucket %s).\n%sRun the same command with --yes.\n", proj.Name, conn.Bucket, stays))
		return &cmdError{ExitNeedsConsent, "confirmation_required", "nothing was changed; add --yes to confirm"}
	}
	if err := team.Remove(st.paths, proj.ID); err != nil {
		return err
	}
	return inv.emit(map[string]any{"disconnected": true, "project_id": proj.ID}, fmt.Sprintf("project %q is no longer connected. %s", proj.Name, stays))
}
