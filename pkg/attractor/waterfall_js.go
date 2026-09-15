//go:build js && wasm

package attractor

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

	// The knobs.
	wfallRangeF float32 = 40 // dB of decay shown
	wfallDepthF float32 = 1  // how far back the surface reaches, a scale
	wfallSwapF  float32      // which channel is the reference
)

func init() {
	registerGenerate("waterfall", generateWaterfall)
	attractorParams["waterfall"] = []paramDef{
		{"wfall-range", "rnge", &wfallRangeF, 40, 10, 80, 5},
		{"wfall-depth", "dpth", &wfallDepthF, 1, 0.2, 3, 0.1},
		{"wfall-swap", "ref", &wfallSwapF, 0, 0, 1, 1},
	}
}

// generateWaterfall captures, measures when a sweep pass completes, and draws.
func generateWaterfall() {
	wfallCapture()
	wfallDraw()
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
	wfallLastPos = pos
	if !wrapped && wfallHaveIR {
		return
	}
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
