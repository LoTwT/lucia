package pet

import (
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	defaultScale = 1.0
	minScale     = 0.6
	maxScale     = 2.5
)

// dimension is one edge length in the pack's base units.
type dimension struct {
	Width  int
	Height int
}

// assetLayout describes one pack's canvas and how the character sits in its
// window. Hotzones are normalized to the cropped artwork, so scaling never
// needs new hard-coded click regions.
type assetLayout struct {
	Canvas dimension
	Window dimension
	Sprite dimension
}

func (m *Model) applyLayout() {
	windowWidth := max(1, int(math.Round(float64(m.layout.Window.Width)*m.scale)))
	windowHeight := max(1, int(math.Round(float64(m.layout.Window.Height)*m.scale)))
	spriteWidth := max(1, int(math.Round(float64(m.layout.Sprite.Width)*m.scale)))
	spriteHeight := max(1, int(math.Round(float64(m.layout.Sprite.Height)*m.scale)))
	originX := float32(windowWidth-spriteWidth) / 2
	originY := float32(windowHeight-spriteHeight) / 2

	// Artwork with a different aspect ratio is letterboxed inside the sprite
	// box. Hotzones and the bubble anchor follow the drawn image, not the box.
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
	anchorX := imageX + float32(m.bubbleAnchorX*float64(imageWidth))
	anchorY := imageY + float32(m.bubbleAnchorY*float64(imageHeight))
	m.bubbleRect = ui.Rect{
		X: anchorX,
		Y: anchorY,
		W: 1,
		H: max(1, float32(windowHeight)-anchorY),
	}
	m.layoutHotzones()
}

func (m *Model) layoutHotzones() {
	m.hotzones = m.hotzones[:0]
	for _, entry := range m.hotzoneEntries {
		rect := entry.Rect
		m.hotzones = append(m.hotzones, Hotzone{
			ID: entry.ID, Label: entry.Label, Interaction: entry.Interaction,
			Rect: ui.Rect{
				X: m.imageRect.X + float32(rect.X*float64(m.imageRect.W)),
				Y: m.imageRect.Y + float32(rect.Y*float64(m.imageRect.H)),
				W: float32(rect.Width * float64(m.imageRect.W)),
				H: float32(rect.Height * float64(m.imageRect.H)),
			},
		})
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

// Hotzones returns the clickable regions in window DIPs. The slice is copied
// so callers cannot mutate the model's layout.
func (m *Model) Hotzones() []Hotzone {
	return append([]Hotzone(nil), m.hotzones...)
}

// ImageRect returns the drawn artwork rectangle in window DIPs. The sprite
// box may be larger when the artwork has a different aspect ratio.
func (m *Model) ImageRect() ui.Rect { return m.imageRect }

// BubbleRect returns a narrow anchor for the speech bubble: its top is the
// point above which the bubble is placed, and its bottom is the pet window's
// bottom so a bubble that flips below clears the whole character.
func (m *Model) BubbleRect() ui.Rect { return m.bubbleRect }
