package main

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/LoTwT/lucia/internal/pet"
	"github.com/LoTwT/lucia/internal/settings"
	"github.com/egoist/mygo"
	mgui "github.com/egoist/mygo/ui"
)

// bubbleWindow is the separate, click-through speech bubble. Keeping it out
// of the pet window preserves the pet's existing rectangular hit area and
// lets the text disappear without affecting dragging.
type bubbleWindow struct {
	win        *mygo.Window
	model      *pet.BubbleModel
	hideTimer  *time.Timer
	generation atomic.Int64
}

func newBubbleWindow(quitting func() bool) *bubbleWindow {
	b := &bubbleWindow{model: &pet.BubbleModel{}}
	b.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Lucia 气泡",
		Width:             pet.BubbleWidth,
		Height:            pet.BubbleHeight,
		Frameless:         true,
		Transparent:       true,
		AlwaysOnTop:       true,
		Panel:             true,
		SkipTaskbar:       true,
		DisableShadow:     true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Hidden:            true,
		Content:           mgui.View(b.model.View),
	})
	b.win.SetIgnoreMouseEvents(true)
	b.win.OnClose(func(e *mygo.CloseEvent) {
		b.handleClose(e, quitting)
	})
	return b
}

// SetTheme applies the console's bubble appearance preference. It also
// repaints a visible bubble so a theme change takes effect immediately.
func (b *bubbleWindow) SetTheme(theme pet.BubbleTheme) {
	if b == nil || b.model == nil {
		return
	}
	b.model.SetTheme(theme)
	if b.win != nil && !b.win.IsDestroyed() {
		b.win.Update(func() {})
	}
}

func themeSourceForPreference(value string) mygo.ThemeSource {
	switch value {
	case settings.ThemeLight:
		return mygo.ThemeLight
	case settings.ThemeDark:
		return mygo.ThemeDark
	default:
		return mygo.ThemeSystem
	}
}

func bubbleThemeForPreference(value string) pet.BubbleTheme {
	switch value {
	case settings.BubbleThemeLight:
		return pet.BubbleThemeLight
	case settings.BubbleThemeDark:
		return pet.BubbleThemeDark
	default:
		return pet.BubbleThemeApp
	}
}

// handleClose keeps the bubble as a hide-on-close window during normal use,
// but lets MyGo's quit sequence close it. Otherwise prepareQuit would cancel
// the whole quit after the pet window had already been destroyed.
func (b *bubbleWindow) handleClose(e *mygo.CloseEvent, quitting func() bool) {
	if quitting != nil && quitting() {
		return
	}
	e.PreventDefault()
	b.Hide()
}

// Show places one authored line relative to the pet and starts its lifetime.
func (b *bubbleWindow) Show(text string, petRect pet.Rect, displays []pet.Display) {
	if b == nil || b.win == nil || b.win.IsDestroyed() {
		return
	}
	// MyGo v0.5.5 has no Linux implementation of SetIgnoreMouseEvents.
	// Keep the bubble hidden there rather than letting it block clicks.
	if runtime.GOOS == "linux" {
		return
	}
	if text == "" {
		return
	}
	// Invalidate the previous lifetime before showing the new line, so an
	// old timer cannot hide the new bubble while it is being placed.
	generation := b.generation.Add(1)
	if b.hideTimer != nil {
		b.hideTimer.Stop()
		b.hideTimer = nil
	}
	placement := pet.PlaceBubble(petRect, displays)
	b.model.SetContent(text, placement.Side)
	b.win.SetPosition(placement.X, placement.Y)
	b.win.Update(func() {})
	b.win.ShowInactive()
	b.win.SetIgnoreMouseEvents(true)

	b.hideTimer = time.AfterFunc(pet.BubbleLineDelay, func() {
		mygo.RunOnMain(func() {
			if b.generation.Load() != generation {
				return
			}
			if b.win != nil && !b.win.IsDestroyed() {
				b.win.Hide()
			}
		})
	})
}

func (b *bubbleWindow) Hide() {
	if b == nil {
		return
	}
	b.generation.Add(1)
	if b.hideTimer != nil {
		b.hideTimer.Stop()
		b.hideTimer = nil
	}
	if b.win != nil && !b.win.IsDestroyed() {
		b.win.Hide()
	}
}
