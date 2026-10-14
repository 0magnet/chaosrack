package attractor

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/led"
)

// The transfer function's measurement and its picture, shared by the page
// (transfer_js.go) and a native rack: the rolling average, the lanes, and the
// curves colored by coherence. Where the samples come from and how the lines
// reach the screen are each side's own.

const (
	// XferFFT is the analysis length. 8192 at 48 kHz is a 5.9 Hz bin and a
	// 171 ms window — long enough to hold a room's early reflections, which is
	// what a response measurement has to span, and short enough that a source
	// moving during it is not the thing being measured.
	XferFFT = 8192

	// xfPeriodMs is how often a window is folded into the average.
	xfPeriodMs = 120
)

// Transfer is a transfer-function analyzer: the average, the last result,
// and the knobs.
type Transfer struct {
	nextMs float64
	accum  acoustics.TransferAccum
	res    acoustics.TransferResult
	lines  []PlotVertex

	// The knobs.
	swapF  float32 // 0 = left is the reference, 1 = right
	fracF  float32 // index into acoustics.RTAFractions; 2 is 1/6 octave
	avgF   float32 // windows in the average
	rangeF float32 // dB either side of 0 on the magnitude curve
	cohF   float32 // minimum coherence, in tenths
	showF  float32 // which curves are drawn
}

// NewTransfer is a transfer function at the page's defaults.
func NewTransfer() *Transfer {
	return &Transfer{fracF: 2, avgF: 24, rangeF: 40, cohF: 5}
}

// TransferKnobs are the transfer function's knobs, by the page's ids.
var TransferKnobs = []string{"xf-swap", "xf-frac", "xf-avg", "xf-range", "xf-coh", "xf-show"}

func (t *Transfer) field(id string) *float32 {
	switch id {
	case "xf-swap":
		return &t.swapF
	case "xf-frac":
		return &t.fracF
	case "xf-avg":
		return &t.avgF
	case "xf-range":
		return &t.rangeF
	case "xf-coh":
		return &t.cohF
	case "xf-show":
		return &t.showF
	}
	return nil
}

// Set turns knob id to v, reporting whether the transfer function has it.
func (t *Transfer) Set(id string, v float32) bool {
	p := t.field(id)
	if p == nil {
		return false
	}
	*p = v
	return true
}

// xfShowLayouts is how many layouts the SHOW knob has (xfShowNames).
const xfShowLayouts = 4

// showSel is the layout knob as an index, clamped — stereoAxisSel's argument,
// and its trap.
func (t *Transfer) showSel() int {
	v := t.showF
	if !(v > 0) {
		return 0
	}
	if last := float32(xfShowLayouts - 1); v > last {
		return xfShowLayouts - 1
	}
	return int(v + 0.5)
}

// fraction is the band-width knob as a 1/b, clamped.
func (t *Transfer) fraction() int {
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

// Due reports whether a window is due at nowMs, and if so starts the next
// period.
func (t *Transfer) Due(nowMs float64) bool {
	if nowMs < t.nextMs {
		return false
	}
	t.nextMs = nowMs + xfPeriodMs
	return true
}

// Analyze folds a window of left and right, the newest XferFFT samples of
// each at rate sr, into the average.
func (t *Transfer) Analyze(l, r []float32, sr int) {
	ref, meas := l, r
	if t.swapF > 0.5 {
		ref, meas = r, l
	}
	t.accum.Add(ref, meas, acoustics.TransferWindowKind)
	// A rolling average: once it is full, start again rather than letting the
	// window stretch to the whole session. A system-tuning measurement has to
	// follow a knob being turned, and an average that never forgets cannot.
	if t.accum.Count() >= int(t.avgF) {
		t.res = t.accum.Result(sr, t.fraction())
		t.accum.Reset()
	} else if res := t.accum.Result(sr, t.fraction()); res.OK {
		t.res = res
	}
}

// DelayText is the delay readout: milliseconds with a sign, as a
// character display writes them, or dashes where the coherence will not let
// the delay be read.
func (t *Transfer) DelayText() string {
	if ms, ok := t.DelayMS(); ok {
		return strings.Replace(led.Format(ms, 2, 2, true), led.Blank, "+", 1) + "ms" // a character display has a plus
	}
	return "-- ms"
}

// DelayMS is the fitted bulk delay between the channels, where the coherence
// lets it be read.
func (t *Transfer) DelayMS() (float64, bool) {
	return acoustics.TransferDelayMS(t.res, float64(t.cohF)/10)
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

// Lines are the curves the SHOW knob picks, colored by col, or none before
// there is a result. They are the transfer function's own until the next call.
func (t *Transfer) Lines(col AnalyzerColors) []PlotVertex {
	t.lines = t.lines[:0]
	n := len(t.res.Bands)
	if !t.res.OK || n < 2 {
		return t.lines
	}
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
	switch t.showSel() {
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
	// The three curves are read together, and reading them together means
	// moving the eye between two lanes and matching up frequencies by
	// position. Coloring the magnitude by the coherence at that frequency puts
	// the second reading ON the first, which is how a system-tuning rig shows
	// it and why the coherence lane is a confirmation rather than the only
	// place the reading exists.
	//
	// ONE rule for all three lanes: the color at a band is its coherence. The
	// coherence lane is then a color ramp of exactly what it plots, which
	// makes it the key to the other two rather than a fourth thing to learn.
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
			t.lines = append(t.lines,
				PlotVertex{xAt(i - 1), xfY(ln.vals[i-1], ln.lo, ln.hi, ln.bandLo, ln.bandHi), col.At(t.res.Coherence[i-1])},
				PlotVertex{xAt(i), xfY(ln.vals[i], ln.lo, ln.hi, ln.bandLo, ln.bandHi), col.At(t.res.Coherence[i])})
		}
	}
	return t.lines
}
