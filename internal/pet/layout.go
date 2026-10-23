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

// assetLayout describes one artwork's canvas and how the character sits in
// its window. Face coordinates are normalized to the sprite rectangle, so
// scaling the window and sprite never needs new hard-coded click regions.
type assetLayout struct {
	Canvas int `json:"canvas"`
	Window int `json:"window"`
	Sprite int `json:"sprite"`
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
	if layout.Canvas <= 0 || layout.Window <= 0 || layout.Sprite <= 0 {
		return assetLayout{}, fmt.Errorf("pet layout has non-positive canvas, window, or sprite")
	}
	if layout.Sprite > layout.Window {
		return assetLayout{}, fmt.Errorf("pet layout sprite %d exceeds window %d", layout.Sprite, layout.Window)
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
	window := int(math.Round(float64(m.layout.Window) * m.scale))
	sprite := int(math.Round(float64(m.layout.Sprite) * m.scale))
	if window < 1 {
		window = 1
	}
	if sprite < 1 {
		sprite = 1
	}
	origin := float32(window-sprite) / 2
	m.windowSize = window
	m.spriteSize = sprite
	m.faceRegion = ui.Rect{
		X: origin + float32(m.layout.Face.X*float64(sprite)),
		Y: origin + float32(m.layout.Face.Y*float64(sprite)),
		W: float32(m.layout.Face.Width * float64(sprite)),
		H: float32(m.layout.Face.Height * float64(sprite)),
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

// Size returns the edge length of the pet window in DIPs.
func (m *Model) Size() int { return m.windowSize }

// SpriteSize returns the edge length of the drawn sprite in DIPs.
func (m *Model) SpriteSize() int { return m.spriteSize }

// FaceRegion returns the clickable face rectangle in window DIPs.
func (m *Model) FaceRegion() ui.Rect { return m.faceRegion }
