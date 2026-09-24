//go:build js && wasm

package attractor

import (
	"math"

	"unsafe"

	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/meters"
	"github.com/0magnet/chaosrack/pkg/takens"
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
// one LUT slot. It must be a power of two (meters.ComputeFFTMags returns nil
// otherwise) and it is deliberately short: 128 samples at 24 kHz is about
// 5 ms, so the color follows the sound closely enough that a transient shows
// as a band on the trail rather than being averaged into its neighbors.
const audioColorFFT = 128

// audioColor is sound as a gradient source: the windows it reads and the
// lookup table it uploads.
type audioColor struct {
	// lut is the current table, 0..1 per slot.
	lut [audioColorLUTSize]float32

	// scratch is one short-time window, reused every slot so the
	// per-frame fill allocates nothing.
	scratch [audioColorFFT]float32
	lutU8   js.Value // persistent Uint8Array backing the upload
	lutF32  js.Value // Float32Array view of it

	// lo/Hi are the adaptive bounds the table is mapped across.
	lo, hi float32

	// win is the reusable copy of the trail's source samples.
	win []float32

	// winL / winR are the pair walk's own buffers, for the
	// reason win has one: the per-frame drain buffers are in use.
	winL, winR []float32
}

var acolor = audioColor{
	lo: 0.5,
	hi: 0.5,
}

var (

	// audioColorFeature names the global feature used to fill the table flat
	// when the trail is not a time axis. Any key afFeat carries works;
	// centroid is the default because it is the one that means "brightness"
	// and so maps to color without needing to be explained.
	audioColorFeature = "centroid"
)

// lutToTyped returns the LUT as a JS Float32Array for uniform upload,
// reusing one persistent typed array — the same trick mat4ToTyped uses, for
// the same reason: this runs every frame and a fresh allocation per frame is
// a fresh garbage collection.
func (a *audioColor) lutToTyped() js.Value {
	if a.lutU8.IsUndefined() {
		a.lutU8 = js.Global().Get("Uint8Array").New(audioColorLUTSize * 4)
		a.lutF32 = js.Global().Get("Float32Array").New(a.lutU8.Get("buffer"), 0, audioColorLUTSize)
	}
	buf := (*[audioColorLUTSize * 4]byte)(unsafe.Pointer(&a.lut)) //nolint:gosec // reinterpreting the float table as its backing bytes to cross into JS without a second copy
	js.CopyBytesToJS(a.lutU8, (*buf)[:])
	return a.lutF32
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
func (a *audioColor) shortTimeCentroids(w []float32, sampleRate int, out []float32) {
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

// audioColorRelax is how fast the bounds close back in, per frame. Slow
// enough that a range opened by one loud transient survives a few seconds of
// quiet, which is what stops the color mapping from flickering between
// phrases.
const audioColorRelax = 0.02

// stretchAudioColorLUT maps lut across its adaptive range, in place.
func (a *audioColor) stretchAudioColorLUT(lut []float32) {
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
	if colorRangeLock {
		// Held: the ends stay where they were, so the same color goes on
		// meaning the same level. Everything outside the frozen span clamps,
		// which is what a locked scale is supposed to do.
		span := a.hi - a.lo
		if span < audioColorMinSpan {
			span = audioColorMinSpan
		}
		for i, v := range lut {
			lut[i] = clampF((v-a.lo)/span, 0, 1)
		}
		return
	}
	// Open at once to admit a new extreme; close slowly toward this frame's.
	if lo < a.lo {
		a.lo = lo
	} else {
		a.lo += (lo - a.lo) * audioColorRelax
	}
	if hi > a.hi {
		a.hi = hi
	} else {
		a.hi += (hi - a.hi) * audioColorRelax
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
	for i, v := range lut {
		lut[i] = clampF((v-a.lo)/span, 0, 1)
	}
}

// fillAudioColorLUTFlat paints the whole table one value, which is what a
// model whose trail is not a time axis wants: the figure tints as one.
func (a *audioColor) fillAudioColorLUTFlat(v float32) {
	v = clampF(v, 0, 1)
	for i := range a.lut {
		a.lut[i] = v
	}
}

// updateAudioColorLUT refreshes the table for this frame.
//
// It asks the mode, not the audio: whether a per-position spectrum is the
// right answer depends on whether this model's trail parameter means time,
// and only the mode knows that. Everything else gets the flat fill, so the
// source is never dead — it always colors by SOMETHING about the sound.
// updateAudioColorLUT fills the trail table for whichever audio source is
// selected: the spectral centroid, or the level the spectrogram paints.
// updateAudioColorLUT fills the trail table for whichever audio source is
// selected.
//
// The ABSOLUTE sources are not stretched: each is already on a scale where
// half way up means one fixed thing, and auto-ranging would take that away.
// See the note at the top of audiocolorsrc_js.go.
func (a *audioColor) updateAudioColorLUT(mode string) {
	switch style.gradientSource {
	case gradientSourceAudio, gradientSourceLevel:
		if w, sr := a.window(mode); w != nil {
			if style.gradientSource == gradientSourceLevel {
				shortTimeLevels(w, a.lut[:])
			} else {
				a.shortTimeCentroids(w, sr, a.lut[:])
			}
			a.stretchAudioColorLUT(a.lut[:])
			return
		}
	default:
		if fillColorLUT(style.gradientSource, mode, a.lut[:]) {
			if !gradientSourceIsAbsolute(style.gradientSource) {
				a.stretchAudioColorLUT(a.lut[:])
			}
			return
		}
		// A stereo-only source in a mono mode, or a mode with no time axis:
		// a flat middle is the honest answer, not a color derived from
		// something that was not measured.
		if gradientSourceIsAbsolute(style.gradientSource) {
			a.fillAudioColorLUTFlat(0.5)
			return
		}
	}
	if style.gradientSource == gradientSourceLevel {
		a.fillAudioColorLUTFlat(af.feat["amp"])
		return
	}
	a.fillAudioColorLUTFlat(af.feat[audioColorFeature])
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
func (a *audioColor) window(mode string) ([]float32, int) {
	if mode == "stereo" {
		return a.stereoColorWindow()
	}
	if mode == "polar" {
		return polar.colorWindow()
	}
	if mode != "takens" || emb.ring == nil {
		return nil, 0
	}
	src := aud.ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(emb.tau, sr)
	n, stride := takensWindow(emb.win, sr, sim.steps)
	if n <= 0 {
		return nil, 0
	}
	rn := len(emb.ring)
	span := (n-1)*stride + 2*tau
	if rn == 0 || emb.w < span+1 {
		return nil, 0 // not enough audio yet; the flat fill is the honest answer
	}
	// Its own buffer, not takensScratch: that one is the per-frame DRAIN
	// buffer, and borrowing it here would overwrite samples on their way into
	// the ring.
	if cap(a.win) < n {
		a.win = make([]float32, n)
	}
	out := a.win[:n]
	base := emb.w - 1 - span
	for k := 0; k < n; k++ {
		out[k] = emb.ring[(base+2*tau+k*stride)%rn]
	}
	return out, sr
}

// gradientSourceAudio is the uGradientSource value meaning "follow the sound".
// Named because it is referenced from the render loop and the panel wiring, and
// a bare 4 in two files is how those two drift apart.
const gradientSourceAudio = 4

// stereoColorWindow is audioColorWindow for the Stereo Embedding.
//
// It qualifies for the same reason Takens does: its vertices carry
// aTrailT = m/(nv−1), a straight ramp across the displayed window, so slot k of
// the table lines up with the k'th slice of that window.
//
// Without this the mode fell through to the flat fill, and with the gradient
// following the sound that fill is one color for the whole trail — which can
// be the dark end of the palette, at which point a correct figure is drawn in
// black on black and the mode looks broken. Not a theoretical case: it is what
// a saved view carrying gs=4&gc=5 did, and it is why this mode was reported as
// showing nothing at all.
//
// The walk mirrors generateStereo: source point k sits at tau + k*stride in the
// snapshot, which is where the plan's undelayed axes read from. The delayed
// axes reach back from there, as Takens' do, and the color follows the trail
// position rather than any one axis.
func (a *audioColor) stereoColorWindow() ([]float32, int) {
	src := aud.ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(stereo.tau, sr)
	align := stereoAlignSamples(stereo.align, sr)
	n, stride := stereoWindow(stereo.win, sr, sim.steps, tau, align)
	if n <= 0 {
		return nil, 0
	}
	span := (n-1)*stride + tau
	// generateStereo's own indexing: the left channel starts at baseL, which is
	// the ALIGN offset when right is the channel being pulled back. Reading from
	// 0 instead would color the trail with audio from a different moment than the
	// trail was drawn from, which is the shift this whole walk exists to avoid.
	baseL := 0
	if align > 0 {
		baseL = align
	}
	if len(stereo.l) < baseL+span+1 {
		return nil, 0 // the snapshot for this window has not been taken yet
	}
	if cap(a.win) < n {
		a.win = make([]float32, n)
	}
	out := a.win[:n]
	for k := 0; k < n; k++ {
		out[k] = stereo.l[baseL+tau+k*stride]
	}
	return out, sr
}

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
		end := start + step
		if end > len(w) {
			end = len(w)
		}
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

// stereoColorWindowPair is stereoColorWindow's walk, keeping BOTH channels.
//
// The single-channel version returns left alone, which is all a centroid or
// a level needs. Correlation, side, balance and position are about the
// relationship between the two, so they need the pair — read from their own
// bases, the same ALIGN-aware indexing generateStereo draws from, or the
// color would describe a different moment than the geometry under it.
//
// Only the stereo mode has two channels to walk; everything else gets nil
// and the caller falls back to a flat tint.
func (a *audioColor) stereoColorWindowPair(mode string) ([]float32, []float32, int) {
	if mode != "stereo" {
		return nil, nil, 0
	}
	src := aud.ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(stereo.tau, sr)
	align := stereoAlignSamples(stereo.align, sr)
	n, stride := stereoWindow(stereo.win, sr, sim.steps, tau, align)
	if n <= 0 {
		return nil, nil, 0
	}
	span := (n-1)*stride + tau
	baseL, baseR := 0, 0
	if align > 0 {
		baseL = align
	} else {
		baseR = -align
	}
	if len(stereo.l) < baseL+span+1 || len(stereo.r) < baseR+span+1 {
		return nil, nil, 0
	}
	if cap(a.winL) < n {
		a.winL = make([]float32, n)
		a.winR = make([]float32, n)
	}
	l, r := a.winL[:n], a.winR[:n]
	for k := 0; k < n; k++ {
		l[k] = stereo.l[baseL+tau+k*stride]
		r[k] = stereo.r[baseR+tau+k*stride]
	}
	return l, r, sr
}
