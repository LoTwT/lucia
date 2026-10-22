package main

import (
	"math"

	"github.com/egoist/mygo/audio"
)

// newPetSound builds a short, soft two-tone pop in memory. Generating it
// avoids shipping an audio asset with a separate license; the sound is part
// of Lucia's own code.
func newPetSound() *audio.Sound {
	const (
		rate       = 48000
		duration   = 0.07 // seconds
		frequency  = 620.0
		overtone   = 930.0
		decay      = 45.0
		attackTime = 0.004
	)
	samples := make([]float32, int(rate*duration))
	for i := range samples {
		t := float64(i) / rate
		attack := math.Min(1, t/attackTime)
		envelope := attack * math.Exp(-t*decay)
		samples[i] = float32(0.32 * envelope * (math.Sin(2*math.Pi*frequency*t) +
			0.35*math.Sin(2*math.Pi*overtone*t)))
	}
	return audio.NewSound(rate, 1, samples)
}

func playPetSound(sound *audio.Sound) {
	if sound == nil {
		return
	}
	sound.Play().SetVolume(0.5)
}
