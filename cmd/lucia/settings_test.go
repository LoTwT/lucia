package main

import (
	"testing"

	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/LoTwT/lucia/internal/settings"
)

func TestUpdateUISettingsPreservesRuntimeFields(t *testing.T) {
	dir := t.TempDir()
	initial := settings.Default()
	initial.PetPack = "naiwa"
	initial.PetIntroPlayed = true
	store := settings.NewStore(dir, initial)

	ui := initial
	ui.SoundEnabled = true
	ui.Theme = settings.ThemeLight
	ui.BubbleTheme = settings.BubbleThemeDark
	ui.PetIntroPlayed = false
	ui.PetPack = ""

	got, err := updateUISettings(store, ui)
	if err != nil {
		t.Fatalf("updateUISettings: %v", err)
	}
	if !got.SoundEnabled || got.Theme != settings.ThemeLight || got.BubbleTheme != settings.BubbleThemeDark {
		t.Fatalf("UI fields were not saved: sound=%v theme=%q bubble=%q", got.SoundEnabled, got.Theme, got.BubbleTheme)
	}
	if got.PetPack != "naiwa" || !got.PetIntroPlayed {
		t.Fatalf("runtime fields changed: pack=%q intro=%v", got.PetPack, got.PetIntroPlayed)
	}
	persisted, err := settings.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if persisted.PetPack != "naiwa" || !persisted.PetIntroPlayed {
		t.Fatalf("persisted runtime fields changed: %+v", persisted)
	}
}

func TestClearFallbackActiveResetsModelAndSettings(t *testing.T) {
	data := t.TempDir()
	prefs := settings.Default()
	prefs.PetPack = "broken"
	settingsStore := settings.NewStore(data, prefs)
	packStore := petpack.NewStore(data, prefs.PetPack)
	if err := packStore.SetActive(prefs.PetPack); err != nil {
		t.Fatal(err)
	}
	if err := clearFallbackActive(settingsStore, packStore, &prefs); err != nil {
		t.Fatal(err)
	}
	if prefs.PetPack != "" {
		t.Fatalf("prefs.PetPack = %q, want empty", prefs.PetPack)
	}
	if packStore.Active() != "" {
		t.Fatalf("store active = %q, want empty", packStore.Active())
	}
	persisted, err := settings.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.PetPack != "" {
		t.Fatalf("persisted PetPack = %q, want empty", persisted.PetPack)
	}
}
