package attractor

// Concert-pitch semitone mapping shared by the generator modules and Model
// Out — pure math, untagged for native testing (pitch_test.go): the slider
// unit is one semitone anchored at A0, so 440 Hz lands exactly on A4.

import "math"

const (
	genFreqLo    = 27.5    // Hz — A0, lowest piano key
	genFreqHi    = 28160.0 // Hz — A10, 10 octaves above genFreqLo
	genSemitones = 120     // 10 octaves × 12 semitones = full-scale slider units
)

// freqFromKnob maps the 0..genSemitones slider position (semitones) to Hz.
func freqFromKnob(v float64) float64 {
	return genFreqLo * math.Pow(genFreqHi/genFreqLo, v/genSemitones)
}

// knobFromFreq is the inverse (for typing a frequency into the LED).
func knobFromFreq(hz float64) float64 {
	if hz < genFreqLo {
		hz = genFreqLo
	}
	return genSemitones * math.Log(hz/genFreqLo) / math.Log(genFreqHi/genFreqLo)
}
