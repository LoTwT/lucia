package settings

import (
	"os"
	"path/filepath"
	"runtime"
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
	if !got.PetVisible {
		t.Error("a first launch must show the pet")
	}
	if got.SoundEnabled {
		t.Error("a first launch must not play interaction sounds")
	}
	if got.BubbleCooldownSeconds != 0 {
		t.Errorf("bubble cooldown = %d, want 0", got.BubbleCooldownSeconds)
	}
	if got.PetScale != 1 {
		t.Errorf("pet scale = %v, want 1", got.PetScale)
	}
	if got.PetPack != "" || got.PetIntroPlayed {
		t.Errorf("pet pack defaults = %q, intro %v; want empty, false", got.PetPack, got.PetIntroPlayed)
	}
}

func TestLoadMergesNewFieldsWithDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte(`{"checkUpdatesOnLaunch":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CheckUpdatesOnLaunch {
		t.Error("an explicit false must override the default")
	}
	if !got.PetVisible {
		t.Error("a missing petVisible field must keep the default true")
	}
	if got.SoundEnabled || got.BubbleCooldownSeconds != 0 {
		t.Errorf("missing bubble fields = sound %v, cooldown %d; want false, 0", got.SoundEnabled, got.BubbleCooldownSeconds)
	}
	if got.PetScale != 1 {
		t.Errorf("a missing petScale field must keep the default 1, got %v", got.PetScale)
	}
}

func TestLoadNormalizesZeroPetScale(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte(`{"petScale":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.PetScale != 1 {
		t.Fatalf("pet scale = %v, want 1", got.PetScale)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Default()
	want.CheckUpdatesOnLaunch = false
	want.PetVisible = false
	want.SoundEnabled = true
	want.BubbleCooldownSeconds = 30
	want.PetScale = 1.25
	want.PetPack = "naiwa"
	want.PetIntroPlayed = true
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
	// Windows does not model Unix permission bits; its ACLs decide access.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o644 {
			t.Errorf("permissions %o, want 644", perm)
		}
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
	if !got.PetVisible {
		t.Error("a damaged file must apply the pet visibility default")
	}
}
