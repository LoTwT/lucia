package pet

import (
	"math"
	"testing"
	"time"

	"github.com/LoTwT/lucia/internal/builtinpack"
	"github.com/egoist/mygo/ui"
)

func newBuiltinPet(t *testing.T) *Model {
	t.Helper()
	pack, err := builtinpack.Load(2)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewAnimated(pack)
	if err != nil {
		t.Fatal(err)
	}
	idle, ok := pack.Clip(pack.Manifest.Actions[pack.Manifest.States.Idle].Loop)
	if !ok {
		t.Fatal("built-in pack has no idle clip")
	}
	m.animated.cacheIdle(idle)
	return m
}

func newPetTester(m *Model) *ui.Tester {
	width, height := m.WindowSize()
	return ui.NewTester(m.View, width, height)
}

func TestSetScaleRejectsInvalidValues(t *testing.T) {
	for _, scale := range []float64{
		0, -1, math.NaN(), math.Inf(1), math.Inf(-1),
	} {
		m := newBuiltinPet(t)
		m.SetScale(scale)
		if got := m.Scale(); got != defaultScale {
			t.Errorf("SetScale(%v) = %v, want %v", scale, got, defaultScale)
		}
	}
}

func TestViewRendersTransparentPet(t *testing.T) {
	m := newBuiltinPet(t)
	tt := newPetTester(m)
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

func TestDragKeepsAwake(t *testing.T) {
	m := newBuiltinPet(t)
	m.lastInteraction = time.Now().Add(-asleepAfter)
	if got := m.activity(time.Now()); got != ActivityAsleep {
		t.Fatalf("activity before drag = %v, want asleep", got)
	}
	m.DragStarted()
	if got := m.activity(time.Now()); got != ActivityAwake {
		t.Fatalf("activity during drag = %v, want awake", got)
	}
}

func TestInteractionWakesForNextFrame(t *testing.T) {
	m := newBuiltinPet(t)
	tt := newPetTester(m)
	tt.Frame()
	tt.Advance(asleepAfter)
	tt.Frame()
	if got := m.activity(tt.Now()); got != ActivityAsleep {
		t.Fatalf("activity after idle = %v, want asleep", got)
	}
	m.TriggerInteraction("poke")
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

func TestHeadPatAmountIsGentler(t *testing.T) {
	face := tapAmount(tapDuration / 2)
	pat := headPatAmount(headPatDuration / 2)
	if pat <= 0 || pat >= face {
		t.Fatalf("head pat peak = %v, want a positive value below face peak %v", pat, face)
	}
	if got := headPatAmount(headPatDuration); got != 0 {
		t.Fatalf("headPatAmount after duration = %v, want 0", got)
	}
}

func TestBubbleHandlerReceivesAuthoredLine(t *testing.T) {
	m := newBuiltinPet(t)
	var got BubbleEvent
	m.SetBubbleHandler(func(event BubbleEvent) { got = event })
	m.TriggerInteraction("poke")
	if got.Text == "" || !got.Sound {
		t.Fatalf("bubble handler = %+v, want a line with sound", got)
	}
}

func TestClickFaceStartsInteraction(t *testing.T) {
	m := newBuiltinPet(t)
	tt := newPetTester(m)
	tt.Frame()
	zones := m.Hotzones()
	if len(zones) != 1 || zones[0].ID != "face" {
		t.Fatalf("hotzones = %+v", zones)
	}
	face := zones[0]
	tt.ClickAt(face.Rect.X+face.Rect.W/2, face.Rect.Y+face.Rect.H/2)
	tt.Frame()
	if m.interactionID != "poke" || m.interactionMotion != "squash" {
		t.Fatalf("interaction = %q/%q, want poke/squash", m.interactionID, m.interactionMotion)
	}
}

func TestClickBodyKeepsDragArea(t *testing.T) {
	m := newBuiltinPet(t)
	tt := newPetTester(m)
	tt.Frame()
	tt.ClickAt(124, 230)
	tt.Frame()
	if m.interactionID != "" {
		t.Fatalf("interaction = %q, want none outside the hotzone", m.interactionID)
	}
}

func TestContextMenuStartsHeadPat(t *testing.T) {
	m := newBuiltinPet(t)
	tt := newPetTester(m)
	tt.Frame()
	tt.RightClickAt(124, 230)
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
	if m.interactionID != "pat" || m.interactionMotion != "lift" {
		t.Fatalf("interaction = %q/%q, want pat/lift", m.interactionID, m.interactionMotion)
	}
}

func TestContextMenuApplicationActions(t *testing.T) {
	m := newBuiltinPet(t)
	var calls []string
	m.SetMenuActions(MenuActions{
		OpenSettings: func() { calls = append(calls, "settings") },
		Hide:         func() { calls = append(calls, "hide") },
		Reset:        func() { calls = append(calls, "reset") },
	})
	tt := newPetTester(m)
	tt.Frame()

	tt.RightClickAt(124, 230)
	items := tt.Menu()
	for _, want := range []string{"摸摸头", "-", "桌宠设置", "隐藏宠物", "重置位置"} {
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

	for _, label := range []string{"桌宠设置", "隐藏宠物", "重置位置"} {
		tt.RightClickAt(124, 230)
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
