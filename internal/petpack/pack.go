package petpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	stddraw "image/draw"
	"math"
	"os"
	"path/filepath"
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
	LoopCount int `json:"loopCount,omitempty"`
}

// Manifest is the pack.json schema.
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
	return loadPackDir(filepath.Join(root, "packs", id), id, displayScale)
}

// loadPackDir validates a pack from its final directory. Import uses it after
// installing the new directory, without assuming the parent directory is
// named "packs".
func loadPackDir(dir, id string, displayScale float64) (*Pack, error) {
	if !validPackID(id) {
		return nil, fmt.Errorf("petpack: invalid pack id %q", id)
	}
	data, err := os.ReadFile(filepath.Join(dir, "pack.json"))
	if err != nil {
		return nil, fmt.Errorf("petpack %q: read manifest: %w", id, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("petpack %q: parse manifest: %w", id, err)
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
		path, err := packPath(dir, entry.File)
		if err != nil {
			return nil, fmt.Errorf("petpack %q clip %q: %w", id, clipID, err)
		}
		clip, err := OpenClip(clipID, path)
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
	if manifest.Schema != 1 {
		return fmt.Errorf("petpack %q: schema %d, want 1", id, manifest.Schema)
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
	if face.Width <= 0 || face.Height <= 0 || face.X < 0 || face.Y < 0 ||
		face.X+face.Width > 1 || face.Y+face.Height > 1 {
		return fmt.Errorf("petpack %q: face is not a normalized sub-rectangle", id)
	}
	if manifest.BubbleAnchor.X < 0 || manifest.BubbleAnchor.X > 1 ||
		manifest.BubbleAnchor.Y < 0 || manifest.BubbleAnchor.Y > 1 {
		return fmt.Errorf("petpack %q: bubble anchor is not normalized", id)
	}
	if len(manifest.Clips) == 0 {
		return fmt.Errorf("petpack %q: no clips", id)
	}
	return nil
}

func actionClips(action Action) []string {
	return []string{action.Enter, action.Loop, action.Exit, action.Once}
}

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

func packPath(dir, name string) (string, error) {
	path := filepath.Join(dir, filepath.FromSlash(name))
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q escapes the pack directory", name)
	}
	return path, nil
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
