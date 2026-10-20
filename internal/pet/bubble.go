package pet

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// BubbleWidth and BubbleHeight are the fixed size of the speech bubble
// window. A fixed canvas keeps text wrapping and positioning predictable.
const (
	BubbleWidth  = 240
	BubbleHeight = 92
	bubbleGap    = 8
)

// BubbleModel is the text shown by the separate, click-through bubble
// window. It holds no timer; the window owner decides when to hide it.
type BubbleModel struct {
	text string
}

// SetText replaces the visible line. The next view build draws it.
func (b *BubbleModel) SetText(text string) { b.text = text }

// Text returns the current line, for tests and diagnostics.
func (b *BubbleModel) Text() string { return b.text }

// View draws a rounded white speech bubble on a transparent root.
func (b *BubbleModel) View(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	ui.Box(c).Fill().Padding(6).Label("Lucia 气泡").Children(func() {
		card := ui.Box(c).Fill().Padding(12, 14, 12, 14).
			Background(ui.RGBA(255, 255, 255, 0.97)).
			Border(1, ui.RGBA(30, 41, 59, 0.14)).
			Radius(18)
		card.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.text).Wrap().FontSize(14).TextColor(ui.RGBA(30, 41, 59, 1))
			})
		})
	})
}

// PlaceBubble centers a bubble above the pet, flips it below when the top of
// the work area is in the way, and keeps its horizontal position on-screen.
func PlaceBubble(petRect Rect, displays []Display) Rect {
	bubble := Rect{Width: BubbleWidth, Height: BubbleHeight}
	if len(displays) == 0 {
		return Rect{
			X:      petRect.X + (petRect.Width-BubbleWidth)/2,
			Y:      petRect.Y - BubbleHeight - bubbleGap,
			Width:  BubbleWidth,
			Height: BubbleHeight,
		}
	}

	area := matchingDisplay(petRect, displays).WorkArea
	bubble.X = clampAxis(
		petRect.X+(petRect.Width-BubbleWidth)/2,
		area.X, BubbleWidth, area.Width,
	)
	bubble.Y = petRect.Y - BubbleHeight - bubbleGap
	if bubble.Y < area.Y {
		bubble.Y = petRect.Y + petRect.Height + bubbleGap
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

// BubbleLine returns the next line for a reaction. Lines cycle; an unknown
// reaction yields an empty string so callers can show nothing.
func BubbleLine(reaction Reaction, n int) string {
	var lines []string
	switch reaction {
	case ReactionFace:
		lines = faceLines
	case ReactionHeadPat:
		lines = headPatLines
	default:
		return ""
	}
	if n < 0 {
		n = 0
	}
	return lines[n%len(lines)]
}

var faceLines = []string{
	"别戳脸啦……",
	"唔，被发现了。",
	"再戳要生气了。",
	"脸都被你戳扁了。",
}

var headPatLines = []string{
	"摸头……还行吧。",
	"哼，才没有很开心。",
	"再摸一下也可以。",
	"头发要乱啦。",
}

// BubbleLineDelay is the default time a line stays on screen.
const BubbleLineDelay = 2600 * time.Millisecond
