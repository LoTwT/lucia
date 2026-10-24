package pet

// Rect is a rectangle in screen DIPs. It mirrors mygo.Rectangle so the
// placement rules stay independent of the window toolkit.
type Rect struct {
	X, Y, Width, Height int
}

// Display is the geometry the placement rules need from a monitor. Bounds is
// the whole display; WorkArea excludes the menu bar, Dock and taskbars.
// ScaleFactor is available to callers that rebuild animation caches when the
// pet moves between displays.
type Display struct {
	Bounds      Rect
	WorkArea    Rect
	ScaleFactor float64
}

// KeepOnScreen returns the window position to use after a move. A window
// inside one display's work area stays put. A window straddling two displays
// may span their physical bounds, so it can cross a seam without being pulled
// back onto one monitor. Anything else is moved by the smallest translation
// into the nearest work area.
//
// The separate work-area rule matters at an outer edge: the pet stops next to
// the Dock or menu bar instead of sliding under it. It also means a clamp is a
// hard stop rather than a jump inward by the size of that chrome.
func KeepOnScreen(win Rect, displays []Display) (Rect, bool) {
	if len(displays) == 0 || win.Width <= 0 || win.Height <= 0 {
		return win, false
	}
	if fitsInsideAny(win, displays, func(display Display) Rect { return display.WorkArea }) {
		return win, false
	}

	// Moving between monitors is allowed even when the window is not inside
	// either work area yet. Count each physical display once so mirrored
	// displays do not make one screen look like a seam.
	area := win.Width * win.Height
	visible, overlaps := 0, 0
	seen := make(map[Rect]struct{}, len(displays))
	for _, display := range displays {
		if _, ok := seen[display.Bounds]; ok {
			continue
		}
		seen[display.Bounds] = struct{}{}
		if overlap := intersectionArea(win, display.Bounds); overlap > 0 {
			overlaps++
			visible += overlap
		}
	}
	if overlaps >= 2 && visible >= area {
		return win, false
	}

	var best Rect
	bestDistance := -1
	for _, display := range displays {
		candidate := clampInside(win, display.WorkArea)
		distance := translationDistanceSquared(win, candidate)
		if bestDistance < 0 || distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best, true
}

func fitsInsideAny(win Rect, displays []Display, area func(Display) Rect) bool {
	for _, display := range displays {
		a := area(display)
		if win.X >= a.X && win.Y >= a.Y &&
			win.X+win.Width <= a.X+a.Width &&
			win.Y+win.Height <= a.Y+a.Height {
			return true
		}
	}
	return false
}

func clampInside(win, area Rect) Rect {
	return Rect{
		X:      clampAxis(win.X, area.X, win.Width, area.Width),
		Y:      clampAxis(win.Y, area.Y, win.Height, area.Height),
		Width:  win.Width,
		Height: win.Height,
	}
}

func intersectionArea(a, b Rect) int {
	width := min(a.X+a.Width, b.X+b.Width) - max(a.X, b.X)
	height := min(a.Y+a.Height, b.Y+b.Height) - max(a.Y, b.Y)
	if width <= 0 || height <= 0 {
		return 0
	}
	return width * height
}

func translationDistanceSquared(a, b Rect) int {
	dx := b.X - a.X
	dy := b.Y - a.Y
	return dx*dx + dy*dy
}

func clampAxis(position, origin, size, extent int) int {
	if size >= extent {
		return origin
	}
	return min(max(position, origin), origin+extent-size)
}
