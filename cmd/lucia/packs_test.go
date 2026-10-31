package main

import (
	"testing"
	"time"

	"github.com/LoTwT/lucia/internal/builtinpack"
	"github.com/LoTwT/lucia/internal/petpack"
)

func TestLoadPetModelBuildsBuiltinAndReportsMissingPacks(t *testing.T) {
	builtin, err := builtinpack.Load(2)
	if err != nil {
		t.Fatal(err)
	}
	model, err := loadPetModel(t.TempDir(), "", builtin)
	if err != nil {
		t.Fatalf("load the built-in pack: %v", err)
	}
	model.Close()
	if _, err := loadPetModel(t.TempDir(), "missing", builtin); err == nil {
		t.Fatal("a missing external pack did not report an error")
	}
}

func TestUIPackSummaryMapsInteractionData(t *testing.T) {
	summary := petpack.Summary{
		ID: "tiny", Name: "Tiny", Active: true, Builtin: true, Valid: true,
		Clips: 2, Frames: 6, Interactions: 1, License: "local-only",
		InstalledAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.Local),
		Hotzones: []petpack.HotzoneEntry{{
			ID: "face", Hotzone: petpack.Hotzone{
				Label: "摸摸脸", Interaction: "poke",
				Rect: petpack.NormalizedRect{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
			},
		}},
		InteractionList: []petpack.InteractionDetail{{
			ID: "poke", Label: "摸摸脸", Bubble: "face", Motion: "bounce",
			Steps: []petpack.Step{{Action: "laugh", DelayMs: 80}},
		}},
		ActionList: []petpack.ActionDetail{{
			ID: "happy", Label: "开心", Timing: petpack.Timing{EnterHoldMs: 40, ExitHoldMs: 100},
		}},
	}
	got := uiPackSummary(summary)
	if got.Name != "Tiny" || !got.Active || !got.Builtin || len(got.Hotzones) != 1 {
		t.Fatalf("summary = %+v", got)
	}
	if got.Hotzones[0].Interaction != "poke" {
		t.Fatalf("hotzone = %+v", got.Hotzones[0])
	}
	if len(got.InteractionList) != 1 || got.InteractionList[0].Steps[0].DelayMs != 80 {
		t.Fatalf("interactions = %+v", got.InteractionList)
	}
	if len(got.ActionList) != 1 || got.ActionList[0].EnterHold != 40 || got.ActionList[0].ExitHold != 100 {
		t.Fatalf("actions = %+v", got.ActionList)
	}
	if got.InstalledAt != "2026-10-24" {
		t.Fatalf("installedAt = %q", got.InstalledAt)
	}
}
