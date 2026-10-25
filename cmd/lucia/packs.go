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
			ID: entry.ID, Label: entry.Label, Interaction: entry.Interaction,
			X: entry.Rect.X, Y: entry.Rect.Y,
			Width: entry.Rect.Width, Height: entry.Rect.Height,
		})
	}
	interactions := make([]appui.PackInteraction, 0, len(summary.InteractionList))
	for _, interaction := range summary.InteractionList {
		steps := make([]appui.PackInteractionStep, 0, len(interaction.Steps))
		for _, step := range interaction.Steps {
			steps = append(steps, appui.PackInteractionStep{Action: step.Action, DelayMs: step.DelayMs})
		}
		interactions = append(interactions, appui.PackInteraction{
			ID: interaction.ID, Label: interaction.Label, Bubble: interaction.Bubble,
			Motion: interaction.Motion, Steps: steps,
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
		ID: summary.ID, Name: name,
		Active: summary.Active, Builtin: summary.Builtin, Valid: summary.Valid, Error: summary.Error,
		Clips: summary.Clips, Frames: summary.Frames, Hotzones: hotzones,
		Interactions: summary.Interactions, InteractionList: interactions, ActionList: actions,
		License: summary.License,
	}
	if !summary.InstalledAt.IsZero() {
		result.InstalledAt = summary.InstalledAt.Local().Format("2006-01-02")
	}
	return result
}
