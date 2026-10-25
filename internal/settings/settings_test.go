package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	if got.Theme != ThemeSystem {
		t.Errorf("theme = %q, want %q", got.Theme, ThemeSystem)
	}
	if got.BubbleTheme != BubbleThemeApp {
		t.Errorf("bubble theme = %q, want %q", got.BubbleTheme, BubbleThemeApp)
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
	if got.Theme != ThemeSystem {
		t.Errorf("a missing theme field must follow the system, got %q", got.Theme)
	}
	if got.BubbleTheme != BubbleThemeApp {
		t.Errorf("a missing bubbleTheme field must follow the application, got %q", got.BubbleTheme)
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

func TestLoadNormalizesUnknownThemes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte(`{"theme":"sepia","bubbleTheme":"sepia"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Theme != ThemeSystem || got.BubbleTheme != BubbleThemeApp {
		t.Fatalf("themes = %q/%q, want %q/%q", got.Theme, got.BubbleTheme, ThemeSystem, BubbleThemeApp)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Default()
	want.CheckUpdatesOnLaunch = false
	want.PetVisible = false
	want.SoundEnabled = true
	want.BubbleCooldownSeconds = 30
	want.Theme = ThemeLight
	want.BubbleTheme = BubbleThemeDark
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

func TestStoreUpdatePreservesFieldsOwnedElsewhere(t *testing.T) {
	dir := t.TempDir()
	initial := Default()
	initial.PetPack = "naiwa"
	initial.PetIntroPlayed = true
	store := NewStore(dir, initial)

	got, err := store.Update(func(s *Settings) {
		s.SoundEnabled = true
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !got.SoundEnabled || got.PetPack != "naiwa" || !got.PetIntroPlayed {
		t.Fatalf("updated settings = %+v, want sound with pack and intro preserved", got)
	}
	persisted, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !persisted.SoundEnabled || persisted.PetPack != "naiwa" || !persisted.PetIntroPlayed {
		t.Fatalf("persisted settings = %+v", persisted)
	}
}

func TestStoreConcurrentUpdatesDoNotLoseFields(t *testing.T) {
	store := NewStore(t.TempDir(), Default())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.Update(func(s *Settings) {
				switch i % 4 {
				case 0:
					s.SoundEnabled = true
				case 1:
					s.BubbleCooldownSeconds = 30
				case 2:
					s.PetScale = 1.25
				case 3:
					s.CheckUpdatesOnLaunch = false
				}
			})
			if err != nil {
				t.Errorf("Update: %v", err)
			}
		}(i)
	}
	wg.Wait()
	got := store.Snapshot()
	if !got.SoundEnabled || got.BubbleCooldownSeconds != 30 || got.PetScale != 1.25 || got.CheckUpdatesOnLaunch {
		t.Fatalf("settings after concurrent updates = %+v", got)
	}
}

func TestStoreFailedSaveKeepsCurrent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir, Default())
	got, err := store.Update(func(s *Settings) { s.SoundEnabled = true })
	if err == nil {
		t.Fatal("Update succeeded with an unusable directory")
	}
	if got.SoundEnabled {
		t.Fatalf("failed update returned mutated settings: %+v", got)
	}
	if store.Snapshot().SoundEnabled {
		t.Fatalf("failed update mutated the in-memory settings")
	}
}
