package brand

import (
	"bytes"
	"image/png"
	"testing"
)

func TestTrayIconIsPNG(t *testing.T) {
	icon, err := png.Decode(bytes.NewReader(TrayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := icon.Bounds().Dx(), icon.Bounds().Dy(); w != 32 || h != 32 {
		t.Fatalf("tray icon size = %dx%d, want 32x32", w, h)
	}
}
