package petpack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchema1UpgradesToDefaults(t *testing.T) {
	dir := writeTinyPack(t)
	pack, err := Load(dir, "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.Schema != 2 {
		t.Fatalf("schema = %d, want 2", pack.Manifest.Schema)
	}
	zones := pack.HotzoneList()
	if len(zones) != 1 || zones[0].ID != "face" || zones[0].Preset != "tap" {
		t.Fatalf("hotzones = %+v, want the legacy face tap zone", zones)
	}
	if _, ok := pack.Preset("tap"); !ok {
		t.Fatal("schema 1 did not get the tap preset")
	}
	if _, ok := pack.Preset("pat"); !ok {
		t.Fatal("schema 1 did not get the pat preset")
	}
}

func TestSchema2HotzonesPresetsAndTiming(t *testing.T) {
	manifest := validTestManifest()
	manifest.Hotzones = map[string]Hotzone{
		"face": {Label: "脸", Preset: "tap", Rect: Face{X: 0.2, Y: 0.2, Width: 0.4, Height: 0.4}},
		"head": {Label: "头", Preset: "pat", Action: "happy", Rect: Face{X: 0.1, Y: 0, Width: 0.8, Height: 0.2}},
	}
	manifest.Presets = map[string]Preset{
		"tap": {Reaction: "face", Bubble: "face", Steps: []PresetStep{{Action: "laugh"}}},
		"pat": {Reaction: "pat", Bubble: "none", Steps: []PresetStep{{Action: "happy", DelayMs: 120}}},
	}
	manifest.Actions["happy"] = withTiming(manifest.Actions["happy"], Timing{EnterHoldMs: 40, LoopHoldMs: 50, ExitHoldMs: 60})
	dir := writeTinyPackManifest(t, manifest)
	pack, err := Load(dir, "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := pack.HotzoneList(); len(got) != 2 || got[0].ID != "face" || got[1].ID != "head" {
		t.Fatalf("hotzones = %+v", got)
	}
	tap, _ := pack.Preset("tap")
	if len(tap.Steps) != 1 || tap.Steps[0].Action != "laugh" {
		t.Fatalf("tap preset = %+v", tap)
	}
	pat, _ := pack.Preset("pat")
	if pat.Bubble != "none" || len(pat.Steps) != 1 || pat.Steps[0].DelayMs != 120 {
		t.Fatalf("pat preset = %+v", pat)
	}
	if timing := pack.Manifest.Actions["happy"].Timing; timing.EnterHoldMs != 40 || timing.ExitHoldMs != 60 {
		t.Fatalf("timing = %+v", timing)
	}
}

func TestSchema2Validation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"schema", func(m *Manifest) { m.Schema = 3 }, "schema 3"},
		{"hotzone preset", func(m *Manifest) {
			m.Hotzones = map[string]Hotzone{"face": {Preset: "missing", Rect: Face{Width: 1, Height: 1}}}
		}, "missing preset"},
		{"hotzone rect", func(m *Manifest) {
			m.Hotzones = map[string]Hotzone{"face": {Preset: "tap", Rect: Face{X: 0.8, Width: 0.4, Height: 1}}}
		}, "normalized sub-rectangle"},
		{"preset action", func(m *Manifest) {
			m.Presets = map[string]Preset{"tap": {Steps: []PresetStep{{Action: "missing"}}}}
		}, "missing action"},
		{"timing", func(m *Manifest) {
			m.Actions["happy"] = withTiming(m.Actions["happy"], Timing{EnterHoldMs: -1})
		}, "enterHoldMs"},
		{"hotzone action", func(m *Manifest) {
			m.Hotzones = map[string]Hotzone{"face": {Preset: "tap", Action: "missing", Rect: Face{Width: 1, Height: 1}}}
		}, "missing action"},
		{"hotzone looping action", func(m *Manifest) {
			m.Hotzones = map[string]Hotzone{"face": {Preset: "tap", Action: "idle", Rect: Face{Width: 1, Height: 1}}}
		}, "loops until interrupted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validTestManifest()
			manifest.Hotzones = map[string]Hotzone{
				"face": {Preset: "tap", Rect: Face{X: 0.2, Y: 0.2, Width: 0.4, Height: 0.4}},
			}
			tt.mutate(&manifest)
			normalizeManifest(&manifest)
			if err := validateManifest("tiny", manifest); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
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

func TestDocumentedSchema2ExampleIsValid(t *testing.T) {
	body, err := os.ReadFile("../../docs/examples/pet-pack.schema2.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	normalizeManifest(&manifest)
	if err := validateManifest("example", manifest); err != nil {
		t.Fatal(err)
	}
}

func TestReadRawManifestKeepsAuthoredSchema(t *testing.T) {
	dir := writeTinyPack(t)
	raw, err := ReadRawManifest(filepath.Join(dir, "packs", "tiny"), "tiny")
	if err != nil {
		t.Fatal(err)
	}
	if raw.Schema != 1 || len(raw.Hotzones) != 0 || len(raw.Presets) != 0 {
		t.Fatalf("raw manifest = schema %d, hotzones %d, presets %d", raw.Schema, len(raw.Hotzones), len(raw.Presets))
	}
	normalized, err := ReadManifest(filepath.Join(dir, "packs", "tiny"), "tiny")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Schema != 2 || len(normalized.Hotzones) == 0 || len(normalized.Presets) == 0 {
		t.Fatalf("normalized manifest = schema %d, hotzones %d, presets %d", normalized.Schema, len(normalized.Hotzones), len(normalized.Presets))
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
		Schema:       2,
		ID:           "tiny",
		Name:         "Tiny",
		Canvas:       Size{Width: 16, Height: 16},
		Crop:         Rect{Width: 16, Height: 16},
		Base:         Base{Window: Size{Width: 20, Height: 20}, Sprite: Size{Width: 16, Height: 16}},
		Face:         Face{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
		BubbleAnchor: Anchor{X: 0.5, Y: 0.1},
		Clips: map[string]ClipEntry{
			"idle_loop": {File: "clips/idle_loop.webp", Frames: 3, Loop: true},
		},
		Actions: map[string]Action{
			"idle":  {Loop: "idle_loop", Poster: "idle_loop#0", FaceClickable: true, LoopCount: -1},
			"happy": {Once: "idle_loop", Poster: "idle_loop#0"},
			"laugh": {Once: "idle_loop", Poster: "idle_loop#0"},
		},
	}
}

func withTiming(action Action, timing Timing) Action {
	action.Timing = timing
	return action
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
