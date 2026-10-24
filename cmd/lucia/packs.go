package main

import (
	"github.com/LoTwT/lucia/internal/petpack"
	appui "github.com/LoTwT/lucia/internal/ui"
)

// uiPackSummary keeps the dependency direction one-way: the UI package
// describes its own small view model, and the application adapts petpack
// summaries into it.
func uiPackSummary(summary petpack.Summary) appui.PackSummary {
	name := summary.Name
	if name == "" {
		name = summary.ID
	}
	hotzones := make([]appui.PackHotzone, 0, len(summary.Hotzones))
	for _, entry := range summary.Hotzones {
		hotzones = append(hotzones, appui.PackHotzone{
			ID: entry.ID, Label: entry.Label, Preset: entry.Preset, Action: entry.Action,
			X: entry.Rect.X, Y: entry.Rect.Y,
			Width: entry.Rect.Width, Height: entry.Rect.Height,
		})
	}
	presets := make([]appui.PackPreset, 0, len(summary.PresetList))
	for _, preset := range summary.PresetList {
		steps := make([]appui.PackPresetStep, 0, len(preset.Steps))
		for _, step := range preset.Steps {
			steps = append(steps, appui.PackPresetStep{Action: step.Action, DelayMs: step.DelayMs})
		}
		presets = append(presets, appui.PackPreset{
			ID: preset.ID, Reaction: preset.Reaction, Bubble: preset.Bubble, Steps: steps,
		})
	}
	actions := make([]appui.PackAction, 0, len(summary.ActionList))
	for _, action := range summary.ActionList {
		actions = append(actions, appui.PackAction{
			ID: action.ID, Label: action.Label, LoopCount: action.LoopCount,
			EnterHold: action.Timing.EnterHoldMs, LoopHold: action.Timing.LoopHoldMs,
			ExitHold: action.Timing.ExitHoldMs,
		})
	}
	result := appui.PackSummary{
		ID: summary.ID, Name: name, Schema: summary.Schema,
		Active: summary.Active, Valid: summary.Valid, Error: summary.Error,
		Clips: summary.Clips, Frames: summary.Frames, Hotzones: hotzones,
		Presets: summary.Presets, PresetList: presets, ActionList: actions,
		License: summary.License,
	}
	if !summary.InstalledAt.IsZero() {
		result.InstalledAt = summary.InstalledAt.Local().Format("2006-01-02")
	}
	return result
}
