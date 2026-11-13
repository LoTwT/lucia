package pet

import (
	"image"
	"image/color"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func regionContains(region []image.Rectangle, x, y int) bool {
	for _, r := range region {
		if image.Pt(x, y).In(r) {
			return true
		}
	}
	return false
}

func TestAlphaInputRegionPreservesHolesAndThinPixels(t *testing.T) {
	// Exercise a subimage's nonzero origin and wider backing stride too.
	backing := image.NewRGBA(image.Rect(0, 0, 20, 20))
	img := backing.SubImage(image.Rect(5, 7, 13, 15)).(*image.RGBA)
	for y := 1; y < 7; y++ {
		for x := 1; x < 7; x++ {
			if x == 1 || x == 6 || y == 1 || y == 6 {
				img.SetRGBA(5+x, 7+y, color.RGBA{A: 255})
			}
		}
	}
	img.SetRGBA(8, 10, color.RGBA{A: 1})
	region := alphaInputRegion(img, 8, 8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want := img.RGBAAt(5+x, 7+y).A != 0
			if got := regionContains(region, x, y); got != want {
				t.Fatalf("input at (%d,%d) = %v, want %v; region %v", x, y, got, want, region)
			}
		}
	}
	if len(region) >= 21 {
		t.Fatalf("unmerged region: %v", region)
	}
}

func TestInputRegionFollowsContainMotionScaleAndReusedBuffer(t *testing.T) {
	m := &Model{scale: 1, layout: assetLayout{
		Canvas: dimension{4, 2}, Window: dimension{12, 12}, Sprite: dimension{8, 8},
	}}
	m.applyLayout()
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.SetRGBA(0, 0, color.RGBA{A: 255})
	m.updateInputRegion(img, 0, 0)
	if want := []image.Rectangle{image.Rect(2, 4, 4, 6)}; !slices.Equal(m.InputRegion(), want) {
		t.Fatalf("Contain region = %v, want %v", m.InputRegion(), want)
	}
	m.updateInputRegion(img, 0, -5)
	if want := []image.Rectangle{image.Rect(2, 0, 4, 1)}; !slices.Equal(m.InputRegion(), want) {
		t.Fatalf("translated/clipped region = %v, want %v", m.InputRegion(), want)
	}
	// A decoder can skip from frame 0 to 2 and reuse the same RGBA buffer.
	img.SetRGBA(0, 0, color.RGBA{})
	img.SetRGBA(3, 1, color.RGBA{A: 255})
	m.updateInputRegion(img, 2, 0)
	if want := []image.Rectangle{image.Rect(8, 6, 10, 8)}; !slices.Equal(m.InputRegion(), want) {
		t.Fatalf("reused buffer region = %v, want %v", m.InputRegion(), want)
	}
	m.SetScale(2)
	m.updateInputRegion(img, 2, 0)
	if want := []image.Rectangle{image.Rect(16, 12, 20, 16)}; !slices.Equal(m.InputRegion(), want) {
		t.Fatalf("scaled region = %v, want %v", m.InputRegion(), want)
	}
	img.SetRGBA(3, 1, color.RGBA{})
	m.updateInputRegion(img, 3, 0)
	if region := m.InputRegion(); region == nil || len(region) != 0 {
		t.Fatalf("transparent frame must have an empty non-nil region: %v", region)
	}
}

func TestAlphaInputRegionBoundsDetailedArtwork(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 129, 129))
	for y := 0; y < 129; y++ {
		for x := y % 2; x < 129; x += 2 {
			img.SetRGBA(x, y, color.RGBA{A: 255})
		}
	}
	region := alphaInputRegion(img, 129, 129)
	if len(region) > maxInputRegionRects {
		t.Fatalf("region has %d rectangles", len(region))
	}
	for y := 0; y < 129; y++ {
		for x := y % 2; x < 129; x += 2 {
			if !regionContains(region, x, y) {
				t.Fatalf("bounded region lost visible pixel (%d,%d)", x, y)
			}
		}
	}
}

func TestViewInputRegionUsesCurrentFrameAndReduceMotionPoster(t *testing.T) {
	m, pack := newTinyAnimatedModel(t)
	frames := []*image.RGBA{image.NewRGBA(image.Rect(0, 0, 16, 16)), image.NewRGBA(image.Rect(0, 0, 16, 16))}
	frames[0].SetRGBA(1, 1, color.RGBA{A: 255})
	frames[1].SetRGBA(12, 1, color.RGBA{A: 255})
	// Keep the fixture independent of NewAnimated's background idle cache.
	m.animated = &animatedModel{pack: pack, idleAction: pack.Manifest.Actions["idle"], poster: frames[0]}
	m.animated.idle.Store(&cachedIdle{frames: frames, durations: []time.Duration{time.Second, time.Second}})
	tt := newPetTester(m)
	tt.Frame()
	if !regionContains(m.InputRegion(), 3, 3) || regionContains(m.InputRegion(), 14, 3) {
		t.Fatalf("initial frame region = %v", m.InputRegion())
	}
	tt.Advance(time.Second)
	tt.Frame()
	if regionContains(m.InputRegion(), 3, 3) || !regionContains(m.InputRegion(), 14, 3) {
		t.Fatalf("second frame region = %v", m.InputRegion())
	}
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	if !regionContains(m.InputRegion(), 3, 3) || regionContains(m.InputRegion(), 14, 3) {
		t.Fatalf("Reduce Motion poster region = %v", m.InputRegion())
	}
}

func TestHotzonesFollowSpriteMotion(t *testing.T) {
	m, _ := newTinyAnimatedModel(t)
	m.animated.pack.Manifest.IdleMotion = "bob"
	tt := newPetTester(m)
	tt.Frame()
	before, ok := tt.Find("摸摸脸")
	if !ok {
		t.Fatal("face hotzone missing")
	}
	tt.Advance(bobPeriod / 4)
	tt.Frame()
	after, _ := tt.Find("摸摸脸")
	if math.Abs(float64(after.Y-before.Y)-bobAmplitude) > .01 {
		t.Fatalf("hotzone moved %v DIPs, want %v", after.Y-before.Y, bobAmplitude)
	}
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	still, _ := tt.Find("摸摸脸")
	if still != before {
		t.Fatalf("Reduce Motion hotzone = %v, want %v", still, before)
	}
}
