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

// BadFile is where an unusable settings file is moved. Keeping the bytes
// preserves manual edits and makes the failure diagnosable after the app has
// written fresh defaults.
const BadFile = File + ".bad"

// Theme values persisted in Settings.Theme and Settings.BubbleTheme.
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"

	// BubbleThemeApp reuses the persisted "system" value for compatibility
	// while meaning "follow the application theme."
	BubbleThemeApp   = ThemeSystem
	BubbleThemeLight = ThemeLight
	BubbleThemeDark  = ThemeDark
)

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
	// Theme selects the application's appearance: follow the system, always
	// light, or always dark. Empty and unknown values follow the system.
	Theme string `json:"theme,omitempty"`
	// BubbleTheme selects the speech bubble's appearance: follow the
	// application, always light, or always dark. Empty and unknown values
	// follow the application.
	BubbleTheme string `json:"bubbleTheme,omitempty"`
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
		Theme:                ThemeSystem,
		BubbleTheme:          BubbleThemeApp,
		PetScale:             1.0,
	}
}

// LoadError reports why the persisted settings could not be used. BadPath
// names the preserved copy when the original was moved aside. PreserveErr is
// set when it could not be moved; the original then holds the only copy and
// the caller's store must not write over it.
type LoadError struct {
	Path        string
	BadPath     string
	Cause       error
	PreserveErr error
}

func (e *LoadError) Error() string {
	if e.PreserveErr != nil {
		return fmt.Sprintf("%s: %v (could not move it to %q: %v; refusing to overwrite it this run)",
			e.Path, e.Cause, e.BadPath, e.PreserveErr)
	}
	return fmt.Sprintf("%s: %v (kept the original as %q)", e.Path, e.Cause, e.BadPath)
}

// Unwrap reports the read or parse failure that made the file unusable.
func (e *LoadError) Unwrap() error { return e.Cause }

// Load reads the settings in dir. A missing file returns the defaults and
// no error. An unreadable or invalid file is moved to File+".bad" so a later
// save cannot destroy the only copy, and the defaults are returned with the
// reason, which the caller logs while continuing with the defaults.
func Load(dir string) (Settings, error) {
	path := filepath.Join(dir, File)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return preserveDamaged(path, fmt.Errorf("read: %w", err))
	}
	// Start from defaults so fields added after an older file was written
	// keep their intended default instead of decoding as the zero value.
	s := Default()
	if err := json.Unmarshal(data, &s); err != nil {
		return preserveDamaged(path, fmt.Errorf("parse: %w", err))
	}
	if s.PetScale <= 0 {
		s.PetScale = 1.0
	}
	s.Theme = normalizeTheme(s.Theme)
	s.BubbleTheme = normalizeTheme(s.BubbleTheme)
	return s, nil
}

// preserveDamaged moves an unusable file aside before the defaults take over.
// The rename intentionally replaces an older .bad copy: the newest failure is
// the one worth diagnosing. A failed rename leaves the original in place and
// is reported in LoadError.PreserveErr so the store can stay read-only.
func preserveDamaged(path string, cause error) (Settings, error) {
	bad := filepath.Join(filepath.Dir(path), BadFile)
	loadErr := &LoadError{Path: path, BadPath: bad, Cause: cause}
	if err := os.Rename(path, bad); err != nil {
		loadErr.PreserveErr = err
	}
	return Default(), loadErr
}

func normalizeTheme(theme string) string {
	switch theme {
	case ThemeLight, ThemeDark:
		return theme
	default:
		return ThemeSystem
	}
}

// Save writes the settings atomically, so a crash mid-write leaves either
// the old file or the new one, never a partial one.
func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the settings directory: %w", err)
	}
	s.Theme = normalizeTheme(s.Theme)
	s.BubbleTheme = normalizeTheme(s.BubbleTheme)
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
