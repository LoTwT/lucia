package pet

import (
	"bytes"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFrameAtFollowsBlink(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    frame
	}{
		{name: "open at start", elapsed: 0, want: frameOpen},
		{name: "open before blink", elapsed: blinkAt - time.Millisecond, want: frameOpen},
		{name: "half closed", elapsed: blinkAt, want: frameHalfClosed},
		{name: "closed", elapsed: blinkAt + halfBlink, want: frameClosed},
		{name: "half closed again", elapsed: blinkAt + halfBlink + closedBlink, want: frameHalfClosed},
		{name: "open after blink", elapsed: blinkAt + 2*halfBlink + closedBlink, want: frameOpen},
		{name: "next period", elapsed: blinkPeriod + blinkAt, want: frameHalfClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameAt(tt.elapsed); got != tt.want {
				t.Fatalf("frameAt(%v) = %v, want %v", tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestDefaultLayout(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Size(); got != 248 {
		t.Fatalf("default window size = %d, want 248", got)
	}
	if got := m.SpriteSize(); got != 240 {
		t.Fatalf("default sprite size = %d, want 240", got)
	}
	face := m.FaceRegion()
	want := ui.Rect{X: 78, Y: 112, W: 100, H: 76}
	if face != want {
		t.Fatalf("default face = %+v, want %+v", face, want)
	}
}

func TestScaleLayout(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	m.SetScale(1.25)
	if got := m.Size(); got != 310 {
		t.Fatalf("scaled window size = %d, want 310", got)
	}
	if got := m.SpriteSize(); got != 300 {
		t.Fatalf("scaled sprite size = %d, want 300", got)
	}
	face := m.FaceRegion()
	if math.Abs(float64(face.X-97.5)) > 0.01 ||
		math.Abs(float64(face.Y-140)) > 0.01 ||
		math.Abs(float64(face.W-125)) > 0.01 ||
		math.Abs(float64(face.H-95)) > 0.01 {
		t.Fatalf("scaled face = %+v, want approximately (97.5,140) 125x95", face)
	}

	m.SetScale(0.1)
	if got := m.Scale(); got != minScale {
		t.Fatalf("minimum scale = %v, want %v", got, minScale)
	}
	m.SetScale(9)
	if got := m.Scale(); got != maxScale {
		t.Fatalf("maximum scale = %v, want %v", got, maxScale)
	}
}

func TestSetScaleRejectsInvalidValues(t *testing.T) {
	for _, scale := range []float64{
		0, -1, math.NaN(), math.Inf(1), math.Inf(-1),
	} {
		m, err := New()
		if err != nil {
			t.Fatal(err)
		}
		m.SetScale(scale)
		if got := m.Scale(); got != defaultScale {
			t.Errorf("SetScale(%v) = %v, want %v", scale, got, defaultScale)
		}
	}
}

func TestNewDecodesEmbeddedFrames(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for i, bitmap := range m.frames {
		if bitmap == nil {
			t.Fatalf("frame %d is nil", i)
		}
		if w, h := bitmap.Size(); w != 800 || h != 800 {
			t.Errorf("frame %d size = %dx%d, want 800x800", i, w, h)
		}
	}
}

func TestViewRendersTransparentPet(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()

	rect, ok := tt.Find("Lucia 桌宠")
	if !ok {
		t.Fatalf("pet label not found; labels: %v", tt.Texts())
	}
	if rect.W <= 0 || rect.H <= 0 {
		t.Fatalf("pet bounds = %+v", rect)
	}

	img := tt.Image()
	b := img.Bounds()
	if _, _, _, a := img.At(b.Min.X, b.Min.Y).RGBA(); a != 0 {
		t.Fatalf("top-left corner alpha = %d, want transparent", a)
	}
	opaque := 0
	for y := b.Min.Y; y < b.Max.Y; y += 8 {
		for x := b.Min.X; x < b.Max.X; x += 8 {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				opaque++
			}
		}
	}
	if opaque == 0 {
		t.Fatal("pet rendered no visible pixels")
	}
}

func TestActivityAt(t *testing.T) {
	tests := []struct {
		name string
		idle time.Duration
		want Activity
	}{
		{name: "awake", idle: 0, want: ActivityAwake},
		{name: "awake before threshold", idle: drowsyAfter - time.Second, want: ActivityAwake},
		{name: "drowsy", idle: drowsyAfter, want: ActivityDrowsy},
		{name: "drowsy before sleep", idle: asleepAfter - time.Second, want: ActivityDrowsy},
		{name: "asleep", idle: asleepAfter, want: ActivityAsleep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activityAt(tt.idle); got != tt.want {
				t.Fatalf("activityAt(%v) = %v, want %v", tt.idle, got, tt.want)
			}
		})
	}
}

func TestFrameForActivity(t *testing.T) {
	if got := frameFor(ActivityAwake, 0); got != frameOpen {
		t.Fatalf("awake frame = %v, want open", got)
	}
	if got := frameFor(ActivityDrowsy, 0); got != frameHalfClosed {
		t.Fatalf("drowsy frame = %v, want half closed", got)
	}
	if got := frameFor(ActivityAsleep, 0); got != frameClosed {
		t.Fatalf("asleep frame = %v, want closed", got)
	}
}

func TestFrameDelay(t *testing.T) {
	tests := []struct {
		name            string
		activity        Activity
		idle            time.Duration
		elapsed         time.Duration
		reactionElapsed time.Duration
		reaction        Reaction
		reduceMotion    bool
		want            time.Duration
	}{
		{
			name:     "awake animation",
			activity: ActivityAwake,
			want:     idleFrameInterval,
		},
		{
			name:            "face reaction",
			activity:        ActivityAwake,
			elapsed:         time.Second,
			reactionElapsed: 100 * time.Millisecond,
			reaction:        ReactionFace,
			want:            activeFrameInterval,
		},
		{
			name:            "reduce motion face reaction",
			activity:        ActivityAwake,
			reactionElapsed: 100 * time.Millisecond,
			reaction:        ReactionFace,
			reduceMotion:    true,
			want:            tapDuration - 100*time.Millisecond,
		},
		{
			name:            "reduce motion head pat",
			activity:        ActivityAwake,
			reactionElapsed: 100 * time.Millisecond,
			reaction:        ReactionHeadPat,
			reduceMotion:    true,
			want:            headPatDuration - 100*time.Millisecond,
		},
		{
			name:         "reduce motion waits for blink",
			activity:     ActivityAwake,
			reduceMotion: true,
			want:         blinkAt,
		},
		{
			name:         "reduce motion wakes for drowsiness",
			activity:     ActivityAwake,
			idle:         drowsyAfter - time.Second,
			reduceMotion: true,
			want:         time.Second,
		},
		{
			name:         "reduce motion waits for sleep",
			activity:     ActivityDrowsy,
			idle:         drowsyAfter,
			reduceMotion: true,
			want:         asleepAfter - drowsyAfter,
		},
		{
			name:         "reduce motion asleep stops",
			activity:     ActivityAsleep,
			idle:         asleepAfter,
			reduceMotion: true,
			want:         0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextFrameDelay(tt.activity, tt.idle, tt.elapsed, tt.reactionElapsed, tt.reaction, tt.reduceMotion)
			if got != tt.want {
				t.Fatalf("nextFrameDelay = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextBlinkDelay(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    time.Duration
	}{
		{name: "open", elapsed: 0, want: blinkAt},
		{name: "half closed", elapsed: blinkAt, want: halfBlink},
		{name: "closed", elapsed: blinkAt + halfBlink, want: closedBlink},
		{name: "half closed again opens at the period", elapsed: blinkAt + halfBlink + closedBlink, want: blinkPeriod - (blinkAt + halfBlink + closedBlink)},
		{name: "next period opens the eyes", elapsed: blinkPeriod - time.Millisecond, want: time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextBlinkDelay(tt.elapsed); got != tt.want {
				t.Fatalf("nextBlinkDelay(%v) = %v, want %v", tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestReduceMotionSkipsContinuousMotion(t *testing.T) {
	bob, squash, lift := motionFor(true, 800*time.Millisecond, ReactionFace, 100*time.Millisecond)
	if bob != 0 || squash != 0 || lift != 0 {
		t.Fatalf("reduce motion transforms = (%v, %v, %v), want zeros", bob, squash, lift)
	}
	if got := displayFrame(ActivityAwake, 0, ReactionFace, 100*time.Millisecond, true); got != frameClosed {
		t.Fatalf("reduce motion face frame = %v, want closed", got)
	}
	if got := displayFrame(ActivityAwake, 0, ReactionHeadPat, 100*time.Millisecond, true); got != frameHalfClosed {
		t.Fatalf("reduce motion head-pat frame = %v, want half closed", got)
	}

	bob, squash, lift = motionFor(false, bobPeriod/4, ReactionFace, 100*time.Millisecond)
	if bob == 0 || squash == 0 || lift != 0 {
		t.Fatalf("normal transforms = (%v, %v, %v), want bob and face squash", bob, squash, lift)
	}
	if got := displayFrame(ActivityAwake, 0, ReactionFace, 100*time.Millisecond, false); got != frameOpen {
		t.Fatalf("normal face frame = %v, want open", got)
	}
}

func TestInteractionsRequestInvalidation(t *testing.T) {
	m := &Model{}
	calls := 0
	m.SetInvalidator(func() { calls++ })
	m.React(ReactionFace)
	m.DragStarted()
	m.DragEnded()
	if calls != 3 {
		t.Fatalf("invalidations = %d, want 3", calls)
	}
}

func TestReactionMotionDiffers(t *testing.T) {
	face, faceLift := reactionMotion(ReactionFace, 100*time.Millisecond)
	headPat, headPatLift := reactionMotion(ReactionHeadPat, 100*time.Millisecond)
	if headPat >= face {
		t.Fatalf("head-pat squash = %v, want less than face squash %v", headPat, face)
	}
	if faceLift != 0 {
		t.Fatalf("face lift = %v, want 0", faceLift)
	}
	if headPatLift >= 0 {
		t.Fatalf("head-pat lift = %v, want a small lift", headPatLift)
	}
	if face <= 0 {
		t.Fatalf("face reaction at 100ms = %v, want a visible squash", face)
	}
	if squash, lift := reactionMotion(ReactionFace, tapDuration); squash != 0 || lift != 0 {
		t.Fatalf("face after duration = (%v, %v), want still", squash, lift)
	}
}

func TestDragKeepsAwake(t *testing.T) {
	m := &Model{lastInteraction: time.Now().Add(-asleepAfter)}
	if got := m.activity(time.Now()); got != ActivityAsleep {
		t.Fatalf("activity before drag = %v, want asleep", got)
	}
	m.DragStarted()
	if got := m.activity(time.Now()); got != ActivityAwake {
		t.Fatalf("activity during drag = %v, want awake", got)
	}
}

func TestInteractionWakesForNextFrame(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()
	tt.Advance(asleepAfter)
	tt.Frame()
	if got := m.activity(tt.Now()); got != ActivityAsleep {
		t.Fatalf("activity after idle = %v, want asleep", got)
	}
	m.React(ReactionFace)
	tt.Frame()
	if got := m.activity(tt.Now()); got != ActivityAwake {
		t.Fatalf("activity after interaction = %v, want awake", got)
	}
}

func TestTapAmount(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    float32
	}{
		{name: "before", elapsed: -time.Millisecond, want: 0},
		{name: "start", elapsed: 0, want: 0},
		{name: "peak", elapsed: tapDuration / 2, want: 1},
		{name: "finished", elapsed: tapDuration, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tapAmount(tt.elapsed); math.Abs(float64(got-tt.want)) > 0.0001 {
				t.Fatalf("tapAmount(%v) = %v, want %v", tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestReactionHandlerReceivesReaction(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var got Reaction
	m.SetReactionHandler(func(reaction Reaction) { got = reaction })
	m.React(ReactionHeadPat)
	if got != ReactionHeadPat {
		t.Fatalf("handler reaction = %v, want ReactionHeadPat", got)
	}
}

func TestClickFaceStartsReaction(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()
	tt.ClickAt(128, 150)
	tt.Frame()
	if m.tapAt.IsZero() {
		t.Fatal("clicking the face did not start the reaction")
	}
	if m.reaction != ReactionFace {
		t.Fatalf("reaction = %v, want ReactionFace", m.reaction)
	}
}

func TestClickFaceScalesWithLayout(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	m.SetScale(1.5)
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()
	face := m.FaceRegion()
	tt.ClickAt(face.X+face.W/2, face.Y+face.H/2)
	tt.Frame()
	if m.reaction != ReactionFace {
		t.Fatalf("reaction = %v, want ReactionFace at scaled face", m.reaction)
	}
}

func TestClickHeadbandDoesNotStartReaction(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()
	tt.ClickAt(124, 60)
	tt.Frame()
	if !m.tapAt.IsZero() {
		t.Fatal("clicking the headband started a face reaction")
	}
	if m.reaction != ReactionNone {
		t.Fatalf("reaction = %v, want ReactionNone", m.reaction)
	}
}

func TestClickBodyKeepsDragArea(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()
	tt.ClickAt(124, 200)
	tt.Frame()
	if !m.tapAt.IsZero() {
		t.Fatal("clicking outside the face started a reaction")
	}
	if m.reaction != ReactionNone {
		t.Fatalf("reaction = %v, want ReactionNone", m.reaction)
	}
}

func TestContextMenuStartsReaction(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()
	tt.RightClickAt(124, 200)
	found := false
	for _, item := range tt.Menu() {
		if item == "摸摸头" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("context menu has no 摸摸头; items: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("摸摸头"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.tapAt.IsZero() {
		t.Fatal("choosing the context menu item did not start the reaction")
	}
	if m.reaction != ReactionHeadPat {
		t.Fatalf("reaction = %v, want ReactionHeadPat", m.reaction)
	}
}

func TestContextMenuApplicationActions(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	m.SetMenuActions(MenuActions{
		OpenSettings: func() { calls = append(calls, "settings") },
		Hide:         func() { calls = append(calls, "hide") },
		Reset:        func() { calls = append(calls, "reset") },
	})
	tt := ui.NewTester(m.View, m.Size(), m.Size())
	tt.Frame()

	tt.RightClickAt(124, 200)
	items := tt.Menu()
	for _, want := range []string{"摸摸头", "-", "桌宠设置…", "隐藏宠物", "重置位置"} {
		found := false
		for _, item := range items {
			if item == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("context menu has no %q; items: %v", want, items)
		}
	}
	tt.CloseMenu()

	for _, label := range []string{"桌宠设置…", "隐藏宠物", "重置位置"} {
		tt.RightClickAt(124, 200)
		if err := tt.ChooseMenuItem(label); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"settings", "hide", "reset"}
	if len(calls) != len(want) {
		t.Fatalf("menu actions = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("menu actions = %v, want %v", calls, want)
		}
	}
}

func TestTrayIconIsPNG(t *testing.T) {
	icon, err := png.Decode(bytes.NewReader(TrayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := icon.Bounds().Dx(), icon.Bounds().Dy(); w != 32 || h != 32 {
		t.Fatalf("tray icon size = %dx%d, want 32x32", w, h)
	}
}
