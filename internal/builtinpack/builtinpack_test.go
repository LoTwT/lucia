package builtinpack

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"testing"
	"time"
)

func TestWhalePackAssetsMatchSources(t *testing.T) {
	pack, err := Load(1)
	if err != nil {
		t.Fatal(err)
	}
	idle, ok := pack.Clip("idle_loop")
	if !ok {
		t.Fatal("missing idle_loop clip")
	}
	frames, err := idle.DecodeAllScaled(800, 800)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("idle frames = %d, want 4", len(frames))
	}
	sources := []string{
		"whale/source/idle-open.png",
		"whale/source/idle-half-closed.png",
		"whale/source/idle-closed.png",
		"whale/source/idle-half-closed.png",
	}
	for i, name := range sources {
		want := readPNG(t, name)
		if !equalRGBA(frames[i], want) {
			t.Errorf("idle frame %d does not match %s", i, name)
		}
	}
	wantDurations := []time.Duration{
		3800 * time.Millisecond,
		60 * time.Millisecond,
		90 * time.Millisecond,
		50 * time.Millisecond,
	}
	for i, want := range wantDurations {
		if got := idle.Duration(i); got != want {
			t.Errorf("idle frame %d duration = %v, want %v", i, got, want)
		}
	}

	scaled, err := idle.DecodeAllScaled(pack.PixelWidth, pack.PixelHeight)
	if err != nil {
		t.Fatal(err)
	}
	for poster, frameIndex := range map[string]int{
		"idle_loop#1": 1,
		"idle_loop#2": 2,
	} {
		img, err := pack.PosterFrame(poster)
		if err != nil {
			t.Fatalf("poster %s: %v", poster, err)
		}
		rgba, ok := img.(*image.RGBA)
		if !ok {
			t.Fatalf("poster %s type = %T, want *image.RGBA", poster, img)
		}
		if want := scaled[frameIndex]; !equalRGBA(rgba, want) {
			t.Errorf("poster %s does not match idle frame %d", poster, frameIndex)
		}
	}
}

func readPNG(t *testing.T, name string) *image.RGBA {
	t.Helper()
	data, err := fs.ReadFile(content, name)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	dst := image.NewRGBA(img.Bounds())
	draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
	return dst
}

func equalRGBA(a, b *image.RGBA) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	return bytes.Equal(a.Pix, b.Pix)
}
