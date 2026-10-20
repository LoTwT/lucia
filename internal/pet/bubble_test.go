package pet

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBubbleLineCyclesByReaction(t *testing.T) {
	if got := BubbleLine(ReactionFace, 0); got != faceLines[0] {
		t.Fatalf("face line 0 = %q, want %q", got, faceLines[0])
	}
	if got := BubbleLine(ReactionFace, len(faceLines)); got != faceLines[0] {
		t.Fatalf("face line wrapped = %q, want %q", got, faceLines[0])
	}
	if got := BubbleLine(ReactionHeadPat, 1); got != headPatLines[1] {
		t.Fatalf("head-pat line 1 = %q, want %q", got, headPatLines[1])
	}
	if got := BubbleLine(ReactionNone, 0); got != "" {
		t.Fatalf("unknown reaction line = %q, want empty", got)
	}
}

func TestBubbleViewShowsText(t *testing.T) {
	m := &BubbleModel{}
	m.SetText("别戳脸啦……")
	tt := ui.NewTester(m.View, BubbleWidth, BubbleHeight)
	tt.Frame()
	if !tt.HasText("别戳脸啦……") {
		t.Fatalf("bubble text not found; texts: %v", tt.Texts())
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
		want Rect
	}{
		{
			name: "above the pet",
			pet:  Rect{X: 400, Y: 400, Width: 248, Height: 248},
			want: Rect{X: 404, Y: 300, Width: BubbleWidth, Height: BubbleHeight},
		},
		{
			name: "below when the work area has no room above",
			pet:  Rect{X: 400, Y: 33, Width: 248, Height: 248},
			want: Rect{X: 404, Y: 289, Width: BubbleWidth, Height: BubbleHeight},
		},
		{
			name: "clamped away from the left dock",
			pet:  Rect{X: 0, Y: 400, Width: 248, Height: 248},
			want: Rect{X: 57, Y: 300, Width: BubbleWidth, Height: BubbleHeight},
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
	want := Rect{X: 1268, Y: -524, Width: BubbleWidth, Height: BubbleHeight}
	if got := PlaceBubble(pet, displays); got != want {
		t.Fatalf("PlaceBubble = %+v, want %+v", got, want)
	}
}
