package petpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	stddraw "image/draw"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Size is an edge length in pixels or DIPs, depending on the field.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Rect is a pixel rectangle in a clip's canvas.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Rect) ImageRect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height)
}

func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// Face is a normalized face hit rectangle relative to the cropped artwork.
type Face struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Anchor is a normalized point on the cropped artwork.
type Anchor struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Base is the presentation size at 1.0×. Sprite is the artwork box; Window
// adds the transparent drag margin around it.
type Base struct {
	Window Size `json:"window"`
	Sprite Size `json:"sprite"`
	Margin int  `json:"margin"`
}

// ClipEntry is one animation file in the manifest.
type ClipEntry struct {
	File   string `json:"file"`
	Frames int    `json:"frames,omitempty"`
	Loop   bool   `json:"loop,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// Timing holds extra pauses after each phase. Frame metadata still decides
// how long every frame stays visible; these values let a pack add a beat
// between the authored segments without changing the WebP.
type Timing struct {
	EnterHoldMs int `json:"enterHoldMs,omitempty"`
	LoopHoldMs  int `json:"loopHoldMs,omitempty"`
	ExitHoldMs  int `json:"exitHoldMs,omitempty"`
}

// Action maps one user-facing behavior to clip IDs. Enter, Loop and Exit are
// a chain; Once is a single clip. Poster names a clip and frame for Reduce
// Motion and is written as "clip#frame".
type Action struct {
	Label         string `json:"label,omitempty"`
	Enter         string `json:"enter,omitempty"`
	Loop          string `json:"loop,omitempty"`
	Exit          string `json:"exit,omitempty"`
	Once          string `json:"once,omitempty"`
	Poster        string `json:"poster,omitempty"`
	FaceClickable bool   `json:"faceClickable,omitempty"`
	// LoopCount is how many full loop passes precede the exit. Zero skips
	// the loop, -1 means "until the state ends" (idle and drag).
	LoopCount int    `json:"loopCount,omitempty"`
	Timing    Timing `json:"timing,omitempty"`
}

// Hotzone is one normalized clickable rectangle on the cropped artwork.
// Preset selects an interaction sequence; Action is used when the preset
// has no explicit steps.
type Hotzone struct {
	Label  string `json:"label,omitempty"`
	Preset string `json:"preset,omitempty"`
	Action string `json:"action,omitempty"`
	Rect   Face   `json:"rect"`
}

// PresetStep is one action in a preset sequence. DelayMs is applied before
// the step starts, after the previous action finishes.
type PresetStep struct {
	Action  string `json:"action"`
	DelayMs int    `json:"delayMs,omitempty"`
}

// Preset is a reusable interaction sequence triggered by a hotzone.
// Reaction and Bubble accept "face", "pat" or "none".
type Preset struct {
	Reaction string       `json:"reaction,omitempty"`
	Bubble   string       `json:"bubble,omitempty"`
	Steps    []PresetStep `json:"steps,omitempty"`
}

// Manifest is the pack.json schema. Schema 1 is upgraded in memory to the
// schema 2 defaults so the player and console only deal with one shape.
type Manifest struct {
	Schema       int                  `json:"schema"`
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	License      json.RawMessage      `json:"license,omitempty"`
	Canvas       Size                 `json:"canvas"`
	Crop         Rect                 `json:"crop"`
	Base         Base                 `json:"base"`
	Face         Face                 `json:"face"`
	BubbleAnchor Anchor               `json:"bubbleAnchor"`
	Clips        map[string]ClipEntry `json:"clips"`
	Actions      map[string]Action    `json:"actions"`
	Hotzones     map[string]Hotzone   `json:"hotzones,omitempty"`
	Presets      map[string]Preset    `json:"presets,omitempty"`
}

// HotzoneEntry is a named hotzone in stable display order.
type HotzoneEntry struct {
	ID string
	Hotzone
}

// Pack is a validated manifest plus its parsed clips.
type Pack struct {
	Dir          string
	Manifest     Manifest
	Clips        map[string]*Clip
	PixelWidth   int
	PixelHeight  int
	DisplayScale float64
}

// Load reads and validates a pack below root/packs/<id>.
func Load(root, id string, displayScale float64) (*Pack, error) {
	return LoadDir(filepath.Join(root, "packs", id), id, displayScale)
}

// LoadDir validates a pack from an explicit directory. It is used by the
// management store and the check command.
func LoadDir(dir, id string, displayScale float64) (*Pack, error) {
	return loadPackFS(os.DirFS(dir), id, displayScale, dir)
}

// LoadFS validates a pack from any filesystem. The built-in pack uses an
// embedded FS; installed packs use os.DirFS.
func LoadFS(fsys fs.FS, id string, displayScale float64) (*Pack, error) {
	return loadPackFS(fsys, id, displayScale, "")
}

// ReadManifest parses and validates only the manifest. Management listing
// uses it so a broken clip does not make an otherwise visible pack vanish.
func ReadManifest(dir, id string) (Manifest, error) {
	return ReadManifestFS(os.DirFS(dir), id)
}

// ReadManifestFS parses and validates only the manifest from a filesystem.
func ReadManifestFS(fsys fs.FS, id string) (Manifest, error) {
	if !validPackDirID(id) {
		return Manifest{}, fmt.Errorf("petpack: invalid pack id %q", id)
	}
	manifest, err := readManifestFS(fsys, id)
	if err != nil {
		return Manifest{}, err
	}
	if err := validateManifest(id, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ReadRawManifest parses pack.json without applying schema defaults. The
// strict authoring check needs to distinguish an explicit schema 2 hotzone
// from one synthesized for a schema 1 pack.
func ReadRawManifest(dir, id string) (Manifest, error) {
	if !validPackDirID(id) {
		return Manifest{}, fmt.Errorf("petpack: invalid pack id %q", id)
	}
	return readRawManifestFS(os.DirFS(dir), id)
}

func readManifestFS(fsys fs.FS, id string) (Manifest, error) {
	manifest, err := readRawManifestFS(fsys, id)
	if err != nil {
		return Manifest{}, err
	}
	normalizeManifest(&manifest)
	return manifest, nil
}

func readRawManifestFS(fsys fs.FS, id string) (Manifest, error) {
	data, err := fs.ReadFile(fsys, "pack.json")
	if err != nil {
		return Manifest{}, fmt.Errorf("petpack %q: read manifest: %w", id, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("petpack %q: parse manifest: %w", id, err)
	}
	return manifest, nil
}

// loadPackFS validates a pack from its final filesystem. Import uses it after
// installing the new directory, without assuming the parent directory is
// named "packs".
func loadPackFS(fsys fs.FS, id string, displayScale float64, dir string) (*Pack, error) {
	if !validPackDirID(id) {
		return nil, fmt.Errorf("petpack: invalid pack id %q", id)
	}
	manifest, err := readManifestFS(fsys, id)
	if err != nil {
		return nil, err
	}
	if err := validateManifest(id, manifest); err != nil {
		return nil, err
	}
	if displayScale <= 0 || math.IsNaN(displayScale) || math.IsInf(displayScale, 0) {
		displayScale = 1
	}
	pack := &Pack{
		Dir:          dir,
		Manifest:     manifest,
		Clips:        make(map[string]*Clip, len(manifest.Clips)),
		DisplayScale: displayScale,
	}
	crop := manifest.Crop.ImageRect()
	if manifest.Crop.Empty() {
		crop = image.Rect(0, 0, manifest.Canvas.Width, manifest.Canvas.Height)
	}
	for clipID, entry := range manifest.Clips {
		if entry.File == "" {
			return nil, fmt.Errorf("petpack %q clip %q: empty file", id, clipID)
		}
		if err := validPackFilePath(entry.File); err != nil {
			return nil, fmt.Errorf("petpack %q clip %q: %w", id, clipID, err)
		}
		clip, err := OpenClipFS(clipID, fsys, entry.File)
		if err != nil {
			return nil, fmt.Errorf("petpack %q clip %q: %w", id, clipID, err)
		}
		if clip.Width != manifest.Canvas.Width || clip.Height != manifest.Canvas.Height {
			return nil, fmt.Errorf("petpack %q clip %q: canvas %dx%d, want %dx%d",
				id, clipID, clip.Width, clip.Height, manifest.Canvas.Width, manifest.Canvas.Height)
		}
		if entry.Frames > 0 && entry.Frames != clip.FrameCount() {
			return nil, fmt.Errorf("petpack %q clip %q: %d frames, want %d",
				id, clipID, clip.FrameCount(), entry.Frames)
		}
		sum := sha256.Sum256(clip.data)
		clip.digest = sum
		clip.hasDigest = true
		if entry.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), entry.SHA256) {
			return nil, fmt.Errorf("petpack %q clip %q: sha256 mismatch", id, clipID)
		}
		clip.Loop = entry.Loop
		clip.Crop = crop
		clip.data = nil // loaded again by each Decoder from clip.path
		pack.Clips[clipID] = clip
	}
	for actionID, action := range manifest.Actions {
		for _, clipID := range actionClips(action) {
			if clipID != "" {
				if _, ok := pack.Clips[clipID]; !ok {
					return nil, fmt.Errorf("petpack %q action %q references missing clip %q", id, actionID, clipID)
				}
			}
		}
		if action.Poster != "" {
			if _, _, err := splitPoster(action.Poster); err != nil {
				return nil, fmt.Errorf("petpack %q action %q: %w", id, actionID, err)
			}
		}
	}
	// Cache at up to 2× the DIP size so a Retina display has enough pixels,
	// but never beyond the crop's native resolution. Both edges use one scale
	// factor so the bitmap keeps the crop's aspect ratio; layout maps face and
	// bubble coordinates onto that same aspect.
	cacheScale := math.Max(displayScale, 2)
	pack.PixelWidth, pack.PixelHeight = cacheSize(
		manifest.Base.Sprite.Width, manifest.Base.Sprite.Height,
		crop.Dx(), crop.Dy(), cacheScale,
	)
	if pack.PixelWidth <= 0 || pack.PixelHeight <= 0 {
		return nil, fmt.Errorf("petpack %q: non-positive cache size", id)
	}
	return pack, nil
}

func validateManifest(id string, manifest Manifest) error {
	if manifest.Schema != 1 && manifest.Schema != 2 {
		return fmt.Errorf("petpack %q: schema %d, want 1 or 2", id, manifest.Schema)
	}
	if manifest.ID != "" && manifest.ID != id {
		return fmt.Errorf("petpack %q: manifest id %q", id, manifest.ID)
	}
	if err := validateCanvasSize(id, manifest.Canvas.Width, manifest.Canvas.Height); err != nil {
		return err
	}
	canvas := image.Rect(0, 0, manifest.Canvas.Width, manifest.Canvas.Height)
	if !manifest.Crop.Empty() && !manifest.Crop.ImageRect().In(canvas) {
		return fmt.Errorf("petpack %q: crop %v outside canvas %v", id, manifest.Crop, canvas)
	}
	if manifest.Base.Window.Width <= 0 || manifest.Base.Window.Height <= 0 ||
		manifest.Base.Sprite.Width <= 0 || manifest.Base.Sprite.Height <= 0 {
		return fmt.Errorf("petpack %q: invalid base sizes", id)
	}
	if manifest.Base.Window.Width > maxCanvasEdge || manifest.Base.Window.Height > maxCanvasEdge {
		return fmt.Errorf("petpack %q: window %dx%d exceeds the %dpx limit",
			id, manifest.Base.Window.Width, manifest.Base.Window.Height, maxCanvasEdge)
	}
	if manifest.Base.Sprite.Width > manifest.Base.Window.Width ||
		manifest.Base.Sprite.Height > manifest.Base.Window.Height {
		return fmt.Errorf("petpack %q: sprite exceeds window", id)
	}
	face := manifest.Face
	if face.Width > 0 || face.Height > 0 {
		if face.Width <= 0 || face.Height <= 0 || face.X < 0 || face.Y < 0 ||
			face.X+face.Width > 1 || face.Y+face.Height > 1 {
			return fmt.Errorf("petpack %q: face is not a normalized sub-rectangle", id)
		}
	} else if len(manifest.Hotzones) == 0 {
		return fmt.Errorf("petpack %q: face is not a normalized sub-rectangle", id)
	}
	if manifest.BubbleAnchor.X < 0 || manifest.BubbleAnchor.X > 1 ||
		manifest.BubbleAnchor.Y < 0 || manifest.BubbleAnchor.Y > 1 {
		return fmt.Errorf("petpack %q: bubble anchor is not normalized", id)
	}
	if len(manifest.Clips) == 0 {
		return fmt.Errorf("petpack %q: no clips", id)
	}
	for actionID, action := range manifest.Actions {
		if action.LoopCount < -1 {
			return fmt.Errorf("petpack %q action %q: loopCount %d", id, actionID, action.LoopCount)
		}
		for name, hold := range map[string]int{
			"enterHoldMs": action.Timing.EnterHoldMs,
			"loopHoldMs":  action.Timing.LoopHoldMs,
			"exitHoldMs":  action.Timing.ExitHoldMs,
		} {
			if hold < 0 || hold > 60000 {
				return fmt.Errorf("petpack %q action %q: %s %d outside 0..60000", id, actionID, name, hold)
			}
		}
	}
	if len(manifest.Presets) > 64 {
		return fmt.Errorf("petpack %q: more than 64 presets", id)
	}
	for presetID, preset := range manifest.Presets {
		if !validPackID(presetID) {
			return fmt.Errorf("petpack %q: invalid preset id %q", id, presetID)
		}
		switch preset.Reaction {
		case "", "face", "pat", "none":
		default:
			return fmt.Errorf("petpack %q preset %q: invalid reaction %q", id, presetID, preset.Reaction)
		}
		switch preset.Bubble {
		case "", "face", "pat", "none":
		default:
			return fmt.Errorf("petpack %q preset %q: invalid bubble %q", id, presetID, preset.Bubble)
		}
		if len(preset.Steps) > 32 {
			return fmt.Errorf("petpack %q preset %q: more than 32 steps", id, presetID)
		}
		for i, step := range preset.Steps {
			if step.Action == "" {
				return fmt.Errorf("petpack %q preset %q step %d: empty action", id, presetID, i)
			}
			action, ok := manifest.Actions[step.Action]
			if !ok {
				return fmt.Errorf("petpack %q preset %q step %d references missing action %q", id, presetID, i, step.Action)
			}
			if err := validateFiniteAction(step.Action, action); err != nil {
				return fmt.Errorf("petpack %q preset %q step %d: %w", id, presetID, i, err)
			}
			if step.DelayMs < 0 || step.DelayMs > 60000 {
				return fmt.Errorf("petpack %q preset %q step %d: delay %d outside 0..60000", id, presetID, i, step.DelayMs)
			}
		}
	}
	if len(manifest.Hotzones) > 64 {
		return fmt.Errorf("petpack %q: more than 64 hotzones", id)
	}
	for hotzoneID, zone := range manifest.Hotzones {
		if !validPackID(hotzoneID) {
			return fmt.Errorf("petpack %q: invalid hotzone id %q", id, hotzoneID)
		}
		rect := zone.Rect
		if rect.Width <= 0 || rect.Height <= 0 || rect.X < 0 || rect.Y < 0 ||
			rect.X+rect.Width > 1 || rect.Y+rect.Height > 1 {
			return fmt.Errorf("petpack %q hotzone %q: rect is not a normalized sub-rectangle", id, hotzoneID)
		}
		if _, ok := manifest.Presets[zone.Preset]; !ok {
			return fmt.Errorf("petpack %q hotzone %q: missing preset %q", id, hotzoneID, zone.Preset)
		}
		if zone.Action != "" {
			action, ok := manifest.Actions[zone.Action]
			if !ok {
				return fmt.Errorf("petpack %q hotzone %q: missing action %q", id, hotzoneID, zone.Action)
			}
			if err := validateFiniteAction(zone.Action, action); err != nil {
				return fmt.Errorf("petpack %q hotzone %q: %w", id, hotzoneID, err)
			}
		}
	}
	return nil
}

func actionClips(action Action) []string {
	return []string{action.Enter, action.Loop, action.Exit, action.Once}
}

func validateFiniteAction(actionID string, action Action) error {
	if action.LoopCount < 0 {
		return fmt.Errorf("action %q loops until interrupted", actionID)
	}
	if action.Once == "" && action.Enter == "" && action.Loop == "" && action.Exit == "" {
		return fmt.Errorf("action %q has no clip", actionID)
	}
	return nil
}

// normalizeManifest turns schema 1 into the same in-memory shape as schema 2
// and fills the built-in interaction defaults. The original file is not
// rewritten; import and export use schema 2 explicitly.
func normalizeManifest(manifest *Manifest) {
	if manifest.Schema == 1 {
		manifest.Schema = 2
	}
	if manifest.Hotzones == nil {
		manifest.Hotzones = make(map[string]Hotzone)
	}
	if manifest.Presets == nil {
		manifest.Presets = make(map[string]Preset)
	}
	if len(manifest.Hotzones) == 0 && manifest.Face.Width > 0 && manifest.Face.Height > 0 {
		manifest.Hotzones["face"] = Hotzone{
			Label:  "摸摸脸",
			Preset: "tap",
			Rect:   manifest.Face,
		}
	}
	for hotspotID, zone := range manifest.Hotzones {
		if zone.Preset == "" {
			zone.Preset = "tap"
			manifest.Hotzones[hotspotID] = zone
		}
	}
	if _, ok := manifest.Presets["tap"]; !ok {
		steps := defaultPresetSteps(manifest.Actions, "laugh", "happy")
		manifest.Presets["tap"] = Preset{Reaction: "face", Bubble: "face", Steps: steps}
	}
	if _, ok := manifest.Presets["pat"]; !ok {
		steps := defaultPresetSteps(manifest.Actions, "happy", "laugh")
		manifest.Presets["pat"] = Preset{Reaction: "pat", Bubble: "pat", Steps: steps}
	}
	for presetID, preset := range manifest.Presets {
		if preset.Reaction == "" {
			switch presetID {
			case "tap":
				preset.Reaction = "face"
			case "pat":
				preset.Reaction = "pat"
			default:
				preset.Reaction = "none"
			}
		}
		if preset.Bubble == "" {
			switch presetID {
			case "tap":
				preset.Bubble = "face"
			case "pat":
				preset.Bubble = "pat"
			default:
				preset.Bubble = "none"
			}
		}
		manifest.Presets[presetID] = preset
	}
}

func defaultPresetSteps(actions map[string]Action, candidates ...string) []PresetStep {
	for _, id := range candidates {
		action, ok := actions[id]
		if !ok || action.LoopCount < 0 {
			continue
		}
		if action.Once == "" && action.Enter == "" && action.Loop == "" && action.Exit == "" {
			continue
		}
		return []PresetStep{{Action: id}}
	}
	return nil
}

// Preset returns a preset by id.
func (p *Pack) Preset(id string) (Preset, bool) {
	preset, ok := p.Manifest.Presets[id]
	return preset, ok
}

// HotzoneList returns hotzones in stable id order. A map cannot be iterated
// on the UI thread without producing a different click order every frame.
func (m Manifest) HotzoneList() []HotzoneEntry {
	out := make([]HotzoneEntry, 0, len(m.Hotzones))
	for id, zone := range m.Hotzones {
		out = append(out, HotzoneEntry{ID: id, Hotzone: zone})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (p *Pack) HotzoneList() []HotzoneEntry { return p.Manifest.HotzoneList() }

func splitPoster(poster string) (string, int, error) {
	clipID, frameText, ok := strings.Cut(poster, "#")
	if !ok || clipID == "" || frameText == "" {
		return "", 0, fmt.Errorf("poster %q must be clip#frame", poster)
	}
	frame, err := strconv.Atoi(frameText)
	if err != nil || frame < 0 {
		return "", 0, fmt.Errorf("poster %q has an invalid frame", poster)
	}
	return clipID, frame, nil
}

func validPackID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// validPackDirID excludes names Windows reserves for devices. Preset and
// hotzone ids do not become paths and intentionally still use validPackID.
func validPackDirID(id string) bool {
	return validPackID(id) && !windowsReservedPackID(id)
}

func windowsReservedPackID(id string) bool {
	switch strings.ToLower(id) {
	case "con", "prn", "aux", "nul":
		return true
	}
	lower := strings.ToLower(id)
	if len(lower) == 4 && (strings.HasPrefix(lower, "com") || strings.HasPrefix(lower, "lpt")) {
		return lower[3] >= '1' && lower[3] <= '9'
	}
	return false
}

func validPackFilePath(name string) error {
	if name == "" || !fs.ValidPath(name) {
		return fmt.Errorf("invalid clip path %q", name)
	}
	return nil
}

// cacheSize fits the crop into the desired sprite box at one uniform scale.
// Keeping a single scale is important: the pet's face hitbox and bubble anchor
// are calculated from the crop's aspect ratio, so stretching one edge would
// move those targets away from the drawn artwork.
func cacheSize(spriteWidth, spriteHeight, cropWidth, cropHeight int, scale float64) (int, int) {
	if cropWidth <= 0 || cropHeight <= 0 || spriteWidth <= 0 || spriteHeight <= 0 {
		return 0, 0
	}
	fit := math.Min(
		float64(spriteWidth)*scale/float64(cropWidth),
		float64(spriteHeight)*scale/float64(cropHeight),
	)
	fit = math.Min(1, fit)
	return max(1, int(math.Round(float64(cropWidth)*fit))),
		max(1, int(math.Round(float64(cropHeight)*fit)))
}

// Action returns one action by id.
func (p *Pack) Action(id string) (Action, bool) {
	action, ok := p.Manifest.Actions[id]
	return action, ok
}

// Clip returns one clip by id.
func (p *Pack) Clip(id string) (*Clip, bool) {
	clip, ok := p.Clips[id]
	return clip, ok
}

// Poster returns a cached image for an action's poster frame. The clip is
// decoded from the start and must be walked sequentially.
func (p *Pack) Poster(action Action) (image.Image, error) {
	if action.Poster == "" {
		return nil, fmt.Errorf("action has no poster")
	}
	clipID, frameIndex, err := splitPoster(action.Poster)
	if err != nil {
		return nil, err
	}
	clip, ok := p.Clips[clipID]
	if !ok {
		return nil, fmt.Errorf("poster references missing clip %q", clipID)
	}
	if frameIndex >= clip.FrameCount() {
		frameIndex = clip.FrameCount() - 1
	}
	decoder, err := clip.NewDecoder()
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	for i := 0; i <= frameIndex; i++ {
		img, _, err := decoder.NextScaled(p.PixelWidth, p.PixelHeight)
		if err != nil {
			return nil, err
		}
		if i == frameIndex {
			return copyImage(img), nil
		}
	}
	return nil, fmt.Errorf("poster frame %d not found", frameIndex)
}

func copyImage(img image.Image) *image.RGBA {
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	stddraw.Draw(dst, dst.Bounds(), img, bounds.Min, stddraw.Src)
	return dst
}
