package pet

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	defaultScale = 1.0
	minScale     = 0.6
	maxScale     = 2.5
)

//go:embed assets/deepseek-whale/layout.json
var layoutJSON []byte

// dimension is one edge length in the legacy integer form or explicit
// width/height form. The legacy form keeps existing artwork layouts valid
// while rectangular skins describe both edges.
type dimension struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (d *dimension) UnmarshalJSON(data []byte) error {
	var edge int
	if err := json.Unmarshal(data, &edge); err == nil {
		d.Width, d.Height = edge, edge
		return nil
	}
	var value struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		W      int `json:"w"`
		H      int `json:"h"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	d.Width = value.Width
	d.Height = value.Height
	if d.Width == 0 {
		d.Width = value.W
	}
	if d.Height == 0 {
		d.Height = value.H
	}
	return nil
}

// assetLayout describes one artwork's canvas and how the character sits in
// its window. Face coordinates are normalized to the sprite rectangle, so
// scaling the window and sprite never needs new hard-coded click regions.
type assetLayout struct {
	Canvas dimension `json:"canvas"`
	Window dimension `json:"window"`
	Sprite dimension `json:"sprite"`
	Face   struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"face"`
}

func loadLayout() (assetLayout, error) {
	var layout assetLayout
	if err := json.Unmarshal(layoutJSON, &layout); err != nil {
		return assetLayout{}, fmt.Errorf("decode pet layout: %w", err)
	}
	if layout.Canvas.Width <= 0 || layout.Canvas.Height <= 0 ||
		layout.Window.Width <= 0 || layout.Window.Height <= 0 ||
		layout.Sprite.Width <= 0 || layout.Sprite.Height <= 0 {
		return assetLayout{}, fmt.Errorf("pet layout has non-positive canvas, window, or sprite")
	}
	if layout.Sprite.Width > layout.Window.Width || layout.Sprite.Height > layout.Window.Height {
		return assetLayout{}, fmt.Errorf("pet layout sprite %dx%d exceeds window %dx%d",
			layout.Sprite.Width, layout.Sprite.Height, layout.Window.Width, layout.Window.Height)
	}
	face := layout.Face
	if face.Width <= 0 || face.Height <= 0 ||
		face.X < 0 || face.Y < 0 ||
		face.X+face.Width > 1 || face.Y+face.Height > 1 {
		return assetLayout{}, fmt.Errorf("pet layout face is not a normalized sub-rectangle")
	}
	return layout, nil
}

func (m *Model) applyLayout() {
	windowWidth := max(1, int(math.Round(float64(m.layout.Window.Width)*m.scale)))
	windowHeight := max(1, int(math.Round(float64(m.layout.Window.Height)*m.scale)))
	spriteWidth := max(1, int(math.Round(float64(m.layout.Sprite.Width)*m.scale)))
	spriteHeight := max(1, int(math.Round(float64(m.layout.Sprite.Height)*m.scale)))
	originX := float32(windowWidth-spriteWidth) / 2
	originY := float32(windowHeight-spriteHeight) / 2

	// Artwork with a different aspect ratio is letterboxed inside the sprite
	// box. Face and bubble coordinates follow the drawn image, not the box.
	imageScale := math.Min(
		float64(spriteWidth)/float64(m.layout.Canvas.Width),
		float64(spriteHeight)/float64(m.layout.Canvas.Height),
	)
	imageWidth := float32(float64(m.layout.Canvas.Width) * imageScale)
	imageHeight := float32(float64(m.layout.Canvas.Height) * imageScale)
	imageX := originX + (float32(spriteWidth)-imageWidth)/2
	imageY := originY + (float32(spriteHeight)-imageHeight)/2

	m.windowWidth = windowWidth
	m.windowHeight = windowHeight
	m.spriteWidth = spriteWidth
	m.spriteHeight = spriteHeight
	m.imageRect = ui.Rect{X: imageX, Y: imageY, W: imageWidth, H: imageHeight}
	m.faceRegion = ui.Rect{
		X: imageX + float32(m.layout.Face.X*float64(imageWidth)),
		Y: imageY + float32(m.layout.Face.Y*float64(imageHeight)),
		W: float32(m.layout.Face.Width * float64(imageWidth)),
		H: float32(m.layout.Face.Height * float64(imageHeight)),
	}
	anchorX := imageX + float32(m.bubbleAnchorX*float64(imageWidth))
	anchorY := imageY + float32(m.bubbleAnchorY*float64(imageHeight))
	m.bubbleRect = ui.Rect{
		X: anchorX,
		Y: anchorY,
		W: 1,
		H: max(1, float32(windowHeight)-anchorY),
	}
}

// SetScale changes the window and sprite scale. It clamps invalid values so a
// hand-edited settings file cannot create a zero-sized window.
func (m *Model) SetScale(scale float64) {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		scale = defaultScale
	}
	scale = min(maxScale, max(minScale, scale))
	if scale == m.scale {
		return
	}
	m.scale = scale
	m.applyLayout()
	m.invalidate()
}

// Scale returns the current scale factor.
func (m *Model) Scale() float64 { return m.scale }

// WindowSize returns the edge lengths of the pet window in DIPs.
func (m *Model) WindowSize() (width, height int) { return m.windowWidth, m.windowHeight }

// SpriteSize returns the edge lengths of the drawn sprite in DIPs.
func (m *Model) SpriteSize() (width, height int) { return m.spriteWidth, m.spriteHeight }

// FaceRegion returns the clickable face rectangle in window DIPs.
func (m *Model) FaceRegion() ui.Rect { return m.faceRegion }

// ImageRect returns the drawn artwork rectangle in window DIPs. The sprite
// box may be larger when the artwork has a different aspect ratio.
func (m *Model) ImageRect() ui.Rect { return m.imageRect }

// BubbleRect returns a narrow anchor for the speech bubble: its top is the
// point above which the bubble is placed, and its bottom is the pet window's
// bottom so a bubble that flips below clears the whole character.
func (m *Model) BubbleRect() ui.Rect { return m.bubbleRect }
