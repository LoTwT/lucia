// Package apppaths resolves the per-user directories Lucia owns: its data,
// cache and logs. The locations follow each platform's conventions and are
// resolved before MyGo starts so that the framework's own state (window
// placement, update state) and the application's files share one root.
//
// The development app is named "Lucia Dev" by MyGo, so the same resolver
// keeps development data, logs and permissions separate from the installed
// app without environment switches.
package apppaths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths is the set of directories a Lucia installation owns.
type Paths struct {
	// Data holds settings and other durable user state.
	Data string
	// Cache holds rebuildable data.
	Cache string
	// Logs holds diagnostic logs.
	Logs string
}

// Resolve returns the directories for appName on the current platform.
func Resolve(appName string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve the user configuration directory: %w", err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve the user cache directory: %w", err)
	}
	return resolve(runtime.GOOS, appName, home, configDir, cacheDir, os.Getenv("LOCALAPPDATA"))
}

// resolve is Resolve with its platform inputs injected for tests.
func resolve(goos, appName, home, configDir, cacheDir, localAppData string) (Paths, error) {
	if appName == "" {
		return Paths{}, errors.New("resolve application directories: the application name is empty")
	}
	switch goos {
	case "darwin":
		if home == "" {
			return Paths{}, errors.New("resolve application directories: the home directory is unknown")
		}
		base := filepath.Join(home, "Library")
		return Paths{
			Data:  filepath.Join(base, "Application Support", appName),
			Cache: filepath.Join(base, "Caches", appName),
			Logs:  filepath.Join(base, "Logs", appName),
		}, nil
	case "windows":
		// Roaming AppData is synchronized between machines and can be
		// redirected by policy; application data and its update state belong
		// to this machine, as %LOCALAPPDATA% is.
		base := localAppData
		if base == "" {
			base = cacheDir
		}
		root := filepath.Join(base, appName)
		return Paths{
			Data:  root,
			Cache: filepath.Join(root, "cache"),
			Logs:  filepath.Join(root, "logs"),
		}, nil
	default:
		return Paths{
			Data:  filepath.Join(configDir, appName),
			Cache: filepath.Join(cacheDir, appName),
			Logs:  filepath.Join(configDir, appName, "logs"),
		}, nil
	}
}

// Ensure creates every directory, including parents, with the permissions
// of ordinary user data. It returns the first failure with the directory
// that could not be created.
func (p Paths) Ensure() error {
	for _, dir := range []string{p.Data, p.Cache, p.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %q: %w", dir, err)
		}
	}
	return nil
}
