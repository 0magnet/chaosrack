//go:build js && wasm

package attractor

import "syscall/js"

// The Waterfall mode — cumulative spectral decay, as a surface in the 3-D
// pipeline.
//
// The analysis is in impulse.go, untagged and checked against a synthetic
// resonance and a decay of known T60. This is the picture, and it is the one
// display here that could only exist in this app: every other analyzer in the
// rack draws its own flat panel, and this one is a genuine three-dimensional
// surface, so it goes through the SAME vertex pipeline the attractors do and
// gets the camera, the drag-to-rotate, the gradient, the persist painting and
// Model Out for nothing.
//
// Frequency runs left to right (logarithmically, because hearing is organised
// in ratios), level up, and TIME INTO THE SCREEN. Each line is one slice: the
// spectrum of what is left of the impulse response from a moment onwards. A
// flat loudspeaker's surface falls away evenly; a resonance is a ridge running
// back into the screen at one frequency, which is the thing a frequency
// response cannot show and this exists for.
//
// ── IT IS A MEASUREMENT, SO IT HOLDS STILL ───────────────────────────────
//
// Unlike every other audio mode here, the picture does not move with the sound.
// An impulse response is a measurement of a system rather than a view of a
// signal: it is made from one sweep pass and then it is the answer until
// another is made. So the surface is recomputed when a pass completes and holds
// between them, which is also what makes it something you can rotate and look
// at rather than something you have to freeze first.

const (
	// wfallSlices is how many time slices the surface has, and wfallSliceMS how
	// far apart they are. Sixteen slices five milliseconds apart is 80 ms of
	// decay, which is where a loudspeaker's resonances live; a room's
	// reverberation is a hundred times longer and is what the RT60 readout is
	// for instead.
	wfallSlices  = 16
	wfallSliceMS = 5.0

	// wfallBins is the frequency resolution of each slice — points across the
	// display, not FFT bins.
	wfallBins = 96

	// wfallFFT is the transform each slice is taken with. 2048 at 48 kHz is a
	// 43 ms window, which is long enough to resolve the low end and short
	// enough that consecutive slices are looking at different audio.
	wfallFFT = 2048

	// wfallCaptureSec is how much audio is kept for the deconvolution. It has
	// to cover a WHOLE sweep pass or the measurement only sees the bottom of
	// the band — a third of a pass is 20 Hz to 200 Hz, and the impulse it
	// recovers is a five-millisecond smear.
	wfallCaptureSec = 6
)

// The live surface.
//
// A CSD is a measurement and holds still between sweeps; this is the other
// thing the word waterfall means, and the one somebody selects the mode with
// music playing expects to see: successive spectra of what is playing, stacked
// into the screen as they age. Same axes, same surface, same gradient — only
// the z axis stops being time-since-the-impulse and becomes time-ago.
const (
	// wfallLiveSlices is how deep the history runs and wfallLivePushMS how often
	// a slice is pushed. Thirty-two at 40 ms is 1.3 seconds, which is about a
	// bar of music — long enough to watch a note decay and short enough that
	// the front of the surface is still what is playing now.
	//
	// Pushed on a CLOCK rather than per frame: a surface built per frame is a
	// different length of history on a 60 Hz display than on a 144 Hz one, and
	// the depth axis stops meaning seconds.
	wfallLiveSlices = 32
	wfallLivePushMS = 40.0

	// wfallLiveFFT is the transform each slice is taken with. 4096 at 48 kHz is
	// an 85 ms window and an 11.7 Hz resolution, which is what it takes to
	// separate anything in the bottom octave — the log axis spends a fifth of
	// its width below 80 Hz and a coarser window draws that fifth as one step.
	wfallLiveFFT = 4096

	// wfallLiveWindow is the window it is taken with. Hann rather than the
	// Blackman-Harris the measurements use: this is a picture of a signal, not a
	// measurement of a level beside a loud neighbour, and Hann is the narrower
	// main lobe of the two — which on a log axis is the difference between two
	// low notes being two ridges and being one.
	wfallLiveWindow = winHann
)

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

var (
	wfallCursor  = tapUnjoined
	wfallRefBuf  []float32 // the reference: what went out
	wfallMeasBuf []float32 // the measurement: what came back
	wfallFill    int
	wfallSurface []CSDSlice
	wfallFreqs   = LogFreqPoints(rtaLo, rtaHi, wfallBins)
	wfallRT60    float64
	wfallRTOK    bool
	wfallLastPos float64
	wfallHaveIR  bool

	// The free-running fallback: when the sweep position was last seen to move,
	// and when the next untriggered measurement is due.
	wfallSweepSeen float64
	wfallNextFree  float64

	// The live surface: its own tap cursor, its own window of audio, its own
	// clock, and the source the surface currently holds so a switch does not
	// leave half of one kind of slice behind half of the other.
	wfallLiveCursor = tapUnjoined
	wfallLiveBuf    []float32
	wfallLiveFill   int
	wfallLiveNext   float64
	wfallSurfaceSrc float32 = -1

	// The knobs.
	wfallRangeF float32 = 40 // dB of decay shown
	wfallDepthF float32 = 1  // how far back the surface reaches, a scale
	wfallSwapF  float32      // which channel is the reference
	wfallSrcF   float32      // 0 = decay from a sweep, 1 = live spectra
)

func init() {
	registerGenerate("waterfall", generateWaterfall)
	attractorParams["waterfall"] = []paramDef{
		{"wfall-range", "rnge", &wfallRangeF, 40, 10, 80, 5},
		{"wfall-depth", "dpth", &wfallDepthF, 1, 0.2, 3, 0.1},
		{"wfall-swap", "ref", &wfallSwapF, 0, 0, 1, 1},
		{"wfall-src", "src", &wfallSrcF, 0, 0, 1, 1},
	}
}

// generateWaterfall runs whichever surface SRC names, and draws it.
func generateWaterfall() {
	defer showWaterfallRT()
	if wfallSrcF != wfallSurfaceSrc {
		// The two surfaces are different lengths on different scales — a decay
		// normalized to its own impulse, a live one in absolute dBFS — so the
		// old one is dropped rather than grown or shrunk into the new one.
		wfallSurface = nil
		wfallHaveIR = false
		wfallLiveFill = 0
		wfallLiveNext = 0
		wfallSurfaceSrc = wfallSrcF
		wfallArmFit()
	}
	if wfallSrcF > 0.5 {
		wfallLiveTick()
	} else {
		wfallCapture()
	}
	wfallDraw()
}

// wfallLiveTick pushes a spectrum of the newest audio onto the front of the
// surface, on wfallLivePushMS centres, and ages everything behind it.
//
// Slice 0 is the front of the surface in wfallDraw, so the newest goes there and
// the rest shift back — which is the direction the display already reads, and
// means the live surface and the decay surface are drawn by the same code with
// no idea which of them they are showing.
func wfallLiveTick() {
	sr := takensSourceRate()
	if len(wfallLiveBuf) != wfallLiveFFT {
		wfallLiveBuf = make([]float32, wfallLiveFFT)
		wfallLiveFill = 0
	}
	// Drain the tap into the window, keeping the newest wfallLiveFFT samples.
	// Drained EVERY frame even though a slice is only pushed every 40 ms: the
	// ring is finite, and a reader that only reads when it wants a slice falls
	// behind and then jumps, which is a surface built from audio that is not
	// adjacent to itself.
	var blk [4096]float32
	for {
		n := tapReadChan(&wfallLiveCursor, blk[:], tapMix)
		if n <= 0 {
			break
		}
		if n >= len(wfallLiveBuf) {
			copy(wfallLiveBuf, blk[n-len(wfallLiveBuf):n])
			wfallLiveFill = len(wfallLiveBuf)
		} else {
			copy(wfallLiveBuf, wfallLiveBuf[n:])
			copy(wfallLiveBuf[len(wfallLiveBuf)-n:], blk[:n])
			if wfallLiveFill += n; wfallLiveFill > len(wfallLiveBuf) {
				wfallLiveFill = len(wfallLiveBuf)
			}
		}
		if n < len(blk) {
			break
		}
	}
	if wfallLiveFill < len(wfallLiveBuf) {
		return
	}
	if frameNowMs < wfallLiveNext {
		return
	}
	// Set forward from NOW rather than by adding the interval to the last due
	// time: after a stall — a tab in the background, a mode just switched into —
	// adding would fire a burst of slices to catch up, and the surface would
	// show a tenth of a second stretched across its whole depth.
	wfallLiveNext = frameNowMs + wfallLivePushMS

	if len(wfallSurface) < wfallLiveSlices {
		wfallSurface = append(wfallSurface, CSDSlice{DB: make([]float64, len(wfallFreqs))})
	}
	// Rotate rather than reallocate: the oldest slice's buffer becomes the
	// newest, so a surface of thirty-two spectra allocates thirty-two times and
	// never again.
	oldest := wfallSurface[len(wfallSurface)-1]
	copy(wfallSurface[1:], wfallSurface[:len(wfallSurface)-1])
	wfallSurface[0] = oldest
	if !SpectrumPoints(wfallLiveBuf, sr, wfallFreqs, wfallLiveWindow, wfallSurface[0].DB) {
		return
	}
	for i := range wfallSurface {
		wfallSurface[i].TimeMS = float64(i) * wfallLivePushMS
	}
	wfallHaveIR = true
}

// wfallCapture keeps the rolling stereo buffer and triggers a measurement each
// time the generator's sweep comes round.
//
// Triggered on the SWEEP'S OWN POSITION rather than on a timer: a measurement
// taken across the wrap between one pass and the next is a measurement of two
// different stimuli spliced together, and it is the generator that knows where
// the pass is.
func wfallCapture() {
	sr := takensSourceRate()
	want := sr * wfallCaptureSec
	if len(wfallRefBuf) != want {
		wfallRefBuf = make([]float32, want)
		wfallMeasBuf = make([]float32, want)
		wfallFill = 0
	}
	var sl, srr [4096]float32
	for {
		n := tapReadStereo(&wfallCursor, sl[:], srr[:])
		if n <= 0 {
			break
		}
		ref, meas := sl[:n], srr[:n]
		if wfallSwapF > 0.5 {
			ref, meas = srr[:n], sl[:n]
		}
		if n >= want {
			copy(wfallRefBuf, ref[n-want:])
			copy(wfallMeasBuf, meas[n-want:])
			wfallFill = want
		} else {
			copy(wfallRefBuf, wfallRefBuf[n:])
			copy(wfallMeasBuf, wfallMeasBuf[n:])
			copy(wfallRefBuf[want-n:], ref)
			copy(wfallMeasBuf[want-n:], meas)
			if wfallFill += n; wfallFill > want {
				wfallFill = want
			}
		}
		if n < len(sl) {
			break
		}
	}
	if wfallFill < want {
		return
	}
	// A pass has completed when the sweep's position wraps back to the start.
	pos := 0.0
	if useFuncGen && funcGen != nil {
		pos = funcGen.SweepPosition()
	}
	wrapped := pos < wfallLastPos
	if pos != wfallLastPos {
		wfallSweepSeen = frameNowMs
	}
	wfallLastPos = pos
	if wrapped {
		wfallMeasure(sr)
		wfallNextFree = frameNowMs + wfallFreeMS
		return
	}
	// Mid-pass of a sweep of ours: wait for the wrap, which is the exact trigger.
	if frameNowMs-wfallSweepSeen < wfallSweepIdleMS {
		return
	}
	// NO SWEEP OF OURS IS RUNNING, so there is no wrap to wait for and waiting
	// for one is how this used to freeze: it measured once from whatever was in
	// the buffer and then held that picture forever, with nothing on screen
	// saying so. An external sweep — the case a measurement rig is in — is a
	// real stimulus and has to be picked up; program material is not, and the
	// surface it gives is one channel deconvolved against the other, which is
	// meaningless but is at least visibly moving rather than pretending to be a
	// measurement. SRC live is the mode for program material.
	if wfallHaveIR && frameNowMs < wfallNextFree {
		return
	}
	wfallNextFree = frameNowMs + wfallFreeMS
	wfallMeasure(sr)
}

// wfallMeasure deconvolves and builds the surface.
func wfallMeasure(sr int) {
	// A power of two of the captured audio, taken from the end — the most
	// recent whole pass.
	n := 1
	for n*2 <= len(wfallRefBuf) {
		n *= 2
	}
	if n < 1<<15 {
		return
	}
	ref := wfallRefBuf[len(wfallRefBuf)-n:]
	meas := wfallMeasBuf[len(wfallMeasBuf)-n:]
	ir := ImpulseResponse(ref, meas, 1e-4)
	if ir == nil {
		return
	}
	wfallSurface = CSD(ir, sr, wfallSlices, wfallSliceMS, wfallFFT, wfallFreqs)
	// The reverberation time from the same impulse, which is the one number the
	// surface does not show: the surface is 80 ms deep and a room's decay is
	// measured over seconds.
	peak, _ := IRPeak(ir)
	if peak < len(ir)-sr/4 {
		wfallRT60, wfallRTOK = ReverbTime(SchroederDecay(ir[peak:]), sr, -5, -25)
	} else {
		wfallRTOK = false
	}
	wfallHaveIR = len(wfallSurface) > 0
}

// wfallDraw builds the surface's vertices and hands them to the normal pipeline.
//
// As LINES rather than a line strip: the slices are separate curves, and a
// strip would draw a diagonal from the end of each one back to the start of the
// next — sixteen bright diagonals across a surface, which is the retrace a
// scope shows and not something a measurement should.
func wfallDraw() {
	n := len(wfallSurface)
	if n == 0 || len(wfallFreqs) < 2 {
		uploadVerticesOnly(vertBuf[:0], glTypes.Lines, 0)
		return
	}
	bins := len(wfallFreqs)
	need := n * (bins - 1) * 2
	if need*4 > len(vertBuf) {
		// More than the trail budget allows. The budget is a knob, so this is
		// a real possibility rather than a theoretical one, and drawing fewer
		// slices is a better answer than drawing a corrupt surface.
		n = len(vertBuf) / 4 / ((bins - 1) * 2)
		if n < 1 {
			uploadVerticesOnly(vertBuf[:0], glTypes.Lines, 0)
			return
		}
		need = n * (bins - 1) * 2
	}
	rng := float64(wfallRangeF)
	if rng < 1 {
		rng = 1
	}
	const span = 9.0 // world units either side, matching the other modes' fit
	depth := span * float64(wfallDepthF)
	v := vertBuf[:need*4]
	o := 0
	put := func(bin, slice int) {
		x := span * (2*float64(bin)/float64(bins-1) - 1)
		db := wfallSurface[slice].DB[bin]
		t := (db + rng) / rng // 0 at the bottom of the range, 1 at the top
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		y := span * (t - 0.5)
		z := depth * (float64(slice)/float64(n-1) - 0.5)
		v[o], v[o+1], v[o+2] = float32(x), float32(y), float32(z)
		// The trail attribute, which the gradient reads: DEPTH rather than
		// position along the line, so the colour says how long ago rather than
		// which frequency — the frequency is already the x axis and colouring
		// it again would spend the gradient saying the same thing twice.
		v[o+3] = float32(slice) / float32(max(n-1, 1))
		o += 4
	}
	for s := 0; s < n; s++ {
		for b := 1; b < bins; b++ {
			put(b-1, s)
			put(b, s)
		}
	}
	uploadVerticesOnly(v, glTypes.Lines, need)
	if !wfallFitted {
		// Fitted to the surface's OWN half-span, which is exactly what its
		// largest coordinate reaches — autoFitCamera measures extent as
		// max|coordinate| and the frequency axis is the widest of the three. The
		// 1.2 that used to pad it was reserving room for nothing: the bound here
		// is exact rather than a worst case, unlike the audio embeddings where
		// the figure's size depends on the signal.
		//
		// The surface still leaves room either side, because the camera frames a
		// SPHERE of that radius and the window is wider than it is tall. That is
		// how every model here is framed and the zoom is how to fill the width.
		wfallFitted = true
		fitExtentOverride = span
		autoFitCamera()
	}
}

// wfallFitted is the one-shot camera fit, as the audio embeddings have.
var wfallFitted bool

// wfallArmFit re-arms it, for a mode change.
func wfallArmFit() { wfallFitted = false }

var (
	wfallRTEl js.Value
	wfallRTTx string
)

// showWaterfallRT writes the reverberation time beside the knobs.
//
// The one number the surface does not show and cannot: it is eighty
// milliseconds deep, which is where a loudspeaker's resonances live, and a
// room's decay is measured over seconds. It was being computed from the same
// impulse response and then thrown away, so the README described a readout
// that was not on screen.
//
// T20 extrapolated, from -5 dB to -25 dB: the first 5 dB is the direct sound
// and the last of a real decay is in the noise, so the standard measures the
// straight part between them and multiplies up. Blank in the live surface,
// which is spectra of a signal and has no impulse to decay from.
func showWaterfallRT() {
	s := "--- s"
	if wfallSrcF < 0.5 && wfallRTOK {
		s = formatLED(wfallRT60, 1, 2, false) + " s"
	}
	if s == wfallRTTx {
		return
	}
	wfallRTTx = s
	if wfallRTEl.Truthy() {
		wfallRTEl.Set("textContent", s)
	}
}

// appendWaterfallReadout adds the RT60 cell to the mode's parameter grid.
func appendWaterfallReadout(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "rt60")
	card.Call("appendChild", lbl)

	wfallRTEl = doc.Call("createElement", "span")
	wfallRTEl.Set("className", "led counter-led")
	wfallRTEl.Set("title", "Reverberation time of the room, in seconds — how long a sound takes to "+
		"fall 60 dB after it stops. Taken from the same impulse response the surface is, by "+
		"Schroeder backward integration: the decay curve is the energy REMAINING after each "+
		"moment, which turns a noisy decay into a smooth one without averaging repeated "+
		"measurements. Measured as T20 and extrapolated — the straight part between -5 dB and "+
		"-25 dB, because the first few decibels are direct sound and the last of a real decay is "+
		"in the noise floor. Blank on the live surface, which has no impulse to decay from, and "+
		"blank until a sweep has been measured.")
	wfallRTTx = ""
	card.Call("appendChild", wfallRTEl)
	grid.Call("appendChild", card)
}
