package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"sessionkeep/internal/fsx"
)

// Capture levels a project can have (PRODUCT_SPEC §4).
const (
	LevelOff          = "off"
	LevelMetadata     = "metadata"
	LevelConversation = "conversation"
	LevelExtended     = "extended"
)

// Auto-add modes: what happens in a folder SessionKeep has never seen (DECISIONS D29).
const (
	ModeManual = "manual" // nothing is saved; the user runs `project add`
	ModeAsk    = "ask"    // nothing is saved; the user gets a one-line reminder
	ModeAuto   = "auto"   // the folder is registered at AutoAdd.Level; only ever set by the owner
)

// ValidLevel reports whether s is one of the four capture levels.
func ValidLevel(s string) bool {
	return s == LevelOff || s == LevelMetadata || s == LevelConversation || s == LevelExtended
}

const settingsSchema = 1

// Settings is the user's configuration file. It holds no secrets.
type Settings struct {
	SchemaVersion int     `json:"schema_version"`
	DeviceID      string  `json:"device_id"`
	AutoAdd       AutoAdd `json:"auto_add"`
	// Exclusions are normalized absolute paths. Nothing at or below them is ever captured,
	// registered automatically, or mentioned in a reminder.
	Exclusions []string `json:"exclusions"`
}

// AutoAdd is the behaviour for unregistered folders.
type AutoAdd struct {
	Mode  string `json:"mode"`
	Level string `json:"level,omitempty"` // required when Mode is auto; never "off"
}

// DefaultSettings is what a fresh install uses: remind, never record by assumption.
func DefaultSettings() Settings {
	return Settings{SchemaVersion: settingsSchema, AutoAdd: AutoAdd{Mode: ModeAsk}, Exclusions: []string{}}
}

// Validate rejects combinations that would capture more than the owner asked for.
func (s Settings) Validate() error {
	if s.SchemaVersion != settingsSchema {
		return fmt.Errorf("settings schema_version %d is not supported by this version (want %d)", s.SchemaVersion, settingsSchema)
	}
	switch s.AutoAdd.Mode {
	case ModeManual, ModeAsk:
		if s.AutoAdd.Level != "" {
			return fmt.Errorf("auto_add.level must be empty when mode is %q", s.AutoAdd.Mode)
		}
	case ModeAuto:
		if !ValidLevel(s.AutoAdd.Level) || s.AutoAdd.Level == LevelOff {
			return fmt.Errorf("auto_add.level must be metadata, conversation or extended when mode is auto (got %q)", s.AutoAdd.Level)
		}
	default:
		return fmt.Errorf("auto_add.mode must be manual, ask or auto (got %q)", s.AutoAdd.Mode)
	}
	for _, e := range s.Exclusions {
		if strings.TrimSpace(e) == "" {
			return errors.New("exclusions must not contain an empty path")
		}
	}
	return nil
}

// SettingsPath is where the settings file lives.
func (p Paths) SettingsPath() string { return p.DataDir + sepOf(p.DataDir) + "config.json" }

// RootsCachePath is the hook-readable list of registered roots, rewritten after every project change.
func (p Paths) RootsCachePath() string { return p.CacheDir + sepOf(p.DataDir) + "roots.json" }

func sepOf(dir string) string {
	if strings.Contains(dir, `\`) {
		return `\`
	}
	return "/"
}

// LoadSettings reads the settings file. A missing file means defaults. A file that cannot be
// parsed or validated is an error: callers on the capture path must then capture nothing,
// because a damaged file might have held exclusions.
func LoadSettings(p Paths) (Settings, error) {
	raw, err := os.ReadFile(p.SettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("settings file is not valid JSON: %w", err)
	}
	if s.Exclusions == nil {
		s.Exclusions = []string{}
	}
	if err := s.Validate(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveSettings validates and atomically replaces the settings file.
func SaveSettings(p Paths, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(p.SettingsPath(), append(raw, '\n'))
}

// NewID returns a random UUID (version 4).
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("no system randomness: " + err.Error())
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
