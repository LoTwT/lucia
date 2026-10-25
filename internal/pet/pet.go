// Package pet renders Lucia's desktop pet and keeps its animation state.
// The window wiring and desktop lifecycle live in cmd/lucia; this package
// only owns the character's frames and the view drawn on the UI thread.
package pet

import (
	"math"
	"time"

	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/egoist/mygo/ui"
)

const (
	// bobPeriod and bobAmplitude make the idle character breathe without
	// moving the window, which would fight the OS drag.
	bobPeriod    = 3200 * time.Millisecond
	bobAmplitude = 2.5
	// bounceDuration is the quick upward reaction to a click.
	bounceDuration = 360 * time.Millisecond
	// headPatDuration is the gentler, longer reaction to the menu action.
	headPatDuration = 520 * time.Millisecond
	// Both interaction motions are vertical-only. Scaling the whole sprite
	// reads as squashing the artwork and fights packs that already animate
	// the same interaction.
	bounceAmplitude  = 4
	headPatAmplitude = 3
	// The pet gets drowsy and then falls asleep after this much time since
	// the last interaction. The view clock decides the state; no timer runs.
	drowsyAfter = 60 * time.Second
	asleepAfter = 3 * time.Minute
	// Frame intervals replace an unconditional AnimationFrame. The breathing
	// bob is slow, so 30 fps is enough while awake; interactions need more
	// frames, and resting states can redraw far less often.
	activeFrameInterval = 16 * time.Millisecond
	idleFrameInterval   = 33 * time.Millisecond
	drowsyFrameInterval = 66 * time.Millisecond
	asleepFrameInterval = 100 * time.Millisecond
	minFrameInterval    = time.Millisecond
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
	layout          assetLayout
	scale           float64
	windowWidth     int
	windowHeight    int
	spriteWidth     int
	spriteHeight    int
	imageRect       ui.Rect
	hotzones        []Hotzone
	hotzoneEntries  []petpack.HotzoneEntry
	bubbleAnchorX   float64
	bubbleAnchorY   float64
	bubbleRect      ui.Rect
	start           time.Time
	lastInteraction time.Time
	interacted      bool
	dragging        bool

	interactionID     string
	interactionMotion string
	interactionAt     time.Time
	interactionReady  bool
	bubbleSeq         map[string]int
	onBubble          func(BubbleEvent)
	onInvalidate      func()
	menuActions       MenuActions
	animated          *animatedModel
}

// Hotzone is one clickable region in window-local DIPs.
type Hotzone struct {
	ID          string
	Label       string
	Interaction string
	Rect        ui.Rect
}

// BubbleEvent is one authored line plus whether the interaction asks for the
// host's short sound.
type BubbleEvent struct {
	Text  string
	Sound bool
}

// MenuActions are the application-level actions offered by the pet's
// context menu. Nil fields hide their item, which keeps the pure pet view
// usable in tests without the application shell.
type MenuActions struct {
	OpenSettings func()
	Hide         func()
	Reset        func()
}

// SetBubbleHandler installs the UI-thread callback for authored bubble lines.
func (m *Model) SetBubbleHandler(fn func(BubbleEvent)) { m.onBubble = fn }

// SetInvalidator asks for a fresh frame when state changes outside the view,
// such as a click or the start of a drag.
func (m *Model) SetInvalidator(fn func()) { m.onInvalidate = fn }

func (m *Model) invalidate() {
	if m.onInvalidate != nil {
		m.onInvalidate()
	}
}

// SetMenuActions installs the application actions shown in the context
// menu. The pet package does not own windows, settings, or visibility.
func (m *Model) SetMenuActions(actions MenuActions) { m.menuActions = actions }

// TriggerHotzone starts the interaction attached to a clickable region.
func (m *Model) TriggerHotzone(id string) {
	for _, candidate := range m.Hotzones() {
		if candidate.ID == id {
			m.TriggerInteraction(candidate.Interaction)
			return
		}
	}
}

// TriggerInteraction is the menu and hotzone entry for a named interaction.
func (m *Model) TriggerInteraction(id string) {
	if id == "" || m.animated == nil {
		return
	}
	interaction, ok := m.animated.pack.Interaction(id)
	if !ok {
		return
	}
	m.interactionID = id
	m.interactionMotion = interaction.Motion
	m.interactionReady = true
	m.interacted = true
	m.animated.requestInteraction(id)
	m.showBubble(interaction)
	m.invalidate()
}

func (m *Model) showBubble(interaction petpack.Interaction) {
	if m.onBubble == nil {
		return
	}
	var text string
	if interaction.Bubble != "" && interaction.Bubble != "none" {
		lines := m.animated.pack.Manifest.Bubbles[interaction.Bubble]
		if len(lines) > 0 {
			if m.bubbleSeq == nil {
				m.bubbleSeq = make(map[string]int)
			}
			index := m.bubbleSeq[interaction.Bubble] % len(lines)
			m.bubbleSeq[interaction.Bubble]++
			text = lines[index]
		}
	}
	if text == "" && !interaction.Sound {
		return
	}
	m.onBubble(BubbleEvent{Text: text, Sound: interaction.Sound})
}

// HasEvent reports whether the current pack maps an application event.
func (m *Model) HasEvent(name string) bool {
	return m.animated != nil && m.animated.pack.Manifest.Events.Action(name) != ""
}

// TriggerEvent plays an event mapped by the current pack.
func (m *Model) TriggerEvent(name string) {
	if m.animated == nil {
		return
	}
	id := m.animated.pack.Manifest.Events.Action(name)
	if id == "" {
		return
	}
	m.interacted = true
	m.animated.requestAction(id)
	m.invalidate()
}

// TriggerBackgroundEvent plays an update-status event without waking a
// sleeping pet.
func (m *Model) TriggerBackgroundEvent(name string) {
	if m.animated == nil || m.animated.asleep {
		return
	}
	id := m.animated.pack.Manifest.Events.Action(name)
	if id == "" {
		return
	}
	m.animated.requestBackgroundAction(id)
	m.invalidate()
}

// TriggerAction starts a one-shot action exposed by the pack menu.
func (m *Model) TriggerAction(id string) {
	if m.animated == nil || id == "" {
		return
	}
	m.interacted = true
	m.animated.requestAction(id)
	m.invalidate()
}

// PlayAction is kept as a test-friendly alias for TriggerAction.
func (m *Model) PlayAction(id string) { m.TriggerAction(id) }

// Close releases the current clip decoder.
func (m *Model) Close() {
	if m != nil && m.animated != nil {
		m.animated.close()
	}
}

// DragStarted and DragEnded keep the pet awake for the whole native drag.
// The window callback calls them on the UI thread.
func (m *Model) DragStarted() {
	m.dragging = true
	m.interacted = true
	if m.animated != nil {
		m.animated.requestDragStart()
	}
	m.invalidate()
}

func (m *Model) DragEnded() {
	m.dragging = false
	m.interacted = true
	if m.animated != nil {
		m.animated.requestDragEnd()
	}
	m.invalidate()
}

func (m *Model) activity(now time.Time) Activity {
	if m.dragging || m.lastInteraction.IsZero() {
		return ActivityAwake
	}
	return activityAt(now.Sub(m.lastInteraction))
}

// View draws the pet on a transparent root. Hotzones are click targets and
// the rest of the window is a DragWindow handle.
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
	if m.interactionReady {
		m.interactionAt = now
		m.interactionReady = false
	}
	elapsed := now.Sub(m.start)
	idle := now.Sub(m.lastInteraction)
	activity := m.activity(now)
	reduceMotion := c.Preferences().ReduceMotion
	bob, interactionLift := animatedMotion(reduceMotion, elapsed,
		m.animated.pack.Manifest.IdleMotion, m.interactionMotion, m.interactionAt, now)
	m.animated.sync(now, activity, idle, m.dragging, reduceMotion)

	c.Root().Background(ui.Transparent)

	outer := ui.Box(c).Fill().DragWindow()
	outer.ContextMenu(func(menu *ui.Menu) {
		addedPackItem := false
		for _, entry := range m.animated.pack.Manifest.Menu {
			if entry.Interaction != "" {
				interaction, ok := m.animated.pack.Interaction(entry.Interaction)
				if !ok || interaction.Label == "" {
					continue
				}
				addedPackItem = true
				if menu.Item(interaction.Label).Chosen() {
					m.TriggerInteraction(entry.Interaction)
				}
				continue
			}
			if entry.Action == "" || !m.animated.canPlayActions() {
				continue
			}
			action, ok := m.animated.pack.Action(entry.Action)
			if !ok || action.Label == "" {
				continue
			}
			addedPackItem = true
			if menu.Item(action.Label).Chosen() {
				m.TriggerAction(entry.Action)
			}
		}
		if m.menuActions.OpenSettings == nil && m.menuActions.Hide == nil && m.menuActions.Reset == nil {
			return
		}
		if addedPackItem {
			menu.Separator()
		}
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
			image := ui.Image(c, m.animated.bitmapFor(
				m.animated.imageFor(now, activity, m.dragging, m.interactionID, m.interactionAt, reduceMotion),
			)).
				Size(float32(m.spriteWidth), float32(m.spriteHeight)).
				Fit(ui.Contain).
				Label("Lucia 桌宠")
			image.Translate(0, bob+interactionLift)
		})
		if m.animated.hotzonesClickable(m.dragging) {
			for _, zone := range m.Hotzones() {
				zone := zone
				ui.Box(c).Absolute().
					Left(zone.Rect.X).Top(zone.Rect.Y).
					Size(zone.Rect.W, zone.Rect.H).
					Label(zone.Label).
					Cursor(ui.CursorPointer).
					OnClick(func() { m.TriggerHotzone(zone.ID) })
			}
		}
	})

	if delay := m.animated.nextDelay(now, activity, idle, m.interactionAt, reduceMotion); delay > 0 {
		c.After(delay)
	}
}

func animatedMotion(reduceMotion bool, elapsed time.Duration, idleMotion, interactionMotion string, interactionAt, now time.Time) (bob, lift float32) {
	if reduceMotion {
		return 0, 0
	}
	if idleMotion == "bob" {
		bob = float32(math.Sin(2*math.Pi*float64(elapsed)/float64(bobPeriod))) * bobAmplitude
	}
	motionElapsed := now.Sub(interactionAt)
	switch interactionMotion {
	case "bounce", "squash":
		// "squash" is a legacy name kept for packs authored before the
		// motion stopped scaling the sprite.
		lift = -bounceAmplitude * pulse(motionElapsed, bounceDuration)
	case "lift":
		lift = -headPatAmplitude * pulse(motionElapsed, headPatDuration)
	}
	return bob, lift
}

// pulse returns 0 outside the reaction and a 0→1→0 envelope inside it.
func pulse(elapsed, duration time.Duration) float32 {
	if elapsed < 0 || elapsed >= duration {
		return 0
	}
	p := float32(elapsed) / float32(duration)
	return float32(math.Sin(math.Pi * float64(p)))
}
