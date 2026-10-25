package pet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/egoist/mygo/ui"
)

func newInteractionModel(t *testing.T) (*Model, *petpack.Pack) {
	t.Helper()
	dir := t.TempDir()
	packDir := filepath.Join(dir, "packs", "tiny")
	if err := os.MkdirAll(filepath.Join(packDir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../petpack/testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "clips", "tiny.webp"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	clips := map[string]petpack.ClipEntry{}
	for _, id := range []string{
		"idle_loop", "happy_enter", "happy_exit", "laugh_enter", "laugh_exit", "wave",
	} {
		clips[id] = petpack.ClipEntry{File: "clips/tiny.webp", Frames: 3, Loop: id == "idle_loop"}
	}
	manifest := petpack.Manifest{
		Schema:       1,
		ID:           "tiny",
		Canvas:       petpack.Size{Width: 16, Height: 16},
		Crop:         petpack.Rect{Width: 16, Height: 16},
		Base:         petpack.Base{Window: petpack.Size{Width: 20, Height: 20}, Sprite: petpack.Size{Width: 16, Height: 16}},
		BubbleAnchor: petpack.Anchor{X: 0.5, Y: 0.1},
		Clips:        clips,
		Actions: map[string]petpack.Action{
			"idle":  {Loop: "idle_loop", Poster: "idle_loop#0", Interruptible: true, LoopCount: -1},
			"happy": {Label: "开心", Enter: "happy_enter", Exit: "happy_exit", Poster: "happy_enter#0", Interruptible: true},
			"laugh": {Label: "大笑", Enter: "laugh_enter", Exit: "laugh_exit", Poster: "laugh_enter#0", Interruptible: true},
			"wave":  {Label: "挥手", Once: "wave", Poster: "wave#0"},
		},
		States: petpack.States{Idle: "idle"},
		Events: petpack.Events{Intro: "wave", Activate: "wave", Reset: "wave"},
		Hotzones: []petpack.Hotzone{
			{ID: "face", Label: "摸摸脸", Rect: petpack.NormalizedRect{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5}, Interaction: "poke"},
			{ID: "head", Label: "摸摸头", Rect: petpack.NormalizedRect{X: 0.1, Width: 0.8, Height: 0.2}, Interaction: "pat"},
		},
		Interactions: map[string]petpack.Interaction{
			"poke": {Label: "摸摸脸", Steps: []petpack.Step{{Action: "laugh"}}, Bubble: "face", Sound: true, Motion: "squash"},
			"pat": {Label: "摸摸头", Steps: []petpack.Step{
				{Action: "happy"}, {Action: "laugh", DelayMs: 100},
			}, Bubble: "pat", Motion: "lift"},
		},
		Bubbles: map[string][]string{
			"face": {"别戳脸啦。"},
			"pat":  {"摸头……还行吧。"},
		},
		Menu: []petpack.MenuEntry{
			{Interaction: "pat"},
			{Action: "wave"},
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	pack, err := petpack.Load(dir, "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewAnimated(pack)
	if err != nil {
		t.Fatal(err)
	}
	idle, _ := pack.Clip("idle_loop")
	m.animated.cacheIdle(idle)
	return m, pack
}

func TestHotzonesClickStartsInteractionSequence(t *testing.T) {
	m, _ := newInteractionModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	zones := m.Hotzones()
	if len(zones) != 2 || zones[0].ID != "face" || zones[1].ID != "head" {
		t.Fatalf("hotzones = %+v", zones)
	}
	var bubbles []BubbleEvent
	m.SetBubbleHandler(func(event BubbleEvent) { bubbles = append(bubbles, event) })
	face := zones[0]
	tt.ClickAt(face.Rect.X+face.Rect.W/2, face.Rect.Y+face.Rect.H/2)
	tt.Frame()
	if len(bubbles) != 1 || bubbles[0].Text != "别戳脸啦。" || !bubbles[0].Sound {
		t.Fatalf("bubbles = %+v", bubbles)
	}
	if m.animated.current == nil || m.animated.current.actionID != "laugh" {
		t.Fatalf("current = %+v, want laugh", m.animated.current)
	}

	m2, _ := newInteractionModel(t)
	tt2 := ui.NewTester(m2.View, 20, 20)
	tt2.Frame()
	var bubbles2 []BubbleEvent
	m2.SetBubbleHandler(func(event BubbleEvent) { bubbles2 = append(bubbles2, event) })
	zones2 := m2.Hotzones()
	head := zones2[1]
	tt2.ClickAt(head.Rect.X+head.Rect.W/2, head.Rect.Y+head.Rect.H/2)
	tt2.Frame()
	if len(bubbles2) != 1 || bubbles2[0].Text != "摸头……还行吧。" {
		t.Fatalf("head bubbles = %+v", bubbles2)
	}
	if m2.animated.current == nil || m2.animated.current.actionID != "happy" {
		t.Fatalf("head current = %+v, want happy", m2.animated.current)
	}
}

func TestInteractionSequenceWaitsForDelay(t *testing.T) {
	m, _ := newInteractionModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	m.TriggerHotzone("head")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "happy" {
		t.Fatalf("current = %+v, want happy", m.animated.current)
	}
	// happy is enter (150ms) + exit (150ms); the 100ms delay is applied
	// before the next step.
	tt.Advance(320 * time.Millisecond)
	tt.Frame()
	if m.animated.current != nil && m.animated.current.actionID == "laugh" {
		t.Fatal("laugh started before the interaction delay elapsed")
	}
	tt.Advance(120 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "laugh" {
		t.Fatalf("current = %+v, want the delayed laugh", m.animated.current)
	}
}

func TestActionTimingHoldsLastFrame(t *testing.T) {
	dir := t.TempDir()
	packDir := filepath.Join(dir, "packs", "tiny")
	if err := os.MkdirAll(filepath.Join(packDir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../petpack/testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"idle_loop", "wave"} {
		if err := os.WriteFile(filepath.Join(packDir, "clips", name+".webp"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := petpack.Manifest{
		Schema:       1,
		ID:           "tiny",
		Canvas:       petpack.Size{Width: 16, Height: 16},
		Crop:         petpack.Rect{Width: 16, Height: 16},
		Base:         petpack.Base{Window: petpack.Size{Width: 20, Height: 20}, Sprite: petpack.Size{Width: 16, Height: 16}},
		BubbleAnchor: petpack.Anchor{X: 0.5, Y: 0.1},
		Clips: map[string]petpack.ClipEntry{
			"idle_loop": {File: "clips/idle_loop.webp", Frames: 3, Loop: true},
			"wave":      {File: "clips/wave.webp", Frames: 3},
		},
		Actions: map[string]petpack.Action{
			"idle": {Loop: "idle_loop", Poster: "idle_loop#0", Interruptible: true, LoopCount: -1},
			"wave": {
				Label: "挥手", Once: "wave", Poster: "wave#0",
				Timing: petpack.Timing{ExitHoldMs: 200},
			},
		},
		States: petpack.States{Idle: "idle"},
		Menu:   []petpack.MenuEntry{{Action: "wave"}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	pack, err := petpack.Load(dir, "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewAnimated(pack)
	if err != nil {
		t.Fatal(err)
	}
	idle, _ := pack.Clip("idle_loop")
	m.animated.cacheIdle(idle)
	tt := ui.NewTester(m.View, 20, 20)
	tt.Frame()
	m.PlayAction("wave")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "wave" {
		t.Fatalf("current = %+v, want wave", m.animated.current)
	}
	tt.Advance(160 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "wave" {
		t.Fatalf("current after clip = %+v, want held wave", m.animated.current)
	}
	if m.animated.current.frame != 2 {
		t.Fatalf("held frame = %d, want 2", m.animated.current.frame)
	}
	tt.Advance(100 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "wave" {
		t.Fatal("timing hold ended too early")
	}
	tt.Advance(120 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle after hold", m.animated.current)
	}
}

func TestFirstStepDelayKeepsIdleAlive(t *testing.T) {
	m, pack := newInteractionModel(t)
	pack.Manifest.Interactions["poke"] = petpack.Interaction{
		Label: "摸摸脸", Bubble: "face", Motion: "squash",
		Steps: []petpack.Step{{Action: "laugh", DelayMs: 100}},
	}
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	m.TriggerHotzone("face")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle during the first delay", m.animated.current)
	}
	tt.Advance(60 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID == "laugh" {
		t.Fatalf("laugh started before the first delay: %+v", m.animated.current)
	}
	tt.Advance(60 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "laugh" {
		t.Fatalf("current = %+v, want laugh after the first delay", m.animated.current)
	}
}

func TestSoundOnlyInteractionEmitsFeedback(t *testing.T) {
	m, pack := newInteractionModel(t)
	pack.Manifest.Interactions["pat"] = petpack.Interaction{
		Label: "摸摸头", Bubble: "none", Sound: true, Motion: "squash",
		ReducedPoster: "idle_loop#1",
	}
	var events []BubbleEvent
	m.SetBubbleHandler(func(event BubbleEvent) { events = append(events, event) })
	m.TriggerHotzone("head")
	if len(events) != 1 || events[0].Text != "" || !events[0].Sound {
		t.Fatalf("feedback = %+v, want an empty-text sound event", events)
	}
}

func TestMotionOnlyInteractionKeepsCurrentClip(t *testing.T) {
	m, pack := newInteractionModel(t)
	pack.Manifest.Interactions["pat"] = petpack.Interaction{
		Label: "摸摸头", Bubble: "pat", Motion: "lift", ReducedPoster: "idle_loop#1",
	}
	m.Close()
	m, err := NewAnimated(pack)
	if err != nil {
		t.Fatal(err)
	}
	idle, _ := pack.Clip("idle_loop")
	m.animated.cacheIdle(idle)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	m.TriggerHotzone("head")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle for feedback-only interaction", m.animated.current)
	}
	if m.interactionID != "pat" || m.interactionMotion != "lift" {
		t.Fatalf("interaction = %q/%q, want pat/lift", m.interactionID, m.interactionMotion)
	}
}

func TestReduceMotionClearsDragActive(t *testing.T) {
	m, _ := newInteractionModel(t)
	m.animated.dragActive = true
	m.animated.sync(time.Now(), ActivityAwake, 0, false, true)
	if m.animated.dragActive {
		t.Fatal("Reduce Motion left dragActive set")
	}
}
