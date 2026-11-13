package pet

import (
	"image"
	"math"
)

const (
	maxInputRegionRects = 4096
	maxInputRegionEdge  = 512
)

// Only the current frame is retained. Streaming decoders reuse two RGBA
// buffers, so pointer identity alone cannot identify their contents.
type inputRegionCache struct {
	image  *image.RGBA
	frame  int
	width  float32
	height float32
	local  []image.Rectangle
	window []image.Rectangle
}

// InputRegion returns the last view's mouse region in window-local DIPs.
// It is read-only and valid until the next View. An empty region takes no
// mouse input; the host must not turn it into nil (which means full input).
func (m *Model) InputRegion() []image.Rectangle { return m.inputRegion.window }

func (m *Model) updateInputRegion(img image.Image, frame int, offsetY float32) {
	cache := &m.inputRegion
	rgba, cacheable := img.(*image.RGBA)
	if !cacheable || cache.image != rgba || cache.frame != frame ||
		cache.width != m.imageRect.W || cache.height != m.imageRect.H {
		cache.image, cache.frame = rgba, frame
		cache.width, cache.height = m.imageRect.W, m.imageRect.H
		cache.local = alphaInputRegion(img, m.imageRect.W, m.imageRect.H)
	}
	cache.window = cache.window[:0]
	if cache.window == nil {
		cache.window = make([]image.Rectangle, 0)
	}
	bounds := image.Rect(0, 0, m.windowWidth, m.windowHeight)
	for _, r := range cache.local {
		// Round outwards: a partly covered DIP still belongs to the pet.
		// Fractional Contain positioning and bob/lift use the same origin
		// as the image. Clip away pixels outside the native content area.
		rect := image.Rect(
			int(math.Floor(float64(m.imageRect.X)+float64(r.Min.X))),
			int(math.Floor(float64(m.imageRect.Y+offsetY)+float64(r.Min.Y))),
			int(math.Ceil(float64(m.imageRect.X)+math.Min(float64(r.Max.X), float64(m.imageRect.W)))),
			int(math.Ceil(float64(m.imageRect.Y+offsetY)+math.Min(float64(r.Max.Y), float64(m.imageRect.H)))),
		).Intersect(bounds)
		if !rect.Empty() {
			cache.window = append(cache.window, rect)
		}
	}
}

// alphaInputRegion samples coverage, not a pixel at the center of a cell:
// thin hair and semitransparent edges must remain reachable. Adjacent runs
// with identical edges merge vertically, preserving transparent holes.
// Unusually detailed/large artwork uses coarser cells to bound native work.
func alphaInputRegion(img image.Image, width, height float32) []image.Rectangle {
	if img == nil || img.Bounds().Empty() || width <= 0 || height <= 0 {
		return nil
	}
	cell := max(1, int(math.Ceil(float64(max(width, height))/maxInputRegionEdge)))
	for {
		if rects, ok := alphaInputRegionCells(img, width, height, cell); ok {
			return rects
		}
		cell *= 2
	}
}

func alphaInputRegionCells(img image.Image, width, height float32, cell int) ([]image.Rectangle, bool) {
	type run struct{ left, right, index int }
	var previous, current []run
	var rects []image.Rectangle
	bounds := img.Bounds()
	w, h := int(math.Ceil(float64(width))), int(math.Ceil(float64(height)))
	sx, sy := float64(bounds.Dx())/float64(width), float64(bounds.Dy())/float64(height)
	for y := 0; y < h; y += cell {
		current = current[:0]
		bottom := min(y+cell, h)
		sourceY := bounds.Min.Y + int(math.Floor(float64(y)*sy))
		sourceBottom := min(bounds.Max.Y, bounds.Min.Y+int(math.Ceil(float64(bottom)*sy)))
		left, prior := -1, 0
		for x := 0; x <= w; x += cell {
			// The final sentinel closes a run even when w is not a multiple
			// of cell. Source bounds can have a nonzero origin and stride.
			right := min(x+cell, w)
			opaque := x < w && hasAlpha(img, image.Rect(
				bounds.Min.X+int(math.Floor(float64(x)*sx)), sourceY,
				min(bounds.Max.X, bounds.Min.X+int(math.Ceil(float64(right)*sx))), sourceBottom,
			))
			if opaque && left < 0 {
				left = x
			}
			if left >= 0 && (!opaque || right == w) {
				end := x
				if opaque {
					end = right
				}
				for prior < len(previous) && previous[prior].left < left {
					prior++
				}
				index := len(rects)
				if prior < len(previous) && previous[prior].left == left && previous[prior].right == end {
					index = previous[prior].index
					rects[index].Max.Y = bottom
				} else {
					if len(rects) == maxInputRegionRects {
						return nil, false
					}
					rects = append(rects, image.Rect(left, y, end, bottom))
				}
				current = append(current, run{left: left, right: end, index: index})
				left = -1
			}
		}
		previous, current = current, previous
	}
	return rects, true
}

func hasAlpha(img image.Image, area image.Rectangle) bool {
	if rgba, ok := img.(*image.RGBA); ok {
		for y := area.Min.Y; y < area.Max.Y; y++ {
			start := rgba.PixOffset(area.Min.X, y)
			row := rgba.Pix[start : start+area.Dx()*4]
			for x := 3; x < len(row); x += 4 {
				if row[x] != 0 {
					return true
				}
			}
		}
		return false
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return true
			}
		}
	}
	return false
}
