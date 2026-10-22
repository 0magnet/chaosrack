//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"
)

// The Transfer mode — magnitude, phase and coherence between two channels.
//
// The measurement is in pkg/acoustics and checked against a known gain,
// a known delay and two independent noises, and the average and its curves
// are Transfer (transfermeter.go), which a native rack draws too. This is the
// page's side: reading the tap, drawing the lines, and the delay readout.
//
// ── WHAT GOES IN WHICH CHANNEL ───────────────────────────────────────────
//
// REFERENCE is what went out and MEASUREMENT what came back. The usual rig is
// the signal generator into one input of the interface and a microphone into
// the other, or a loopback of the send against the return. The two are read
// through ONE tap cursor (tap.readStereo) so they arrive sample-aligned, which
// matters more here than anywhere else in the app: a one-sample slip between
// the channels IS a delay, and this display's whole subject is delay.
//
// The knob swaps which is which, because half the time the cabling makes it the
// other way round and rewiring to satisfy a display is absurd.

// transferMode is the Transfer mode: the analyzer, the two channels' windows
// of the tap, and the delay readout.
type transferMode struct {
	Transfer
	cursor  int
	bufL    []float32
	bufR    []float32
	fill    int
	delayEl js.Value
	delayTx string
}

var xf = transferMode{cursor: tapUnjoined, Transfer: *NewTransfer()}

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

// generateTransfer is the mode's frame.
func generateTransfer() {
	xf.analyze(frameNowMs)
	xf.drawTransfer()
}

// analyze drains the tap and folds a window into the average when its period
// is up.
func (t *transferMode) analyze(nowMs float64) {
	if t.bufL == nil {
		t.bufL = make([]float32, XferFFT)
		t.bufR = make([]float32, XferFFT)
	}
	var sl, sr [4096]float32
	for {
		n := tap.readStereo(&t.cursor, sl[:], sr[:])
		if n <= 0 {
			break
		}
		if n >= XferFFT {
			copy(t.bufL, sl[n-XferFFT:n])
			copy(t.bufR, sr[n-XferFFT:n])
			t.fill = XferFFT
		} else {
			copy(t.bufL, t.bufL[n:])
			copy(t.bufR, t.bufR[n:])
			copy(t.bufL[XferFFT-n:], sl[:n])
			copy(t.bufR[XferFFT-n:], sr[:n])
			if t.fill += n; t.fill > XferFFT {
				t.fill = XferFFT
			}
		}
		if n < len(sl) {
			break
		}
	}
	if t.fill < XferFFT || !t.Due(nowMs) {
		return
	}
	t.Analyze(t.bufL, t.bufR, takensSourceRate())
}

// drawTransfer draws the curves.
func (t *transferMode) drawTransfer() {
	if !xy.ready {
		xy.initXY()
	}
	lines := t.Lines(analyzerColors())
	glctx.GL.Call("disable", glctx.Types.DepthTest)
	clearScreen()
	if len(lines) == 0 {
		return
	}
	vc.initVColor()
	vc.fit(len(lines))
	for i, p := range lines {
		vc.put(i, p.X, p.Y, p.C)
	}
	glctx.GL.Call("enable", glctx.GL.Get("BLEND"))
	glctx.GL.Call("blendFunc", glctx.GL.Get("SRC_ALPHA"), glctx.GL.Get("ONE"))
	vc.upload(len(lines))
	dx := float32(1.2) / float32(gpu.width)
	dy := float32(1.2) / float32(gpu.height)
	for _, h := range [][3]float32{{dx, 0, 0.35}, {-dx, 0, 0.35}, {0, dy, 0.35}, {0, -dy, 0.35}} {
		vc.span(glctx.Types.Lines, 0, len(lines), h[2], h[0], h[1])
	}
	vc.span(glctx.Types.Lines, 0, len(lines), 1, 0, 0)
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
	s := t.DelayText()
	if s == t.delayTx {
		return
	}
	t.delayTx = s
	if t.delayEl.Truthy() {
		setDotText(t.delayEl, s)
	}
}

// appendTransferReadout adds the delay readout to the model's readout line
// (liveReadoutHost).
func (t *transferMode) appendTransferReadout(host js.Value) {
	t.delayTx = ""
	t.delayEl = liveReadout(host, "dly", dispFullChars, "-- ms", doc("ro.dly"))
}
