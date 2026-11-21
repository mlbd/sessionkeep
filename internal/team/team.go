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

type configFile struct {
	Version     int                   `json:"version"`
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
const MinPassphrase = 12

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
	c := configFile{Version: 1, Connections: map[string]Connection{}}
	raw, err := fsx.ReadFile(configPath(p))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.Version != 1 {
		return c, errors.New("the team settings file is damaged; connect the project again")
	}
	if c.Connections == nil {
		c.Connections = map[string]Connection{}
	}
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
	sealed, err := base64.StdEncoding.DecodeString(conn.Sealed)
	if err != nil {
		return conn, Secrets{}, true, errors.New("the stored keys are damaged; connect the project again")
	}
	plain, err := keep.Open(sealed)
	var sec Secrets
	if err != nil || json.Unmarshal(plain, &sec) != nil {
		return conn, Secrets{}, true, errors.New("the stored keys cannot be opened by this user on this computer; connect the project again")
	}
	return conn, sec, true, nil
}

// Put saves a project's connection with its secrets sealed.
func Put(p config.Paths, conn Connection, sec Secrets) error {
	plain, err := json.Marshal(sec)
	if err != nil {
		return err
	}
	sealed, err := keep.Seal(plain)
	if err != nil {
		return fmt.Errorf("the keys could not be protected for storing: %w", err)
	}
	conn.Sealed = base64.StdEncoding.EncodeToString(sealed)
	c, err := load(p)
	if err != nil {
		return err
	}
	c.Connections[conn.ProjectID] = conn
	raw, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(configPath(p), append(raw, '\n'))
}

// Remove forgets a project's connection and what was sent and received. The bucket is untouched.
func Remove(p config.Paths, projectID string) error {
	c, err := load(p)
	if err != nil {
		return err
	}
	delete(c.Connections, projectID)
	raw, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	for _, f := range []string{statePath(p, projectID), statePath(p, projectID) + fsx.AltSuffix} {
		os.Remove(f)
	}
	return fsx.WriteAtomic(configPath(p), append(raw, '\n'))
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
