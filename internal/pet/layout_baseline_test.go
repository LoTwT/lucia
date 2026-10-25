package pet

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

type layoutSnapshot struct {
	scale    float64
	windowW  int
	windowH  int
	spriteW  int
	spriteH  int
	image    ui.Rect
	face     ui.Rect
	bubble   ui.Rect
	hotzones int
}

// TestWhaleLayoutBaseline freezes the layout of the built-in artwork before it
// becomes a pack. The built-in manifest must reproduce these rectangles at
// every supported scale.
func TestWhaleLayoutBaseline(t *testing.T) {
	want := []layoutSnapshot{
		{
			scale: 0.6, windowW: 149, windowH: 149, spriteW: 144, spriteH: 144,
			image:  ui.Rect{X: 2.5, Y: 2.5, W: 144, H: 144},
			face:   ui.Rect{X: 46.9, Y: 67.3, W: 60, H: 45.6},
			bubble: ui.Rect{X: 74.5, Y: 2.5, W: 1, H: 146.5}, hotzones: 1,
		},
		{
			scale: 1, windowW: 248, windowH: 248, spriteW: 240, spriteH: 240,
			image:  ui.Rect{X: 4, Y: 4, W: 240, H: 240},
			face:   ui.Rect{X: 78, Y: 112, W: 100, H: 76},
			bubble: ui.Rect{X: 124, Y: 4, W: 1, H: 244}, hotzones: 1,
		},
		{
			scale: 1.25, windowW: 310, windowH: 310, spriteW: 300, spriteH: 300,
			image:  ui.Rect{X: 5, Y: 5, W: 300, H: 300},
			face:   ui.Rect{X: 97.5, Y: 140, W: 125, H: 95},
			bubble: ui.Rect{X: 155, Y: 5, W: 1, H: 305}, hotzones: 1,
		},
		{
			scale: 2.5, windowW: 620, windowH: 620, spriteW: 600, spriteH: 600,
			image:  ui.Rect{X: 10, Y: 10, W: 600, H: 600},
			face:   ui.Rect{X: 195, Y: 280, W: 250, H: 190},
			bubble: ui.Rect{X: 310, Y: 10, W: 1, H: 610}, hotzones: 1,
		},
	}
	for _, want := range want {
		m, err := New()
		if err != nil {
			t.Fatal(err)
		}
		m.SetScale(want.scale)
		windowW, windowH := m.WindowSize()
		if windowW != want.windowW || windowH != want.windowH {
			t.Errorf("scale %.2f window = %dx%d, want %dx%d", want.scale, windowW, windowH, want.windowW, want.windowH)
		}
		spriteW, spriteH := m.SpriteSize()
		if spriteW != want.spriteW || spriteH != want.spriteH {
			t.Errorf("scale %.2f sprite = %dx%d, want %dx%d", want.scale, spriteW, spriteH, want.spriteW, want.spriteH)
		}
		assertRectClose(t, "image", m.ImageRect(), want.image)
		assertRectClose(t, "face", m.FaceRegion(), want.face)
		assertRectClose(t, "bubble", m.BubbleRect(), want.bubble)
		zones := m.Hotzones()
		if len(zones) != want.hotzones {
			t.Fatalf("scale %.2f hotzones = %d, want %d", want.scale, len(zones), want.hotzones)
		}
		if zones[0].ID != "face" || zones[0].Label != "摸摸脸" {
			t.Errorf("scale %.2f hotzone = %+v, want face/摸摸脸", want.scale, zones[0])
		}
		assertRectClose(t, "hotzone", zones[0].Rect, want.face)
	}
}

func assertRectClose(t *testing.T, name string, got, want ui.Rect) {
	t.Helper()
	if math.Abs(float64(got.X-want.X)) > 0.05 ||
		math.Abs(float64(got.Y-want.Y)) > 0.05 ||
		math.Abs(float64(got.W-want.W)) > 0.05 ||
		math.Abs(float64(got.H-want.H)) > 0.05 {
		t.Errorf("%s = %+v, want %+v", name, got, want)
	}
}
