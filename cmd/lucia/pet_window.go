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
	width        atomic.Int32
	height       atomic.Int32
	onMove       func()
	onHide       func()
	onDragStart  func()
	onDragEnd    func()
	onFirstFrame func()
	firstFrame   atomic.Bool
	dragWatching atomic.Bool
}

// windowSize reads the current edge lengths from the atomic fields: drag
// clamping runs on a background goroutine while the settings UI can resize
// the window on the main thread.
func (p *petWindow) windowSize() (width, height int) {
	if p == nil {
		return 0, 0
	}
	return int(p.width.Load()), int(p.height.Load())
}

func newPetWindow(model *pet.Model, quitting func() bool, visible bool) *petWindow {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	p := &petWindow{model: model}
	width, height := model.WindowSize()
	p.width.Store(int32(width))
	p.height.Store(int32(height))
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Lucia 桌宠",
		Width:             width,
		Height:            height,
		X:                 area.X + area.Width - width - 24,
		Y:                 area.Y + area.Height - height - 24,
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
		Content: mgui.View(func(c *mgui.Context) {
			p.model.View(c)
			if p.onFirstFrame != nil && p.firstFrame.CompareAndSwap(false, true) {
				p.onFirstFrame()
			}
		}),
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
		width, height := p.windowSize()
		if _, changed := pet.KeepOnScreen(pet.Rect{
			X: x, Y: y, Width: width, Height: height,
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

// SetModel swaps the artwork and interaction map while keeping the window's
// bottom center in place. It runs on the UI thread.
func (p *petWindow) SetModel(model *pet.Model) {
	if p == nil || p.win == nil || p.win.IsDestroyed() || model == nil {
		return
	}
	old := p.Rect()
	p.model = model
	model.SetInvalidator(p.win.Invalidate)
	width, height := model.WindowSize()
	p.width.Store(int32(width))
	p.height.Store(int32(height))
	p.win.SetSize(width, height)
	p.win.SetPosition(old.X+(old.Width-width)/2, old.Y+old.Height-height)
	p.clampOutside()
	p.win.Update(func() {})
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
	width, height := p.windowSize()
	p.win.SetPosition(
		area.X+area.Width-width-24,
		area.Y+area.Height-height-24,
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
func (p *petWindow) SetOnFirstFrame(fn func()) {
	p.onFirstFrame = fn
}

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
		width, height := p.windowSize()
		return pet.Rect{Width: width, Height: height}
	}
	x, y := p.win.Position()
	width, height := p.windowSize()
	return pet.Rect{X: x, Y: y, Width: width, Height: height}
}

// BubbleRect maps the model's head anchor into screen coordinates. The
// bubble keeps using the window bottom when it has to flip below the pet.
func (p *petWindow) BubbleRect() pet.Rect {
	if p == nil || p.model == nil || p.win == nil || p.win.IsDestroyed() {
		return p.Rect()
	}
	x, y := p.win.Position()
	anchor := p.model.BubbleRect()
	return pet.Rect{
		X:      x + int(anchor.X),
		Y:      y + int(anchor.Y),
		Width:  int(anchor.W),
		Height: int(anchor.H),
	}
}

// ApplyScale resizes the native window and keeps its bottom-center anchored,
// then clamps the result to the available display area. The model owns the
// layout math so the face target and sprite stay in step with the window.
func (p *petWindow) ApplyScale(scale float64) {
	if p == nil || p.win == nil || p.win.IsDestroyed() || p.model == nil {
		return
	}
	old := p.Rect()
	p.model.SetScale(scale)
	width, height := p.model.WindowSize()
	oldWidth, oldHeight := p.windowSize()
	if width == oldWidth && height == oldHeight {
		return
	}
	p.width.Store(int32(width))
	p.height.Store(int32(height))
	p.win.SetSize(width, height)
	p.win.SetPosition(
		old.X+(old.Width-width)/2,
		old.Y+old.Height-height,
	)
	p.clampOutside()
	p.win.Update(func() {})
}

func (p *petWindow) clampOutside() {
	if p == nil || p.win == nil || p.win.IsDestroyed() {
		return
	}
	x, y := p.win.Position()
	width, height := p.windowSize()
	next, changed := pet.KeepOnScreen(pet.Rect{
		X: x, Y: y, Width: width, Height: height,
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
	width, height := p.windowSize()
	if w, h := p.win.Size(); w != width || h != height {
		x, y := p.win.Position()
		p.win.SetSize(width, height)
		// A restored window may still have the previous pack's square size.
		// Keep the feet in place while growing or shrinking.
		p.win.SetPosition(x+(w-width)/2, y+h-height)
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
			ScaleFactor: display.ScaleFactor,
		})
	}
	return out
}
