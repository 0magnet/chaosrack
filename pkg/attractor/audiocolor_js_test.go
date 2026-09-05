//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// tone fills n samples with a sine at hz.
func tone(n, hz, sampleRate int) []float32 {
	w := make([]float32, n)
	for i := range w {
		w[i] = float32(math.Sin(2 * math.Pi * float64(hz) * float64(i) / float64(sampleRate)))
	}
	return w
}

// The centroid has to actually track pitch, or the gradient is decoration.
// A high tone must land higher up the color ramp than a low one — that single
// property is what makes the audio source mean "frequency" rather than "some
// number that changes when the sound does".
func TestShortTimeCentroidsRisesWithPitch(t *testing.T) {
	const sr = 24000
	out := make([]float32, 8)

	shortTimeCentroids(tone(2048, 300, sr), sr, out)
	low := out[len(out)/2]

	shortTimeCentroids(tone(2048, 6000, sr), sr, out)
	high := out[len(out)/2]

	if !(high > low) {
		t.Errorf("6 kHz gave centroid %v, 300 Hz gave %v; the high tone must sit higher on the ramp", high, low)
	}
}

// Every slot has to be filled with something usable. A slot left at zero is
// not neutral — zero is the bottom of the color ramp, so a gap in the table
// paints as a black band across the trail rather than as nothing.
func TestShortTimeCentroidsFillsEverySlot(t *testing.T) {
	const sr = 24000
	out := make([]float32, audioColorLUTSize)
	shortTimeCentroids(tone(4096, 1000, sr), sr, out)
	for i, v := range out {
		if v <= 0 || v > 1 {
			t.Errorf("slot %d = %v, want a usable 0..1 value", i, v)
		}
	}
}

// Silence is the case that used to paint pure bass across the whole figure:
// with no energy to weigh, a centroid is undefined, and returning 0 put it at
// the bottom of the ramp. The neutral middle is the honest answer.
func TestShortTimeCentroidsSilenceIsNeutral(t *testing.T) {
	out := make([]float32, 4)
	shortTimeCentroids(make([]float32, 1024), 24000, out)
	for i, v := range out {
		if v != 0.5 {
			t.Errorf("slot %d of silence = %v, want 0.5", i, v)
		}
	}
}

// A window shorter than one FFT frame still has to produce a full table: the
// mode is live from the first frames of audio, before a whole window exists,
// and a half-filled table is a half-colored figure.
func TestShortTimeCentroidsShortWindow(t *testing.T) {
	out := make([]float32, audioColorLUTSize)
	shortTimeCentroids(tone(64, 2000, 24000), 24000, out)
	for i, v := range out {
		if v <= 0 || v > 1 {
			t.Errorf("slot %d = %v with a short window, want a usable value", i, v)
		}
	}
}

// Degenerate inputs must not panic and must not leave the table at zero.
func TestShortTimeCentroidsDegenerate(t *testing.T) {
	out := make([]float32, 4)
	shortTimeCentroids(nil, 24000, out)
	for i, v := range out {
		if v != 0.5 {
			t.Errorf("slot %d of a nil window = %v, want 0.5", i, v)
		}
	}
	shortTimeCentroids(tone(512, 1000, 24000), 0, out)
	for i, v := range out {
		if v != 0.5 {
			t.Errorf("slot %d at a zero sample rate = %v, want 0.5", i, v)
		}
	}
	shortTimeCentroids(tone(512, 1000, 24000), 24000, nil) // must not panic
}

// The flat fill is what every non-audio model gets, so it has to clamp: the
// features are adaptively normalized and can overshoot 1 on a transient, and
// an out-of-range t reads off the end of the color ramp.
func TestFillAudioColorLUTFlatClamps(t *testing.T) {
	fillAudioColorLUTFlat(3)
	for i, v := range audioColorLUT {
		if v != 1 {
			t.Errorf("slot %d = %v after an over-range fill, want 1", i, v)
		}
	}
	fillAudioColorLUTFlat(-2)
	for i, v := range audioColorLUT {
		if v != 0 {
			t.Errorf("slot %d = %v after an under-range fill, want 0", i, v)
		}
	}
}

// The shader indexes the table with int(clamp(aTrailT,0,1)*31.0+0.5), so the
// table's length and that constant have to agree. If the size is ever changed
// without the shader, the top of the trail silently reads slot 31 forever.
func TestLUTSizeMatchesTheShaderIndex(t *testing.T) {
	if audioColorLUTSize != 32 {
		t.Fatalf("audioColorLUTSize = %d; the vertex shader indexes with *31.0 and declares uAudioLUT[32]", audioColorLUTSize)
	}
	if got := len(audioColorLUT); got != audioColorLUTSize {
		t.Errorf("the table holds %d slots, want %d", got, audioColorLUTSize)
	}
}

// The stretch is what makes the mapping usable rather than merely correct: a
// centroid table that all sits in the bottom fifth of the ramp has to come out
// spanning it, or the trail is one shade of red no matter what is playing.
func TestStretchUsesTheWholeRamp(t *testing.T) {
	audioColorLo, audioColorHi = 0.5, 0.5
	// Held audio, frame after frame. The bounds CONVERGE rather than snapping:
	// a mapping that re-scaled itself on every frame would flicker, so the
	// contract is that a steady signal reaches the full ramp within about a
	// second, not that one frame does it.
	var lut []float32
	for i := 0; i < 120; i++ {
		lut = []float32{0.10, 0.12, 0.14, 0.16, 0.18, 0.20}
		stretchAudioColorLUT(lut)
	}
	var lo, hi float32 = 1, 0
	for _, v := range lut {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi-lo < 0.5 {
		t.Errorf("a table spanning 0.10..0.20 stretched to %v..%v; it should reach across the ramp", lo, hi)
	}
	if lo < 0 || hi > 1 {
		t.Errorf("stretched outside 0..1: %v..%v", lo, hi)
	}
}

// A steady tone puts nearly the same centroid in every slot. Without a floor
// on the span, its remaining hundredths get blown up to the whole ramp and a
// pure sine strobes through the spectrum on arithmetic noise alone.
func TestStretchDoesNotAmplifyAFlatTable(t *testing.T) {
	audioColorLo, audioColorHi = 0.5, 0.5
	lut := []float32{0.400, 0.401, 0.400, 0.399, 0.400}
	stretchAudioColorLUT(lut)
	var lo, hi float32 = 1, 0
	for _, v := range lut {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi-lo > 0.2 {
		t.Errorf("a nearly flat table spread to %v..%v; the minimum span should have held it together", lo, hi)
	}
}

// The bounds open at once and close slowly. A transient must widen the range
// immediately — a color mapping that lags the sound is worse than one that
// does not move — and the range must not snap shut when it passes.
func TestStretchOpensFastAndClosesSlowly(t *testing.T) {
	audioColorLo, audioColorHi = 0.4, 0.5
	stretchAudioColorLUT([]float32{0.4, 0.9}) // a transient
	if audioColorHi < 0.9 {
		t.Errorf("the upper bound is %v after a 0.9 peak; it should have opened to admit it at once", audioColorHi)
	}
	wide := audioColorHi
	for i := 0; i < 5; i++ {
		stretchAudioColorLUT([]float32{0.4, 0.5}) // the transient is over
	}
	if audioColorHi < wide*0.8 {
		t.Errorf("the upper bound fell from %v to %v in five frames; it should ease back, not snap", wide, audioColorHi)
	}
}
