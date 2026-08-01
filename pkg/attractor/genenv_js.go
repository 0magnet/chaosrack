//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// The Envelope module — the Digital Complex Sound Generator's shaper,
// applied to the signal generator's SPEAKER path (per-osc gains → pans →
// envelope gain → destination). In RPT mode the envelope cycles
// attack → decay continuously, a shaped tremolo over whatever Gen X/Y/Z
// are routed out; paired with the noise waveform this is the classic
// chuff-chuff / siren / steam-train territory the SN76477 was sold on.
// Analysis paths (scope, spectrogram, features) stay unshaped so the
// visuals don't pump with the tremolo.

var (
	genEnvPhase float64 // seconds into the current attack+decay cycle
	genEnvLast  float64 // frameNowMs at the previous tick
)

// genEnvTick runs every frame from the render loop and steers the shaper
// gain along the attack/decay ramps (smoothed by setTargetAtTime so the
// 60 Hz stepping never zippers).
func genEnvTick() {
	if !genRunning || !genEnvGain.Truthy() {
		return
	}
	mode := "off"
	if m := doc.Call("getElementById", "gen-env-mode"); m.Truthy() {
		mode = m.Get("value").String()
	}
	now := frameNowMs
	dt := (now - genEnvLast) / 1000
	genEnvLast = now
	if dt < 0 || dt > 0.25 { // first frame / tab was parked
		dt = 0
	}
	g := genEnvGain.Get("gain")
	ctxNow := genCtx.Get("currentTime").Float()
	if mode != "rpt" {
		genEnvPhase = 0
		g.Call("setTargetAtTime", 1, ctxNow, 0.02)
		return
	}
	atk := fgFloat(doc.Call("getElementById", "gen-env-atk")) / 1000
	dcy := fgFloat(doc.Call("getElementById", "gen-env-dcy")) / 1000
	if atk < 0.001 {
		atk = 0.001
	}
	if dcy < 0.001 {
		dcy = 0.001
	}
	genEnvPhase += dt
	period := atk + dcy
	for genEnvPhase >= period {
		genEnvPhase -= period
	}
	v := 0.0
	if genEnvPhase < atk {
		v = genEnvPhase / atk
	} else {
		v = 1 - (genEnvPhase-atk)/dcy
	}
	g.Call("setTargetAtTime", v, ctxNow, 0.02)
}

// buildEnvModule wires the Envelope module's attack/decay knobs and mode
// switch. Called once from Run.
func buildEnvModule() {
	atk := doc.Call("getElementById", "gen-env-atk")
	atkLED := doc.Call("getElementById", "gen-env-atk-led")
	dcy := doc.Call("getElementById", "gen-env-dcy")
	dcyLED := doc.Call("getElementById", "gen-env-dcy-led")
	mode := doc.Call("getElementById", "gen-env-mode")
	astack := doc.Call("getElementById", "gen-env-astack")
	dstack := doc.Call("getElementById", "gen-env-dstack")
	mstack := doc.Call("getElementById", "gen-env-mstack")
	if !atk.Truthy() || !astack.Truthy() {
		return
	}
	wire := func(sl, led js.Value, max float64) {
		led.Set("value", formatLED(fgFloat(sl), intDigits(max), 0, false))
		sizeLEDField(led, 1, max, 0, false)
		sl.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			led.Set("value", formatLED(fgFloat(sl), intDigits(max), 0, false))
			return nil
		}))
		led.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			if v, err := strconv.ParseFloat(led.Get("value").String(), 64); err == nil {
				sl.Set("value", strconv.FormatFloat(v, 'f', 0, 64))
				sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
			return nil
		}))
	}
	wire(atk, atkLED, 2000)
	wire(dcy, dcyLED, 5000)
	astack.Call("appendChild", makeKnob(atk, js.Undefined(), true, false, true))
	dstack.Call("appendChild", makeKnob(dcy, js.Undefined(), true, false, true))
	mstack.Call("appendChild", singleSelectorKnob(mode, []string{"off", "rpt"}, 50))
}
