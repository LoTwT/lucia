package pet

import (
	"errors"
	"fmt"
	"image"
	"log"
	"sync/atomic"
	"time"

	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/egoist/mygo/ui"
)

const (
	animatedReduceMotionFeedback = 1200 * time.Millisecond
	// animatedResetGap is longer than the longest authored frame (1083 ms)
	// but short enough that a suspended window does not decode a long
	// backlog of frames when it becomes visible again.
	animatedResetGap = 1500 * time.Millisecond
)

type cachedIdle struct {
	frames    []*image.RGBA
	durations []time.Duration
}

type frameProvider interface {
	Len() int
	Duration(int) time.Duration
	Frame(int) (image.Image, bool)
	Failed() bool
	Reset()
	Close()
}

type cachedProvider struct {
	frames    []*image.RGBA
	durations []time.Duration
}

func (p *cachedProvider) Len() int { return len(p.frames) }
func (p *cachedProvider) Duration(i int) time.Duration {
	if i < 0 || i >= len(p.durations) {
		return 0
	}
	return p.durations[i]
}
func (p *cachedProvider) Frame(i int) (image.Image, bool) {
	if i < 0 || i >= len(p.frames) {
		return nil, false
	}
	return p.frames[i], true
}
func (p *cachedProvider) Reset() {}
func (p *cachedProvider) Close() {}
func (p *cachedProvider) Failed() bool {
	return false
}

type decoderProvider struct {
	clip    *petpack.Clip
	decoder *petpack.Decoder
	width   int
	height  int
	next    int
	last    image.Image
	err     error
}

func (p *decoderProvider) Len() int { return p.clip.FrameCount() }
func (p *decoderProvider) Duration(i int) time.Duration {
	return p.clip.Duration(i)
}
func (p *decoderProvider) Frame(i int) (image.Image, bool) {
	if p.err != nil || p.clip == nil {
		return nil, false
	}
	if p.last != nil && i == p.next-1 {
		return p.last, true
	}
	if i < p.next-1 {
		p.decoder.Reset()
		p.next = 0
		p.last = nil
	}
	for p.next <= i {
		img, _, err := p.decoder.NextScaled(p.width, p.height)
		if err != nil {
			p.err = err
			log.Printf("lucia: pet pack clip %q frame %d: %v", p.clip.ID, p.next, err)
			return nil, false
		}
		p.last = img
		p.next++
	}
	return p.last, p.last != nil
}
func (p *decoderProvider) Reset() {
	if p.decoder != nil {
		p.decoder.Reset()
	}
	p.next = 0
	p.last = nil
}
func (p *decoderProvider) Failed() bool {
	return p.err != nil
}
func (p *decoderProvider) Close() {
	if p.decoder != nil {
		p.decoder.Close()
	}
	p.last = nil
}

type playPhase uint8

const (
	phaseEnter playPhase = iota
	phaseLoop
	phaseExit
	phaseOnce
)

type clipPlay struct {
	actionID   string
	action     petpack.Action
	phase      playPhase
	provider   frameProvider
	frame      int
	frameStart time.Time
	holdUntil  time.Time
	loops      int
}

type animatedModel struct {
	pack        *petpack.Pack
	idleAction  petpack.Action
	poster      image.Image
	bitmap      *ui.Bitmap
	bitmapImage *image.RGBA
	idle        atomic.Pointer[cachedIdle]
	idleFailed  atomic.Bool

	current    *clipPlay
	restPoster image.Image

	sequence      []petpack.Step
	sequenceIndex int
	sequenceAt    time.Time

	pendingOnce string
	queuedOnce  []string
	pendingDrag bool
	pendingDrop bool
	dragActive  bool

	drowsyPlayed bool
	asleep       bool
	reduceMotion bool
	lastView     time.Time

	posters        map[string]image.Image
	reducedPosters map[string]image.Image
}

func newAnimatedModel(pack *petpack.Pack) (*animatedModel, error) {
	idleAction, ok := pack.Action(pack.Manifest.States.Idle)
	if !ok || idleAction.Loop == "" {
		return nil, fmt.Errorf("pet pack has no idle action %q", pack.Manifest.States.Idle)
	}
	if idleAction.Poster == "" {
		idleAction.Poster = idleAction.Loop + "#0"
	}
	idleClip, ok := pack.Clip(idleAction.Loop)
	if !ok {
		return nil, errNoIdleClip
	}
	poster, err := pack.Poster(idleAction)
	if err != nil {
		return nil, err
	}
	a := &animatedModel{
		pack:           pack,
		idleAction:     idleAction,
		poster:         poster,
		posters:        map[string]image.Image{pack.Manifest.States.Idle: poster},
		reducedPosters: make(map[string]image.Image),
	}
	go a.cacheIdle(idleClip)
	return a, nil
}

// NewAnimated builds a pet model backed by an imported animated pack. The
// caller keeps the built-in whale as a fallback when loading fails.
func NewAnimated(pack *petpack.Pack) (*Model, error) {
	if pack == nil {
		return nil, errors.New("pet pack is nil")
	}
	canvas := pack.Manifest.Canvas
	if !pack.Manifest.Crop.Empty() {
		canvas = petpack.Size{
			Width:  pack.Manifest.Crop.Width,
			Height: pack.Manifest.Crop.Height,
		}
	}
	layout := assetLayout{
		Canvas: dimension{Width: canvas.Width, Height: canvas.Height},
		Window: dimension{
			Width:  pack.Manifest.Base.Window.Width,
			Height: pack.Manifest.Base.Window.Height,
		},
		Sprite: dimension{
			Width:  pack.Manifest.Base.Sprite.Width,
			Height: pack.Manifest.Base.Sprite.Height,
		},
	}
	m := &Model{
		layout:         layout,
		scale:          defaultScale,
		hotzoneEntries: pack.HotzoneList(),
		bubbleAnchorX:  pack.Manifest.BubbleAnchor.X,
		bubbleAnchorY:  pack.Manifest.BubbleAnchor.Y,
	}
	if m.bubbleAnchorX == 0 && m.bubbleAnchorY == 0 {
		m.bubbleAnchorX = 0.5
		m.bubbleAnchorY = 0.05
	}
	m.applyLayout()
	animated, err := newAnimatedModel(pack)
	if err != nil {
		return nil, err
	}
	m.animated = animated
	return m, nil
}

var errNoIdleClip = errors.New("pet pack has no idle clip")

func (a *animatedModel) cacheIdle(clip *petpack.Clip) {
	frames, err := clip.DecodeAllScaled(a.pack.PixelWidth, a.pack.PixelHeight)
	if err != nil {
		log.Printf("lucia: pet pack idle cache: %v", err)
		a.idleFailed.Store(true)
		return
	}
	durations := make([]time.Duration, len(frames))
	for i := range frames {
		durations[i] = clip.Duration(i)
	}
	a.idle.Store(&cachedIdle{frames: frames, durations: durations})
}

func (a *animatedModel) idleProvider() *cachedProvider {
	cache := a.idle.Load()
	if cache == nil || len(cache.frames) == 0 {
		return nil
	}
	return &cachedProvider{frames: cache.frames, durations: cache.durations}
}

func (a *animatedModel) providerFor(clipID string) frameProvider {
	if clipID == "" {
		return nil
	}
	if a.idleAction.Loop == clipID {
		if cached := a.idleProvider(); cached != nil {
			return cached
		}
		return nil
	}
	clip, ok := a.pack.Clip(clipID)
	if !ok {
		return nil
	}
	decoder, err := clip.NewDecoder()
	if err != nil {
		log.Printf("lucia: pet pack clip %q: %v", clipID, err)
		return nil
	}
	return &decoderProvider{
		clip:    clip,
		decoder: decoder,
		width:   a.pack.PixelWidth,
		height:  a.pack.PixelHeight,
	}
}

func (a *animatedModel) startSegment(actionID string, action petpack.Action, phase playPhase, clipID string, now time.Time) bool {
	provider := a.providerFor(clipID)
	if provider == nil {
		return false
	}
	if a.current != nil && a.current.provider != nil {
		a.current.provider.Close()
	}
	a.current = &clipPlay{
		actionID:   actionID,
		action:     action,
		phase:      phase,
		provider:   provider,
		frameStart: now,
	}
	return true
}

func (a *animatedModel) startAction(actionID string, now time.Time) bool {
	action, ok := a.pack.Action(actionID)
	if !ok {
		return false
	}
	switch {
	case action.Once != "":
		return a.startSegment(actionID, action, phaseOnce, action.Once, now)
	case action.Enter != "":
		return a.startSegment(actionID, action, phaseEnter, action.Enter, now)
	case action.Loop != "":
		return a.startSegment(actionID, action, phaseLoop, action.Loop, now)
	default:
		return false
	}
}

func (a *animatedModel) startIdle(now time.Time) {
	if a.idleAction.Loop == "" {
		return
	}
	if a.startSegment(a.idleActionID(), a.idleAction, phaseLoop, a.idleAction.Loop, now) {
		a.asleep = false
	}
}

func (a *animatedModel) close() {
	if a == nil {
		return
	}
	if a.current != nil && a.current.provider != nil {
		a.current.provider.Close()
	}
	a.current = nil
	a.clearSequence()
	a.pendingOnce = ""
	a.queuedOnce = nil
	a.pendingDrag = false
	a.pendingDrop = false
	a.dragActive = false
}

func (a *animatedModel) idleActionID() string { return a.pack.Manifest.States.Idle }

func (a *animatedModel) dragActionID() string { return a.pack.Manifest.States.Drag }

func (a *animatedModel) requestInteraction(interactionID string) bool {
	if a.reduceMotion || interactionID == "" {
		return false
	}
	interaction, ok := a.pack.Interaction(interactionID)
	if !ok {
		return false
	}
	steps := append([]petpack.Step(nil), interaction.Steps...)
	if len(steps) == 0 {
		// Feedback-only interactions (bubble, sound, squash/lift) do not
		// interrupt a running clip. The host still plays them immediately.
		return true
	}
	a.sequence = steps
	a.sequenceIndex = 0
	a.sequenceAt = time.Time{}
	a.pendingOnce = ""
	a.queuedOnce = nil
	return true
}

func (a *animatedModel) clearSequence() {
	a.sequence = nil
	a.sequenceIndex = 0
	a.sequenceAt = time.Time{}
}

func (a *animatedModel) startNextSequence(now time.Time) bool {
	for a.sequenceIndex < len(a.sequence) {
		step := a.sequence[a.sequenceIndex]
		a.sequenceIndex++
		if step.Action == "" {
			continue
		}
		if a.startAction(step.Action, now) {
			a.sequenceAt = time.Time{}
			return true
		}
	}
	a.clearSequence()
	return false
}

func (a *animatedModel) requestAction(id string) {
	if a.reduceMotion || id == "" {
		return
	}
	a.clearSequence()
	// The first-launch intro can be requested before the first View syncs.
	// Background update notifications must queue behind it instead of
	// replacing the only chance to play the intro.
	intro := ""
	if a.pack != nil {
		intro = a.pack.Manifest.Events.Intro
	}
	if intro != "" && a.pendingOnce == intro && id != intro {
		for _, queued := range a.queuedOnce {
			if queued == id {
				return
			}
		}
		a.queuedOnce = append(a.queuedOnce, id)
		return
	}
	a.pendingOnce = id
}

func (a *animatedModel) requestBackgroundAction(id string) {
	if a.asleep {
		return
	}
	a.requestAction(id)
}

func (a *animatedModel) requestDragStart() {
	if a.reduceMotion || a.dragActionID() == "" {
		return
	}
	a.clearSequence()
	a.pendingDrag = true
	a.pendingDrop = false
	a.pendingOnce = ""
}

func (a *animatedModel) requestDragEnd() {
	if a.reduceMotion || a.dragActionID() == "" {
		return
	}
	a.pendingDrop = true
}

func (a *animatedModel) sync(now time.Time, activity Activity, idle time.Duration, dragging bool, reduceMotion bool) {
	if !a.lastView.IsZero() && now.Sub(a.lastView) > animatedResetGap &&
		!dragging && !a.dragActive && !a.pendingDrop {
		// A hidden or suspended window must not replay a long backlog of
		// frames when it becomes visible again. Drop the current clip and
		// resume from idle; pending user actions still run below. Keep a
		// completed drag alive so its drop_exit is not skipped.
		if a.current != nil && a.current.provider != nil {
			a.current.provider.Close()
		}
		a.current = nil
		a.dragActive = false
		a.pendingDrop = false
		a.restPoster = nil
	}
	a.lastView = now
	if reduceMotion {
		a.reduceMotion = true
		if a.current != nil {
			if a.current.provider != nil {
				a.current.provider.Close()
			}
			a.current = nil
		}
		a.pendingDrag = false
		a.pendingDrop = false
		a.dragActive = false
		a.pendingOnce = ""
		a.queuedOnce = nil
		a.clearSequence()
		a.restPoster = nil
		a.asleep = activity == ActivityAsleep
		return
	}
	if a.reduceMotion {
		a.reduceMotion = false
		a.current = nil
		a.restPoster = nil
	}
	a.asleep = false
	a.restPoster = nil
	idleID := a.idleActionID()
	dragID := a.dragActionID()

	if a.pendingDrop && a.dragActive {
		a.dragActive = false
		a.pendingDrop = false
		a.startExit(dragID, now)
		return
	}
	if a.pendingDrag && !dragging {
		// The drag started and ended between two frames; skip the grab
		// segment and go straight to the drop.
		a.pendingDrag = false
		a.pendingDrop = dragID != ""
	}
	if a.pendingDrop && !a.dragActive {
		a.pendingDrop = false
		a.startExit(dragID, now)
		a.advance(now)
		return
	}
	if a.pendingDrag && dragging {
		a.pendingDrag = false
		if dragID != "" && a.startAction(dragID, now) {
			a.dragActive = true
		}
	}

	if a.dragActive {
		if !dragging {
			a.dragActive = false
			a.startExit(dragID, now)
			return
		}
		if a.current == nil || a.current.actionID != dragID || a.current.phase == phaseExit {
			a.startAction(dragID, now)
		}
		a.advance(now)
		return
	}

	if a.sequenceIndex < len(a.sequence) && (a.current == nil || a.current.actionID == idleID) {
		// The first step's delay starts when the interaction is requested.
		// Later delays are scheduled by finishCurrent; keeping idle alive
		// while waiting avoids freezing on a static poster.
		if a.sequenceAt.IsZero() && a.sequenceIndex == 0 {
			if delay := a.sequence[0].DelayMs; delay > 0 {
				a.sequenceAt = now.Add(time.Duration(delay) * time.Millisecond)
				if a.current == nil {
					a.startIdle(now)
				}
			}
		}
		if a.sequenceAt.IsZero() || !now.Before(a.sequenceAt) {
			if a.startNextSequence(now) {
				a.advance(now)
				return
			}
		}
	}

	if a.pendingOnce != "" && a.current != nil && a.current.actionID == a.pendingOnce {
		a.pendingOnce = ""
	}
	if a.pendingOnce != "" && (a.current == nil || a.current.actionID == idleID) {
		id := a.pendingOnce
		a.pendingOnce = ""
		a.startAction(id, now)
	}

	if activity == ActivityAwake {
		a.drowsyPlayed = false
		a.asleep = false
		if a.current != nil && (a.current.actionID == a.pack.Manifest.States.Drowsy ||
			a.current.actionID == a.pack.Manifest.States.Asleep) {
			a.current = nil
		}
	}
	if activity == ActivityAsleep {
		a.enterAsleep(now)
		return
	}
	if activity == ActivityDrowsy {
		drowsyID := a.pack.Manifest.States.Drowsy
		if !a.drowsyPlayed && (a.current == nil || a.current.actionID == idleID) {
			a.drowsyPlayed = true
			if drowsyID != "" && a.startAction(drowsyID, now) {
				a.advance(now)
				return
			}
		}
		// A drowsy action with clips plays once and returns to idle. Only a
		// poster-only action holds a static pose for the whole drowsy period.
		if drowsyID != "" && (a.current == nil || a.current.actionID == idleID) {
			if action, ok := a.pack.Action(drowsyID); ok && !actionHasClip(action) &&
				a.holdStatePoster(drowsyID) {
				return
			}
		}
	}
	if a.current == nil {
		a.startIdle(now)
	}
	a.advance(now)
}

func actionHasClip(action petpack.Action) bool {
	return action.Once != "" || action.Enter != "" || action.Loop != "" || action.Exit != ""
}

// holdStatePoster displays an action's poster without playing a clip. Drowsy
// and asleep states use this when a pack only declares a static pose.
func (a *animatedModel) holdStatePoster(actionID string) bool {
	action, ok := a.pack.Action(actionID)
	if !ok || action.Poster == "" {
		return false
	}
	if a.current != nil {
		if a.current.provider != nil {
			a.current.provider.Close()
		}
		a.current = nil
	}
	a.restPoster = a.posterFor(actionID)
	return a.restPoster != nil
}

func (a *animatedModel) enterAsleep(now time.Time) {
	if a.current != nil && a.current.provider != nil {
		a.current.provider.Close()
	}
	a.current = nil
	a.asleep = true
	id := a.pack.Manifest.States.Asleep
	if id == "" {
		return
	}
	action, ok := a.pack.Action(id)
	if !ok {
		return
	}
	if action.LoopCount < 0 && action.Loop != "" {
		if a.startAction(id, now) {
			a.advance(now)
		}
		return
	}
	if action.Poster != "" {
		a.restPoster = a.posterFor(id)
	}
}

func (a *animatedModel) startExit(actionID string, now time.Time) {
	action, ok := a.pack.Action(actionID)
	if !ok || action.Exit == "" {
		a.current = nil
		a.startIdle(now)
		return
	}
	if !a.startSegment(actionID, action, phaseExit, action.Exit, now) {
		a.startIdle(now)
	}
}

func (a *animatedModel) advance(now time.Time) {
	for a.current != nil {
		current := a.current
		if current.provider != nil && current.provider.Failed() {
			a.finishCurrent(now)
			return
		}
		duration := current.provider.Duration(current.frame)
		if duration <= 0 {
			duration = 33 * time.Millisecond
		}
		if now.Sub(current.frameStart) < duration {
			return
		}
		if current.frame+1 < current.provider.Len() {
			current.frameStart = current.frameStart.Add(duration)
			current.frame++
			continue
		}
		// The last frame is on screen. A pack can hold it before entering
		// the next phase; this is the asset-level timing beat.
		if current.holdUntil.IsZero() {
			if hold := actionHold(current.action, current.phase); hold > 0 {
				current.holdUntil = current.frameStart.Add(duration).Add(hold)
				return
			}
		} else if now.Before(current.holdUntil) {
			return
		} else {
			current.holdUntil = time.Time{}
		}
		switch current.phase {
		case phaseEnter:
			if current.action.Loop != "" && current.action.LoopCount != 0 {
				if a.startSegment(current.actionID, current.action, phaseLoop, current.action.Loop, now) {
					continue
				}
			}
			if current.action.Exit != "" && a.startSegment(current.actionID, current.action, phaseExit, current.action.Exit, now) {
				continue
			}
			a.finishCurrent(now)
			return
		case phaseLoop:
			if current.action.LoopCount < 0 {
				current.frame = 0
				current.frameStart = now
				current.provider.Reset()
				continue
			}
			current.loops++
			if current.loops < current.action.LoopCount {
				current.frame = 0
				current.frameStart = now
				current.provider.Reset()
				continue
			}
			if current.action.Exit != "" && a.startSegment(current.actionID, current.action, phaseExit, current.action.Exit, now) {
				continue
			}
			a.finishCurrent(now)
			return
		case phaseExit, phaseOnce:
			a.finishCurrent(now)
			return
		}
	}
}

func actionHold(action petpack.Action, phase playPhase) time.Duration {
	var millis int
	switch phase {
	case phaseEnter:
		millis = action.Timing.EnterHoldMs
	case phaseLoop:
		millis = action.Timing.LoopHoldMs
	case phaseExit, phaseOnce:
		millis = action.Timing.ExitHoldMs
	}
	if millis <= 0 {
		return 0
	}
	return time.Duration(millis) * time.Millisecond
}

func (a *animatedModel) finishCurrent(now time.Time) {
	if a.current != nil && a.current.provider != nil {
		a.current.provider.Close()
	}
	a.current = nil
	if a.sequenceIndex < len(a.sequence) {
		if delay := a.sequence[a.sequenceIndex].DelayMs; delay > 0 {
			a.sequenceAt = now.Add(time.Duration(delay) * time.Millisecond)
			a.startIdle(now)
		} else {
			a.sequenceAt = time.Time{}
		}
		return
	}
	a.clearSequence()
	if a.pendingOnce != "" {
		id := a.pendingOnce
		a.pendingOnce = ""
		if a.startAction(id, now) {
			return
		}
	}
	if len(a.queuedOnce) > 0 {
		id := a.queuedOnce[0]
		a.queuedOnce = a.queuedOnce[1:]
		if a.startAction(id, now) {
			return
		}
	}
	a.startIdle(now)
}

func (a *animatedModel) currentImage() image.Image {
	if a.current != nil && a.current.provider != nil {
		if img, ok := a.current.provider.Frame(a.current.frame); ok && img != nil {
			return img
		}
	}
	if a.restPoster != nil {
		return a.restPoster
	}
	return a.poster
}

func (a *animatedModel) imageFor(now time.Time, activity Activity, dragging bool, interactionID string, interactionAt time.Time, reduceMotion bool) image.Image {
	if reduceMotion {
		return a.reducedImage(activity, dragging, interactionID, interactionAt, now)
	}
	return a.currentImage()
}

func (a *animatedModel) bitmapFor(img image.Image) *ui.Bitmap {
	if a.bitmap == nil {
		a.bitmap = ui.NewBitmap(img)
		a.bitmapImage, _ = img.(*image.RGBA)
		return a.bitmap
	}
	if next, ok := img.(*image.RGBA); ok && a.bitmapImage == next {
		// The current frame is unchanged; updating the texture again would
		// only add work on the UI thread.
		return a.bitmap
	}
	a.bitmap.Update(img)
	a.bitmapImage, _ = img.(*image.RGBA)
	return a.bitmap
}

func (a *animatedModel) nextDelay(now time.Time, activity Activity, idle time.Duration, interactionAt time.Time, reduceMotion bool) time.Duration {
	if reduceMotion {
		if a.reactionActive(now, interactionAt) {
			return max(animatedReduceMotionFeedback-now.Sub(interactionAt), time.Millisecond)
		}
		switch activity {
		case ActivityAsleep:
			return 0
		case ActivityDrowsy:
			return max(asleepAfter-idle, time.Millisecond)
		default:
			return max(drowsyAfter-idle, 0)
		}
	}
	base := a.clipDelay(now, activity)
	if a.reactionActive(now, interactionAt) {
		return activeFrameInterval
	}
	if a.idleMotionBob() {
		interval := idleFrameInterval
		switch activity {
		case ActivityDrowsy:
			interval = drowsyFrameInterval
		case ActivityAsleep:
			interval = asleepFrameInterval
		}
		if base == 0 || interval < base {
			return interval
		}
		return base
	}
	if a.asleep {
		return 0
	}
	return base
}

func (a *animatedModel) clipDelay(now time.Time, activity Activity) time.Duration {
	idleID := a.idleActionID()
	if a.sequenceIndex < len(a.sequence) && (a.current == nil || a.current.actionID == idleID) {
		wait := time.Millisecond
		if !a.sequenceAt.IsZero() {
			wait = max(a.sequenceAt.Sub(now), time.Millisecond)
		}
		if a.current != nil && a.current.provider != nil {
			duration := a.current.provider.Duration(a.current.frame)
			if duration <= 0 {
				duration = 33 * time.Millisecond
			}
			idleWait := max(duration-now.Sub(a.current.frameStart), time.Millisecond)
			if idleWait < wait {
				return idleWait
			}
		}
		return wait
	}
	if a.current != nil && a.current.provider != nil {
		if a.current.provider.Failed() {
			// Ask for one more frame so sync can finish the broken clip and
			// return to idle. Returning zero here would freeze on the poster.
			return time.Millisecond
		}
		if !a.current.holdUntil.IsZero() {
			return max(a.current.holdUntil.Sub(now), time.Millisecond)
		}
		duration := a.current.provider.Duration(a.current.frame)
		if duration <= 0 {
			duration = 33 * time.Millisecond
		}
		return max(duration-now.Sub(a.current.frameStart), time.Millisecond)
	}
	if a.idleFailed.Load() || activity == ActivityAsleep {
		return 0
	}
	return 100 * time.Millisecond
}

func (a *animatedModel) idleMotionBob() bool {
	return a.pack.Manifest.IdleMotion == "bob"
}

func (a *animatedModel) reactionActive(now, reactionAt time.Time) bool {
	return !reactionAt.IsZero() && now.Sub(reactionAt) < animatedReduceMotionFeedback
}

func (a *animatedModel) hotzonesClickable(dragging bool) bool {
	if dragging {
		return false
	}
	if a.current == nil {
		return true
	}
	return a.current.action.Interruptible
}

func (a *animatedModel) reducedImage(activity Activity, dragging bool, interactionID string, interactionAt time.Time, now time.Time) image.Image {
	if dragging && a.dragActionID() != "" {
		if poster := a.posterFor(a.dragActionID()); poster != nil {
			return poster
		}
	}
	if interactionID != "" && !interactionAt.IsZero() && now.Sub(interactionAt) < animatedReduceMotionFeedback {
		if interaction, ok := a.pack.Interaction(interactionID); ok && interaction.ReducedPoster != "" {
			if poster := a.reducedPoster(interaction.ReducedPoster); poster != nil {
				return poster
			}
		}
	}
	switch activity {
	case ActivityDrowsy:
		if id := a.pack.Manifest.States.Drowsy; id != "" {
			if poster := a.posterFor(id); poster != nil {
				return poster
			}
		}
	case ActivityAsleep:
		if id := a.pack.Manifest.States.Asleep; id != "" {
			if poster := a.posterFor(id); poster != nil {
				return poster
			}
		}
	}
	return a.poster
}

func (a *animatedModel) posterFor(actionID string) image.Image {
	if poster, ok := a.posters[actionID]; ok {
		return poster
	}
	action, ok := a.pack.Action(actionID)
	if !ok || action.Poster == "" {
		return nil
	}
	poster, err := a.pack.Poster(action)
	if err != nil {
		log.Printf("lucia: pet pack poster %q: %v", actionID, err)
		return nil
	}
	a.posters[actionID] = poster
	return poster
}

func (a *animatedModel) reducedPoster(poster string) image.Image {
	if cached, ok := a.reducedPosters[poster]; ok {
		return cached
	}
	img, err := a.pack.PosterFrame(poster)
	if err != nil {
		log.Printf("lucia: pet pack reduced poster %q: %v", poster, err)
		return nil
	}
	a.reducedPosters[poster] = img
	return img
}

func (a *animatedModel) canPlayActions() bool {
	return !a.reduceMotion && !a.dragActive
}
