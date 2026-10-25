package pet

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBubbleViewShowsText(t *testing.T) {
	m := &BubbleModel{}
	m.SetContent("别戳脸啦……", BubbleAbove)
	tt := ui.NewTester(m.View, BubbleWidth, BubbleHeight)
	tt.Frame()
	if !tt.HasText("别戳脸啦……") {
		t.Fatalf("bubble text not found; texts: %v", tt.Texts())
	}
}

func TestBubbleViewUsesThemePalette(t *testing.T) {
	m := &BubbleModel{}
	m.SetContent("摸头……", BubbleAbove)

	light := ui.NewTester(m.View, BubbleWidth, BubbleHeight)
	light.Frame()
	assertBubblePixel(t, light.Image(), 25, 70, color.NRGBA{R: 0xFA, G: 0xF6, B: 0xEE, A: 0xF7})

	dark := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(ui.DarkTheme())
		m.View(c)
	}, BubbleWidth, BubbleHeight)
	dark.Frame()
	assertBubblePixel(t, dark.Image(), 25, 70, color.NRGBA{R: 0x2B, G: 0x27, B: 0x23, A: 0xF7})
}

func TestBubbleThemeOverridesApp(t *testing.T) {
	light := &BubbleModel{}
	light.SetContent("摸头……", BubbleAbove)
	light.SetTheme(BubbleThemeLight)
	lightTT := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(ui.DarkTheme())
		light.View(c)
	}, BubbleWidth, BubbleHeight)
	lightTT.Frame()
	assertBubblePixel(t, lightTT.Image(), 25, 70, color.NRGBA{R: 0xFA, G: 0xF6, B: 0xEE, A: 0xF7})

	dark := &BubbleModel{}
	dark.SetContent("摸头……", BubbleAbove)
	dark.SetTheme(BubbleThemeDark)
	darkTT := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(ui.LightTheme())
		dark.View(c)
	}, BubbleWidth, BubbleHeight)
	darkTT.Frame()
	assertBubblePixel(t, darkTT.Image(), 25, 70, color.NRGBA{R: 0x2B, G: 0x27, B: 0x23, A: 0xF7})
}

func TestBubbleHeightFollowsTextLines(t *testing.T) {
	short := &BubbleModel{}
	short.SetContent("短句", BubbleAbove)
	shortTT := ui.NewTester(short.View, BubbleWidth, BubbleHeight)
	shortTT.Frame()

	long := &BubbleModel{}
	long.SetContent(strings.Repeat("这是一句很长的台词。", 4), BubbleAbove)
	longTT := ui.NewTester(long.View, BubbleWidth, BubbleHeight)
	longTT.Frame()

	shortBounds := opaqueBounds(shortTT.Image())
	longBounds := opaqueBounds(longTT.Image())
	if longBounds.Dy() <= shortBounds.Dy()+20 {
		t.Fatalf("long bubble height = %d, short = %d; want room for wrapped lines", longBounds.Dy(), shortBounds.Dy())
	}
	if shortBounds.Dx() != longBounds.Dx() {
		t.Fatalf("bubble width changed with height: short=%d long=%d", shortBounds.Dx(), longBounds.Dx())
	}
}

func TestBubblePaletteHighContrast(t *testing.T) {
	light := bubblePaletteFor(false, false)
	lightContrast := bubblePaletteFor(false, true)
	if lightContrast.text == light.text || lightContrast.border.A <= light.border.A {
		t.Fatal("light high-contrast palette must strengthen text and border")
	}
	dark := bubblePaletteFor(true, false)
	darkContrast := bubblePaletteFor(true, true)
	if darkContrast.text == dark.text || darkContrast.border.A <= dark.border.A {
		t.Fatal("dark high-contrast palette must strengthen text and border")
	}
}

func TestPlaceBubble(t *testing.T) {
	displays := []Display{{
		Bounds:   Rect{X: 0, Y: 0, Width: 1512, Height: 982},
		WorkArea: Rect{X: 57, Y: 33, Width: 1455, Height: 949},
	}}
	tests := []struct {
		name string
		pet  Rect
		want BubblePlacement
	}{
		{
			name: "above the pet",
			pet:  Rect{X: 400, Y: 400, Width: 248, Height: 248},
			want: BubblePlacement{
				Rect: Rect{X: 392, Y: 288, Width: BubbleWidth, Height: BubbleHeight},
				Side: BubbleAbove,
			},
		},
		{
			name: "below when the work area has no room above",
			pet:  Rect{X: 400, Y: 33, Width: 248, Height: 248},
			want: BubblePlacement{
				Rect: Rect{X: 392, Y: 277, Width: BubbleWidth, Height: BubbleHeight},
				Side: BubbleBelow,
			},
		},
		{
			name: "clamped away from the left dock",
			pet:  Rect{X: 0, Y: 400, Width: 248, Height: 248},
			want: BubblePlacement{
				Rect: Rect{X: 57, Y: 288, Width: BubbleWidth, Height: BubbleHeight},
				Side: BubbleAbove,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlaceBubble(tt.pet, displays); got != tt.want {
				t.Fatalf("PlaceBubble(%+v) = %+v, want %+v", tt.pet, got, tt.want)
			}
		})
	}
}

func TestPlaceBubbleGapUsesCardEdge(t *testing.T) {
	displays := []Display{{
		Bounds:   Rect{X: 0, Y: 0, Width: 1512, Height: 982},
		WorkArea: Rect{X: 57, Y: 33, Width: 1455, Height: 949},
	}}

	pet := Rect{X: 400, Y: 400, Width: 248, Height: 248}
	above := PlaceBubble(pet, displays)
	if above.Side != BubbleAbove {
		t.Fatalf("side = %v, want above", above.Side)
	}
	if cardBottom := above.Y + bubbleAboveShadow + bubbleCardHeight; cardBottom != pet.Y-bubbleGap {
		t.Fatalf("card bottom = %d, want %d", cardBottom, pet.Y-bubbleGap)
	}

	pet.Y = 33
	below := PlaceBubble(pet, displays)
	if below.Side != BubbleBelow {
		t.Fatalf("side = %v, want below", below.Side)
	}
	if cardTop := below.Y + bubbleBelowShadow; cardTop != pet.Y+pet.Height+bubbleGap {
		t.Fatalf("card top = %d, want %d", cardTop, pet.Y+pet.Height+bubbleGap)
	}
}

func TestPlaceBubbleOnExternalDisplay(t *testing.T) {
	displays := []Display{
		{
			Bounds:   Rect{X: 0, Y: 0, Width: 1512, Height: 982},
			WorkArea: Rect{X: 57, Y: 33, Width: 1455, Height: 949},
		},
		{
			Bounds:   Rect{X: 0, Y: -1440, Width: 2560, Height: 1440},
			WorkArea: Rect{X: 0, Y: -1410, Width: 2560, Height: 1410},
		},
	}
	pet := Rect{X: 1264, Y: -424, Width: 248, Height: 248}
	want := BubblePlacement{
		Rect: Rect{X: 1256, Y: -536, Width: BubbleWidth, Height: BubbleHeight},
		Side: BubbleAbove,
	}
	if got := PlaceBubble(pet, displays); got != want {
		t.Fatalf("PlaceBubble = %+v, want %+v", got, want)
	}
}

func assertBubblePixel(t *testing.T, img image.Image, x, y int, want color.NRGBA) {
	t.Helper()
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	close := func(a, b uint8) bool {
		return a == b || a+1 == b || b+1 == a
	}
	if !close(got.R, want.R) || !close(got.G, want.G) || !close(got.B, want.B) || got.A != want.A {
		t.Fatalf("pixel (%d,%d) = %+v, want %+v (+-1 RGB)", x, y, got, want)
	}
}

func opaqueBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	out := image.Rectangle{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a>>8 <= 20 {
				continue
			}
			if out.Empty() {
				out = image.Rect(x, y, x+1, y+1)
				continue
			}
			out = out.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	return out
}
