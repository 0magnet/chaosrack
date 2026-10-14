package attractor

import (
	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/meters"
)

// The waterfall's surface, shared by the page (waterfall_js.go, where the
// file comment says what the two surfaces are and why each is built as it is)
// and a native rack: the knobs and the defaults each surface wants, when a
// slice or a measurement is due, the measurement, and the lines of the
// surface. Where the samples come from and how the lines reach the screen are
// each side's own.

const (
	// wfallBins is the frequency resolution of each slice — points across the
	// display, not FFT bins.
	wfallBins = 96

	// WaterfallCaptureSec is how much audio is kept for the deconvolution. It
	// has to cover a WHOLE sweep pass or the measurement only sees the bottom
	// of the band — a third of a pass is 20 Hz to 200 Hz, and the impulse it
	// recovers is a five-millisecond smear.
	WaterfallCaptureSec = 6
)

// wfallFFTSizes are the transform sizes the FFT knob offers.
//
// The one control on a spectrum analyzer that is a genuine TRADE rather than a
// preference: a longer transform resolves frequency better and time worse, and
// there is no setting that is good at both. 2048 at 48 kHz is a 43 ms window
// and 23 Hz bins; 8192 is 171 ms and 5.9 Hz. On the decay surface the window
// wants to be SHORT — the slices are milliseconds apart and a window longer
// than the spacing means consecutive slices are looking at the same audio, so
// the decay smears. On the live surface it wants to be LONG, because the bottom
// octave is a fifth of a logarithmic axis and 23 Hz bins draw that fifth as a
// staircase.
var wfallFFTSizes = []int{1024, 2048, 4096, 8192}

// wfallSurfaceDefaults are the line count, spacing and transform each surface
// wants, because the two want quite different ones.
//
// A decay is 16 lines 5 ms apart through a short window: 80 ms deep, which is
// where a loudspeaker's resonances live. A live surface is 32 lines 40 ms apart
// through a long one: 1.3 seconds, about a bar of music. Sharing one default
// would make one of the two useless out of the box — 80 ms of live history
// shimmers and shows no note decaying, and 1.3 seconds of CSD is past the end
// of the impulse response.
//
// Applied on a source change the way takens_js.go applies a measured τ: only
// while the knob still holds a value NOBODY CHOSE, so switching back and forth
// does not throw away a setting somebody made.
type wfallDefaults struct{ lines, step, fft float32 }

var (
	wfallDecayDefaults = wfallDefaults{lines: 16, step: 5, fft: 1}  // 2048
	wfallLiveDefaults  = wfallDefaults{lines: 32, step: 40, fft: 2} // 4096
)

// wfallLiveWindow is the window the live surface's spectra are taken with.
// Hann rather than the Blackman-Harris the measurements use: this is a picture
// of a signal, not a measurement of a level beside a loud neighbor, and Hann
// is the narrower main lobe of the two — which on a log axis is the difference
// between two low notes being two ridges and being one.
const wfallLiveWindow = meters.WinHann

// How the decay surface re-measures when there is no generator sweep to trigger
// on.
//
// The trigger is the sweep wrapping, which is exact and is right whenever the
// stimulus is this app's own. It is also the only trigger there was, so with an
// EXTERNAL sweep — a measurement rig feeding both channels in, which is the
// case this mode is most useful in — the surface was computed once and then
// held forever, because the position it was watching never moved. Free-running
// on a timer is the fallback: slower than a pass, so a pass is never cut in
// half, and only when no sweep of ours has moved for a while.
const (
	wfallFreeMS      = 2500.0
	wfallSweepIdleMS = 1000.0
)

var wfallFreqs = acoustics.LogFreqPoints(acoustics.RTALo, acoustics.RTAHi, wfallBins)

// Waterfall is the waterfall's surface and its knobs.
type Waterfall struct {
	surface []acoustics.CSDSlice
	rt60    float64
	rtOK    bool
	lastPos float64
	haveIR  bool

	// The free-running fallback: when the sweep position was last seen to move,
	// and when the next untriggered measurement is due.
	sweepSeen float64
	nextFree  float64

	// The live surface's clock, and the source the surface currently holds so
	// a switch does not leave half of one kind of slice behind half of the
	// other.
	liveNext   float64
	surfaceSrc float32

	// The knobs.
	rangeF float32 // dB shown, below TOP
	topF   float32 // dBFS at the top of the scale
	depthF float32 // how far back the surface reaches, a scale
	swapF  float32 // which channel is the reference
	srcF   float32 // 0 = decay from a sweep, 1 = live spectra
	chanF  float32 // which channel the live surface analyses
	linesF float32 // how many slices the surface has
	stepF  float32 // milliseconds between them
	fftF   float32 // index into wfallFFTSizes

	// autoSet is the triple the source switch last wrote, so a knob
	// somebody turned can be told from one this put there — takens_js.go's rule
	// for its measured τ, and for the same reason.
	autoSet wfallDefaults

	// userSet remembers which of the three somebody has turned, so the
	// source switch never takes one back.
	userSet struct{ lines, step, fft bool }
}

// NewWaterfall is a waterfall at the page's defaults.
func NewWaterfall() *Waterfall {
	return &Waterfall{
		surfaceSrc: -1,
		rangeF:     40,
		depthF:     1,
		linesF:     16,
		stepF:      5,
		fftF:       1,
		autoSet:    wfallDecayDefaults,
	}
}

// WaterfallKnobs are the waterfall's knobs, by the page's ids.
var WaterfallKnobs = []string{"wfall-src", "wfall-chan", "wfall-lines", "wfall-step", "wfall-fft", "wfall-top", "wfall-range", "wfall-depth", "wfall-swap"}

func (w *Waterfall) field(id string) *float32 {
	switch id {
	case "wfall-src":
		return &w.srcF
	case "wfall-chan":
		return &w.chanF
	case "wfall-lines":
		return &w.linesF
	case "wfall-step":
		return &w.stepF
	case "wfall-fft":
		return &w.fftF
	case "wfall-top":
		return &w.topF
	case "wfall-range":
		return &w.rangeF
	case "wfall-depth":
		return &w.depthF
	case "wfall-swap":
		return &w.swapF
	}
	return nil
}

// Set turns knob id to v, reporting whether the waterfall has it.
func (w *Waterfall) Set(id string, v float32) bool {
	p := w.field(id)
	if p == nil {
		return false
	}
	*p = v
	return true
}

// The knobs as the values the code wants, clamped.
//
// Clamped rather than trusted, because every one of these is an audio-modulation
// target: a modulator drives the knob past its own ends and a line count that
// comes out zero or a transform size that indexes off the end of the table is a
// crash rather than a wrong picture.

// lines is how many slices the surface holds.
func (w *Waterfall) lines() int {
	n := int(w.linesF + 0.5)
	if n < 4 {
		n = 4
	} else if n > 64 {
		n = 64
	}
	return n
}

// stepMS is how far apart in time they are.
func (w *Waterfall) stepMS() float64 {
	ms := float64(w.stepF)
	if ms < 1 {
		ms = 1
	} else if ms > 100 {
		ms = 100
	}
	return ms
}

// fftLen is the transform each slice is taken with.
func (w *Waterfall) fftLen() int {
	i := int(w.fftF + 0.5)
	if i < 0 {
		i = 0
	} else if i >= len(wfallFFTSizes) {
		i = len(wfallFFTSizes) - 1
	}
	return wfallFFTSizes[i]
}

// LiveFFT is how many of the newest samples a live slice is taken from.
func (w *Waterfall) LiveFFT() int { return w.fftLen() }

// channel is the channel the live surface analyses. The decay surface does
// not have one: it needs BOTH channels, and REF says which of the two is the
// reference.
func (w *Waterfall) channel() tapChan {
	i := int(w.chanF + 0.5)
	if i < 0 {
		i = 0
	} else if i >= len(tapChanNames) {
		i = len(tapChanNames) - 1
	}
	// Clamped to the name table just above, so the value is 0..4 — gosec sees
	// only int → uint8 and cannot see the clamp.
	return tapChan(i) //nolint:gosec // clamped to len(tapChanNames)-1 above
}

// Live reports whether SRC picks the live surface rather than the decay.
func (w *Waterfall) Live() bool { return w.srcF > 0.5 }

// Swapped reports whether REF makes the right channel the reference.
func (w *Waterfall) Swapped() bool { return w.swapF > 0.5 }

// Fold is l and r reduced to the channel the live surface analyses.
func (w *Waterfall) Fold(l, r []float32) []float32 {
	c := w.channel()
	out := make([]float32, min(len(l), len(r)))
	for i := range out {
		out[i] = tapFold(c, l[i], r[i])
	}
	return out
}

// useDefaults moves LINE, STEP and FFT to what the surface now selected
// wants — but only the ones still holding a value nobody chose — through set,
// which turns the knob itself so the panel shows the new value.
//
// The three differ by roughly an order of magnitude between the two surfaces,
// so carrying one set across a switch leaves the other surface useless: 80 ms
// of live history shimmers and never shows a note decay, and 1.3 seconds of CSD
// runs off the end of the impulse response. Deferring to a turned knob is
// takens_js.go's rule for its measured τ, and it is the same problem — a
// control that silently takes the knob away is worse than no automation.
func (w *Waterfall) useDefaults(d wfallDefaults, set func(id string, p *float32, v float32)) {
	// A knob holding anything but what this last wrote was turned by hand, and
	// from then on it is the user's for good.
	//
	// The "for good" is the part that is not obvious and is the part that was
	// wrong first: recording the deferred-to value as though this had written it
	// makes a chosen value indistinguishable from an automatic one at the NEXT
	// switch, so a hand-set line count survived one switch and was reclaimed on
	// the way back. A permalink falls out of the same rule — it arrives holding a
	// value that differs from the default, so the first switch marks it chosen.
	if w.linesF != w.autoSet.lines {
		w.userSet.lines = true
	}
	if w.stepF != w.autoSet.step {
		w.userSet.step = true
	}
	if w.fftF != w.autoSet.fft {
		w.userSet.fft = true
	}
	if !w.userSet.lines {
		set("wfall-lines", &w.linesF, d.lines)
	}
	if !w.userSet.step {
		set("wfall-step", &w.stepF, d.step)
	}
	if !w.userSet.fft {
		set("wfall-fft", &w.fftF, d.fft)
	}
	w.autoSet = wfallDefaults{lines: w.linesF, step: w.stepF, fft: w.fftF}
}

// Switched starts the surface SRC now names, if it has changed, and reports
// whether it had. The knobs go to that surface's defaults through set.
func (w *Waterfall) Switched(set func(id string, p *float32, v float32)) bool {
	if w.srcF == w.surfaceSrc {
		return false
	}
	// The two surfaces are different lengths on different scales — a decay
	// normalized to its own impulse, a live one in absolute dBFS — so the
	// old one is dropped rather than grown or shrunk into the new one.
	w.surface = nil
	w.haveIR = false
	w.liveNext = 0
	w.surfaceSrc = w.srcF
	if w.Live() {
		w.useDefaults(wfallLiveDefaults, set)
	} else {
		w.useDefaults(wfallDecayDefaults, set)
	}
	return true
}

// PushLive pushes a spectrum of x, the newest LiveFFT samples at rate sr,
// onto the front of the live surface when a slice is due at nowMs, and ages
// everything behind it.
//
// Slice 0 is the front of the surface, so the newest goes there and the rest
// shift back — which is the direction the display already reads, and means the
// live surface and the decay surface are drawn by the same code with no idea
// which of them they are showing.
//
// Slices are pushed on a CLOCK rather than per frame: a surface built per frame
// is a different length of history on a 60 Hz display than on a 144 Hz one, and
// the depth axis stops meaning seconds. STEP below a frame is the one case that
// cannot be honored — there is no more audio to have — and it degrades to one
// slice per frame rather than to a burst.
func (w *Waterfall) PushLive(x []float32, sr int, nowMs float64) {
	if nowMs < w.liveNext {
		return
	}
	// Set forward from NOW rather than by adding the interval to the last due
	// time: after a stall — a tab in the background, a mode just switched into —
	// adding would fire a burst of slices to catch up, and the surface would
	// show a tenth of a second stretched across its whole depth.
	w.liveNext = nowMs + w.stepMS()

	lines := w.lines()
	if len(w.surface) < lines {
		w.surface = append(w.surface, acoustics.CSDSlice{DB: make([]float64, len(wfallFreqs))})
	} else if len(w.surface) > lines {
		// LINE turned down: drop from the BACK, which is the oldest, so the
		// front of the surface — what is playing now — never jumps.
		w.surface = w.surface[:lines]
	}
	// Rotate rather than reallocate: the oldest slice's buffer becomes the
	// newest, so a surface of thirty-two spectra allocates thirty-two times and
	// never again.
	oldest := w.surface[len(w.surface)-1]
	copy(w.surface[1:], w.surface[:len(w.surface)-1])
	w.surface[0] = oldest
	if !acoustics.SpectrumPoints(x, sr, wfallFreqs, wfallLiveWindow, w.surface[0].DB) {
		return
	}
	for i := range w.surface {
		w.surface[i].TimeMS = float64(i) * w.stepMS()
	}
	w.haveIR = true
}

// DropSurface throws the surface away: the window it was taken from no
// longer exists, or not at the resolution it was taken at. Half a surface at
// one resolution against half at another is a picture of the knob being
// turned, not of the sound.
func (w *Waterfall) DropSurface() { w.surface = nil }

// MeasureDue reports whether the decay surface should be measured again at
// nowMs, given the generator's sweep position pos (0 with no sweep).
//
// Triggered on the SWEEP'S OWN POSITION rather than on a timer: a measurement
// taken across the wrap between one pass and the next is a measurement of two
// different stimuli spliced together, and it is the generator that knows where
// the pass is.
func (w *Waterfall) MeasureDue(pos, nowMs float64) bool {
	// A pass has completed when the sweep's position wraps back to the start.
	wrapped := pos < w.lastPos
	if pos != w.lastPos {
		w.sweepSeen = nowMs
	}
	w.lastPos = pos
	if wrapped {
		w.nextFree = nowMs + wfallFreeMS
		return true
	}
	// Mid-pass of a sweep of ours: wait for the wrap, which is the exact trigger.
	if nowMs-w.sweepSeen < wfallSweepIdleMS {
		return false
	}
	// NO SWEEP OF OURS IS RUNNING, so there is no wrap to wait for and waiting
	// for one is how this used to freeze: it measured once from whatever was in
	// the buffer and then held that picture forever, with nothing on screen
	// saying so. An external sweep — the case a measurement rig is in — is a
	// real stimulus and has to be picked up; program material is not, and the
	// surface it gives is one channel deconvolved against the other, which is
	// meaningless but is at least visibly moving rather than pretending to be a
	// measurement. SRC live is the mode for program material.
	if w.haveIR && nowMs < w.nextFree {
		return false
	}
	w.nextFree = nowMs + wfallFreeMS
	return true
}

// Measure deconvolves the measurement against the reference, the newest
// WaterfallCaptureSec of each at rate sr, and builds the decay surface.
func (w *Waterfall) Measure(ref, meas []float32, sr int) {
	// A power of two of the captured audio, taken from the end — the most
	// recent whole pass.
	n := 1
	for n*2 <= len(ref) {
		n *= 2
	}
	if n < 1<<15 || len(meas) < n {
		return
	}
	ir := acoustics.ImpulseResponse(ref[len(ref)-n:], meas[len(meas)-n:], 1e-4)
	if ir == nil {
		return
	}
	w.surface = acoustics.CSD(ir, sr, w.lines(), w.stepMS(), w.fftLen(), wfallFreqs)
	// The reverberation time from the same impulse, which is the one number the
	// surface does not show: the surface is 80 ms deep and a room's decay is
	// measured over seconds.
	peak, _ := acoustics.IRPeak(ir)
	if peak < len(ir)-sr/4 {
		w.rt60, w.rtOK = acoustics.ReverbTime(acoustics.SchroederDecay(ir[peak:]), sr, -5, -25)
	} else {
		w.rtOK = false
	}
	w.haveIR = len(w.surface) > 0
}

// RT60 is the decay's reverberation time, when the decay surface has one.
func (w *Waterfall) RT60() (float64, bool) { return w.rt60, !w.Live() && w.rtOK }

// WaterfallSpan is the surface's half-width in world units, matching the
// other models' fit.
const WaterfallSpan = 9.0

// Slices is how many slices the surface has now.
func (w *Waterfall) Slices() int {
	if len(wfallFreqs) < 2 {
		return 0
	}
	return len(w.surface)
}

// Depth is the surface's depth in world units, as DPTH sets it.
func (w *Waterfall) Depth() float64 { return WaterfallSpan * float64(w.depthF) }

// eachSegment hands put the ends of each line of the first n slices, in
// order: frequency across, level up, slice into the screen, and the slice's
// age as the trail attribute the gradient reads — DEPTH rather than position
// along the line, so the color says how long ago rather than which frequency.
// The frequency is already the x axis and coloring it again would spend the
// gradient saying the same thing twice.
func (w *Waterfall) eachSegment(n int, put func(x, y, z, age float32)) {
	bins := len(wfallFreqs)
	rng := float64(w.rangeF)
	if rng < 1 {
		rng = 1
	}
	top := float64(w.topF)
	span, depth := WaterfallSpan, w.Depth()
	vert := func(bin, slice int) {
		x := span * (2*float64(bin)/float64(bins-1) - 1)
		db := w.surface[slice].DB[bin]
		t := (db - (top - rng)) / rng // 0 at the bottom of the range, 1 at the top
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		y := span * (t - 0.5)
		z := depth * (float64(slice)/float64(max(n-1, 1)) - 0.5)
		put(float32(x), float32(y), float32(z), float32(slice)/float32(max(n-1, 1)))
	}
	for s := range n {
		for b := 1; b < bins; b++ {
			vert(b-1, s)
			vert(b, s)
		}
	}
}

// Figure is the surface as lines, every slice of it, for a renderer that
// colors by position: a pair of points per line, and the two corners of the
// surface's exact bounds as points no line joins, so the picture is framed
// and colored over the whole TOP..TOP-RNGE scale rather than over what the
// sound reached.
func (w *Waterfall) Figure() Figure {
	n := w.Slices()
	f := Figure{Kind: FigureLines}
	if n == 0 {
		return f
	}
	w.eachSegment(n, func(x, y, z, _ float32) {
		f.Edges = append(f.Edges, uint16(len(f.Points))) //nolint:gosec // 64 slices × 95 lines × 2 ends
		f.Points = append(f.Points, [3]float64{float64(x), float64(y), float64(z)})
	})
	d := w.Depth() / 2
	f.Points = append(f.Points,
		[3]float64{-WaterfallSpan, -WaterfallSpan / 2, -d},
		[3]float64{WaterfallSpan, WaterfallSpan / 2, d})
	return f
}
