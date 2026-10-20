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
}

// Default returns the preferences of a first launch.
func Default() Settings {
	return Settings{CheckUpdatesOnLaunch: true}
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
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Default(), fmt.Errorf("parse %q: %w", path, err)
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
