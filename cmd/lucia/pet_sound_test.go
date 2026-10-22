package main

import (
	"testing"
	"time"
)

func TestNewPetSoundHasExpectedLength(t *testing.T) {
	sound := newPetSound()
	if sound == nil {
		t.Fatal("newPetSound returned nil")
	}
	if got := sound.Duration(); got <= 0 {
		t.Fatalf("sound duration = %v, want a short positive clip", got)
	}
}

func TestBubbleAllowed(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		last     time.Time
		cooldown int
		want     bool
	}{
		{name: "always", cooldown: 0, want: true},
		{name: "off", cooldown: -1, want: false},
		{name: "first positive cooldown", cooldown: 30, want: true},
		{name: "recent bubble", last: now.Add(-10 * time.Second), cooldown: 30, want: false},
		{name: "cooldown elapsed", last: now.Add(-30 * time.Second), cooldown: 30, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bubbleAllowed(now, tt.last, tt.cooldown); got != tt.want {
				t.Fatalf("bubbleAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
