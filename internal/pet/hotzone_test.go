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
		Schema:       2,
		ID:           "tiny",
		Canvas:       petpack.Size{Width: 16, Height: 16},
		Crop:         petpack.Rect{Width: 16, Height: 16},
		Base:         petpack.Base{Window: petpack.Size{Width: 20, Height: 20}, Sprite: petpack.Size{Width: 16, Height: 16}},
		Face:         petpack.Face{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
		BubbleAnchor: petpack.Anchor{X: 0.5, Y: 0.1},
		Clips:        clips,
		Actions: map[string]petpack.Action{
			"idle": {Loop: "idle_loop", Poster: "idle_loop#0", FaceClickable: true, LoopCount: -1},
			"happy": {
				Enter: "happy_enter", Exit: "happy_exit", Poster: "happy_enter#0", FaceClickable: true,
			},
			"laugh": {
				Enter: "laugh_enter", Exit: "laugh_exit", Poster: "laugh_enter#0", FaceClickable: true,
			},
			"wave": {Label: "挥手", Once: "wave", Poster: "wave#0"},
		},
		Hotzones: map[string]petpack.Hotzone{
			"face": {Label: "摸摸脸", Preset: "tap", Action: "laugh", Rect: petpack.Face{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5}},
			"head": {Label: "摸摸头", Preset: "pat", Action: "happy", Rect: petpack.Face{X: 0.1, Y: 0, Width: 0.8, Height: 0.2}},
		},
		Presets: map[string]petpack.Preset{
			"tap": {Reaction: "face", Bubble: "face", Steps: []petpack.PresetStep{{Action: "laugh"}}},
			"pat": {Reaction: "pat", Bubble: "pat", Steps: []petpack.PresetStep{
				{Action: "happy"}, {Action: "laugh", DelayMs: 100},
			}},
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

func TestHotzonesClickStartsPresetSequence(t *testing.T) {
	m, _ := newInteractionModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	zones := m.Hotzones()
	if len(zones) != 2 || zones[0].ID != "face" || zones[1].ID != "head" {
		t.Fatalf("hotzones = %+v", zones)
	}
	var reaction Reaction
	m.SetReactionHandler(func(r Reaction) { reaction = r })
	face := zones[0]
	tt.ClickAt(face.Rect.X+face.Rect.W/2, face.Rect.Y+face.Rect.H/2)
	tt.Frame()
	if reaction != ReactionFace {
		t.Fatalf("reaction = %v, want face", reaction)
	}
	if m.animated.current == nil || m.animated.current.actionID != "laugh" {
		t.Fatalf("current = %+v, want laugh", m.animated.current)
	}

	m2, _ := newInteractionModel(t)
	tt2 := ui.NewTester(m2.View, 20, 20)
	tt2.Frame()
	var reaction2 Reaction
	m2.SetReactionHandler(func(r Reaction) { reaction2 = r })
	zones2 := m2.Hotzones()
	head := zones2[1]
	tt2.ClickAt(head.Rect.X+head.Rect.W/2, head.Rect.Y+head.Rect.H/2)
	tt2.Frame()
	if reaction2 != ReactionHeadPat {
		t.Fatalf("head reaction = %v, want pat", reaction2)
	}
	if m2.animated.current == nil || m2.animated.current.actionID != "happy" {
		t.Fatalf("head current = %+v, want happy", m2.animated.current)
	}
}

func TestPresetSequenceWaitsForDelay(t *testing.T) {
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
		t.Fatal("laugh started before the preset delay elapsed")
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
		Schema:       2,
		ID:           "tiny",
		Canvas:       petpack.Size{Width: 16, Height: 16},
		Crop:         petpack.Rect{Width: 16, Height: 16},
		Base:         petpack.Base{Window: petpack.Size{Width: 20, Height: 20}, Sprite: petpack.Size{Width: 16, Height: 16}},
		Face:         petpack.Face{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
		BubbleAnchor: petpack.Anchor{X: 0.5, Y: 0.1},
		Clips: map[string]petpack.ClipEntry{
			"idle_loop": {File: "clips/idle_loop.webp", Frames: 3, Loop: true},
			"wave":      {File: "clips/wave.webp", Frames: 3},
		},
		Actions: map[string]petpack.Action{
			"idle": {Loop: "idle_loop", Poster: "idle_loop#0", FaceClickable: true, LoopCount: -1},
			"wave": {
				Once: "wave", Poster: "wave#0",
				Timing: petpack.Timing{ExitHoldMs: 200},
			},
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

func TestPresetFirstStepDelayKeepsIdleAlive(t *testing.T) {
	m, pack := newInteractionModel(t)
	pack.Manifest.Presets["tap"] = petpack.Preset{
		Reaction: "face", Bubble: "face",
		Steps: []petpack.PresetStep{{Action: "laugh", DelayMs: 100}},
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

func TestPresetFallbackUsesPresetReaction(t *testing.T) {
	m, pack := newInteractionModel(t)
	pack.Manifest.Presets["pat"] = petpack.Preset{Reaction: "pat", Bubble: "pat"}
	zone := pack.Manifest.Hotzones["head"]
	zone.Action = ""
	pack.Manifest.Hotzones["head"] = zone
	m.Close()
	m, err := NewAnimated(pack)
	if err != nil {
		t.Fatal(err)
	}
	idle, _ := pack.Clip("idle_loop")
	m.animated.cacheIdle(idle)
	tt := ui.NewTester(m.View, 20, 20)
	tt.Frame()
	m.TriggerHotzone("head")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "happy" {
		t.Fatalf("current = %+v, want happy from the pat reaction fallback", m.animated.current)
	}
}

func TestReduceMotionClearsDragActive(t *testing.T) {
	m, _ := newInteractionModel(t)
	m.animated.dragActive = true
	m.animated.sync(time.Now(), ActivityAwake, 0, false, ReactionNone, time.Time{}, true)
	if m.animated.dragActive {
		t.Fatal("Reduce Motion left dragActive set")
	}
}
