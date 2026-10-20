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

// A generator's FREQ is an encoder that turns once an octave, so it can go
// below the piano without squeezing the octaves on it: down to genKnobLo,
// eight octaves under A0 (0.107 Hz), where a generator is a slow modulator
// and a figure the model can follow at a frame a step. Its laps start at C,
// as the piano strip under it does, and lap zero, the octave lit in the
// rack's own LED color, is C3 to C4.
const (
	genKnobLo     = -96          // semitones above A0: A-8, 0.107 Hz
	genKnobHi     = genSemitones // A10
	genTurnLo     = 27           // C3, 130.8 Hz
	genTurnHi     = genTurnLo + 12
	genFreqDetent = 12 // a click of FREQ is a semitone
)

// genFreqTurns is FREQ's laps: an octave a turn, from genKnobLo to genKnobHi.
var genFreqTurns = turnSpec{kind: turnLinear, lo: genTurnLo, hi: genTurnHi, floor: genKnobLo, ceil: genKnobHi, bounded: true}

// freqFromKnob maps a slider position (semitones above A0) to Hz.
func freqFromKnob(v float64) float64 {
	return genFreqLo * math.Pow(genFreqHi/genFreqLo, v/genSemitones)
}

// knobFromFreq is the inverse (for typing a frequency into the LED).
func knobFromFreq(hz float64) float64 {
	if lo := freqFromKnob(genKnobLo); hz < lo {
		hz = lo
	}
	return genSemitones * math.Log(hz/genFreqLo) / math.Log(genFreqHi/genFreqLo)
}
