package pet

import (
	"errors"
	"image"
	"log"
	"sort"
	"sync/atomic"
	"time"

	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/egoist/mygo/ui"
)

const (
	animatedReduceMotionReaction = 1200 * time.Millisecond
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
	loops      int
}

// AnimatedAction is one manual action exposed by the pet's context menu.
type AnimatedAction struct {
	ID    string
	Label string
}

type animatedModel struct {
	pack        *petpack.Pack
	idleAction  petpack.Action
	poster      image.Image
	bitmap      *ui.Bitmap
	bitmapImage *image.RGBA
	idle        atomic.Pointer[cachedIdle]
	idleFailed  atomic.Bool

	current *clipPlay

	pendingOnce     string
	queuedOnce      []string
	pendingReaction Reaction
	pendingDrag     bool
	pendingDrop     bool
	dragActive      bool

	drowsyPlayed bool
	asleep       bool
	reduceMotion bool
	lastView     time.Time

	posters map[string]image.Image
}

func newAnimatedModel(pack *petpack.Pack) (*animatedModel, error) {
	idleAction, ok := pack.Action("idle")
	if !ok || idleAction.Loop == "" {
		idleAction = petpack.Action{
			Loop:          "idle_loop",
			Poster:        "idle_loop#0",
			FaceClickable: true,
			LoopCount:     -1,
		}
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
		pack:       pack,
		idleAction: idleAction,
		poster:     poster,
		posters:    map[string]image.Image{"idle": poster},
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
	layout.Face.X = pack.Manifest.Face.X
	layout.Face.Y = pack.Manifest.Face.Y
	layout.Face.Width = pack.Manifest.Face.Width
	layout.Face.Height = pack.Manifest.Face.Height
	m := &Model{
		layout:        layout,
		scale:         defaultScale,
		bubbleAnchorX: pack.Manifest.BubbleAnchor.X,
		bubbleAnchorY: pack.Manifest.BubbleAnchor.Y,
	}
	if m.bubbleAnchorX == 0 && m.bubbleAnchorY == 0 {
		m.bubbleAnchorX = 0.5
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
	if a.startSegment("idle", a.idleAction, phaseLoop, a.idleAction.Loop, now) {
		a.asleep = false
	}
}

func (a *animatedModel) requestReaction(reaction Reaction) {
	if reaction == ReactionNone || a.reduceMotion {
		return
	}
	a.pendingReaction = reaction
	a.pendingOnce = ""
}

func (a *animatedModel) requestAction(id string) {
	if a.reduceMotion || id == "" {
		return
	}
	// The first-launch wave can be requested before the first View syncs.
	// Background update notifications must queue behind it instead of
	// replacing the only chance to play the intro.
	if a.pendingOnce == "wave" && id != "wave" {
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
	if a.reduceMotion {
		return
	}
	a.pendingDrag = true
	a.pendingDrop = false
	a.pendingReaction = ReactionNone
	a.pendingOnce = ""
}

func (a *animatedModel) requestDragEnd() {
	if a.reduceMotion {
		return
	}
	a.pendingDrop = true
}

func (a *animatedModel) sync(now time.Time, activity Activity, idle time.Duration, dragging bool, reaction Reaction, reactionAt time.Time, reduceMotion bool) {
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
		a.pendingReaction = ReactionNone
		a.pendingOnce = ""
		a.queuedOnce = nil
		a.asleep = activity == ActivityAsleep
		return
	}
	if a.reduceMotion {
		a.reduceMotion = false
		a.current = nil
	}
	a.asleep = false

	if a.pendingDrop && a.dragActive {
		a.dragActive = false
		a.pendingDrop = false
		a.startExit("drag", now)
		return
	}
	if a.pendingDrag && !dragging {
		// The drag started and ended between two frames; skip the grab
		// segment and go straight to the drop.
		a.pendingDrag = false
		a.pendingDrop = true
	}
	if a.pendingDrop && !a.dragActive {
		a.pendingDrop = false
		a.startExit("drag", now)
		a.advance(now)
		return
	}
	if a.pendingDrag && dragging {
		a.pendingDrag = false
		a.dragActive = true
		a.startAction("drag", now)
	}

	if a.dragActive {
		if !dragging {
			a.dragActive = false
			a.startExit("drag", now)
			return
		}
		if a.current == nil || a.current.actionID != "drag" || a.current.phase == phaseExit {
			a.startAction("drag", now)
		}
		a.advance(now)
		return
	}

	if a.pendingReaction != ReactionNone {
		actionID := "happy"
		if a.pendingReaction == ReactionFace {
			actionID = "laugh"
		}
		a.startAction(actionID, now)
		a.pendingReaction = ReactionNone
	}
	if a.pendingOnce != "" && a.current != nil && a.current.actionID == a.pendingOnce {
		a.pendingOnce = ""
	}
	if a.pendingOnce != "" && (a.current == nil || a.current.actionID == "idle") {
		id := a.pendingOnce
		a.pendingOnce = ""
		a.startAction(id, now)
	}

	if activity == ActivityAwake {
		a.drowsyPlayed = false
		a.asleep = false
	}
	if activity == ActivityAsleep && (a.current == nil || a.current.actionID == "idle") {
		if a.current != nil && a.current.provider != nil {
			a.current.provider.Close()
		}
		a.current = nil
		a.asleep = true
		return
	}
	if activity == ActivityDrowsy && !a.drowsyPlayed && (a.current == nil || a.current.actionID == "idle") {
		a.drowsyPlayed = true
		if a.startAction("sleep_denial", now) {
			a.advance(now)
			return
		}
	}
	if a.current == nil {
		a.startIdle(now)
	}
	a.advance(now)
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
		current.frameStart = current.frameStart.Add(duration)
		current.frame++
		if current.frame < current.provider.Len() {
			continue
		}
		switch current.phase {
		case phaseEnter:
			if current.action.Loop != "" && current.action.LoopCount != 0 {
				if a.startSegment(current.actionID, current.action, phaseLoop, current.action.Loop, current.frameStart) {
					continue
				}
			}
			if current.action.Exit != "" && a.startSegment(current.actionID, current.action, phaseExit, current.action.Exit, current.frameStart) {
				continue
			}
			a.finishCurrent(now)
			return
		case phaseLoop:
			if current.action.LoopCount < 0 {
				current.frame = 0
				current.provider.Reset()
				continue
			}
			current.loops++
			if current.loops < current.action.LoopCount {
				current.frame = 0
				current.provider.Reset()
				continue
			}
			if current.action.Exit != "" && a.startSegment(current.actionID, current.action, phaseExit, current.action.Exit, current.frameStart) {
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

func (a *animatedModel) finishCurrent(now time.Time) {
	if a.current != nil && a.current.provider != nil {
		a.current.provider.Close()
	}
	a.current = nil
	if a.pendingReaction != ReactionNone {
		actionID := "happy"
		if a.pendingReaction == ReactionFace {
			actionID = "laugh"
		}
		a.startAction(actionID, now)
		a.pendingReaction = ReactionNone
		return
	}
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
	return a.poster
}

func (a *animatedModel) imageFor(now time.Time, activity Activity, dragging bool, reaction Reaction, reactionAt time.Time, reduceMotion bool) image.Image {
	if reduceMotion {
		return a.reducedImage(activity, dragging, reaction, reactionAt, now)
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

func (a *animatedModel) nextDelay(now time.Time, activity Activity, idle time.Duration, reactionAt time.Time, reduceMotion bool) time.Duration {
	if reduceMotion {
		if a.reactionActive(now, reactionAt) {
			return max(animatedReduceMotionReaction-now.Sub(reactionAt), time.Millisecond)
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
	if a.asleep {
		return 0
	}
	if a.current != nil && a.current.provider != nil {
		if a.current.provider.Failed() {
			// Stop asking for frames after a decode error. The next sync or
			// interaction will finish the broken clip and return to idle.
			return 0
		}
		duration := a.current.provider.Duration(a.current.frame)
		if duration <= 0 {
			duration = 33 * time.Millisecond
		}
		return max(duration-now.Sub(a.current.frameStart), time.Millisecond)
	}
	if a.idleFailed.Load() {
		return 0
	}
	return 100 * time.Millisecond
}

func (a *animatedModel) reactionActive(now, reactionAt time.Time) bool {
	return !reactionAt.IsZero() && now.Sub(reactionAt) < animatedReduceMotionReaction
}

func (a *animatedModel) faceClickable(dragging bool) bool {
	if dragging {
		return false
	}
	if a.current == nil {
		return true
	}
	return a.current.action.FaceClickable
}

func (a *animatedModel) reducedImage(activity Activity, dragging bool, reaction Reaction, reactionAt time.Time, now time.Time) image.Image {
	if dragging {
		if poster := a.posterFor("drag"); poster != nil {
			return poster
		}
	}
	if reaction != ReactionNone && !reactionAt.IsZero() && now.Sub(reactionAt) < animatedReduceMotionReaction {
		actionID := "happy"
		if reaction == ReactionFace {
			actionID = "laugh"
		}
		if poster := a.posterFor(actionID); poster != nil {
			return poster
		}
	}
	if activity == ActivityDrowsy {
		if poster := a.posterFor("sleep_denial"); poster != nil {
			return poster
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

func (a *animatedModel) onceActions() []AnimatedAction {
	order := []string{"belly_taunt", "pretend_deaf", "smug_stumble", "sleep_denial", "play_innocent", "edge_peek"}
	var out []AnimatedAction
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if action, ok := a.pack.Action(id); ok && action.Once != "" && action.Label != "" {
			out = append(out, AnimatedAction{ID: id, Label: action.Label})
			seen[id] = true
		}
	}
	var rest []AnimatedAction
	for id, action := range a.pack.Manifest.Actions {
		if seen[id] || action.Once == "" || action.Label == "" {
			continue
		}
		rest = append(rest, AnimatedAction{ID: id, Label: action.Label})
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].ID < rest[j].ID })
	return append(out, rest...)
}

func (a *animatedModel) canPlayActions() bool {
	return !a.reduceMotion && !a.dragActive
}
