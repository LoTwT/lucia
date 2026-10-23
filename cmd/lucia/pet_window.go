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
	model        *pet.Model
	size         atomic.Int32
	onMove       func()
	onHide       func()
	onDragStart  func()
	onDragEnd    func()
	dragWatching atomic.Bool
}

// windowSize reads the current edge length from the atomic field: drag
// clamping runs on a background goroutine while the settings UI can resize
// the window on the main thread.
func (p *petWindow) windowSize() int {
	if p == nil {
		return 0
	}
	return int(p.size.Load())
}

func newPetWindow(model *pet.Model, quitting func() bool, visible bool) *petWindow {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	p := &petWindow{model: model}
	p.size.Store(int32(model.Size()))
	size := p.windowSize()
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Lucia 桌宠",
		Width:             size,
		Height:            size,
		X:                 area.X + area.Width - size - 24,
		Y:                 area.Y + area.Height - size - 24,
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
		Hidden:            !visible,
		// Opens where the user left it, keeping the size across launches.
		StateKey: "pet",
		Content:  mgui.View(model.View),
	})
	// A click or drag can change the pet while it is resting between
	// scheduled frames; ask for that frame explicitly.
	model.SetInvalidator(p.win.Invalidate)
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
		size := p.windowSize()
		if _, changed := pet.KeepOnScreen(pet.Rect{
			X: x, Y: y, Width: size, Height: size,
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

func (p *petWindow) SetVisible(visible bool) {
	if visible {
		p.ShowInactive()
		return
	}
	p.Hide()
}

func (p *petWindow) ShowInactive() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	p.win.ShowInactive()
}

// ResetPosition puts the pet back at its first-launch corner and brings it
// back if the user had hidden it.
func (p *petWindow) ResetPosition() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	area := mygo.Screen.PrimaryDisplay().WorkArea
	size := p.windowSize()
	p.win.SetPosition(
		area.X+area.Width-size-24,
		area.Y+area.Height-size-24,
	)
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
	if p == nil {
		return pet.Rect{}
	}
	if p.win == nil || p.win.IsDestroyed() {
		size := p.windowSize()
		return pet.Rect{Width: size, Height: size}
	}
	x, y := p.win.Position()
	size := p.windowSize()
	return pet.Rect{X: x, Y: y, Width: size, Height: size}
}

// ApplyScale resizes the native window and keeps its old center, then clamps
// the result to the available display area. The model owns the layout math so
// the face target and sprite stay in step with the window.
func (p *petWindow) ApplyScale(scale float64) {
	if p == nil || p.win == nil || p.win.IsDestroyed() || p.model == nil {
		return
	}
	old := p.Rect()
	p.model.SetScale(scale)
	size := p.model.Size()
	if size == p.windowSize() {
		return
	}
	p.size.Store(int32(size))
	p.win.SetSize(size, size)
	p.win.SetPosition(
		old.X+(old.Width-size)/2,
		old.Y+(old.Height-size)/2,
	)
	p.clampOutside()
	p.win.Update(func() {})
}

func (p *petWindow) clampOutside() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	x, y := p.win.Position()
	size := p.windowSize()
	next, changed := pet.KeepOnScreen(pet.Rect{
		X: x, Y: y, Width: size, Height: size,
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
	size := p.windowSize()
	if w, h := p.win.Size(); w != size || h != size {
		p.win.SetSize(size, size)
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
