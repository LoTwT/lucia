package main

import (
	"sync/atomic"
	"time"

	"github.com/LoTwT/lucia/internal/pet"
	"github.com/egoist/mygo"
	mgui "github.com/egoist/mygo/ui"
)

// bubbleWindow is the separate, click-through speech bubble. Keeping it out
// of the pet window preserves the pet's existing rectangular hit area and
// lets the text disappear without affecting dragging.
type bubbleWindow struct {
	win        *mygo.Window
	model      *pet.BubbleModel
	seq        int
	hideTimer  *time.Timer
	generation atomic.Int64
}

func newBubbleWindow() *bubbleWindow {
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
		e.PreventDefault()
		b.Hide()
	})
	return b
}

// Show selects the next line for reaction, places the bubble relative to the
// pet, and starts its lifetime.
func (b *bubbleWindow) Show(reaction pet.Reaction, petRect pet.Rect, displays []pet.Display) {
	if b == nil || b.win == nil || b.win.IsDestroyed() {
		return
	}
	text := pet.BubbleLine(reaction, b.seq)
	if text == "" {
		return
	}
	b.seq++
	b.model.SetText(text)
	rect := pet.PlaceBubble(petRect, displays)
	b.win.SetPosition(rect.X, rect.Y)
	b.win.Update(func() {})
	b.win.ShowInactive()
	b.win.SetIgnoreMouseEvents(true)

	generation := b.generation.Add(1)
	if b.hideTimer != nil {
		b.hideTimer.Stop()
	}
	b.hideTimer = time.AfterFunc(pet.BubbleLineDelay, func() {
		if b.generation.Load() != generation {
			return
		}
		if b.win != nil && !b.win.IsDestroyed() {
			b.win.Hide()
		}
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
