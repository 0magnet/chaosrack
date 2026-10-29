package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/meters"
)

// Sound as a gradient source, the arithmetic of it: the 32-slot table a
// trace's color is looked up in by where along the trail a point is, filled
// from the audio the trail was drawn from (or, where the trail is not a
// time axis, flat from the sound now), and stretched across the range it
// uses. Shared by the page (audiocolor_js.go, which says why it is a table,
// and walks each model's window to fill it) and a native rack, which colors
// its figures from the same table.

// audioColorLUTSize is the resolution of the gradient LUT. 32 is chosen
// against the shader's uniform budget, not the eye: GLSL ES 1.0 guarantees
// only 128 vec4 vertex uniform slots and an implementation is free to give a
// float array one slot per element, so a larger table risks failing to link
// on exactly the low-end hardware this has to keep working on. A color
// gradient across 32 stops is smooth to look at regardless — the trail is
// hundreds of vertices, and the varying interpolates between the stops.
const audioColorLUTSize = 32

// audioColorFFT is the short-time window, in samples, whose spectrum fills
// one LUT slot. It must be a power of two (meters.ComputeFFTMags returns nil
// otherwise) and it is deliberately short: 128 samples at 24 kHz is about
// 5 ms, so the color follows the sound closely enough that a transient shows
// as a band on the trail rather than being averaged into its neighbors.
const audioColorFFT = 128

// AudioColor is the table and what fills it: the slot windows' scratch and
// the auto-range's bounds.
type AudioColor struct {
	// lut is the current table, 0..1 per slot.
	lut [audioColorLUTSize]float32

	// scratch is one short-time window, reused every slot so the
	// per-frame fill allocates nothing.
	scratch [audioColorFFT]float32

	// lo/hi are the adaptive bounds the table is mapped across, and locked
	// holds them where they are (the page's range lock).
	lo, hi float32
	locked bool
	// k is how many 60 Hz frames the current fill stands for.
	k float32
	// ranged is whether the last fill was stretched, and rlo..rhi the raw
	// span it was mapped across, at sample rate sr: what the ends of the ramp
	// mean now, for the color key (colorkey.go).
	ranged   bool
	rlo, rhi float32
	sr       int
}

// NewAudioColor is a table whose range opens from the middle.
func NewAudioColor() AudioColor { return AudioColor{lo: 0.5, hi: 0.5} }

// LUT is the table as it was last filled.
func (a *AudioColor) LUT() []float32 { return a.lut[:] }

// shortTimeCentroids fills out with the spectral centroid of successive
// slices of w, one per slot, each normalized to 0..1 against the Nyquist
// frequency. It is the whole of the "color by frequency" arithmetic and is
// kept free of GL and DOM so it can be tested directly.
//
// A slice shorter than the FFT is zero-padded rather than skipped: the last
// slot of a window that does not divide evenly is still real audio, and
// leaving it at zero put a black band at one end of every trail.
//
// Silence returns 0.5 rather than 0. A centroid is undefined with no energy
// to weigh, and 0 is not a neutral answer — it is the bottom of the color
// ramp, so a quiet passage was being painted as if it were pure bass.
func (a *AudioColor) shortTimeCentroids(w []float32, sampleRate int, out []float32) {
	if len(out) == 0 {
		return
	}
	if len(w) == 0 || sampleRate <= 0 {
		for i := range out {
			out[i] = 0.5
		}
		return
	}
	step := max(len(w)/len(out), 1)
	for i := range out {
		start := i * step
		if start >= len(w) {
			out[i] = out[max(i-1, 0)]
			continue
		}
		n := copy(a.scratch[:], w[start:])
		for j := n; j < audioColorFFT; j++ {
			a.scratch[j] = 0
		}
		mags := meters.ComputeFFTMags(a.scratch[:])
		if mags == nil {
			out[i] = 0.5
			continue
		}
		_, _, _, c := bandEnergies(mags, sampleRate)
		if c <= 0 {
			c = 0.5
		}
		out[i] = clampF(float32(c), 0, 1)
	}
}

// ── Making the range usable ──────────────────────────────────────────────
//
// A raw centroid is correct and nearly useless as a color. Normalized against
// Nyquist, ordinary music sits somewhere around 0.05..0.25: the arithmetic is
// right, the high notes really are higher than the low ones, and the figure
// still comes out one shade of red because the whole performance happens in
// the bottom fifth of the color ramp. The first version of this shipped like
// that and the trail was visibly monochrome against real audio.
//
// So the table is stretched across whatever range it is actually using. The
// bounds follow the sound the way the rest of the audio features do — open
// instantly to admit a new extreme, close slowly — so a cymbal widens the
// range at once and the range creeps back in over the following seconds
// rather than snapping shut the moment the cymbal stops. Snapping is what
// makes a color mapping flicker.
//
// Only the SPECTRUM path is stretched. The flat fill is the feature's own
// level and is already 0..1 by construction; stretching a table whose entries
// are all identical is a division by nothing, and the answer it wants is the
// level itself, not the middle of the ramp.

// audioColorMinSpan is the narrowest range the stretch will map across.
// Without a floor, a steady tone — whose every slot holds nearly the same
// centroid — has its remaining hundredths of variation blown up to the whole
// ramp, and a pure sine wave strobes through the entire spectrum on nothing
// but arithmetic noise.
const audioColorMinSpan = 0.05

// audioColorRelax is how fast the bounds close back in, per 60 Hz frame
// (featurefollow.go): a fill stands for k of them, so a throttled tab or a
// 144 Hz display closes the range in the same time as a 60 Hz one. Slow
// enough that a range opened by one loud transient survives a few seconds of
// quiet, which is what stops the color mapping from flickering between
// phrases.
const audioColorRelax = 0.02

// stretchAudioColorLUT maps lut across its adaptive range, in place.
func (a *AudioColor) stretchAudioColorLUT(lut []float32) {
	if len(lut) == 0 {
		return
	}
	lo, hi := lut[0], lut[0]
	for _, v := range lut {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if a.locked {
		// Held: the ends stay where they were, so the same color goes on
		// meaning the same level. Everything outside the frozen span clamps,
		// which is what a locked scale is supposed to do.
		span := a.hi - a.lo
		if span < audioColorMinSpan {
			span = audioColorMinSpan
		}
		a.ranged, a.rlo, a.rhi = true, a.lo, a.lo+span
		for i, v := range lut {
			lut[i] = clampF((v-a.lo)/span, 0, 1)
		}
		return
	}
	// Open at once to admit a new extreme; close slowly toward this frame's.
	relax := perFrames(audioColorRelax, a.k)
	if lo < a.lo {
		a.lo = lo
	} else {
		a.lo += (lo - a.lo) * relax
	}
	if hi > a.hi {
		a.hi = hi
	} else {
		a.hi += (hi - a.hi) * relax
	}
	span := a.hi - a.lo
	if span < audioColorMinSpan {
		// Widen about the middle rather than from the bottom, so a narrow
		// range sits where the sound actually is instead of being dragged
		// down the ramp.
		mid := (a.hi + a.lo) / 2
		a.lo = mid - audioColorMinSpan/2
		a.hi = mid + audioColorMinSpan/2
		span = audioColorMinSpan
	}
	a.ranged, a.rlo, a.rhi = true, a.lo, a.lo+span
	for i, v := range lut {
		lut[i] = clampF((v-a.lo)/span, 0, 1)
	}
}

// fillAudioColorLUTFlat paints the whole table one value, which is what a
// model whose trail is not a time axis wants: the figure tints as one.
func (a *AudioColor) fillAudioColorLUTFlat(v float32) {
	v = clampF(v, 0, 1)
	for i := range a.lut {
		a.lut[i] = v
	}
}

// gradientSourceAudio is the uGradientSource value meaning "follow the sound".
// Named because it is referenced from the render loop and the panel wiring, and
// a bare 4 in two files is how those two drift apart.
const gradientSourceAudio = 4

// ── coloring by LEVEL, which is what the spectrogram is painting ─────────
//
// gradientSourceAudio colors by spectral CENTROID: one frequency summarizing
// each moment, run through whichever gradient is selected. That is a useful
// thing and it is not what the spectrogram backdrop shows, which is
// MAGNITUDE per bin through audioprism's own map. So a figure colored by
// "audio" over a spectrogram disagrees with it on all three counts — a
// different quantity, a different palette, and an auto-range against a fixed
// scale — and the disagreement looks like a bug rather than a choice.
//
// This source is the other half of that pair: t is the short-time LEVEL of
// the same slice the centroid was taken from. Put one of the colormap
// palettes behind it (heat, turbo, viridis, magma — the ones the spectrogram
// itself uses) and the figure and the backdrop are then saying the same
// thing in the same language: loud is the hot end in both.
//
// It shares the LUT, the stretch and the trail indexing with the centroid
// source, so it is one more fill rather than a second pipeline, and it is a
// scalar like every other source — no per-vertex color, no shader branch
// beyond naming it.
const gradientSourceLevel = 6

// shortTimeLevels fills out with the RMS level of successive slices of w,
// one per slot, on the same slicing shortTimeCentroids uses so the two
// sources index the trail identically.
//
// RMS rather than peak: peak follows single samples and makes a trace that
// flickers a slot at a time, where the spectrogram's columns are an average
// over their window and move smoothly.
//
// The values go out RAW, in 0..1 of full scale, and stretchAudioColorLUT
// does the ranging — the same auto-range the centroid gets, which is what
// keeps a quiet passage from being a flat black trace. Without a window
// (a mode whose trail is not a time axis) it falls back to the "amp"
// feature, flat, exactly as the centroid source falls back to "centroid".
func shortTimeLevels(w []float32, out []float32) {
	if len(out) == 0 {
		return
	}
	if len(w) == 0 {
		for i := range out {
			out[i] = 0
		}
		return
	}
	step := max(len(w)/len(out), 1)
	for i := range out {
		start := i * step
		if start >= len(w) {
			out[i] = out[max(i-1, 0)]
			continue
		}
		end := min(start+step, len(w))
		var sum float64
		for _, v := range w[start:end] {
			sum += float64(v) * float64(v)
		}
		n := end - start
		if n <= 0 {
			out[i] = 0
			continue
		}
		out[i] = clampF(float32(math.Sqrt(sum/float64(n))), 0, 1)
	}
}

// Fill fills the table for gradient source src from the audio the trail was
// drawn from: w, one channel of it in trail order, or l and r, the pair, for
// a source that reads both, at sample rate sr — nil where the model's trail
// is not a time axis, or has no pair. Without them the table is filled flat
// from the sound now: its centroid, or for LEVEL its amp, each 0..1. The
// absolute sources are not stretched (audiocolorsrc.go); locked holds the
// others' range where it is, and k is how many 60 Hz frames since the last fill
// (FollowFrames) the range relaxes by.
func (a *AudioColor) Fill(src int, w, l, r []float32, sr int, centroid, amp float32, locked bool, k float32) {
	a.locked, a.k, a.sr, a.ranged = locked, k, sr, false
	switch src {
	case gradientSourceAudio, gradientSourceLevel:
		if w != nil {
			if src == gradientSourceLevel {
				shortTimeLevels(w, a.lut[:])
			} else {
				a.shortTimeCentroids(w, sr, a.lut[:])
			}
			a.stretchAudioColorLUT(a.lut[:])
			return
		}
	default:
		if a.fillColorLUT(src, w, l, r, sr, a.lut[:]) {
			if !gradientSourceIsAbsolute(src) {
				a.stretchAudioColorLUT(a.lut[:])
			}
			return
		}
		// A stereo-only source in a mono mode, or a mode with no time axis:
		// a flat middle is the honest answer, not a color derived from
		// something that was not measured.
		if gradientSourceIsAbsolute(src) {
			a.fillAudioColorLUTFlat(0.5)
			return
		}
	}
	if src == gradientSourceLevel {
		a.fillAudioColorLUTFlat(amp)
		return
	}
	a.fillAudioColorLUTFlat(centroid)
}

// IsAudioSource reports whether gradient source src follows the sound,
// colored from the table, rather than a coordinate of the figure.
func IsAudioSource(src int) bool { return gradientSourceIsAudio(src) }

// bandEnergies sums magnitudes in bass/mid/treble bands and computes the
// spectral centroid (0..1) for one channel's FFT.
func bandEnergies(mags []float64, sr int) (bass, mid, treble, centroid float64) {
	nyq := float64(sr) / 2
	var total, cw float64
	for i, m := range mags {
		f := float64(i) / float64(len(mags)) * nyq
		switch {
		case f < 250:
			bass += m
		case f < 2000:
			mid += m
		default:
			treble += m
		}
		total += m
		cw += f * m
	}
	if total > 0 {
		centroid = cw / total / nyq
	}
	return
}

// BandEnergies is a spectrum's bass, mid and treble energy and its centroid
// against Nyquist, as the page's features take them.
func BandEnergies(mags []float64, sr int) (bass, mid, treble, centroid float64) {
	return bandEnergies(mags, sr)
}
