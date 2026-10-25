package pet

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// BubbleWidth and BubbleHeight are the fixed size of the speech bubble
// window. The card is smaller than the window so its custom shadow has room
// to fade out; a fixed canvas keeps text wrapping and placement predictable.
const (
	bubbleCardWidth  = 228
	bubbleCardHeight = 88
	bubbleCardRadius = 14
	bubbleSideInset  = 18
	// The card sits close to the pet. The asymmetric insets keep the shadow
	// on the pet-facing side from being clipped by the transparent window.
	bubbleAboveShadow = 12
	bubbleBelowShadow = 16
	BubbleWidth       = bubbleCardWidth + 2*bubbleSideInset
	BubbleHeight      = bubbleCardHeight + bubbleAboveShadow + bubbleBelowShadow
	// bubbleGap is measured from the card edge, not the window edge, so the
	// transparent shadow area never reads as extra space above the pet.
	bubbleGap = 12
)

// BubbleSide says which side of the pet the bubble is on.
type BubbleSide uint8

const (
	BubbleAbove BubbleSide = iota
	BubbleBelow
)

// BubbleTheme overrides the appearance the bubble inherits from the
// application theme.
type BubbleTheme uint8

const (
	BubbleThemeApp BubbleTheme = iota
	BubbleThemeLight
	BubbleThemeDark
)

// BubblePlacement is the bubble rectangle and the side it was placed on.
type BubblePlacement struct {
	Rect
	Side BubbleSide
}

// BubbleModel is the text shown by the separate, click-through bubble
// window. It holds no timer; the window owner decides when to hide it.
type BubbleModel struct {
	text  string
	side  BubbleSide
	theme BubbleTheme
}

// SetContent replaces the visible line and its placement. The next view
// build draws both.
func (b *BubbleModel) SetContent(text string, side BubbleSide) {
	b.text = text
	b.side = side
}

// SetTheme overrides the application appearance used by the bubble.
func (b *BubbleModel) SetTheme(theme BubbleTheme) { b.theme = theme }

// Text returns the current line, for tests and diagnostics.
func (b *BubbleModel) Text() string { return b.text }

// View draws a warm, near-opaque speech card on a transparent root. There is
// deliberately no tail: the card is already centered close to a single pet,
// and a tail would add a second outline and shadow at every seam.
func (b *BubbleModel) View(c *ui.Context) {
	c.Root().Background(ui.Transparent)

	top, bottom := float32(bubbleAboveShadow), float32(bubbleBelowShadow)
	justify := ui.End
	if b.side == BubbleBelow {
		top, bottom = bottom, top
		justify = ui.Start
	}
	// The card is intrinsically as tall as its wrapped lines. It stays
	// anchored to the pet-facing edge of the window, so a one-line bubble
	// does not inherit the empty height reserved for a three-line one.
	outer := ui.Column(c).Fill().Padding(top, bubbleSideInset, bottom, bubbleSideInset).
		Justify(justify).Label("Lucia 气泡")
	outer.Children(func() {
		dark := c.Theme().Dark
		switch b.theme {
		case BubbleThemeLight:
			dark = false
		case BubbleThemeDark:
			dark = true
		}
		palette := bubblePaletteFor(dark, c.Preferences().HighContrast)
		card := ui.Box(c).FillWidth().
			Padding(12, 14, 12, 14).
			Background(palette.fill).
			Border(1, palette.border).
			Radius(bubbleCardRadius).
			Shadow(0, 2, 4, 0, palette.shadowSoft).
			Shadow(0, 5, 11, 0, palette.shadowDeep)
		if palette.highlight.A != 0 {
			card = card.InsetShadow(0, 1, 0, 0, palette.highlight)
		}
		card.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.text).Wrap().Grow(1).
					FontSize(14).FontWeight(500).
					FixedLineHeight(20).
					LetterSpacing(0.2).
					TextColor(palette.text).
					MaxLines(3).Ellipsis("…").
					TextAlign(ui.Start)
			})
		})
	})
}

// bubblePalette keeps the host-drawn bubble readable over arbitrary
// wallpapers. It follows the effective application appearance rather than
// sampling the wallpaper: a near-opaque warm paper in light mode and warm
// charcoal in dark mode, both with their own border and shadow.
type bubblePalette struct {
	fill       ui.Color
	text       ui.Color
	border     ui.Color
	shadowSoft ui.Color
	shadowDeep ui.Color
	highlight  ui.Color
}

func bubblePaletteFor(dark, highContrast bool) bubblePalette {
	if dark {
		palette := bubblePalette{
			fill:       ui.Hex("#2B2723").Alpha(0.97),
			text:       ui.Hex("#EDE6DA"),
			border:     ui.RGBA(255, 238, 214, 0.16),
			shadowSoft: ui.RGBA(0, 0, 0, 0.22),
			shadowDeep: ui.RGBA(0, 0, 0, 0.30),
			highlight:  ui.RGBA(255, 255, 255, 0.06),
		}
		if highContrast {
			palette.text = ui.Hex("#FFF8EE")
			palette.border = ui.RGBA(255, 245, 230, 0.30)
		}
		return palette
	}
	palette := bubblePalette{
		fill:       ui.Hex("#FAF6EE").Alpha(0.97),
		text:       ui.Hex("#3B342B"),
		border:     ui.RGBA(90, 72, 48, 0.14),
		shadowSoft: ui.RGBA(70, 52, 30, 0.10),
		shadowDeep: ui.RGBA(70, 52, 30, 0.12),
	}
	if highContrast {
		palette.text = ui.Hex("#2A231C")
		palette.border = ui.RGBA(60, 45, 30, 0.30)
	}
	return palette
}

// PlaceBubble centers a bubble above the pet, flips it below when the top of
// the work area is in the way, and keeps its horizontal position on-screen.
// The gap is measured from the card edge so the transparent shadow inset does
// not change the visual distance from the pet.
func PlaceBubble(petRect Rect, displays []Display) BubblePlacement {
	bubble := BubblePlacement{
		Rect: Rect{Width: BubbleWidth, Height: BubbleHeight},
		Side: BubbleAbove,
	}
	if len(displays) == 0 {
		bubble.X = petRect.X + (petRect.Width-BubbleWidth)/2
		bubble.Y = petRect.Y - bubbleGap - bubbleCardHeight - bubbleAboveShadow
		return bubble
	}

	area := matchingDisplay(petRect, displays).WorkArea
	bubble.X = clampAxis(
		petRect.X+(petRect.Width-BubbleWidth)/2,
		area.X, BubbleWidth, area.Width,
	)
	bubble.Y = petRect.Y - bubbleGap - bubbleCardHeight - bubbleAboveShadow
	if bubble.Y < area.Y {
		bubble.Y = petRect.Y + petRect.Height + bubbleGap - bubbleBelowShadow
		bubble.Side = BubbleBelow
	}
	bubble.Y = clampAxis(bubble.Y, area.Y, BubbleHeight, area.Height)
	return bubble
}

func matchingDisplay(win Rect, displays []Display) Display {
	best := displays[0]
	bestArea := -1
	bestDistance := -1
	for _, display := range displays {
		overlap := intersectionArea(win, display.Bounds)
		distance := translationDistanceSquared(win, clampInside(win, display.WorkArea))
		if overlap > bestArea || (overlap == bestArea && (bestDistance < 0 || distance < bestDistance)) {
			best = display
			bestArea = overlap
			bestDistance = distance
		}
	}
	return best
}

// BubbleLineDelay is the default time a line stays on screen.
const BubbleLineDelay = 2600 * time.Millisecond
