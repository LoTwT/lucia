// Package settings persists the small set of user preferences Lucia keeps
// between launches. A missing file means the defaults; a damaged file is
// reported and the defaults apply, so preferences can never keep the app
// from starting.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File is the settings file's name in the data directory.
const File = "settings.json"

// Settings is the persisted preference set.
type Settings struct {
	// CheckUpdatesOnLaunch runs an update check when the app starts and
	// keeps checking while it runs. Turning it off leaves the manual
	// "检查更新" action available.
	CheckUpdatesOnLaunch bool `json:"checkUpdatesOnLaunch"`
	// PetVisible keeps the desktop pet on screen across launches. Hiding
	// it leaves the tray and console available.
	PetVisible bool `json:"petVisible"`
	// SoundEnabled plays a short generated sound on interactions. It is
	// off by default so a new install does not surprise the user.
	SoundEnabled bool `json:"soundEnabled"`
	// BubbleCooldownSeconds limits how often a speech bubble appears:
	// 0 every interaction, a positive number the minimum gap, -1 off.
	BubbleCooldownSeconds int `json:"bubbleCooldownSeconds"`
	// PetScale scales the pet window and sprite together. Invalid values are
	// clamped by the pet model; the default is 1.0.
	PetScale float64 `json:"petScale"`
	// PetPack selects an imported artwork pack below the data directory's
	// packs folder. An empty value uses the built-in whale.
	PetPack string `json:"petPack,omitempty"`
	// PetIntroPlayed records that the animated pack already greeted the user
	// once. It keeps the greeting from replaying on every launch.
	PetIntroPlayed bool `json:"petIntroPlayed,omitempty"`
}

// Default returns the preferences of a first launch.
func Default() Settings {
	return Settings{
		CheckUpdatesOnLaunch: true,
		PetVisible:           true,
		PetScale:             1.0,
	}
}

// Load reads the settings in dir. A missing file returns the defaults and
// no error; an unreadable or invalid file returns the defaults and the
// reason, which the caller logs while continuing with the defaults.
func Load(dir string) (Settings, error) {
	path := filepath.Join(dir, File)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("read %q: %w", path, err)
	}
	// Start from defaults so fields added after an older file was written
	// keep their intended default instead of decoding as the zero value.
	s := Default()
	if err := json.Unmarshal(data, &s); err != nil {
		return Default(), fmt.Errorf("parse %q: %w", path, err)
	}
	if s.PetScale <= 0 {
		s.PetScale = 1.0
	}
	return s, nil
}

// Save writes the settings atomically, so a crash mid-write leaves either
// the old file or the new one, never a partial one.
func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the settings directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the settings: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, File)
	tmp, err := os.CreateTemp(dir, File+".tmp-*")
	if err != nil {
		return fmt.Errorf("create a temporary settings file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %q: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("set the permissions of %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}
