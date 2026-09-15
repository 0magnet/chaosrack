//go:build js && wasm

package attractor

import "syscall/js"

// The Transfer mode — magnitude, phase and coherence between two channels.
//
// The measurement is in transfer.go, untagged and checked against a known gain,
// a known delay and two independent noises. This is the picture: three curves
// stacked on one screen over a logarithmic frequency axis, drawn with the xy
// scope's program as the RTA is.
//
// ── WHAT GOES IN WHICH CHANNEL ───────────────────────────────────────────
//
// REFERENCE is what went out and MEASUREMENT what came back. The usual rig is
// the signal generator into one input of the interface and a microphone into
// the other, or a loopback of the send against the return. The two are read
// through ONE tap cursor (tapReadStereo) so they arrive sample-aligned, which
// matters more here than anywhere else in the app: a one-sample slip between
// the channels IS a delay, and this display's whole subject is delay.
//
// The knob swaps which is which, because half the time the cabling makes it the
// other way round and rewiring to satisfy a display is absurd.

const (
	// xfFFT is the analysis length. 8192 at 48 kHz is a 5.9 Hz bin and a 171 ms
	// window — long enough to hold a room's early reflections, which is what a
	// response measurement has to span, and short enough that a source moving
	// during it is not the thing being measured.
	xfFFT = 8192

	// xfPeriodMs is how often a window is folded into the average.
	xfPeriodMs = 120
)

var (
	xfCursor = tapUnjoined
	xfBufL   []float32
	xfBufR   []float32
	xfFill   int
	xfNextMs float64
	xfAccum  TransferAccum
	xfRes    TransferResult
	xfLine   []float32
	xfU8     js.Value
	xfF32    js.Value

	// The knobs.
	xfSwapF  float32      // 0 = left is the reference, 1 = right
	xfFracF  float32 = 2  // index into rtaFractions; 2 is 1/6 octave
	xfAvgF   float32 = 24 // windows in the average
	xfRangeF float32 = 40 // dB either side of 0 on the magnitude curve
	xfCohF   float32 = 5  // minimum coherence, in tenths
	xfShowF  float32      // which curves are drawn
)

// xfShowNames are the display's layouts, and xfShowRing what fits on the dial.
var (
	xfShowNames = []string{"mag + phase + coh", "magnitude only", "phase only", "coherence only"}
	xfShowRing  = []string{"all", "mag", "phz", "coh"}
)

func init() {
	registerGenerate("xfer", generateTransfer)
	attractorParams["xfer"] = []paramDef{
		{"xf-swap", "ref", &xfSwapF, 0, 0, 1, 1},
		{"xf-frac", "band", &xfFracF, 2, 0, float32(len(rtaFractions) - 1), 1},
		{"xf-avg", "avg", &xfAvgF, 24, float32(transferMinAvg), 128, 1},
		{"xf-range", "rnge", &xfRangeF, 40, 6, 60, 2},
		{"xf-coh", "coh", &xfCohF, 5, 0, 10, 1},
		{"xf-show", "show", &xfShowF, 0, 0, float32(len(xfShowNames) - 1), 1},
	}
}

// xfShowSel is the layout knob as an index, clamped — stereoAxisSel's argument,
// and its trap.
func xfShowSel() int {
	v := xfShowF
	if !(v > 0) {
		return 0
	}
	if last := float32(len(xfShowNames) - 1); v > last {
		return len(xfShowNames) - 1
	}
	return int(v + 0.5)
}

// xfFraction is the band-width knob as a 1/b, clamped.
func xfFraction() int {
	v := xfFracF
	if !(v > 0) {
		return rtaFractions[0]
	}
	last := len(rtaFractions) - 1
	if v > float32(last) {
		return rtaFractions[last]
	}
	return rtaFractions[int(v+0.5)]
}

// generateTransfer is the mode's frame.
func generateTransfer() {
	xfAnalyze(frameNowMs)
	drawTransfer()
}

// xfAnalyze accumulates windows into the average.
func xfAnalyze(nowMs float64) {
	if xfBufL == nil {
		xfBufL = make([]float32, xfFFT)
		xfBufR = make([]float32, xfFFT)
	}
	var sl, sr [4096]float32
	for {
		n := tapReadStereo(&xfCursor, sl[:], sr[:])
		if n <= 0 {
			break
		}
		if n >= xfFFT {
			copy(xfBufL, sl[n-xfFFT:n])
			copy(xfBufR, sr[n-xfFFT:n])
			xfFill = xfFFT
		} else {
			copy(xfBufL, xfBufL[n:])
			copy(xfBufR, xfBufR[n:])
			copy(xfBufL[xfFFT-n:], sl[:n])
			copy(xfBufR[xfFFT-n:], sr[:n])
			if xfFill += n; xfFill > xfFFT {
				xfFill = xfFFT
			}
		}
		if n < len(sl) {
			break
		}
	}
	if xfFill < xfFFT || nowMs < xfNextMs {
		return
	}
	xfNextMs = nowMs + xfPeriodMs
	ref, meas := xfBufL, xfBufR
	if xfSwapF > 0.5 {
		ref, meas = xfBufR, xfBufL
	}
	xfAccum.Add(ref, meas, xfWindowKind)
	// A rolling average: once it is full, start again rather than letting the
	// window stretch to the whole session. A system-tuning measurement has to
	// follow a knob being turned, and an average that never forgets cannot.
	if xfAccum.count >= int(xfAvgF) {
		xfRes = xfAccum.Result(takensSourceRate(), xfFraction())
		xfAccum.Reset()
	} else if r := xfAccum.Result(takensSourceRate(), xfFraction()); r.OK {
		xfRes = r
	}
}

// xfY maps a value in one of the three curve's own units to clip space, within
// a horizontal band of the screen.
func xfY(v, lo, hi, bandLo, bandHi float64) float32 {
	t := (v - lo) / (hi - lo)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return float32(bandLo + t*(bandHi-bandLo))
}

// drawTransfer draws the curves.
func drawTransfer() {
	if !xyReady {
		initXY()
	}
	n := len(xfRes.Bands)
	if !xfRes.OK || n < 2 {
		gl.Call("disable", glTypes.DepthTest)
		gl.Call("clearColor", 0, 0, 0, 0)
		gl.Call("clear", glTypes.ColorBufferBit)
		return
	}
	need := n * 3 * 2 * 2
	if len(xfLine) < need {
		xfLine = make([]float32, need+need/2)
		xfU8 = js.Global().Get("Uint8Array").New(len(xfLine) * 4)
		xfF32 = js.Global().Get("Float32Array").New(xfU8.Get("buffer"), 0, len(xfLine))
	}
	show := xfShowSel()
	minCoh := float64(xfCohF) / 10
	rng := float64(xfRangeF)

	// The three curves' vertical lanes. All of them share the screen when
	// "all" is selected, because the three are read TOGETHER: a dip in the
	// magnitude with the coherence still high is the system, and the same dip
	// with the coherence collapsed is the measurement giving up.
	type lane struct {
		lo, hi         float64 // the curve's own units
		bandLo, bandHi float64 // where it sits in clip space
		vals           []float64
		gate           bool // drawn only where the coherence allows
	}
	var lanes []lane
	switch show {
	case 1:
		lanes = []lane{{-rng, rng, -0.85, 0.85, xfRes.MagDB, true}}
	case 2:
		lanes = []lane{{-180, 180, -0.85, 0.85, xfRes.PhaseDeg, true}}
	case 3:
		lanes = []lane{{0, 1, -0.85, 0.85, xfRes.Coherence, false}}
	default:
		// MAGNITUDE on top, COHERENCE in a thin strip directly under it, PHASE
		// at the bottom.
		//
		// The order is not cosmetic. The panel is docked across the bottom of the
		// window by default and covers roughly its lower third, so a lane placed
		// there is a lane nobody sees — and with coherence last, the one reading
		// that says whether to believe the display was the one permanently
		// hidden behind the controls. Magnitude and coherence are read TOGETHER
		// (a dip with the coherence high is the system; the same dip with it
		// collapsed is the measurement giving up), so they are the two that stay
		// above the panel. Phase is the one you go looking for, and hiding the
		// panel or docking it elsewhere is how.
		lanes = []lane{
			{-rng, rng, 0.22, 0.88, xfRes.MagDB, true},
			{0, 1, 0.02, 0.17, xfRes.Coherence, false},
			{-180, 180, -0.88, -0.08, xfRes.PhaseDeg, true},
		}
	}

	o := 0
	seg := func(x0, y0, x1, y1 float32) {
		xfLine[o], xfLine[o+1], xfLine[o+2], xfLine[o+3] = x0, y0, x1, y1
		o += 4
	}
	xAt := func(i int) float32 { return float32(-0.92 + 1.84*float64(i)/float64(n-1)) }
	for _, ln := range lanes {
		for i := 1; i < n; i++ {
			// A segment is drawn only when BOTH of its ends are trustworthy.
			// Joining across an untrustworthy band would draw a line through
			// the one place the measurement said not to look.
			if ln.gate {
				if xfRes.Coherence[i-1] < minCoh || xfRes.Coherence[i] < minCoh {
					continue
				}
				if xfRes.RefDB[i-1] < -40 || xfRes.RefDB[i] < -40 {
					continue
				}
			}
			seg(xAt(i-1), xfY(ln.vals[i-1], ln.lo, ln.hi, ln.bandLo, ln.bandHi),
				xAt(i), xfY(ln.vals[i], ln.lo, ln.hi, ln.bandLo, ln.bandHi))
		}
	}
	verts := o / 2
	if verts == 0 {
		gl.Call("disable", glTypes.DepthTest)
		gl.Call("clearColor", 0, 0, 0, 0)
		gl.Call("clear", glTypes.ColorBufferBit)
		return
	}

	gl.Call("disable", glTypes.DepthTest)
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", glTypes.ColorBufferBit)
	gl.Call("useProgram", xyProgram)
	gl.Call("bindBuffer", glTypes.ArrayBuffer, xyBuf)
	js.CopyBytesToJS(xfU8, sliceToByteSlice(xfLine))
	gl.Call("bufferData", glTypes.ArrayBuffer, xfF32, glTypes.DynamicDraw)
	gl.Call("enableVertexAttribArray", xyAPos)
	gl.Call("vertexAttribPointer", xyAPos, 2, glTypes.Float, false, 0, 0)

	col := [3]float32{0.4, 1.0, 0.45}
	if phosphorActive() {
		p := phosphors[phosphorIdx]
		col = [3]float32{float32(p.tr), float32(p.tg), float32(p.tb)}
	}
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE"))
	gl.Call("uniform3f", xyUColor, col[0], col[1], col[2])
	dx := float32(1.2) / float32(width)
	dy := float32(1.2) / float32(height)
	for _, h := range [][3]float32{{dx, 0, 0.35}, {-dx, 0, 0.35}, {0, dy, 0.35}, {0, -dy, 0.35}} {
		gl.Call("uniform2f", xyUOffset, h[0], h[1])
		gl.Call("uniform1f", xyUAlpha, h[2])
		gl.Call("drawArrays", glTypes.Lines, 0, verts)
	}
	gl.Call("uniform2f", xyUOffset, 0, 0)
	gl.Call("uniform1f", xyUAlpha, 1)
	gl.Call("drawArrays", glTypes.Lines, 0, verts)
	gl.Call("disable", gl.Get("BLEND"))

	showTransferDelay()
}

// ── The delay readout ────────────────────────────────────────────────────

var (
	xfDelayEl js.Value
	xfDelayTx string
)

// showTransferDelay writes the fitted bulk delay, which is the number a
// system-tuning rig is actually reached for: the slope of the phase IS the
// offset between the two channels, and that is what gets dialled into a delay
// line.
func showTransferDelay() {
	s := "-- ms"
	if ms, ok := TransferDelayMS(xfRes, float64(xfCohF)/10); ok {
		s = formatLED(ms, 2, 2, true) + "ms"
	}
	if s == xfDelayTx {
		return
	}
	xfDelayTx = s
	if xfDelayEl.Truthy() {
		xfDelayEl.Set("textContent", s)
	}
}

// appendTransferReadout adds the delay cell to the mode's parameter grid.
func appendTransferReadout(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "dly")
	card.Call("appendChild", lbl)

	xfDelayEl = doc.Call("createElement", "span")
	xfDelayEl.Set("className", "led counter-led")
	xfDelayEl.Set("title", "Bulk delay between the two channels, fitted from the slope of the phase — "+
		"a pure delay is a phase that falls linearly with frequency, and the slope is the delay. "+
		"This is the number a system-tuning rig is reached for: it is what gets dialled into a delay "+
		"line to line a loudspeaker up with the rest of the system. Fitted only across bands the "+
		"stimulus actually reached and whose coherence clears the COH knob, and on the UNWRAPPED "+
		"phase — a real delay turns through 360° many times across the band, and a slope fitted to "+
		"the wrapped curve is a slope fitted to a sawtooth.")
	xfDelayTx = ""
	card.Call("appendChild", xfDelayEl)
	grid.Call("appendChild", card)
}
