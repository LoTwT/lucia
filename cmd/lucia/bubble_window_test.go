package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestBubbleCloseDoesNotCancelQuit(t *testing.T) {
	tests := []struct {
		name     string
		quitting func() bool
		prevent  bool
	}{
		{name: "normal close hides", prevent: true},
		{name: "quit closes", quitting: func() bool { return true }, prevent: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bubbleWindow{}
			e := &mygo.CloseEvent{}
			b.handleClose(e, tt.quitting)
			if got := e.DefaultPrevented(); got != tt.prevent {
				t.Fatalf("default prevented = %v, want %v", got, tt.prevent)
			}
		})
	}
}
