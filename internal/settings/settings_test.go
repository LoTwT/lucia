package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingUsesDefaults(t *testing.T) {
	got, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.CheckUpdatesOnLaunch {
		t.Error("a first launch must check for updates")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Settings{CheckUpdatesOnLaunch: false}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("settings %+v, want %+v", got, want)
	}
	info, err := os.Stat(filepath.Join(dir, File))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("permissions %o, want 644", perm)
	}
}

func TestLoadDamagedFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err == nil {
		t.Fatal("Load accepted a damaged file without reporting it")
	}
	if !got.CheckUpdatesOnLaunch {
		t.Error("a damaged file must apply the defaults")
	}
}
