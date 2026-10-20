package pet

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// BubbleWidth and BubbleHeight are the fixed size of the speech bubble
// window. A fixed canvas keeps text wrapping and positioning predictable.
// The extra height below the card leaves room for the tail.
const (
	BubbleWidth      = 240
	BubbleHeight     = 100
	bubbleGap        = 8
	bubbleInset      = 6
	bubbleTailHeight = 8
	bubbleTailHalf   = 9
	bubbleCardRadius = 18
)

// BubbleSide says which side of the pet the bubble is on, and therefore
// which edge the tail points from.
type BubbleSide uint8

const (
	BubbleAbove BubbleSide = iota
	BubbleBelow
)

// BubblePlacement is the bubble rectangle plus the data its view needs to
// draw a tail aimed at the pet.
type BubblePlacement struct {
	Rect
	Side  BubbleSide
	TailX int
}

// BubbleModel is the text shown by the separate, click-through bubble
// window. It holds no timer; the window owner decides when to hide it.
type BubbleModel struct {
	text  string
	side  BubbleSide
	tailX int
}

// SetContent replaces the visible line and its placement. The next view
// build draws both.
func (b *BubbleModel) SetContent(text string, side BubbleSide, tailX int) {
	b.text = text
	b.side = side
	b.tailX = tailX
}

// Text returns the current line, for tests and diagnostics.
func (b *BubbleModel) Text() string { return b.text }

// View draws a rounded white speech bubble on a transparent root. The tail
// is painted after the card so its fill covers the card border where they
// meet.
func (b *BubbleModel) View(c *ui.Context) {
	c.Root().Background(ui.Transparent)

	top, bottom := float32(bubbleInset), float32(bubbleInset+bubbleTailHeight)
	if b.side == BubbleBelow {
		top, bottom = bottom, top
	}
	outer := ui.Box(c).Fill().Padding(top, bubbleInset, bottom, bubbleInset).
		Label("Lucia 气泡")
	outer.DrawOver(func(p *ui.Painter, r ui.Rect) {
		drawBubbleTail(p, r, b.side, b.tailX, top, bottom)
	})
	outer.Children(func() {
		card := ui.Box(c).Fill().Padding(12, 14, 12, 14).
			Background(ui.RGBA(255, 255, 255, 0.97)).
			Border(1, ui.RGBA(30, 41, 59, 0.14)).
			Radius(bubbleCardRadius)
		card.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.text).Wrap().FontSize(14).TextColor(ui.RGBA(30, 41, 59, 1))
			})
		})
	})
}

func drawBubbleTail(p *ui.Painter, r ui.Rect, side BubbleSide, tailX int, top, bottom float32) {
	x := float32(tailX)
	if x <= 0 {
		x = r.W / 2
	}
	x = min(max(x, bubbleCardRadius+bubbleTailHalf), r.W-bubbleCardRadius-bubbleTailHalf)

	var baseY, tipY float32
	if side == BubbleBelow {
		baseY = top
		tipY = top - bubbleTailHeight
	} else {
		baseY = r.H - bottom
		tipY = baseY + bubbleTailHeight
	}

	fill := ui.RGBA(255, 255, 255, 0.97)
	border := ui.RGBA(30, 41, 59, 0.14)
	p.FillPath((&ui.Path{}).
		MoveTo(x-bubbleTailHalf, baseY).
		LineTo(x, tipY).
		LineTo(x+bubbleTailHalf, baseY).
		Close(), fill)
	p.StrokePath((&ui.Path{}).
		MoveTo(x-bubbleTailHalf, baseY).
		LineTo(x, tipY).
		LineTo(x+bubbleTailHalf, baseY), 1, border)
}

// PlaceBubble centers a bubble above the pet, flips it below when the top of
// the work area is in the way, and keeps its horizontal position on-screen.
// The tail follows the pet even when the bubble is clamped to a screen edge.
func PlaceBubble(petRect Rect, displays []Display) BubblePlacement {
	bubble := BubblePlacement{
		Rect: Rect{Width: BubbleWidth, Height: BubbleHeight},
		Side: BubbleAbove,
	}
	if len(displays) == 0 {
		bubble.X = petRect.X + (petRect.Width-BubbleWidth)/2
		bubble.Y = petRect.Y - BubbleHeight - bubbleGap
		bubble.TailX = bubbleTailX(petRect, bubble.Rect)
		return bubble
	}

	area := matchingDisplay(petRect, displays).WorkArea
	bubble.X = clampAxis(
		petRect.X+(petRect.Width-BubbleWidth)/2,
		area.X, BubbleWidth, area.Width,
	)
	bubble.Y = petRect.Y - BubbleHeight - bubbleGap
	if bubble.Y < area.Y {
		bubble.Y = petRect.Y + petRect.Height + bubbleGap
		bubble.Side = BubbleBelow
	}
	bubble.Y = clampAxis(bubble.Y, area.Y, BubbleHeight, area.Height)
	bubble.TailX = bubbleTailX(petRect, bubble.Rect)
	return bubble
}

func bubbleTailX(petRect, bubble Rect) int {
	tail := petRect.X + petRect.Width/2 - bubble.X
	return min(max(tail, bubbleCardRadius+bubbleTailHalf), BubbleWidth-bubbleCardRadius-bubbleTailHalf)
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
