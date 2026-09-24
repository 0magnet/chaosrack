//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/meters"
)

// The Loudness module — LUFS, loudness range and true peak.
//
// The meter is in loudness.go, untagged and checked against BS.1770's own
// published coefficients and against the loudness equation. This is the panel
// around it: where the samples come from, when the readouts are written, and
// what the reset button means.
//
// ── IT READS BOTH CHANNELS, CONTINUOUSLY ─────────────────────────────────
//
// Loudness is a property of the program rather than of a window, so unlike
// every other measurement here this one cannot run on a timer and look at the
// newest window: the integrated reading is over EVERYTHING since the reset, and
// a block missed is a block missing from the answer. So it drains the tap every
// frame and folds whatever arrived into the meter, which is cheap — two biquads
// a sample — and the readouts are what happen on a timer.
//
// The true peak is measured on the raw buffers as they pass, with the
// oversampling TruePeak does; the meter's own peak is the sample peak, and the
// higher of the two is what is shown.

// loudness is the Loudness module: the meter, its refresh clock and its LEDs.
type loudness struct {
	// periodMs is how often the readouts latch, and it is a DISPLAY rate
	// only: the meter itself integrates every sample that arrives, because an
	// integrated loudness with a block missing is a block missing from the
	// answer. Nothing about the measurement changes when this moves — only how
	// often you are shown it. On the panel as RATE; see meterswitch_js.go.
	periodMs        float64
	cursor          int
	meter           *meters.LoudnessMeter
	nextMs          float64
	res             meters.LoudnessResult
	target          float32
	mEl, sEl, iEl   js.Value
	lraEl, tpEl, dl js.Value
}

var lufs = loudness{
	periodMs: 200,
	cursor:   tapUnjoined,
	target:   -23,
}

// lufsTick drains the tap into the meter and updates the readouts on its own
// clock. Called once a frame; does nothing while the module is off screen.
func (l *loudness) tick(nowMs float64) {
	// Not merely "not display:none" — actually on screen. See
	// moduleOnScreen: this module's DSP and readouts are most of what the
	// panel costs per frame, and the drawer usually has it scrolled away.
	if !moduleOnScreen("lufs-module") {
		return
	}
	sr := takensSourceRate()
	if l.meter == nil {
		l.meter = meters.NewLoudnessMeter(sr)
	} else if l.meter.SampleRate() != sr {
		// A change of source rate retunes the weighting — and drops the
		// measurement with it, because an integrated loudness averaged across
		// two different filters describes neither.
		l.meter.Reset(sr)
	}
	var sl, sr2 [4096]float32
	for {
		n := tap.readStereo(&l.cursor, sl[:], sr2[:])
		if n <= 0 {
			break
		}
		l.meter.Add(sl[:n], sr2[:n])
		// The true peak on the raw buffer, oversampled. Done here rather than
		// inside the meter because it needs the samples either side of each
		// point and the meter is a per-sample loop.
		if p := meters.TruePeak(sl[:n]); p > 0 {
			l.meter.SetTruePeak(p)
		}
		if p := meters.TruePeak(sr2[:n]); p > 0 {
			l.meter.SetTruePeak(p)
		}
		if n < len(sl) {
			break
		}
	}
	if nowMs < l.nextMs {
		return
	}
	l.nextMs = nowMs + l.periodMs
	l.res = l.meter.Result()
	l.showLoudness()
}

// showLoudness writes the readouts.
func (l *loudness) showLoudness() {
	set := func(key string, el js.Value, v float64, ok bool) {
		if !ok || v <= meters.LoudnessFloor {
			readouts.Set(key, el, "  --.-")
			return
		}
		readouts.Set(key, el, led.Format(v, 3, 1, true))
	}
	set("lufs-m", l.mEl, l.res.Momentary, l.res.Momentary > meters.LoudnessFloor)
	set("lufs-s", l.sEl, l.res.ShortTerm, l.res.ShortTerm > meters.LoudnessFloor)
	set("lufs-i", l.iEl, l.res.Integrated, l.res.OK)
	if l.res.OK {
		readouts.Set("lufs-lra", l.lraEl, led.Format(l.res.LRA, 3, 1, false))
	} else {
		readouts.Set("lufs-lra", l.lraEl, "  --.-")
	}
	set("lufs-tp", l.tpEl, l.res.TruePeak, l.res.TruePeak > meters.LoudnessFloor)
	// Through lufsDistanceToTarget rather than subtracting here: it is the
	// same arithmetic plus the floor guard, and an integrated reading that
	// has not risen off the floor is not a distance from anything.
	d := lufsDistanceToTarget(l.res.Integrated, float64(l.target))
	if l.res.OK && !math.IsNaN(d) {
		readouts.Set("lufs-d", l.dl, led.Format(d, 3, 1, true))
	} else {
		readouts.Set("lufs-d", l.dl, "  --.-")
	}
}

// wireLoudnessModule finds the readouts and wires the target knob and the
// reset. Called once from Run.
func (l *loudness) wireLoudnessModule() {
	l.mEl = dom.Doc.Call("getElementById", "lufs-m-led")
	l.sEl = dom.Doc.Call("getElementById", "lufs-s-led")
	l.iEl = dom.Doc.Call("getElementById", "lufs-i-led")
	l.lraEl = dom.Doc.Call("getElementById", "lufs-lra-led")
	l.tpEl = dom.Doc.Call("getElementById", "lufs-tp-led")
	l.dl = dom.Doc.Call("getElementById", "lufs-delta-led")
	tgt := dom.Doc.Call("getElementById", "lufs-target")
	rst := dom.Doc.Call("getElementById", "lufs-reset")
	if !tgt.Truthy() {
		return
	}
	// The target knob: the descriptor owns its LED, typed entry, wheel and
	// reset. Signed, because a loudness target is always negative and the sign
	// is not decoration. LEDStep 10 keeps it at whole LU.
	adoptDescControl(ControlDesc{
		ID: "lufs-target", Label: "tgt", Min: -40, Max: 0, Step: 1, Def: -23,
		Signed: true, LEDID: "lufs-target-led", ResetID: "rst-lufs-target", LEDStep: 10,
		Apply: func(v float64) {
			l.target = float32(v)
			l.showLoudness()
		},
	})
	if rst.Truthy() {
		rst.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			if l.meter != nil {
				l.meter.Reset(l.meter.SampleRate())
			}
			l.res = meters.LoudnessResult{
				Momentary: meters.LoudnessFloor, ShortTerm: meters.LoudnessFloor,
				Integrated: meters.LoudnessFloor, TruePeak: meters.LoudnessFloor,
			}
			l.showLoudness()
			return nil
		}))
	}
	l.showLoudness()
}

// lufsDistanceToTarget is how far a reading is from a target, in LU. Positive
// is too loud, which is the direction that gets a delivery rejected.
func lufsDistanceToTarget(integrated, target float64) float64 {
	if integrated <= meters.LoudnessFloor {
		return math.NaN()
	}
	return integrated - target
}
