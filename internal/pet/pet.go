// Package pet renders Lucia's desktop pet and keeps its idle animation.
// The window wiring and desktop lifecycle live in cmd/lucia; this package
// only owns the character's frames and the view drawn on the UI thread.
package pet

import (
	_ "embed"
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// Size is the pet window's edge length in DIPs. The sprite is slightly
// smaller so the idle bob never clips the character at the window edge.
const Size = 248

const spriteSize = 240

// The three frames share one 800x800 canvas, so switching among them never
// moves the character.
var (
	//go:embed assets/deepseek-whale/idle-open.png
	idleOpenPNG []byte
	//go:embed assets/deepseek-whale/idle-half-closed.png
	idleHalfClosedPNG []byte
	//go:embed assets/deepseek-whale/idle-closed.png
	idleClosedPNG []byte
	//go:embed assets/deepseek-whale/tray.png
	trayPNG []byte
)

const (
	// blinkPeriod is how long one open-eyed stretch lasts before the next
	// blink. The blink itself starts shortly before the period ends.
	blinkPeriod = 4 * time.Second
	blinkAt     = 3800 * time.Millisecond
	halfBlink   = 60 * time.Millisecond
	closedBlink = 90 * time.Millisecond
	// bobPeriod and bobAmplitude make the idle character breathe without
	// moving the window, which would fight the OS drag.
	bobPeriod    = 3200 * time.Millisecond
	bobAmplitude = 2.5
	// tapDuration is the squash-and-recover reaction to a click.
	tapDuration = 450 * time.Millisecond
)

// The face region receives clicks; the rest of the window stays a
// DragWindow handle so the pet can still be moved.
var faceRegion = ui.Rect{X: 94, Y: 28, W: 60, H: 64}

type frame uint8

const (
	frameOpen frame = iota
	frameHalfClosed
	frameClosed
)

// Model is the pet's UI-thread state. It is intentionally tiny: the clock
// decides which frame and bob offset to show, so there is no ticker or
// background goroutine to stop.
type Model struct {
	frames     [3]*ui.Bitmap
	start      time.Time
	tapAt      time.Time
	tapReady   bool
	reaction   Reaction
	onReaction func(Reaction)
}

// New decodes the embedded frames. Failing here means the build's embedded
// assets are broken, so the caller should report it and stop.
func New() (*Model, error) {
	m := &Model{}
	for i, data := range [][]byte{idleOpenPNG, idleHalfClosedPNG, idleClosedPNG} {
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			return nil, fmt.Errorf("decode pet frame %d: %w", i, err)
		}
		m.frames[i] = bitmap
	}
	return m, nil
}

// TrayIcon returns the PNG shown in the menu bar or notification area.
func TrayIcon() []byte { return trayPNG }

// Reaction identifies the interaction that started a response. It keeps the
// face click and the context-menu head pat distinct so later bubbles and
// animations can say and do different things.
type Reaction uint8

const (
	ReactionNone Reaction = iota
	ReactionFace
	ReactionHeadPat
)

// SetReactionHandler installs the UI-thread callback for reactions. The
// bubble window uses it to choose the line that belongs to the interaction.
func (m *Model) SetReactionHandler(fn func(Reaction)) { m.onReaction = fn }

// React starts the squash-and-recover reaction. It is safe to call from the
// UI thread's click and menu actions; the next view build turns the request
// into a timestamp from the UI clock.
func (m *Model) React(reaction Reaction) {
	m.reaction = reaction
	m.tapReady = true
	if m.onReaction != nil {
		m.onReaction(reaction)
	}
}

// View draws the pet on a transparent root. The face is a click target and
// the rest of the window is a DragWindow handle, so clicking the face reacts
// while dragging anywhere else moves the window.
func (m *Model) View(c *ui.Context) {
	now := c.Now()
	if m.start.IsZero() {
		m.start = now
	}
	if m.tapReady {
		m.tapAt = now
		m.tapReady = false
	}
	elapsed := now.Sub(m.start)

	// The character is always moving: either breathing or blinking. Ask
	// for the next display frame while the window is visible.
	c.AnimationFrame()
	c.Root().Background(ui.Transparent)

	bob := float32(math.Sin(2*math.Pi*float64(elapsed)/float64(bobPeriod))) * bobAmplitude
	squash := tapAmount(now.Sub(m.tapAt))

	outer := ui.Box(c).Fill().DragWindow()
	outer.ContextMenu(func(menu *ui.Menu) {
		if menu.Item("摸摸头").Chosen() {
			m.React(ReactionHeadPat)
		}
	})
	outer.Children(func() {
		ui.Row(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			image := ui.Image(c, m.frames[frameAt(elapsed)]).
				Size(spriteSize, spriteSize).
				Fit(ui.Contain).
				Label("Lucia 桌宠").
				TransformOrigin(0.5, 1)
			if squash > 0 {
				// Keep the feet in place while the body squashes.
				image.Scale(1+squash*0.5, 1-squash)
			}
			image.Translate(0, bob)
		})
		ui.Box(c).Absolute().
			Left(faceRegion.X).Top(faceRegion.Y).
			Size(faceRegion.W, faceRegion.H).
			Label("摸摸头").
			Cursor(ui.CursorPointer).
			OnClick(func() { m.React(ReactionFace) })
	})
}

// tapAmount returns 0 outside the reaction and a 0→1→0 squash inside it.
func tapAmount(elapsed time.Duration) float32 {
	if elapsed < 0 || elapsed >= tapDuration {
		return 0
	}
	p := float32(elapsed) / float32(tapDuration)
	return float32(math.Sin(math.Pi * float64(p)))
}

// frameAt maps elapsed time to the blink frame. The first half-blink is
// deliberately at the end of the period so the character starts open-eyed.
func frameAt(elapsed time.Duration) frame {
	phase := elapsed % blinkPeriod
	d := phase - blinkAt
	if d < 0 {
		return frameOpen
	}
	switch {
	case d < halfBlink:
		return frameHalfClosed
	case d < halfBlink+closedBlink:
		return frameClosed
	case d < halfBlink+closedBlink+halfBlink:
		return frameHalfClosed
	default:
		return frameOpen
	}
}
