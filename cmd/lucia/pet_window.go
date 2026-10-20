package main

import (
	"sync/atomic"
	"time"

	"github.com/LoTwT/lucia/internal/pet"
	"github.com/egoist/mygo"
	mgui "github.com/egoist/mygo/ui"
)

// petWindow owns the desktop pet's native window and the rules that keep it
// reachable. The console and tray live in main.go; keeping this wiring here
// leaves room for the bubble window without turning main into a window module.
type petWindow struct {
	win          *mygo.Window
	onMove       func()
	onHide       func()
	onDragStart  func()
	onDragEnd    func()
	dragWatching atomic.Bool
}

func newPetWindow(model *pet.Model, quitting func() bool) *petWindow {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	p := &petWindow{}
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Lucia 桌宠",
		Width:             pet.Size,
		Height:            pet.Size,
		X:                 area.X + area.Width - pet.Size - 24,
		Y:                 area.Y + area.Height - pet.Size - 24,
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
		// Opens where the user left it, keeping the size across launches.
		StateKey: "pet",
		Content:  mgui.View(model.View),
	})
	// Closing the pet hides it; only the tray's quit closes the app.
	p.win.OnClose(func(e *mygo.CloseEvent) {
		if quitting() {
			return
		}
		e.PreventDefault()
		p.Hide()
	})
	// DragWindow's double click asks macOS to zoom the window, even though a
	// pet has no meaningful maximized state. Undo it at once.
	p.win.OnMaximize(func() { p.win.Unmaximize() })
	// Native DragWindow crosses displays without fighting the window server,
	// but it lets the pet hang past an outer edge. Correct that after the
	// mouse is released instead of calling SetPosition during the drag: a
	// position change in the native drag loop is what caused the bounce at
	// the edge and at the display seam.
	var clampWatching atomic.Bool
	p.win.OnMove(func() {
		if p.onMove != nil {
			p.onMove()
		}
		if !p.dragWatching.Load() {
			if down, ok := leftMouseButtonDown(); ok && down {
				p.dragWatching.Store(true)
				if p.onDragStart != nil {
					p.onDragStart()
				}
				go p.watchDrag()
			}
		}
		x, y := p.win.Position()
		if _, changed := pet.KeepOnScreen(pet.Rect{
			X: x, Y: y, Width: pet.Size, Height: pet.Size,
		}, currentPetDisplays()); !changed {
			return
		}
		if !clampWatching.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer clampWatching.Store(false)
			down, ok := leftMouseButtonDown()
			if !ok {
				// No native button state: give the drag time to settle.
				time.Sleep(300 * time.Millisecond)
			} else {
				for down {
					time.Sleep(25 * time.Millisecond)
					down, ok = leftMouseButtonDown()
					if !ok {
						break
					}
				}
			}
			p.clampOutside()
		}()
	})
	// Startup and display changes are a recovery path, not a drag: put the
	// whole window back inside one work area so a position saved on a
	// disconnected display cannot leave the pet unreachable.
	p.recover()
	mygo.Screen.OnDisplaysChanged(p.recover)
	return p
}

func (p *petWindow) Toggle() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	if p.win.IsVisible() {
		p.Hide()
		return
	}
	p.win.ShowInactive()
}

func (p *petWindow) Hide() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	p.win.Hide()
	if p.onHide != nil {
		p.onHide()
	}
}

func (p *petWindow) ShowInactive() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	p.win.ShowInactive()
}

func (p *petWindow) IsVisible() bool {
	return p != nil && p.win != nil && !p.win.IsDestroyed() && p.win.IsVisible()
}

func (p *petWindow) SetOnMove(fn func())      { p.onMove = fn }
func (p *petWindow) SetOnHide(fn func())      { p.onHide = fn }
func (p *petWindow) SetOnDragStart(fn func()) { p.onDragStart = fn }
func (p *petWindow) SetOnDragEnd(fn func())   { p.onDragEnd = fn }

// watchDrag waits for the native drag's mouse button to come up. The end
// callback runs on the UI thread, where the pet's state lives.
func (p *petWindow) watchDrag() {
	for {
		down, ok := leftMouseButtonDown()
		if !ok || !down {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	mygo.RunOnMain(func() {
		p.dragWatching.Store(false)
		if p.onDragEnd != nil {
			p.onDragEnd()
		}
	})
}

func (p *petWindow) Rect() pet.Rect {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return pet.Rect{Width: pet.Size, Height: pet.Size}
	}
	x, y := p.win.Position()
	return pet.Rect{X: x, Y: y, Width: pet.Size, Height: pet.Size}
}

func (p *petWindow) clampOutside() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	x, y := p.win.Position()
	next, changed := pet.KeepOnScreen(pet.Rect{
		X: x, Y: y, Width: pet.Size, Height: pet.Size,
	}, currentPetDisplays())
	if changed {
		p.win.SetPosition(next.X, next.Y)
	}
}

// recover also undoes an accidental zoom and restores the fixed size. It is
// only for startup and display changes, not for every drag event.
func (p *petWindow) recover() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	if p.win.IsMaximized() {
		p.win.Unmaximize()
	}
	if w, h := p.win.Size(); w != pet.Size || h != pet.Size {
		p.win.SetSize(pet.Size, pet.Size)
	}
	p.clampOutside()
}

// currentPetDisplays snapshots the monitor geometry for the pet placement
// rules. Display changes are rare, so reading it on each move keeps drag
// behavior correct when a monitor is plugged in or rearranged mid-session.
func currentPetDisplays() []pet.Display {
	displays := mygo.Screen.Displays()
	out := make([]pet.Display, 0, len(displays))
	for _, display := range displays {
		out = append(out, pet.Display{
			Bounds: pet.Rect{
				X:      display.Bounds.X,
				Y:      display.Bounds.Y,
				Width:  display.Bounds.Width,
				Height: display.Bounds.Height,
			},
			WorkArea: pet.Rect{
				X:      display.WorkArea.X,
				Y:      display.WorkArea.Y,
				Width:  display.WorkArea.Width,
				Height: display.WorkArea.Height,
			},
		})
	}
	return out
}
