//go:build js && wasm

package attractor

import (
	"syscall/js"
	"unsafe"

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

// audioColor is sound as a gradient source: the windows it reads and the
// lookup table it uploads.
type audioColor struct {
	// AudioColor is the table and its arithmetic (audiocolorlut.go).
	AudioColor
	lutU8  js.Value // persistent Uint8Array backing the upload
	lutF32 js.Value // Float32Array view of it

	// win is the reusable copy of the trail's source samples.
	win []float32

	// winL / winR are the pair walk's own buffers, for the
	// reason win has one: the per-frame drain buffers are in use.
	winL, winR []float32
}

var acolor = audioColor{AudioColor: NewAudioColor()}

// stretchAudioColorLUT is the table's stretch under the page's range lock.
func (a *audioColor) stretchAudioColorLUT(lut []float32) {
	a.locked = colorRangeLock
	a.AudioColor.stretchAudioColorLUT(lut)
}

var (

	// audioColorFeature names the global feature used to fill the table flat
	// when the trail is not a time axis. Any key af.feat carries works;
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
	src := style.gradientSource
	var w, l, r []float32
	var sr int
	if gradientSourceNeedsStereo(src) {
		l, r, sr = a.stereoColorWindowPair(mode)
	} else {
		w, sr = a.window(mode)
	}
	a.Fill(src, w, l, r, sr, af.feat[audioColorFeature], af.feat["amp"], colorRangeLock, followK)
}

// window returns the audio the current mode's trail was drawn
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
	if src.SampleRate() > 0 {
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
	// Its own buffer, not emb.scratch: that one is the per-frame DRAIN
	// buffer, and borrowing it here would overwrite samples on their way into
	// the ring.
	if cap(a.win) < n {
		a.win = make([]float32, n)
	}
	out := a.win[:n]
	base := emb.w - 1 - span
	for k := range n {
		out[k] = emb.ring[(base+2*tau+k*stride)%rn]
	}
	return out, sr
}

// stereoColorWindow is acolor.window for the Stereo Embedding.
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
// The walk mirrors stereoInst.generate: source point k sits at tau + k*stride in the
// snapshot, which is where the plan's undelayed axes read from. The delayed
// axes reach back from there, as Takens' do, and the color follows the trail
// position rather than any one axis.
func (a *audioColor) stereoColorWindow() ([]float32, int) {
	src := aud.ensureAudioSource()
	sr := 24000
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(stereo.tau, sr)
	align := stereoAlignSamples(stereo.align, sr)
	n, stride := stereoWindow(stereo.win, sr, sim.steps, tau, align, takensSmooth())
	if n <= 0 {
		return nil, 0
	}
	span := (n-1)*stride + tau
	// stereoInst.generate's own indexing: the left channel starts at baseL, which is
	// the ALIGN offset when right is the channel being pulled back. Reading from
	// 0 instead would color the trail with audio from a different moment than the
	// trail was drawn from, which is the shift this whole walk exists to avoid.
	// The trigger moves the window start past that by stereo.start.
	baseL := max(align, 0) + stereo.start
	if len(stereo.l) < baseL+span+1 {
		return nil, 0 // the snapshot for this window has not been taken yet
	}
	if cap(a.win) < n {
		a.win = make([]float32, n)
	}
	out := a.win[:n]
	for k := range n {
		out[k] = stereo.l[baseL+tau+k*stride]
	}
	return out, sr
}

// stereoColorWindowPair is stereoColorWindow's walk, keeping BOTH channels.
//
// The single-channel version returns left alone, which is all a centroid or
// a level needs. Correlation, side, balance and position are about the
// relationship between the two, so they need the pair — read from their own
// bases, the same ALIGN-aware indexing stereoInst.generate draws from, or the
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
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(stereo.tau, sr)
	align := stereoAlignSamples(stereo.align, sr)
	n, stride := stereoWindow(stereo.win, sr, sim.steps, tau, align, takensSmooth())
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
	baseL += stereo.start // where the trigger moved the window
	baseR += stereo.start
	if len(stereo.l) < baseL+span+1 || len(stereo.r) < baseR+span+1 {
		return nil, nil, 0
	}
	if cap(a.winL) < n {
		a.winL = make([]float32, n)
		a.winR = make([]float32, n)
	}
	l, r := a.winL[:n], a.winR[:n]
	for k := range n {
		l[k] = stereo.l[baseL+tau+k*stride]
		r[k] = stereo.r[baseR+tau+k*stride]
	}
	return l, r, sr
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
