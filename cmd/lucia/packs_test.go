package main

import (
	"testing"
	"time"

	"github.com/LoTwT/lucia/internal/petpack"
)

func TestUIPackSummaryMapsInteractionData(t *testing.T) {
	summary := petpack.Summary{
		ID: "tiny", Name: "Tiny", Active: true, Valid: true,
		Clips: 2, Frames: 6, Interactions: 1, License: "local-only",
		InstalledAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.Local),
		Hotzones: []petpack.HotzoneEntry{{
			ID: "face", Hotzone: petpack.Hotzone{
				Label: "摸摸脸", Interaction: "poke",
				Rect: petpack.NormalizedRect{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
			},
		}},
		InteractionList: []petpack.InteractionDetail{{
			ID: "poke", Label: "摸摸脸", Bubble: "face", Motion: "squash",
			Steps: []petpack.Step{{Action: "laugh", DelayMs: 80}},
		}},
		ActionList: []petpack.ActionDetail{{
			ID: "happy", Label: "开心", Timing: petpack.Timing{EnterHoldMs: 40, ExitHoldMs: 100},
		}},
	}
	got := uiPackSummary(summary)
	if got.Name != "Tiny" || !got.Active || len(got.Hotzones) != 1 {
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
