//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"syscall/js"

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

// lufsPeriodMs is how often the readouts latch, and it is a DISPLAY rate
// only: the meter itself integrates every sample that arrives, because an
// integrated loudness with a block missing is a block missing from the
// answer. Nothing about the measurement changes when this moves — only how
// often you are shown it. On the panel as RATE; see meterswitch_js.go.
var lufsPeriodMs float64 = 200

var (
	lufsCursor = tapUnjoined
	lufsMeter  *meters.LoudnessMeter
	lufsNextMs float64
	lufsRes    meters.LoudnessResult
	lufsTarget float32 = -23

	lufsMEl, lufsSEl, lufsIEl   js.Value
	lufsLRAEl, lufsTPEl, lufsDl js.Value
)

// lufsTick drains the tap into the meter and updates the readouts on its own
// clock. Called once a frame; does nothing while the module is off screen.
func lufsTick(nowMs float64) {
	// Not merely "not display:none" — actually on screen. See
	// moduleOnScreen: this module's DSP and readouts are most of what the
	// panel costs per frame, and the drawer usually has it scrolled away.
	if !moduleOnScreen("lufs-module") {
		return
	}
	sr := takensSourceRate()
	if lufsMeter == nil {
		lufsMeter = meters.NewLoudnessMeter(sr)
	} else if lufsMeter.SampleRate() != sr {
		// A change of source rate retunes the weighting — and drops the
		// measurement with it, because an integrated loudness averaged across
		// two different filters describes neither.
		lufsMeter.Reset(sr)
	}
	var sl, sr2 [4096]float32
	for {
		n := tapReadStereo(&lufsCursor, sl[:], sr2[:])
		if n <= 0 {
			break
		}
		lufsMeter.Add(sl[:n], sr2[:n])
		// The true peak on the raw buffer, oversampled. Done here rather than
		// inside the meter because it needs the samples either side of each
		// point and the meter is a per-sample loop.
		if p := meters.TruePeak(sl[:n]); p > 0 {
			lufsMeter.SetTruePeak(p)
		}
		if p := meters.TruePeak(sr2[:n]); p > 0 {
			lufsMeter.SetTruePeak(p)
		}
		if n < len(sl) {
			break
		}
	}
	if nowMs < lufsNextMs {
		return
	}
	lufsNextMs = nowMs + lufsPeriodMs
	lufsRes = lufsMeter.Result()
	showLoudness()
}

// showLoudness writes the readouts.
func showLoudness() {
	set := func(key string, el js.Value, v float64, ok bool) {
		if !ok || v <= meters.LoudnessFloor {
			setLEDText(key, el, "  --.-")
			return
		}
		setLEDText(key, el, formatLED(v, 3, 1, true))
	}
	set("lufs-m", lufsMEl, lufsRes.Momentary, lufsRes.Momentary > meters.LoudnessFloor)
	set("lufs-s", lufsSEl, lufsRes.ShortTerm, lufsRes.ShortTerm > meters.LoudnessFloor)
	set("lufs-i", lufsIEl, lufsRes.Integrated, lufsRes.OK)
	if lufsRes.OK {
		setLEDText("lufs-lra", lufsLRAEl, formatLED(lufsRes.LRA, 3, 1, false))
	} else {
		setLEDText("lufs-lra", lufsLRAEl, "  --.-")
	}
	set("lufs-tp", lufsTPEl, lufsRes.TruePeak, lufsRes.TruePeak > meters.LoudnessFloor)
	// Through lufsDistanceToTarget rather than subtracting here: it is the
	// same arithmetic plus the floor guard, and an integrated reading that
	// has not risen off the floor is not a distance from anything.
	d := lufsDistanceToTarget(lufsRes.Integrated, float64(lufsTarget))
	if lufsRes.OK && !math.IsNaN(d) {
		setLEDText("lufs-d", lufsDl, formatLED(d, 3, 1, true))
	} else {
		setLEDText("lufs-d", lufsDl, "  --.-")
	}
}

// wireLoudnessModule finds the readouts and wires the target knob and the
// reset. Called once from Run.
func wireLoudnessModule() {
	lufsMEl = dom.Doc.Call("getElementById", "lufs-m-led")
	lufsSEl = dom.Doc.Call("getElementById", "lufs-s-led")
	lufsIEl = dom.Doc.Call("getElementById", "lufs-i-led")
	lufsLRAEl = dom.Doc.Call("getElementById", "lufs-lra-led")
	lufsTPEl = dom.Doc.Call("getElementById", "lufs-tp-led")
	lufsDl = dom.Doc.Call("getElementById", "lufs-delta-led")
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
			lufsTarget = float32(v)
			showLoudness()
		},
	})
	if rst.Truthy() {
		rst.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			if lufsMeter != nil {
				lufsMeter.Reset(lufsMeter.SampleRate())
			}
			lufsRes = meters.LoudnessResult{
				Momentary: meters.LoudnessFloor, ShortTerm: meters.LoudnessFloor,
				Integrated: meters.LoudnessFloor, TruePeak: meters.LoudnessFloor,
			}
			showLoudness()
			return nil
		}))
	}
	showLoudness()
}

// lufsDistanceToTarget is how far a reading is from a target, in LU. Positive
// is too loud, which is the direction that gets a delivery rejected.
func lufsDistanceToTarget(integrated, target float64) float64 {
	if integrated <= meters.LoudnessFloor {
		return math.NaN()
	}
	return integrated - target
}
