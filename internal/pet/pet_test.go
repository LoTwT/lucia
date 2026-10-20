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
	tt := ui.NewTester(m.View, Size, Size)
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
	if !tt.FramePending() {
		t.Fatal("idle animation did not request another frame")
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
	tt := ui.NewTester(m.View, Size, Size)
	tt.Frame()
	tt.ClickAt(124, 60)
	tt.Frame()
	if m.tapAt.IsZero() {
		t.Fatal("clicking the face did not start the reaction")
	}
	if m.reaction != ReactionFace {
		t.Fatalf("reaction = %v, want ReactionFace", m.reaction)
	}
}

func TestClickBodyKeepsDragArea(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.View, Size, Size)
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
	tt := ui.NewTester(m.View, Size, Size)
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

func TestTrayIconIsPNG(t *testing.T) {
	icon, err := png.Decode(bytes.NewReader(TrayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := icon.Bounds().Dx(), icon.Bounds().Dy(); w != 32 || h != 32 {
		t.Fatalf("tray icon size = %dx%d, want 32x32", w, h)
	}
}
