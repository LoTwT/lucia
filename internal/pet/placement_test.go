package pet

import "testing"

var placementDisplays = []Display{
	{
		Bounds:   Rect{X: 0, Y: 0, Width: 1440, Height: 900},
		WorkArea: Rect{X: 0, Y: 25, Width: 1440, Height: 850},
	},
	{
		Bounds:   Rect{X: 1440, Y: 0, Width: 1920, Height: 1080},
		WorkArea: Rect{X: 1440, Y: 25, Width: 1920, Height: 1030},
	},
}

func TestKeepOnScreen(t *testing.T) {
	const size = 248
	tests := []struct {
		name     string
		window   Rect
		displays []Display
		changed  bool
		wantX    int
		wantY    int
	}{
		{
			name:   "inside primary display",
			window: Rect{X: 100, Y: 100, Width: size, Height: size},
		},
		{
			name:   "inside secondary display",
			window: Rect{X: 2000, Y: 300, Width: size, Height: size},
		},
		{
			name:   "straddling the seam between displays",
			window: Rect{X: 1316, Y: 300, Width: size, Height: size},
		},
		{
			name:   "straddling into the secondary work area margin",
			window: Rect{X: 1316, Y: 0, Width: size, Height: size},
		},
		{
			name:    "one pixel past the right edge",
			window:  Rect{X: 3113, Y: 300, Width: size, Height: size},
			changed: true,
			wantX:   3112,
			wantY:   300,
		},
		{
			name:    "one pixel past the top edge",
			window:  Rect{X: 100, Y: -1, Width: size, Height: size},
			changed: true,
			wantX:   100,
			wantY:   25,
		},
		{
			name:    "one pixel past the left edge",
			window:  Rect{X: -1, Y: 300, Width: size, Height: size},
			changed: true,
			wantX:   0,
			wantY:   300,
		},
		{
			name:    "past the right edge of the secondary display",
			window:  Rect{X: 3260, Y: 300, Width: size, Height: size},
			changed: true,
			wantX:   3112,
			wantY:   300,
		},
		{
			name:    "fully beyond the right edge",
			window:  Rect{X: 4000, Y: 300, Width: size, Height: size},
			changed: true,
			wantX:   3112,
			wantY:   300,
		},
		{
			name:    "display removed from under the window",
			window:  Rect{X: 2000, Y: 300, Width: size, Height: size},
			changed: true,
			wantX:   1192,
			wantY:   300,
		},
		{
			name:   "straddling a gap between displays",
			window: Rect{X: 1440, Y: 100, Width: size, Height: size},
			displays: []Display{
				{Bounds: Rect{X: 0, Y: 0, Width: 1440, Height: 900}, WorkArea: Rect{X: 0, Y: 25, Width: 1440, Height: 850}},
				{Bounds: Rect{X: 1600, Y: 0, Width: 1920, Height: 1080}, WorkArea: Rect{X: 1600, Y: 25, Width: 1920, Height: 1030}},
			},
			changed: true,
			wantX:   1600,
			wantY:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displays := placementDisplays
			if tt.name == "display removed from under the window" {
				displays = placementDisplays[:1]
			}
			if tt.displays != nil {
				displays = tt.displays
			}
			got, changed := KeepOnScreen(tt.window, displays)
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v", changed, tt.changed)
			}
			if !tt.changed {
				if got != tt.window {
					t.Fatalf("position changed to %+v, want %+v", got, tt.window)
				}
				return
			}
			want := Rect{X: tt.wantX, Y: tt.wantY, Width: size, Height: size}
			if got != want {
				t.Fatalf("position = %+v, want %+v", got, want)
			}
		})
	}
}

func TestKeepOnScreenStopsBeforeDock(t *testing.T) {
	displays := []Display{{
		Bounds:   Rect{X: 0, Y: 0, Width: 1512, Height: 982},
		WorkArea: Rect{X: 57, Y: 33, Width: 1455, Height: 949},
	}}
	window := Rect{X: 0, Y: 400, Width: 248, Height: 248}
	got, changed := KeepOnScreen(window, displays)
	if !changed {
		t.Fatal("window under the Dock was left in place")
	}
	want := Rect{X: 57, Y: 400, Width: 248, Height: 248}
	if got != want {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
}

func TestKeepOnScreenAcrossVerticalSeam(t *testing.T) {
	// A 5K display above the built-in one: the window may straddle y=0 as
	// long as its horizontal span stays over one of the two displays.
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
	for _, window := range []Rect{
		{X: 1264, Y: -100, Width: 248, Height: 248},
		{X: 1264, Y: -424, Width: 248, Height: 248},
	} {
		if got, changed := KeepOnScreen(window, displays); changed || got != window {
			t.Fatalf("position = %+v, changed = %v; want unchanged", got, changed)
		}
	}
}

func TestKeepOnScreenWithDisplayLeftOfPrimary(t *testing.T) {
	displays := []Display{
		{
			Bounds:   Rect{X: 0, Y: 0, Width: 1440, Height: 900},
			WorkArea: Rect{X: 0, Y: 25, Width: 1440, Height: 850},
		},
		{
			Bounds:   Rect{X: -1920, Y: 0, Width: 1920, Height: 1080},
			WorkArea: Rect{X: -1920, Y: 25, Width: 1920, Height: 1030},
		},
	}
	window := Rect{X: -1800, Y: 300, Width: 248, Height: 248}
	if got, changed := KeepOnScreen(window, displays); changed || got != window {
		t.Fatalf("position = %+v, changed = %v; want unchanged", got, changed)
	}
}

func TestKeepOnScreenWithMirroredDisplays(t *testing.T) {
	display := Display{
		Bounds:   Rect{X: 0, Y: 0, Width: 1440, Height: 900},
		WorkArea: Rect{X: 0, Y: 25, Width: 1440, Height: 850},
	}
	displays := []Display{display, display}
	window := Rect{X: 1300, Y: 300, Width: 248, Height: 248}
	got, changed := KeepOnScreen(window, displays)
	if !changed {
		t.Fatalf("position = %+v, changed = false; want the window pulled back", got)
	}
	want := Rect{X: 1192, Y: 300, Width: 248, Height: 248}
	if got != want {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
}

func TestKeepOnScreenWithoutDisplays(t *testing.T) {
	window := Rect{X: 100, Y: 100, Width: 248, Height: 248}
	if got, changed := KeepOnScreen(window, nil); changed || got != window {
		t.Fatalf("position = %+v, changed = %v; want unchanged", got, changed)
	}
}
