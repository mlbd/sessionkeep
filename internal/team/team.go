// Package team keeps a project's memory in a storage bucket the owner chose, so that colleagues
// who connect the same project can send what they have and get what the others sent.
//
// What is sent is a bundle: a handoff file (the same content and the same checks as a file
// passed by hand), compressed and then encrypted on this computer with a passphrase the team
// shares, using the age format. The storage service holds only bytes it cannot read; what it can
// see is how many bundles there are, how large, when each was sent and from which computer id.
// A bundle is written once and never replaced, so two people sending at once cannot overwrite
// each other, and receiving one twice adds nothing.
package team

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"filippo.io/age"

	"sessionkeep/internal/config"
	"sessionkeep/internal/fsx"
	"sessionkeep/internal/handoff"
	"sessionkeep/internal/keep"
)

// Connection is one project's link to a bucket. The secrets are kept sealed (see package keep).
type Connection struct {
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"` // the name the team gave the shared project
	Endpoint    string `json:"endpoint"`
	Bucket      string `json:"bucket"`
	Region      string `json:"region"`
	As          string `json:"as,omitempty"` // how this person is named to the others
	Sealed      string `json:"sealed"`
	ConnectedAt string `json:"connected_at"`
}

// Secrets are the storage key pair and the team's passphrase.
type Secrets struct {
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	Passphrase string `json:"passphrase"`
}

// Storage is a bucket saved once on this computer, so that sharing a project through it needs
// only a shared name and a passphrase. Its keys are kept sealed. There are two: "personal" (the
// owner's own bucket, used unless another is chosen) and "shared" (one the owner shares with
// others, such as a company's). A project may also use a bucket of its own (custom), which is
// kept only in its connection.
type Storage struct {
	Endpoint string `json:"endpoint"`
	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	As       string `json:"as,omitempty"`
	Sealed   string `json:"sealed"`
	SavedAt  string `json:"saved_at,omitempty"`
}

// The names of the saved storages.
const (
	Personal = "personal"
	Shared   = "shared"
	// Custom describes a project that uses a bucket of its own; nothing is saved under it.
	Custom = "custom"
)

// CheckSlot says whether name is one of the saved storages.
func CheckSlot(name string) error {
	if name != Personal && name != Shared {
		return fmt.Errorf("the storage is %q or %q, not %q", Personal, Shared, name)
	}
	return nil
}

type configFile struct {
	Version  int                `json:"version"`
	Storages map[string]Storage `json:"storages,omitempty"`
	// Storage is the single storage of version 1 files; it is read as the personal one.
	Storage     *Storage              `json:"storage,omitempty"`
	Connections map[string]Connection `json:"connections"`
}

// State is what this computer has already sent and received for one project.
type State struct {
	Version int      `json:"version"`
	Pushed  []string `json:"pushed"` // ids of instructions, checkpoints and pieces of text sent
	Pulled  []string `json:"pulled"` // names of bundles received or known to be this computer's own
}

// MinPassphrase is the shortest passphrase accepted. The passphrase is all that stands between
// the stored bundles and anyone who can read the bucket.
const MinPassphrase = 8

var nameForm = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

// CheckName says whether a shared project name is usable: lower-case letters, digits and dashes.
func CheckName(name string) error {
	if !nameForm.MatchString(name) {
		return errors.New("the shared name must be 2 to 63 lower-case letters, digits or dashes, such as shop-app")
	}
	return nil
}

func configPath(p config.Paths) string { return filepath.Join(p.DataDir, "team.json") }

func statePath(p config.Paths, projectID string) string {
	sum := sha256.Sum256([]byte(projectID))
	return filepath.Join(p.DataDir, "team", hex.EncodeToString(sum[:12])+".json")
}

func load(p config.Paths) (configFile, error) {
	c := configFile{Version: 2, Storages: map[string]Storage{}, Connections: map[string]Connection{}}
	raw, err := fsx.ReadFile(configPath(p))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil || (c.Version != 1 && c.Version != 2) {
		return c, errors.New("the team settings file is damaged; connect the project again")
	}
	if c.Connections == nil {
		c.Connections = map[string]Connection{}
	}
	if c.Storages == nil {
		c.Storages = map[string]Storage{}
	}
	// Version 1 had one storage; it becomes the personal one.
	if c.Storage != nil {
		if _, taken := c.Storages[Personal]; !taken {
			c.Storages[Personal] = *c.Storage
		}
		c.Storage = nil
	}
	c.Version = 2
	return c, nil
}

// Get returns a project's connection and its opened secrets. ok is false when there is none.
func Get(p config.Paths, projectID string) (Connection, Secrets, bool, error) {
	c, err := load(p)
	if err != nil {
		return Connection{}, Secrets{}, false, err
	}
	conn, ok := c.Connections[projectID]
	if !ok {
		return Connection{}, Secrets{}, false, nil
	}
	sec, err := unseal(conn.Sealed)
	if err != nil {
		return conn, Secrets{}, true, err
	}
	return conn, sec, true, nil
}

func unseal(text string) (Secrets, error) {
	sealed, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return Secrets{}, errors.New("the stored keys are damaged; connect the project again")
	}
	plain, err := keep.Open(sealed)
	var sec Secrets
	if err != nil || json.Unmarshal(plain, &sec) != nil {
		return Secrets{}, errors.New("the stored keys cannot be opened by this user on this computer; connect the project again")
	}
	return sec, nil
}

func seal(sec Secrets) (string, error) {
	plain, err := json.Marshal(sec)
	if err != nil {
		return "", err
	}
	sealed, err := keep.Seal(plain)
	if err != nil {
		return "", fmt.Errorf("the keys could not be protected for storing: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func save(p config.Paths, c configFile) error {
	raw, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(configPath(p), append(raw, '\n'))
}

// Connections returns every project's connection, by project id. No secret is opened.
func Connections(p config.Paths) (map[string]Connection, error) {
	c, err := load(p)
	return c.Connections, err
}

// GetStorage returns a saved storage and its keys. When no personal storage was saved but a
// project is connected, the storage of the most recently connected project stands in for it, so
// that someone who connected one project in a terminal is not asked for the same keys again.
// saved says which. The shared storage has no stand-in.
func GetStorage(p config.Paths, slot string) (s Storage, keys Secrets, ok, saved bool, err error) {
	if err := CheckSlot(slot); err != nil {
		return s, keys, false, false, err
	}
	c, err := load(p)
	if err != nil {
		return s, keys, false, false, err
	}
	if got, found := c.Storages[slot]; found {
		keys, err = unseal(got.Sealed)
		return got, keys, true, true, err
	}
	if slot != Personal {
		return s, keys, false, false, nil
	}
	var latest *Connection
	for id := range c.Connections {
		conn := c.Connections[id]
		if latest == nil || conn.ConnectedAt > latest.ConnectedAt {
			latest = &conn
		}
	}
	if latest == nil {
		return s, keys, false, false, nil
	}
	keys, err = unseal(latest.Sealed)
	keys.Passphrase = ""
	return Storage{Endpoint: latest.Endpoint, Bucket: latest.Bucket, Region: latest.Region, As: latest.As}, keys, true, false, err
}

// SlotOf says which saved storage a connection goes to: Personal or Shared when its endpoint and
// bucket are those of a saved storage (personal first), otherwise Custom.
func SlotOf(storages map[string]Storage, conn Connection) string {
	for _, slot := range []string{Personal, Shared} {
		if s, ok := storages[slot]; ok && s.Endpoint == conn.Endpoint && s.Bucket == conn.Bucket {
			return slot
		}
	}
	return Custom
}

// Storages returns the saved storages by name. No key is opened.
func Storages(p config.Paths) (map[string]Storage, error) {
	c, err := load(p)
	return c.Storages, err
}

// PutStorage saves a storage with its keys sealed. Projects already connected to the same
// endpoint and bucket get the new keys too, so replacing a key is done in one place.
func PutStorage(p config.Paths, slot string, s Storage, keys Secrets) error {
	if err := CheckSlot(slot); err != nil {
		return err
	}
	keys.Passphrase = ""
	sealed, err := seal(keys)
	if err != nil {
		return err
	}
	s.Sealed = sealed
	c, err := load(p)
	if err != nil {
		return err
	}
	c.Storages[slot] = s
	for id, conn := range c.Connections {
		if conn.Endpoint != s.Endpoint || conn.Bucket != s.Bucket {
			continue
		}
		sec, err := unseal(conn.Sealed)
		if err != nil {
			continue
		}
		sec.AccessKey, sec.SecretKey = keys.AccessKey, keys.SecretKey
		if conn.Sealed, err = seal(sec); err != nil {
			return err
		}
		c.Connections[id] = conn
	}
	return save(p, c)
}

// RemoveStorage forgets a saved storage. Connected projects keep their own copy of the keys.
func RemoveStorage(p config.Paths, slot string) error {
	if err := CheckSlot(slot); err != nil {
		return err
	}
	c, err := load(p)
	if err != nil {
		return err
	}
	delete(c.Storages, slot)
	return save(p, c)
}

// Put saves a project's connection with its secrets sealed.
func Put(p config.Paths, conn Connection, sec Secrets) error {
	sealed, err := seal(sec)
	if err != nil {
		return err
	}
	conn.Sealed = sealed
	c, err := load(p)
	if err != nil {
		return err
	}
	c.Connections[conn.ProjectID] = conn
	return save(p, c)
}

// Remove forgets a project's connection and what was sent and received. The bucket is untouched.
func Remove(p config.Paths, projectID string) error {
	c, err := load(p)
	if err != nil {
		return err
	}
	delete(c.Connections, projectID)
	for _, f := range []string{statePath(p, projectID), statePath(p, projectID) + fsx.AltSuffix} {
		os.Remove(f)
	}
	return save(p, c)
}

// LoadState reads what was sent and received; a missing file is an empty state.
func LoadState(p config.Paths, projectID string) (State, error) {
	s := State{Version: 1}
	raw, err := fsx.ReadFile(statePath(p, projectID))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil || s.Version != 1 {
		return State{Version: 1}, errors.New("the record of what was sent and received is damaged")
	}
	return s, nil
}

// SaveState writes what was sent and received.
func SaveState(p config.Paths, projectID string, s State) error {
	s.Version = 1
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statePath(p, projectID)), 0o700); err != nil {
		return err
	}
	return fsx.WriteAtomic(statePath(p, projectID), append(raw, '\n'))
}

// Prefix is where a shared project's bundles live in the bucket. The name is hashed so that the
// listing does not show it.
func Prefix(name string) string {
	sum := sha256.Sum256([]byte("sessionkeep-team:" + name))
	return "sessionkeep/v1/" + hex.EncodeToString(sum[:12]) + "/b/"
}

// BundleKey names a new bundle so that names sort by time and say which computer sent them.
func BundleKey(name, deviceID string, at time.Time, random string) string {
	return Prefix(name) + at.UTC().Format("20060102T150405Z") + "-" + DeviceTag(deviceID) + "-" + random + ".age"
}

// DeviceTag is the short form of a computer's id used in bundle names.
func DeviceTag(deviceID string) string {
	sum := sha256.Sum256([]byte(deviceID))
	return hex.EncodeToString(sum[:5])
}

// SentBy reports whether a bundle name says it was sent from the computer with deviceID.
func SentBy(key, deviceID string) bool {
	return strings.Contains(key[strings.LastIndex(key, "/")+1:], "-"+DeviceTag(deviceID)+"-")
}

// MaxBundleBytes bounds a stored bundle.
const MaxBundleBytes = 256 << 20

// ErrPassphrase means a bundle could not be opened with the passphrase.
var ErrPassphrase = errors.New("the passphrase does not open this bundle")

// SealBundle compresses f and encrypts it with the passphrase.
func SealBundle(f handoff.File, passphrase string) ([]byte, error) {
	f.Kind, f.Format = handoff.Kind, handoff.Format
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	rec, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc, err := age.Encrypt(&out, rec)
	if err != nil {
		return nil, err
	}
	zw := gzip.NewWriter(enc)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// OpenBundle decrypts and checks a bundle. What comes out has passed the same checks as a
// handoff file read from disk.
func OpenBundle(sealed []byte, passphrase string) (handoff.File, error) {
	id, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return handoff.File{}, err
	}
	plain, err := age.Decrypt(bytes.NewReader(sealed), id)
	if err != nil {
		var none *age.NoIdentityMatchError
		if errors.As(err, &none) || errors.Is(err, age.ErrIncorrectIdentity) {
			return handoff.File{}, ErrPassphrase
		}
		return handoff.File{}, fmt.Errorf("%w: it is not an encrypted bundle (%v)", handoff.ErrInvalid, err)
	}
	zr, err := gzip.NewReader(plain)
	if err != nil {
		return handoff.File{}, fmt.Errorf("%w: it was altered or is not a bundle (%v)", handoff.ErrInvalid, err)
	}
	raw, err := io.ReadAll(io.LimitReader(zr, handoff.MaxFileBytes+1))
	if err != nil {
		return handoff.File{}, fmt.Errorf("%w: it was altered or cut short (%v)", handoff.ErrInvalid, err)
	}
	return handoff.Parse(raw)
}
