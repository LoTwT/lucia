package main

import (
	"testing"

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
	ui.PetIntroPlayed = false
	ui.PetPack = ""

	got, err := updateUISettings(store, ui)
	if err != nil {
		t.Fatalf("updateUISettings: %v", err)
	}
	if !got.SoundEnabled {
		t.Fatal("UI field was not saved")
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
