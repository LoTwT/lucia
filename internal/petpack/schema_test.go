package petpack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"schema", func(m *Manifest) { m.Schema = 2 }, "want 1"},
		{"idle required", func(m *Manifest) { m.States.Idle = "" }, "states.idle is required"},
		{"idle action", func(m *Manifest) { m.States.Idle = "missing" }, "missing action"},
		{"idle loops", func(m *Manifest) {
			m.Actions["idle"] = Action{Loop: "idle_loop", Poster: "idle_loop#0", LoopCount: 0, Interruptible: true}
		}, "must be an infinite loop"},
		{"idle interruptible", func(m *Manifest) {
			m.Actions["idle"] = Action{Loop: "idle_loop", Poster: "idle_loop#0", LoopCount: -1}
		}, "must be interruptible"},
		{"asleep needs loop or poster", func(m *Manifest) {
			m.States.Asleep = "happy"
			m.Actions["happy"] = Action{Once: "idle_loop", LoopCount: 0}
		}, "must loop or declare a poster"},
		{"drag structure", func(m *Manifest) { m.States.Drag = "happy" }, "must declare enter, loop and exit"},
		{"action poster", func(m *Manifest) {
			action := m.Actions["happy"]
			action.Poster = "missing#0"
			m.Actions["happy"] = action
		}, "missing clip"},
		{"event action", func(m *Manifest) { m.Events.Intro = "missing" }, "missing action"},
		{"idle motion", func(m *Manifest) { m.IdleMotion = "sway" }, "invalid idleMotion"},
		{"hotzone interaction", func(m *Manifest) { m.Hotzones[0].Interaction = "missing" }, "missing interaction"},
		{"hotzone rect", func(m *Manifest) {
			m.Hotzones[0].Rect = NormalizedRect{X: 0.8, Width: 0.4, Height: 1}
		}, "normalized sub-rectangle"},
		{"duplicate hotzone", func(m *Manifest) {
			m.Hotzones = append(m.Hotzones, m.Hotzones[0])
		}, "duplicate hotzone"},
		{"interaction action", func(m *Manifest) {
			m.Interactions["poke"] = Interaction{Steps: []Step{{Action: "missing"}}}
		}, "missing action"},
		{"interaction loop", func(m *Manifest) {
			m.Interactions["poke"] = Interaction{Steps: []Step{{Action: "idle"}}}
		}, "loops until interrupted"},
		{"interaction motion", func(m *Manifest) {
			m.Interactions["poke"] = Interaction{Motion: "shake"}
		}, "invalid motion"},
		{"interaction bubble", func(m *Manifest) {
			m.Interactions["poke"] = Interaction{Bubble: "missing"}
		}, "missing bubble"},
		{"interaction no effect", func(m *Manifest) {
			m.Interactions["poke"] = Interaction{Sound: true}
		}, "has no effect"},
		{"motion needs poster", func(m *Manifest) {
			m.Interactions["poke"] = Interaction{Motion: "squash"}
		}, "reducedPoster is required"},
		{"menu both", func(m *Manifest) {
			m.Menu = []MenuEntry{{Interaction: "pat", Action: "happy"}}
		}, "exactly one"},
		{"menu empty", func(m *Manifest) { m.Menu = []MenuEntry{{}} }, "exactly one"},
		{"menu action label", func(m *Manifest) {
			m.Actions["plain"] = Action{Once: "idle_loop"}
			m.Menu = []MenuEntry{{Action: "plain"}}
		}, "label is required"},
		{"bubble empty line", func(m *Manifest) {
			m.Bubbles["face"] = []string{""}
		}, "empty line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validTestManifest()
			tt.mutate(&manifest)
			if err := validateManifest("tiny", manifest); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStrictManifestDecode(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "old face field",
			body: `{"schema":1,"face":{"x":0.1}}`,
			want: "unknown field",
		},
		{
			name: "old faceClickable field",
			body: `{"schema":1,"actions":{"idle":{"faceClickable":true}}}`,
			want: "unknown field",
		},
		{
			name: "trailing value",
			body: `{"schema":1}{"schema":1}`,
			want: "more than one JSON value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeManifest([]byte(tt.body)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeManifest error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestHotzonesKeepDeclarationOrder(t *testing.T) {
	manifest := validTestManifest()
	manifest.Hotzones = []Hotzone{
		{ID: "face", Label: "脸", Rect: NormalizedRect{X: 0.2, Y: 0.2, Width: 0.4, Height: 0.4}, Interaction: "poke"},
		{ID: "head", Label: "头", Rect: NormalizedRect{X: 0.1, Width: 0.8, Height: 0.2}, Interaction: "pat"},
	}
	dir := writeTinyPackManifest(t, manifest)
	pack, err := Load(dir, "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	zones := pack.HotzoneList()
	if len(zones) != 2 || zones[0].ID != "face" || zones[1].ID != "head" {
		t.Fatalf("hotzones = %+v", zones)
	}
}

func TestWindowsReservedPackIDs(t *testing.T) {
	for _, id := range []string{"con", "CON", "aux", "nul", "com1", "LPT9"} {
		if validPackDirID(id) {
			t.Errorf("validPackDirID accepted reserved name %q", id)
		}
	}
	for _, id := range []string{"console", "com0", "lpt10", "tiny", "tiny-pack"} {
		if !validPackDirID(id) {
			t.Errorf("validPackDirID rejected %q", id)
		}
	}
}

func TestBuiltinPackIDIsReserved(t *testing.T) {
	for _, id := range []string{"lucia-whale", "Lucia-Whale", "LUCIA-WHALE"} {
		if !IsReservedPackID(id) {
			t.Errorf("IsReservedPackID(%q) = false", id)
		}
	}
	if IsReservedPackID("whale") {
		t.Fatal("IsReservedPackID rejected an ordinary id")
	}
}

func TestDocumentedExampleIsValid(t *testing.T) {
	body, err := os.ReadFile("../../docs/examples/pet-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := validateManifest("example", manifest); err != nil {
		t.Fatal(err)
	}
}

func TestReadManifestRejectsReservedID(t *testing.T) {
	dir := writeTinyPack(t)
	if _, err := ReadManifest(filepath.Join(dir, "packs", "tiny"), BuiltinPackID); err == nil ||
		!strings.Contains(err.Error(), "reserved") {
		t.Fatalf("ReadManifest error = %v, want reserved id error", err)
	}
}

func TestReadManifestDoesNotDecodeClips(t *testing.T) {
	manifest := validTestManifest()
	dir := writeTinyPackManifest(t, manifest)
	if err := os.Remove(filepath.Join(dir, "packs", "tiny", "clips", "idle_loop.webp")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(filepath.Join(dir, "packs", "tiny"), "tiny"); err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if _, err := Load(dir, "tiny", 1); err == nil {
		t.Fatal("Load accepted a missing clip")
	}
}

func validTestManifest() Manifest {
	return Manifest{
		Schema:       1,
		ID:           "tiny",
		Name:         "Tiny",
		Canvas:       Size{Width: 16, Height: 16},
		Crop:         Rect{Width: 16, Height: 16},
		Base:         Base{Window: Size{Width: 20, Height: 20}, Sprite: Size{Width: 16, Height: 16}},
		BubbleAnchor: Anchor{X: 0.5, Y: 0.1},
		Clips: map[string]ClipEntry{
			"idle_loop": {File: "clips/idle_loop.webp", Frames: 3, Loop: true},
		},
		Actions: map[string]Action{
			"idle":  {Loop: "idle_loop", Poster: "idle_loop#0", Interruptible: true, LoopCount: -1},
			"happy": {Label: "开心", Once: "idle_loop", Poster: "idle_loop#0"},
			"laugh": {Label: "大笑", Once: "idle_loop", Poster: "idle_loop#0"},
		},
		States: States{Idle: "idle"},
		Hotzones: []Hotzone{
			{ID: "face", Label: "脸", Rect: NormalizedRect{X: 0.2, Y: 0.2, Width: 0.4, Height: 0.4}, Interaction: "poke"},
		},
		Interactions: map[string]Interaction{
			"poke": {Label: "摸摸脸", Steps: []Step{{Action: "laugh"}}, Bubble: "face", Motion: "squash"},
			"pat":  {Label: "摸摸头", Bubble: "pat", Motion: "lift", ReducedPoster: "idle_loop#1"},
		},
		Bubbles: map[string][]string{
			"face": {"别戳脸啦。"},
			"pat":  {"摸头……还行吧。"},
		},
		Menu: []MenuEntry{
			{Interaction: "pat"},
			{Action: "happy"},
		},
	}
}

func writeTinyPackManifest(t *testing.T, manifest Manifest) string {
	t.Helper()
	dir := t.TempDir()
	packDir := filepath.Join(dir, "packs", "tiny")
	if err := os.MkdirAll(filepath.Join(packDir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "clips", "idle_loop.webp"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
