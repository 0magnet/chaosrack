//go:build js && wasm

package attractor

import (
	"unsafe"

	"syscall/js"
)

// Sound as a gradient source: the fifth thing the trace color can follow,
// after X, Y, Z and trail age.
//
// The gradient already had a source knob, but every option on it was
// geometric — the color told you where a point was, never what was playing
// when it got there. In the audio modes that is the more interesting
// question: a Takens trail IS a window of sound, so the part of the figure
// drawn from a bright moment and the part drawn from a bass thump are
// different pieces of information that were being painted the same color.
//
// It is a LUT rather than a scalar, and that is the whole design. A single
// uniform can only tint the WHOLE figure at once — the picture pulses, which
// is a level meter with extra steps. A 32-slot table indexed by the point's
// position along the trail gives each stretch of the trace the spectrum of
// the moment it came from, which is what "color by frequency" has to mean if
// it is to mean anything on a curve that is itself a time axis.
//
// The flat case falls out of the same mechanism instead of needing its own
// branch: for a model whose trail is not a clock — a Lorenz attractor, a
// torus — every slot is filled with the current global feature and the
// figure tints as one. Same shader, same upload, no special case.
//
// THE LOOKUP IS IN THE VERTEX SHADER, and that is not a preference either.
// GLSL ES 1.0 only requires dynamic indexing of a uniform array in the
// vertex stage; a fragment shader is allowed to reject an index that is not
// a constant expression, and some drivers do. The vertex shader reads the
// table and hands the result across as a varying, which is portable and also
// cheaper — one lookup per vertex instead of one per fragment.

// audioColorLUTSize is the resolution of the gradient LUT. 32 is chosen
// against the shader's uniform budget, not the eye: GLSL ES 1.0 guarantees
// only 128 vec4 vertex uniform slots and an implementation is free to give a
// float array one slot per element, so a larger table risks failing to link
// on exactly the low-end hardware this has to keep working on. A color
// gradient across 32 stops is smooth to look at regardless — the trail is
// hundreds of vertices, and the varying interpolates between the stops.
const audioColorLUTSize = 32

// audioColorFFT is the short-time window, in samples, whose spectrum fills
// one LUT slot. It must be a power of two (computeFFTMags returns nil
// otherwise) and it is deliberately short: 128 samples at 24 kHz is about
// 5 ms, so the color follows the sound closely enough that a transient shows
// as a band on the trail rather than being averaged into its neighbors.
const audioColorFFT = 128

var (
	// audioColorLUT is the current table, 0..1 per slot.
	audioColorLUT [audioColorLUTSize]float32

	// audioColorFeature names the global feature used to fill the table flat
	// when the trail is not a time axis. Any key afFeat carries works;
	// centroid is the default because it is the one that means "brightness"
	// and so maps to color without needing to be explained.
	audioColorFeature = "centroid"

	// audioColorScratch is one short-time window, reused every slot so the
	// per-frame fill allocates nothing.
	audioColorScratch [audioColorFFT]float32

	jsLUTUint8 js.Value // persistent Uint8Array backing the upload
	jsLUTFloat js.Value // Float32Array view of it
)

// lutToTyped returns the LUT as a JS Float32Array for uniform upload,
// reusing one persistent typed array — the same trick mat4ToTyped uses, for
// the same reason: this runs every frame and a fresh allocation per frame is
// a fresh garbage collection.
func lutToTyped() js.Value {
	if jsLUTUint8.IsUndefined() {
		jsLUTUint8 = js.Global().Get("Uint8Array").New(audioColorLUTSize * 4)
		jsLUTFloat = js.Global().Get("Float32Array").New(jsLUTUint8.Get("buffer"), 0, audioColorLUTSize)
	}
	buf := (*[audioColorLUTSize * 4]byte)(unsafe.Pointer(&audioColorLUT)) //nolint:gosec // reinterpreting the float table as its backing bytes to cross into JS without a second copy
	js.CopyBytesToJS(jsLUTUint8, (*buf)[:])
	return jsLUTFloat
}

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
func shortTimeCentroids(w []float32, sampleRate int, out []float32) {
	if len(out) == 0 {
		return
	}
	if len(w) == 0 || sampleRate <= 0 {
		for i := range out {
			out[i] = 0.5
		}
		return
	}
	step := len(w) / len(out)
	if step < 1 {
		step = 1
	}
	for i := range out {
		start := i * step
		if start >= len(w) {
			out[i] = out[max(i-1, 0)]
			continue
		}
		n := copy(audioColorScratch[:], w[start:])
		for j := n; j < audioColorFFT; j++ {
			audioColorScratch[j] = 0
		}
		mags := computeFFTMags(audioColorScratch[:])
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

// audioColorLo/Hi are the adaptive bounds the table is mapped across.
var audioColorLo, audioColorHi float32 = 0.5, 0.5

// audioColorMinSpan is the narrowest range the stretch will map across.
// Without a floor, a steady tone — whose every slot holds nearly the same
// centroid — has its remaining hundredths of variation blown up to the whole
// ramp, and a pure sine wave strobes through the entire spectrum on nothing
// but arithmetic noise.
const audioColorMinSpan = 0.05

// audioColorRelax is how fast the bounds close back in, per frame. Slow
// enough that a range opened by one loud transient survives a few seconds of
// quiet, which is what stops the color mapping from flickering between
// phrases.
const audioColorRelax = 0.02

// stretchAudioColorLUT maps lut across its adaptive range, in place.
func stretchAudioColorLUT(lut []float32) {
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
	// Open at once to admit a new extreme; close slowly toward this frame's.
	if lo < audioColorLo {
		audioColorLo = lo
	} else {
		audioColorLo += (lo - audioColorLo) * audioColorRelax
	}
	if hi > audioColorHi {
		audioColorHi = hi
	} else {
		audioColorHi += (hi - audioColorHi) * audioColorRelax
	}
	span := audioColorHi - audioColorLo
	if span < audioColorMinSpan {
		// Widen about the middle rather than from the bottom, so a narrow
		// range sits where the sound actually is instead of being dragged
		// down the ramp.
		mid := (audioColorHi + audioColorLo) / 2
		audioColorLo = mid - audioColorMinSpan/2
		audioColorHi = mid + audioColorMinSpan/2
		span = audioColorMinSpan
	}
	for i, v := range lut {
		lut[i] = clampF((v-audioColorLo)/span, 0, 1)
	}
}

// fillAudioColorLUTFlat paints the whole table one value, which is what a
// model whose trail is not a time axis wants: the figure tints as one.
func fillAudioColorLUTFlat(v float32) {
	v = clampF(v, 0, 1)
	for i := range audioColorLUT {
		audioColorLUT[i] = v
	}
}

// updateAudioColorLUT refreshes the table for this frame.
//
// It asks the mode, not the audio: whether a per-position spectrum is the
// right answer depends on whether this model's trail parameter means time,
// and only the mode knows that. Everything else gets the flat fill, so the
// source is never dead — it always colors by SOMETHING about the sound.
func updateAudioColorLUT(mode string) {
	if w, sr := audioColorWindow(mode); w != nil {
		shortTimeCentroids(w, sr, audioColorLUT[:])
		stretchAudioColorLUT(audioColorLUT[:])
		return
	}
	fillAudioColorLUTFlat(afFeat[audioColorFeature])
}

// audioColorWindow returns the audio the current mode's trail was drawn
// from, in trail order, or nil when the trail is not a time axis.
//
// Takens is the case this exists for: its vertices carry aTrailT = m/(nv-1),
// a straight ramp across the displayed window, so slot k of the table lines
// up with the k'th slice of that window with no further arithmetic. Any mode
// that fills the trail attribute the same way can be added here.
//
// The walk mirrors generateTakens exactly, including the 2*tau offset: that
// function reads source point k at base+2*tau+k*stride, because the two
// delayed coordinates reach BACK from there and the oldest of them has to
// land inside the ring. Sampling the window from base instead would color
// the trail with audio from two delays earlier than the trail was drawn
// from, which is a shift of exactly the thing the mode is about.
func audioColorWindow(mode string) ([]float32, int) {
	if mode != "takens" || takensRing == nil {
		return nil, 0
	}
	src := ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := int(takensTau)
	if tau < 1 {
		tau = 1
	}
	n, stride := takensWindow(takensWin, sr, steps)
	if n <= 0 {
		return nil, 0
	}
	rn := len(takensRing)
	span := (n-1)*stride + 2*tau
	if rn == 0 || takensW < span+1 {
		return nil, 0 // not enough audio yet; the flat fill is the honest answer
	}
	// Its own buffer, not takensScratch: that one is the per-frame DRAIN
	// buffer, and borrowing it here would overwrite samples on their way into
	// the ring.
	if cap(audioColorWin) < n {
		audioColorWin = make([]float32, n)
	}
	out := audioColorWin[:n]
	base := takensW - 1 - span
	for k := 0; k < n; k++ {
		out[k] = takensRing[(base+2*tau+k*stride)%rn]
	}
	return out, sr
}

// audioColorWin is the reusable copy of the trail's source samples.
var audioColorWin []float32

// gradientSourceAudio is the uGradientSource value meaning "follow the sound".
// Named because it is referenced from the render loop and the panel wiring, and
// a bare 4 in two files is how those two drift apart.
const gradientSourceAudio = 4
