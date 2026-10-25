package pet

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LoTwT/lucia/internal/petpack"
	"github.com/egoist/mygo/ui"
)

func newTinyPack(t *testing.T) *petpack.Pack {
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
		"idle_loop", "happy_enter", "happy_exit", "laugh_enter", "laugh_exit",
		"grab_enter", "drag_loop", "drop_exit", "belly_taunt", "sleep_denial",
		"wave", "think", "sad",
	} {
		clips[id] = petpack.ClipEntry{File: "clips/tiny.webp", Frames: 3, Loop: id == "idle_loop" || id == "drag_loop"}
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
			"idle": {
				Label: "待机", Loop: "idle_loop", Poster: "idle_loop#0", Interruptible: true, LoopCount: -1,
			},
			"happy": {
				Label: "开心", Enter: "happy_enter", Exit: "happy_exit", Poster: "happy_enter#0", Interruptible: true,
			},
			"laugh": {
				Label: "大笑", Enter: "laugh_enter", Exit: "laugh_exit", Poster: "laugh_enter#0", Interruptible: true,
			},
			"drag": {
				Label: "拖动", Enter: "grab_enter", Loop: "drag_loop", Exit: "drop_exit", Poster: "drag_loop#0", LoopCount: -1,
			},
			"belly_taunt": {
				Label: "肚肚挑衅", Once: "belly_taunt", Poster: "belly_taunt#0",
			},
			"sleep_denial": {
				Label: "困到点头", Once: "sleep_denial", Poster: "sleep_denial#0",
			},
			"wave":  {Label: "挥手", Once: "wave", Poster: "wave#0"},
			"think": {Label: "思考", Once: "think", Poster: "think#0"},
			"sad":   {Label: "难过", Once: "sad", Poster: "sad#0"},
		},
		States: petpack.States{Idle: "idle", Drag: "drag", Drowsy: "sleep_denial"},
		Events: petpack.Events{
			Intro: "wave", Activate: "wave", Reset: "wave",
			UpdateChecking: "think", UpdateFailed: "sad",
		},
		Hotzones: []petpack.Hotzone{
			{ID: "face", Label: "摸摸脸", Rect: petpack.NormalizedRect{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5}, Interaction: "poke"},
		},
		Interactions: map[string]petpack.Interaction{
			"poke": {
				Label: "摸摸脸", Steps: []petpack.Step{{Action: "laugh"}},
				Bubble: "face", Sound: true, Motion: "squash", ReducedPoster: "idle_loop#2",
			},
		},
		Bubbles: map[string][]string{"face": {"别戳脸啦。"}},
		Menu:    []petpack.MenuEntry{{Action: "belly_taunt"}},
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
	return pack
}

func newTinyAnimatedModel(t *testing.T) (*Model, *petpack.Pack) {
	t.Helper()
	pack := newTinyPack(t)
	m, err := NewAnimated(pack)
	if err != nil {
		t.Fatal(err)
	}
	idle, ok := pack.Clip("idle_loop")
	if !ok {
		t.Fatal("missing idle clip")
	}
	m.animated.cacheIdle(idle)
	return m, pack
}

func TestAnimatedIdleLoops(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle", m.animated.current)
	}
	if m.animated.current.frame != 0 {
		t.Fatalf("idle frame = %d, want 0", m.animated.current.frame)
	}
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	if m.animated.current.frame != 1 {
		t.Fatalf("idle frame = %d, want 1", m.animated.current.frame)
	}
	tt.Advance(100 * time.Millisecond)
	tt.Frame()
	if m.animated.current.frame != 0 {
		t.Fatalf("idle frame after loop = %d, want 0", m.animated.current.frame)
	}
}

func TestAnimatedIdleProviderIsNilWhileCaching(t *testing.T) {
	pack := newTinyPack(t)
	a := &animatedModel{
		pack:       pack,
		idleAction: petpack.Action{Loop: "idle_loop", Poster: "idle_loop#0"},
	}
	if provider := a.providerFor("idle_loop"); provider != nil {
		t.Fatalf("provider = %T, want nil while the idle cache is absent", provider)
	}
}

func TestAnimatedPokeChainsToIdle(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	m.TriggerInteraction("poke")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "laugh" || m.animated.current.phase != phaseEnter {
		t.Fatalf("current = %+v, want laugh/enter", m.animated.current)
	}
	if m.animated.hotzonesClickable(false) != true {
		t.Fatal("laugh should keep the face clickable")
	}
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.phase != phaseExit {
		t.Fatalf("current = %+v, want laugh/exit", m.animated.current)
	}
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle", m.animated.current)
	}
}

func TestTriggerEventUsesManifestMapping(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	tt := ui.NewTester(m.View, 20, 20)
	tt.Frame()
	if !m.HasEvent("intro") || !m.HasEvent("update.checking") {
		t.Fatal("expected pack events are missing")
	}
	m.TriggerEvent("update.checking")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "think" {
		t.Fatalf("current = %+v, want think", m.animated.current)
	}
	m.TriggerEvent("missing")
	if m.animated.pendingOnce != "" {
		t.Fatalf("unknown event queued %q", m.animated.pendingOnce)
	}
}

func TestEnteringSleepClearsPendingSequence(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	m.animated.sequence = []petpack.Step{{Action: "happy"}}
	m.animated.sequenceIndex = 0
	m.animated.enterAsleep(time.Now())
	if len(m.animated.sequence) != 0 || m.animated.sequenceIndex != 0 {
		t.Fatalf("sequence = %+v index=%d, want cleared", m.animated.sequence, m.animated.sequenceIndex)
	}
}

func TestLoopingAsleepKeepsAdvancing(t *testing.T) {
	m, pack := newTinyAnimatedModel(t)
	pack.Manifest.Actions["asleep_loop"] = petpack.Action{
		Label: "睡着", Loop: "idle_loop", Poster: "idle_loop#0", LoopCount: -1,
	}
	pack.Manifest.States.Asleep = "asleep_loop"
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	tt.Advance(asleepAfter)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "asleep_loop" {
		t.Fatalf("current = %+v, want the looping asleep action", m.animated.current)
	}
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.frame != 1 {
		t.Fatalf("current = %+v, want asleep frame 1", m.animated.current)
	}
}

func TestFiniteDrowsyReturnsToIdle(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	tt.Advance(drowsyAfter)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "sleep_denial" {
		t.Fatalf("current = %+v, want the finite drowsy action", m.animated.current)
	}
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle after the finite drowsy action", m.animated.current)
	}
	if m.animated.restPoster != nil {
		t.Fatal("finite drowsy action left a static poster")
	}
}

func TestAnimatedDragChain(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	m.DragStarted()
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "drag" || m.animated.current.phase != phaseEnter {
		t.Fatalf("current = %+v, want drag/enter", m.animated.current)
	}
	if m.animated.hotzonesClickable(true) {
		t.Fatal("drag must disable the face target")
	}
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.phase != phaseLoop {
		t.Fatalf("current = %+v, want drag/loop", m.animated.current)
	}
	loopProvider := m.animated.current.provider
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.frame != 0 {
		t.Fatalf("current = %+v, want the drag loop restarted", m.animated.current)
	}
	if m.animated.current.provider != loopProvider {
		t.Fatal("the drag loop recreated its provider instead of rewinding")
	}
	m.DragEnded()
	tt.Frame()
	if m.animated.current == nil || m.animated.current.phase != phaseExit {
		t.Fatalf("current = %+v, want drag/exit", m.animated.current)
	}
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle", m.animated.current)
	}
}

func TestAnimatedSpecialMenu(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	tt.RightClickAt(1, 1)
	items := tt.Menu()
	found := false
	for _, item := range items {
		if item == "小动作" {
			t.Fatalf("menu still contains the old 小动作 submenu: %v", items)
		}
		if item == "肚肚挑衅" {
			found = true
		}
	}
	if !found {
		t.Fatalf("menu = %v, want 肚肚挑衅 at the top level", items)
	}
	if err := tt.ChooseMenuItem("肚肚挑衅"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "belly_taunt" {
		t.Fatalf("current = %+v, want belly_taunt", m.animated.current)
	}
	if m.animated.hotzonesClickable(false) {
		t.Fatal("specials must disable the face target")
	}
	provider, ok := m.animated.current.provider.(*decoderProvider)
	if !ok {
		t.Fatalf("provider = %T, want *decoderProvider", m.animated.current.provider)
	}
	decoded := provider.next
	m.animated.currentImage()
	m.animated.currentImage()
	if provider.next != decoded {
		t.Fatalf("repeated currentImage decoded %d frames, want %d", provider.next, decoded)
	}
}

func TestAnimatedSpecialWakesFromSleep(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.Frame()
	tt.Advance(asleepAfter + time.Second)
	tt.Frame()
	if !m.animated.asleep {
		t.Fatal("the pet did not enter the asleep state")
	}
	m.PlayAction("belly_taunt")
	tt.Frame()
	if m.animated.current == nil || m.animated.current.actionID != "belly_taunt" {
		t.Fatalf("current = %+v, want belly_taunt", m.animated.current)
	}
	if m.animated.asleep {
		t.Fatal("a special action must keep the animation clock awake")
	}
}

func TestAnimatedQueuesActionBehindIntro(t *testing.T) {
	a := &animatedModel{pack: newTinyPack(t)}
	a.requestAction("wave")
	a.requestAction("think")
	a.requestAction("think")
	if a.pendingOnce != "wave" {
		t.Fatalf("pendingOnce = %q, want wave", a.pendingOnce)
	}
	if len(a.queuedOnce) != 1 || a.queuedOnce[0] != "think" {
		t.Fatalf("queuedOnce = %v, want [think]", a.queuedOnce)
	}
}

func TestAnimatedBackgroundActionDoesNotWakeSleepingPet(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	m.animated.asleep = true
	m.TriggerBackgroundEvent("update.checking")
	if m.animated.pendingOnce != "" {
		t.Fatalf("pendingOnce = %q, want empty while asleep", m.animated.pendingOnce)
	}
	m.PlayAction("think")
	if m.animated.pendingOnce != "think" {
		t.Fatalf("pendingOnce = %q, want a user action to remain allowed", m.animated.pendingOnce)
	}
}

type failingProvider struct{}

func (failingProvider) Len() int                      { return 1 }
func (failingProvider) Duration(int) time.Duration    { return 10 * time.Millisecond }
func (failingProvider) Frame(int) (image.Image, bool) { return nil, false }
func (failingProvider) Failed() bool                  { return true }
func (failingProvider) Reset()                        {}
func (failingProvider) Close()                        {}

func TestAnimatedDecodeFailureSchedulesRecovery(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	now := time.Now()
	m.animated.current = &clipPlay{
		actionID:   "drag",
		phase:      phaseLoop,
		provider:   failingProvider{},
		frameStart: now,
	}
	if delay := m.animated.nextDelay(now, ActivityAwake, 0, time.Time{}, false); delay <= 0 {
		t.Fatalf("nextDelay = %v, want a positive delay so sync can recover", delay)
	}
	m.animated.advance(now)
	if m.animated.current == nil || m.animated.current.actionID != "idle" {
		t.Fatalf("current = %+v, want idle after a decode failure", m.animated.current)
	}
}

func TestAnimatedDragEndAfterLongGapStillDrops(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	a := m.animated
	now := time.Now()
	a.sync(now, ActivityAwake, 0, false, false)
	a.requestDragStart()
	a.sync(now.Add(50*time.Millisecond), ActivityAwake, 0, true, false)
	a.requestDragEnd()
	a.sync(now.Add(2*time.Second), ActivityAwake, 0, false, false)
	if a.current == nil || a.current.actionID != "drag" || a.current.phase != phaseExit {
		t.Fatalf("current = %+v, want drag exit after the gap", a.current)
	}
}

func TestIdleMotionBobKeepsTheAnimationClockRunning(t *testing.T) {
	m, pack := newTinyAnimatedModel(t)
	pack.Manifest.IdleMotion = "bob"
	now := time.Now()
	if delay := m.animated.nextDelay(now, ActivityAwake, 0, time.Time{}, false); delay <= 0 || delay > idleFrameInterval {
		t.Fatalf("awake bob delay = %v, want 1..%v", delay, idleFrameInterval)
	}
	if delay := m.animated.nextDelay(now, ActivityDrowsy, drowsyAfter, time.Time{}, false); delay <= 0 || delay > drowsyFrameInterval {
		t.Fatalf("drowsy bob delay = %v, want 1..%v", delay, drowsyFrameInterval)
	}
	if delay := m.animated.nextDelay(now, ActivityAsleep, asleepAfter, time.Time{}, false); delay != asleepFrameInterval {
		t.Fatalf("asleep bob delay = %v, want %v", delay, asleepFrameInterval)
	}
	if delay := m.animated.nextDelay(now, ActivityAwake, 0, now, false); delay != activeFrameInterval {
		t.Fatalf("interaction bob delay = %v, want %v", delay, activeFrameInterval)
	}
}

func TestAnimatedReduceMotionUsesPoster(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	width, height := m.WindowSize()
	tt := ui.NewTester(m.View, width, height)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	if m.animated.current != nil {
		t.Fatalf("current = %+v, want nil in Reduce Motion", m.animated.current)
	}
	m.TriggerInteraction("poke")
	tt.Frame()
	if m.animated.current != nil {
		t.Fatalf("current = %+v, want nil in Reduce Motion", m.animated.current)
	}
	img := tt.Image()
	if img.Bounds().Empty() {
		t.Fatal("Reduce Motion rendered no image")
	}
}
