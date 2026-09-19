//go:build js && wasm

package attractor

// The rest of the things a trace color can follow.
//
// Every one of these is a scalar per trail slot, filled on the CPU into the
// same 32-slot table the centroid and level sources use, so the shader only
// ever learns another source number and the palette, the reverse switch and
// the colormap window all keep working unchanged.
//
// ── absolute sources and relative ones ──────────────────────────────────
//
// The split that matters here is not what is measured but whether the answer
// means anything on its own.
//
// LEVEL, SIDE, FLUX and the centroid are RELATIVE: they are auto-ranged
// against the window's own extremes, because the alternative is a trace that
// is flat black through a quiet passage. The cost is that the same color
// means different things a minute apart.
//
// CORRELATION, BALANCE, POSITION, dB and PITCH are ABSOLUTE: each has a
// natural full scale — ±1, the stereo field, a decibel, the octave — so they
// are mapped onto it FIXED and never stretched. Half way up the ramp is then
// always mono, always centered, always −30 dB, whatever else is happening,
// and two moments can be compared by eye. Auto-ranging one of these would
// destroy the only property that makes it worth having.
//
// That is also why the range lock (see colorRangeLock) applies to the
// relative ones alone: the absolute sources are already locked, by
// construction.

import (
	"math"
)

const (
	gradientSourceCorr     = 7  // instantaneous L/R correlation
	gradientSourceSide     = 8  // |L−R|, the width of the image
	gradientSourceBalance  = 9  // |L|−|R|, which side the energy is on
	gradientSourceFlux     = 10 // spectral change between slices
	gradientSourceDB       = 11 // level on a decibel scale
	gradientSourcePitch    = 12 // centroid folded to the octave
	gradientSourcePosition = 13 // the angle of the (L,R) vector
	gradientSourceMax      = gradientSourcePosition
)

// gradientSourceIsAudio reports whether a source is filled from the audio
// trail table rather than from a coordinate of the figure.
//
// Everything from the centroid up is, except OFF, which sits in the middle
// of the range for historical reasons and is not a source at all.
func gradientSourceIsAudio(s int) bool {
	return s >= gradientSourceAudio && s <= gradientSourceMax && s != GradientSourceOff
}

// gradientSourceIsAbsolute reports whether a source is on a fixed scale and
// must not be auto-ranged. See the note at the top of this file.
func gradientSourceIsAbsolute(s int) bool {
	switch s {
	case gradientSourceCorr, gradientSourceBalance, gradientSourcePosition,
		gradientSourceDB, gradientSourcePitch:
		return true
	}
	return false
}

// gradientSourceNeedsStereo reports whether a source reads both channels,
// and so has nothing to say in a mode that draws one.
func gradientSourceNeedsStereo(s int) bool {
	switch s {
	case gradientSourceCorr, gradientSourceSide, gradientSourceBalance, gradientSourcePosition:
		return true
	}
	return false
}

// colorRangeLock freezes the auto-range at whatever it currently spans.
//
// The auto-range is what keeps a quiet passage visible, and it is also why
// the same color means different things a minute apart: the ramp is
// continuously refitted to the loudest and quietest thing in view. With this
// set the ends stay where they are, so color becomes comparable between
// moments — the difference between a picture that looks right and a reading
// that can be trusted.
//
// It does nothing to the absolute sources, which are never stretched.
var colorRangeLock bool

// fillColorLUT fills the trail table for one of the sources here, and
// reports whether it managed to. A false answer leaves the caller to fall
// back to a flat tint, which is what a mode with no time axis, or a mono
// source under a stereo-only coloring, honestly deserves.
func fillColorLUT(src int, mode string, out []float32) bool {
	if gradientSourceNeedsStereo(src) {
		l, r, _ := stereoColorWindowPair(mode)
		if l == nil || r == nil {
			return false
		}
		switch src {
		case gradientSourceCorr:
			shortTimeCorrelation(l, r, out)
		case gradientSourceSide:
			shortTimeSide(l, r, out)
		case gradientSourceBalance:
			shortTimeBalance(l, r, out)
		case gradientSourcePosition:
			shortTimePosition(l, r, out)
		}
		return true
	}
	w, sr := audioColorWindow(mode)
	if w == nil {
		return false
	}
	switch src {
	case gradientSourceFlux:
		shortTimeFlux(w, out)
	case gradientSourceDB:
		shortTimeDB(w, out)
	case gradientSourcePitch:
		shortTimePitch(w, sr, out)
	default:
		return false
	}
	return true
}

// slices walks out's slots over w, handing each its span. Shared by every
// fill here so they all index the trail identically — a source that sliced
// differently would put its color a slot away from the geometry it
// describes.
func slices(n int, out []float32, f func(i, start, end int)) {
	if len(out) == 0 || n <= 0 {
		return
	}
	step := n / len(out)
	if step < 1 {
		step = 1
	}
	for i := range out {
		start := i * step
		if start >= n {
			f(i, -1, -1)
			continue
		}
		end := start + step
		if end > n {
			end = n
		}
		f(i, start, end)
	}
}

// shortTimeCorrelation fills out with the Pearson correlation of L against R
// per slice, mapped −1..+1 onto 0..1 so that 0.5 is uncorrelated.
//
// This is the reading a goniometer is kept for. Out-of-phase material —
// the thing that disappears when a mix is summed to mono — lands at the
// bottom of the ramp, and because the table indexes the trail it lands AT
// THE POINT OF THE SWEEP WHERE IT HAPPENS rather than as one number for the
// whole window. A correlation meter says "something in here is out of
// phase"; this says which part.
func shortTimeCorrelation(l, r, out []float32) {
	n := min(len(l), len(r))
	slices(n, out, func(i, start, end int) {
		if start < 0 {
			out[i] = out[max(i-1, 0)]
			return
		}
		var sl, sr, sll, srr, slr float64
		for k := start; k < end; k++ {
			a, b := float64(l[k]), float64(r[k])
			sl += a
			sr += b
			sll += a * a
			srr += b * b
			slr += a * b
		}
		m := float64(end - start)
		cov := slr/m - (sl/m)*(sr/m)
		vl := sll/m - (sl/m)*(sl/m)
		vr := srr/m - (sr/m)*(sr/m)
		d := math.Sqrt(vl * vr)
		if d <= 1e-12 {
			out[i] = 0.5 // no energy to correlate; neither in nor out of phase
			return
		}
		out[i] = clampF(float32((cov/d+1)/2), 0, 1)
	})
}

// shortTimeSide fills out with the RMS of the SIDE signal, (L−R)/2: how
// wide the image is at each moment. Relative, so it is auto-ranged — the
// question it answers is which parts are wider than which.
func shortTimeSide(l, r, out []float32) {
	n := min(len(l), len(r))
	slices(n, out, func(i, start, end int) {
		if start < 0 {
			out[i] = out[max(i-1, 0)]
			return
		}
		var sum float64
		for k := start; k < end; k++ {
			s := (float64(l[k]) - float64(r[k])) * 0.5
			sum += s * s
		}
		out[i] = clampF(float32(math.Sqrt(sum/float64(end-start))), 0, 1)
	})
}

// shortTimeBalance fills out with (|L|−|R|) over their sum: −1 hard left,
// +1 hard right, mapped onto 0..1 so that 0.5 is centered.
//
// Absolute, and meant for a diverging palette: with one, left-leaning and
// right-leaning material take opposite hues and anything centered stays
// neutral, so an image that drifts off center shows as a color drift rather
// than as a shape one has to judge by eye.
func shortTimeBalance(l, r, out []float32) {
	n := min(len(l), len(r))
	slices(n, out, func(i, start, end int) {
		if start < 0 {
			out[i] = out[max(i-1, 0)]
			return
		}
		var al, ar float64
		for k := start; k < end; k++ {
			al += math.Abs(float64(l[k]))
			ar += math.Abs(float64(r[k]))
		}
		sum := al + ar
		if sum <= 1e-12 {
			out[i] = 0.5
			return
		}
		out[i] = clampF(float32(((al-ar)/sum+1)/2), 0, 1)
	})
}

// shortTimePosition fills out with the ANGLE of the (L,R) vector, a quarter
// turn mapped onto the whole ramp: 0 is hard right, 0.5 the mono diagonal,
// 1 hard left.
//
// This is the per-axis coloring the goniometer wants. Every other source
// derives a quantity from the pair and colors by that; this one colors by
// the pair's own direction, so hue IS the stereo position of the sample
// rather than something computed about it. On a hue palette the figure's
// arms take a color per direction, which is the display the mode is for.
func shortTimePosition(l, r, out []float32) {
	n := min(len(l), len(r))
	slices(n, out, func(i, start, end int) {
		if start < 0 {
			out[i] = out[max(i-1, 0)]
			return
		}
		// The MEAN direction of the slice, taken from the summed magnitudes
		// rather than per sample: a waveform crosses zero constantly, and an
		// angle averaged through those crossings is noise. |L| against |R|
		// is the direction the energy is in, which is what the eye reads off
		// the figure.
		var al, ar float64
		for k := start; k < end; k++ {
			al += math.Abs(float64(l[k]))
			ar += math.Abs(float64(r[k]))
		}
		if al+ar <= 1e-12 {
			out[i] = 0.5
			return
		}
		// atan2 over a quarter turn: both magnitudes are non-negative, so the
		// angle runs 0 (all right) to π/2 (all left).
		out[i] = clampF(float32(math.Atan2(al, ar)/(math.Pi/2)), 0, 1)
	})
}

// shortTimeFlux fills out with spectral flux: how much the spectrum CHANGED
// from the previous slice. Steady tones go flat and transients flash, so it
// paints onsets onto the trace.
//
// Relative, and auto-ranged: flux has no natural full scale.
func shortTimeFlux(w []float32, out []float32) {
	var prev []float64
	slices(len(w), out, func(i, start, end int) {
		if start < 0 {
			out[i] = out[max(i-1, 0)]
			return
		}
		n := copy(audioColorScratch[:], w[start:])
		for j := n; j < audioColorFFT; j++ {
			audioColorScratch[j] = 0
		}
		mags := computeFFTMags(audioColorScratch[:])
		if mags == nil {
			out[i] = 0
			return
		}
		if prev == nil {
			prev = make([]float64, len(mags))
			copy(prev, mags)
			out[i] = 0
			return
		}
		// Half-wave rectified: only INCREASES count, which is what an onset
		// is. Counting decreases too makes a note ending look like a note
		// starting.
		var sum float64
		for k := range mags {
			if k >= len(prev) {
				break
			}
			if d := mags[k] - prev[k]; d > 0 {
				sum += d
			}
		}
		copy(prev, mags)
		out[i] = float32(sum)
	})
}

// dbFloor is the bottom of the decibel ramp. −60 dB is the quietest thing
// worth a color: below it is the noise floor of most material, and giving
// it ramp only compresses everything audible into the top.
const dbFloor = -60.0

// shortTimeDB fills out with level in decibels against full scale, mapped
// from dbFloor..0 onto 0..1.
//
// Absolute, unlike LEVEL: linear RMS spends most of its range on the loudest
// few percent, so quiet material is all one color at the bottom of the ramp.
// A decibel scale is what a meter shows for the same reason, and half way up
// this ramp is always −30 dB rather than "half as loud as the loudest thing
// currently in the window".
func shortTimeDB(w []float32, out []float32) {
	slices(len(w), out, func(i, start, end int) {
		if start < 0 {
			out[i] = out[max(i-1, 0)]
			return
		}
		var sum float64
		for k := start; k < end; k++ {
			sum += float64(w[k]) * float64(w[k])
		}
		rms := math.Sqrt(sum / float64(end-start))
		if rms <= 1e-9 {
			out[i] = 0
			return
		}
		db := 20 * math.Log10(rms)
		out[i] = clampF(float32((db-dbFloor)/-dbFloor), 0, 1)
	})
}

// shortTimePitch fills out with the centroid folded into one octave and
// mapped round the ramp, so the same note is the same color in any register.
//
// Absolute: the octave is the scale. On a hue palette this is the one that
// looks least like the others — a melody becomes a sequence of hues that
// repeat when it repeats, an octave apart or not.
//
// Folded from the centroid rather than from a pitch detector on purpose: a
// detector has an opinion about what the note IS and is wrong in an
// interesting way on anything polyphonic, where the centroid just reports
// where the energy sits and folds honestly.
func shortTimePitch(w []float32, sampleRate int, out []float32) {
	if sampleRate <= 0 {
		fillFlat(out, 0.5)
		return
	}
	// The centroid arrives normalized against Nyquist; turn it back into a
	// frequency before taking its log, or the fold is of the wrong number.
	nyq := float64(sampleRate) / 2
	shortTimeCentroids(w, sampleRate, out)
	for i, v := range out {
		f := float64(v) * nyq
		if f < 20 { // below hearing: no pitch class to speak of
			out[i] = 0.5
			continue
		}
		oct := math.Log2(f / 440.0) // A4 as the reference, as tuning does
		out[i] = clampF(float32(oct-math.Floor(oct)), 0, 1)
	}
}

func fillFlat(out []float32, v float32) {
	for i := range out {
		out[i] = v
	}
}
