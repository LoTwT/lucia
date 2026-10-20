package main

import "testing"

func TestNewPetSoundHasExpectedLength(t *testing.T) {
	sound := newPetSound()
	if sound == nil {
		t.Fatal("newPetSound returned nil")
	}
	if got := sound.Duration(); got <= 0 {
		t.Fatalf("sound duration = %v, want a short positive clip", got)
	}
}
