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
	// headPatDuration is the gentler, longer reaction to the menu action.
	headPatDuration = 620 * time.Millisecond
	// The pet gets drowsy and then falls asleep after this much time since
	// the last interaction. The view clock decides the state; no timer runs.
	drowsyAfter = 60 * time.Second
	asleepAfter = 3 * time.Minute
	// Frame intervals replace an unconditional AnimationFrame. The breathing
	// bob is slow, so 30 fps is enough while awake; reactions need more
	// frames, and resting states can redraw far less often. This keeps the
	// idle pet from rebuilding its whole view on every display refresh.
	activeFrameInterval = 16 * time.Millisecond
	idleFrameInterval   = 33 * time.Millisecond
	drowsyFrameInterval = 66 * time.Millisecond
	asleepFrameInterval = 100 * time.Millisecond
	minFrameInterval    = time.Millisecond
)

type frame uint8

const (
	frameOpen frame = iota
	frameHalfClosed
	frameClosed
)

// Activity is the pet's rest state. It follows the time since the last
// interaction, except while the window is being dragged.
type Activity uint8

const (
	ActivityAwake Activity = iota
	ActivityDrowsy
	ActivityAsleep
)

func activityAt(idle time.Duration) Activity {
	switch {
	case idle >= asleepAfter:
		return ActivityAsleep
	case idle >= drowsyAfter:
		return ActivityDrowsy
	default:
		return ActivityAwake
	}
}

// Model is the pet's UI-thread state. It is intentionally tiny: the clock
// decides which frame and bob offset to show, so there is no ticker or
// background goroutine to stop.
type Model struct {
	frames          [3]*ui.Bitmap
	layout          assetLayout
	scale           float64
	windowSize      int
	spriteSize      int
	faceRegion      ui.Rect
	start           time.Time
	lastInteraction time.Time
	interacted      bool
	dragging        bool
	tapAt           time.Time
	tapReady        bool
	reaction        Reaction
	onReaction      func(Reaction)
	onInvalidate    func()
	menuActions     MenuActions
}

// MenuActions are the application-level actions offered by the pet's
// context menu. Nil fields hide their item, which keeps the pure pet view
// usable in tests without the application shell.
type MenuActions struct {
	OpenSettings func()
	Hide         func()
	Reset        func()
}

// New decodes the embedded frames. Failing here means the build's embedded
// assets are broken, so the caller should report it and stop.
func New() (*Model, error) {
	layout, err := loadLayout()
	if err != nil {
		return nil, fmt.Errorf("load pet layout: %w", err)
	}
	m := &Model{layout: layout, scale: defaultScale}
	for i, data := range [][]byte{idleOpenPNG, idleHalfClosedPNG, idleClosedPNG} {
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			return nil, fmt.Errorf("decode pet frame %d: %w", i, err)
		}
		m.frames[i] = bitmap
	}
	m.applyLayout()
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

// SetInvalidator asks for a fresh frame when state changes outside the view,
// such as a click or the start of a drag. Without this, a resting pet that
// schedules only its next blink could stay visually asleep after a drag.
func (m *Model) SetInvalidator(fn func()) { m.onInvalidate = fn }

func (m *Model) invalidate() {
	if m.onInvalidate != nil {
		m.onInvalidate()
	}
}

// SetMenuActions installs the application actions shown in the context
// menu. The pet package does not own windows, settings, or visibility.
func (m *Model) SetMenuActions(actions MenuActions) { m.menuActions = actions }

// React starts the squash-and-recover reaction. It is safe to call from the
// UI thread's click and menu actions; the next view build turns the request
// into a timestamp from the UI clock.
func (m *Model) React(reaction Reaction) {
	m.reaction = reaction
	m.tapReady = true
	m.interacted = true
	if m.onReaction != nil {
		m.onReaction(reaction)
	}
	m.invalidate()
}

// DragStarted and DragEnded keep the pet awake for the whole native drag.
// The window callback calls them on the UI thread.
func (m *Model) DragStarted() {
	m.dragging = true
	m.interacted = true
	m.invalidate()
}

func (m *Model) DragEnded() {
	m.dragging = false
	m.interacted = true
	m.invalidate()
}

func (m *Model) activity(now time.Time) Activity {
	if m.dragging || m.lastInteraction.IsZero() {
		return ActivityAwake
	}
	return activityAt(now.Sub(m.lastInteraction))
}

// View draws the pet on a transparent root. The face is a click target and
// the rest of the window is a DragWindow handle, so clicking the face reacts
// while dragging anywhere else moves the window.
func (m *Model) View(c *ui.Context) {
	now := c.Now()
	if m.start.IsZero() {
		m.start = now
	}
	if m.lastInteraction.IsZero() {
		m.lastInteraction = now
	}
	if m.interacted {
		m.lastInteraction = now
		m.interacted = false
	}
	if m.tapReady {
		m.tapAt = now
		m.tapReady = false
	}
	elapsed := now.Sub(m.start)
	idle := now.Sub(m.lastInteraction)
	activity := m.activity(now)
	reactionElapsed := now.Sub(m.tapAt)
	reduceMotion := c.Preferences().ReduceMotion
	bob, squash, lift := motionFor(reduceMotion, elapsed, m.reaction, reactionElapsed)

	c.Root().Background(ui.Transparent)

	outer := ui.Box(c).Fill().DragWindow()
	outer.ContextMenu(func(menu *ui.Menu) {
		if menu.Item("摸摸头").Chosen() {
			m.React(ReactionHeadPat)
		}
		if m.menuActions.OpenSettings == nil && m.menuActions.Hide == nil && m.menuActions.Reset == nil {
			return
		}
		menu.Separator()
		if m.menuActions.OpenSettings != nil && menu.Item("桌宠设置").Chosen() {
			m.menuActions.OpenSettings()
		}
		if m.menuActions.Hide != nil && menu.Item("隐藏宠物").Chosen() {
			m.menuActions.Hide()
		}
		if m.menuActions.Reset != nil && menu.Item("重置位置").Chosen() {
			m.menuActions.Reset()
		}
	})
	outer.Children(func() {
		ui.Row(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			image := ui.Image(c, m.frames[displayFrame(activity, elapsed, m.reaction, reactionElapsed, reduceMotion)]).
				Size(float32(m.spriteSize), float32(m.spriteSize)).
				Fit(ui.Contain).
				Label("Lucia 桌宠").
				TransformOrigin(0.5, 1)
			if squash > 0 {
				// Keep the feet in place while the body squashes.
				image.Scale(1+squash*0.5, 1-squash)
			}
			image.Translate(0, bob+lift)
		})
		ui.Box(c).Absolute().
			Left(m.faceRegion.X).Top(m.faceRegion.Y).
			Size(m.faceRegion.W, m.faceRegion.H).
			Label("摸摸脸").
			Cursor(ui.CursorPointer).
			OnClick(func() { m.React(ReactionFace) })
	})

	// Ask only for the frames this state needs. The view clock still derives
	// every pose, so there is no ticker or background goroutine.
	if delay := nextFrameDelay(activity, idle, elapsed, reactionElapsed, m.reaction, reduceMotion); delay > 0 {
		c.After(delay)
	}
}

// motionFor keeps every continuous transform in one place. Reduce Motion
// removes the breathing bob and the squash/lift reaction, leaving the
// discrete frame change from displayFrame as the only feedback.
func motionFor(reduceMotion bool, elapsed time.Duration, reaction Reaction, reactionElapsed time.Duration) (bob, squash, lift float32) {
	if reduceMotion {
		return 0, 0, 0
	}
	bob = float32(math.Sin(2*math.Pi*float64(elapsed)/float64(bobPeriod))) * bobAmplitude
	squash, lift = reactionMotion(reaction, reactionElapsed)
	return bob, squash, lift
}

// displayFrame chooses the sprite frame. With Reduce Motion, a reaction
// briefly closes or half-closes the eyes instead of moving the body.
func displayFrame(activity Activity, elapsed time.Duration, reaction Reaction, reactionElapsed time.Duration, reduceMotion bool) frame {
	if reduceMotion && reactionActive(reaction, reactionElapsed) {
		if reaction == ReactionHeadPat {
			return frameHalfClosed
		}
		return frameClosed
	}
	return frameFor(activity, elapsed)
}

func reactionDuration(reaction Reaction) time.Duration {
	switch reaction {
	case ReactionFace:
		return tapDuration
	case ReactionHeadPat:
		return headPatDuration
	default:
		return 0
	}
}

func reactionActive(reaction Reaction, elapsed time.Duration) bool {
	duration := reactionDuration(reaction)
	return duration > 0 && elapsed >= 0 && elapsed < duration
}

// nextFrameDelay returns how long the view can wait before its next visible
// change. A zero delay means no future frame is needed until an interaction
// or preference change invalidates the window.
func nextFrameDelay(activity Activity, idle, elapsed, reactionElapsed time.Duration, reaction Reaction, reduceMotion bool) time.Duration {
	if reactionActive(reaction, reactionElapsed) {
		if reduceMotion {
			return max(reactionDuration(reaction)-reactionElapsed, minFrameInterval)
		}
		return activeFrameInterval
	}

	if !reduceMotion {
		switch activity {
		case ActivityAsleep:
			return asleepFrameInterval
		case ActivityDrowsy:
			return drowsyFrameInterval
		default:
			return idleFrameInterval
		}
	}

	// Reduce Motion: wake only for a discrete state change.
	switch activity {
	case ActivityAsleep:
		return 0
	case ActivityDrowsy:
		return max(asleepAfter-idle, minFrameInterval)
	default:
		next := nextBlinkDelay(elapsed)
		if untilDrowsy := drowsyAfter - idle; untilDrowsy > 0 && untilDrowsy < next {
			next = untilDrowsy
		}
		return max(next, minFrameInterval)
	}
}

// nextBlinkDelay returns the time until the next blink frame. Each edge in
// the sequence is a visible change, and the final edge is the next period's
// open frame at the period boundary.
func nextBlinkDelay(elapsed time.Duration) time.Duration {
	phase := elapsed % blinkPeriod
	switch {
	case phase < blinkAt:
		return blinkAt - phase
	case phase < blinkAt+halfBlink:
		return blinkAt + halfBlink - phase
	case phase < blinkAt+halfBlink+closedBlink:
		return blinkAt + halfBlink + closedBlink - phase
	case phase < blinkAt+2*halfBlink+closedBlink:
		// The blink sequence ends just after the period boundary. The
		// visible change there is the next period's open frame, not a
		// final half-closed frame.
		return min(blinkAt+2*halfBlink+closedBlink, blinkPeriod) - phase
	default:
		return blinkPeriod - phase + blinkAt
	}
}

// tapAmount returns 0 outside the reaction and a 0→1→0 squash inside it.
func tapAmount(elapsed time.Duration) float32 {
	if elapsed < 0 || elapsed >= tapDuration {
		return 0
	}
	p := float32(elapsed) / float32(tapDuration)
	return float32(math.Sin(math.Pi * float64(p)))
}

// headPatAmount is a gentler squash with a small lift, so the two reactions
// read differently without another sprite frame.
func headPatAmount(elapsed time.Duration) float32 {
	if elapsed < 0 || elapsed >= headPatDuration {
		return 0
	}
	p := float32(elapsed) / float32(headPatDuration)
	return float32(math.Sin(math.Pi*float64(p))) * 0.65
}

func reactionMotion(reaction Reaction, elapsed time.Duration) (squash, lift float32) {
	switch reaction {
	case ReactionFace:
		return tapAmount(elapsed), 0
	case ReactionHeadPat:
		squash = headPatAmount(elapsed)
		return squash, -3 * squash
	default:
		return 0, 0
	}
}

func frameFor(activity Activity, elapsed time.Duration) frame {
	switch activity {
	case ActivityAsleep:
		return frameClosed
	case ActivityDrowsy:
		return frameHalfClosed
	default:
		return frameAt(elapsed)
	}
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
