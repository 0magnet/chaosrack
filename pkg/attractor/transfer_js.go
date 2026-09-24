//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
)

// The Transfer mode — magnitude, phase and coherence between two channels.
//
// The measurement is in pkg/acoustics and checked against a known gain,
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

// transferMode is the Transfer mode: the two channels' windows, the last
// result, the delay readout and its knobs.
type transferMode struct {
	cursor int
	bufL   []float32
	bufR   []float32
	fill   int
	nextMs float64
	accum  acoustics.TransferAccum
	res    acoustics.TransferResult

	// The knobs.
	swapF   float32 // 0 = left is the reference, 1 = right
	fracF   float32 // index into acoustics.RTAFractions; 2 is 1/6 octave
	avgF    float32 // windows in the average
	rangeF  float32 // dB either side of 0 on the magnitude curve
	cohF    float32 // minimum coherence, in tenths
	showF   float32 // which curves are drawn
	delayEl js.Value
	delayTx string
}

var xf = transferMode{
	cursor: tapUnjoined,
	fracF:  2,
	avgF:   24,
	rangeF: 40,
	cohF:   5,
}

// xfShowNames are the display's layouts, and xfShowRing what fits on the dial.
var (
	xfShowNames = []string{"mag + phase + coh", "magnitude only", "phase only", "coherence only"}
	xfShowRing  = []string{"all", "mag", "phz", "coh"}
)

func init() {
	registerGenerate("xfer", generateTransfer)
	attractorParams["xfer"] = []paramDef{
		{"xf-swap", "ref", &xf.swapF, 0, 0, 1, 1},
		{"xf-frac", "band", &xf.fracF, 2, 0, float32(len(acoustics.RTAFractions) - 1), 1},
		{"xf-avg", "avg", &xf.avgF, 24, float32(acoustics.TransferMinAvg), 128, 1},
		{"xf-range", "rnge", &xf.rangeF, 40, 6, 60, 2},
		{"xf-coh", "coh", &xf.cohF, 5, 0, 10, 1},
		{"xf-show", "show", &xf.showF, 0, 0, float32(len(xfShowNames) - 1), 1},
	}
}

// xfShowSel is the layout knob as an index, clamped — stereoAxisSel's argument,
// and its trap.
func (t *transferMode) showSel() int {
	v := t.showF
	if !(v > 0) {
		return 0
	}
	if last := float32(len(xfShowNames) - 1); v > last {
		return len(xfShowNames) - 1
	}
	return int(v + 0.5)
}

// xfFraction is the band-width knob as a 1/b, clamped.
func (t *transferMode) fraction() int {
	v := t.fracF
	if !(v > 0) {
		return acoustics.RTAFractions[0]
	}
	last := len(acoustics.RTAFractions) - 1
	if v > float32(last) {
		return acoustics.RTAFractions[last]
	}
	return acoustics.RTAFractions[int(v+0.5)]
}

// generateTransfer is the mode's frame.
func generateTransfer() {
	xf.analyze(frameNowMs)
	xf.drawTransfer()
}

// xfAnalyze accumulates windows into the average.
func (t *transferMode) analyze(nowMs float64) {
	if t.bufL == nil {
		t.bufL = make([]float32, xfFFT)
		t.bufR = make([]float32, xfFFT)
	}
	var sl, sr [4096]float32
	for {
		n := tap.readStereo(&t.cursor, sl[:], sr[:])
		if n <= 0 {
			break
		}
		if n >= xfFFT {
			copy(t.bufL, sl[n-xfFFT:n])
			copy(t.bufR, sr[n-xfFFT:n])
			t.fill = xfFFT
		} else {
			copy(t.bufL, t.bufL[n:])
			copy(t.bufR, t.bufR[n:])
			copy(t.bufL[xfFFT-n:], sl[:n])
			copy(t.bufR[xfFFT-n:], sr[:n])
			if t.fill += n; t.fill > xfFFT {
				t.fill = xfFFT
			}
		}
		if n < len(sl) {
			break
		}
	}
	if t.fill < xfFFT || nowMs < t.nextMs {
		return
	}
	t.nextMs = nowMs + xfPeriodMs
	ref, meas := t.bufL, t.bufR
	if t.swapF > 0.5 {
		ref, meas = t.bufR, t.bufL
	}
	t.accum.Add(ref, meas, acoustics.TransferWindowKind)
	// A rolling average: once it is full, start again rather than letting the
	// window stretch to the whole session. A system-tuning measurement has to
	// follow a knob being turned, and an average that never forgets cannot.
	if t.accum.Count() >= int(t.avgF) {
		t.res = t.accum.Result(takensSourceRate(), t.fraction())
		t.accum.Reset()
	} else if r := t.accum.Result(takensSourceRate(), t.fraction()); r.OK {
		t.res = r
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
func (t *transferMode) drawTransfer() {
	if !xy.ready {
		xy.initXY()
	}
	n := len(t.res.Bands)
	if !t.res.OK || n < 2 {
		glctx.GL.Call("disable", glctx.Types.DepthTest)
		glctx.GL.Call("clearColor", 0, 0, 0, 0)
		glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
		return
	}
	show := t.showSel()
	minCoh := float64(t.cohF) / 10
	rng := float64(t.rangeF)

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
		lanes = []lane{{-rng, rng, -0.85, 0.85, t.res.MagDB, true}}
	case 2:
		lanes = []lane{{-180, 180, -0.85, 0.85, t.res.PhaseDeg, true}}
	case 3:
		lanes = []lane{{0, 1, -0.85, 0.85, t.res.Coherence, false}}
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
			{-rng, rng, 0.22, 0.88, t.res.MagDB, true},
			{0, 1, 0.02, 0.17, t.res.Coherence, false},
			{-180, 180, -0.88, -0.08, t.res.PhaseDeg, true},
		}
	}

	// COLORED BY COHERENCE, which is the whole reason this display has a
	// color at all.
	//
	// The three curves are read together — a dip in the magnitude with the
	// coherence high is the system, and the same dip with it collapsed is the
	// measurement giving up — and reading them together means moving the eye
	// between two lanes and matching up frequencies by position. Coloring the
	// magnitude by the coherence at that frequency puts the second reading ON
	// the first, which is how a system-tuning rig shows it and why the coherence
	// lane is a confirmation rather than the only place the reading exists.
	//
	// The colormap is the Colors module's own, so the value paints the same
	// color here as it does in the spectrogram and on the trail. With a
	// swatch-mixing palette selected it falls back to the single trace color,
	// which is what this drew before.
	pal, colored := analyzerPalette()
	flat := analyzerTraceColor()
	// ONE rule for all three lanes: the color at a band is its coherence.
	// The coherence lane is then a color ramp of exactly what it plots, which
	// makes it the key to the other two rather than a fourth thing to learn.
	colourFor := func(i int) [3]float32 {
		if !colored {
			return flat
		}
		return analyzerColorAt(pal, t.res.Coherence[i])
	}

	vc.fit(n * 3 * 2)
	v := 0
	xAt := func(i int) float32 { return float32(-0.92 + 1.84*float64(i)/float64(n-1)) }
	for _, ln := range lanes {
		for i := 1; i < n; i++ {
			// A segment is drawn only when BOTH of its ends are trustworthy.
			// Joining across an untrustworthy band would draw a line through
			// the one place the measurement said not to look.
			if ln.gate {
				if t.res.Coherence[i-1] < minCoh || t.res.Coherence[i] < minCoh {
					continue
				}
				if t.res.RefDB[i-1] < -40 || t.res.RefDB[i] < -40 {
					continue
				}
			}
			vc.put(v, xAt(i-1), xfY(ln.vals[i-1], ln.lo, ln.hi, ln.bandLo, ln.bandHi), colourFor(i-1))
			vc.put(v+1, xAt(i), xfY(ln.vals[i], ln.lo, ln.hi, ln.bandLo, ln.bandHi), colourFor(i))
			v += 2
		}
	}
	if v == 0 {
		glctx.GL.Call("disable", glctx.Types.DepthTest)
		glctx.GL.Call("clearColor", 0, 0, 0, 0)
		glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
		return
	}

	vc.initVColor()
	glctx.GL.Call("disable", glctx.Types.DepthTest)
	glctx.GL.Call("clearColor", 0, 0, 0, 0)
	glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
	glctx.GL.Call("enable", glctx.GL.Get("BLEND"))
	glctx.GL.Call("blendFunc", glctx.GL.Get("SRC_ALPHA"), glctx.GL.Get("ONE"))
	vc.upload(v)
	dx := float32(1.2) / float32(gpu.width)
	dy := float32(1.2) / float32(gpu.height)
	for _, h := range [][3]float32{{dx, 0, 0.35}, {-dx, 0, 0.35}, {0, dy, 0.35}, {0, -dy, 0.35}} {
		vc.span(glctx.Types.Lines, 0, v, h[2], h[0], h[1])
	}
	vc.span(glctx.Types.Lines, 0, v, 1, 0, 0)
	vc.done()
	glctx.GL.Call("disable", glctx.GL.Get("BLEND"))

	t.showTransferDelay()
}

// ── The delay readout ────────────────────────────────────────────────────

// showTransferDelay writes the fitted bulk delay, which is the number a
// system-tuning rig is actually reached for: the slope of the phase IS the
// offset between the two channels, and that is what gets dialed into a delay
// line.
func (t *transferMode) showTransferDelay() {
	s := "-- ms"
	if ms, ok := acoustics.TransferDelayMS(t.res, float64(t.cohF)/10); ok {
		s = led.Format(ms, 2, 2, true) + "ms"
	}
	if s == t.delayTx {
		return
	}
	t.delayTx = s
	if t.delayEl.Truthy() {
		t.delayEl.Set("textContent", s)
	}
}

// appendTransferReadout adds the delay cell to the mode's parameter grid.
func (t *transferMode) appendTransferReadout(grid js.Value) {
	card, top := newPunitCard("dly")

	t.delayEl = dom.Doc.Call("createElement", "span")
	t.delayEl.Set("className", "led counter-led")
	t.delayEl.Set("title", "Bulk delay between the two channels, fitted from the slope of the phase — "+
		"a pure delay is a phase that falls linearly with frequency, and the slope is the delay. "+
		"This is the number a system-tuning rig is reached for: it is what gets dialed into a delay "+
		"line to line a loudspeaker up with the rest of the system. Fitted only across bands the "+
		"stimulus actually reached and whose coherence clears the COH knob, and on the UNWRAPPED "+
		"phase — a real delay turns through 360° many times across the band, and a slope fitted to "+
		"the wrapped curve is a slope fitted to a sawtooth.")
	t.delayTx = ""
	top.Call("appendChild", t.delayEl)
	grid.Call("appendChild", card)
}
