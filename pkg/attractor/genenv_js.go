//go:build js && wasm

package attractor

import (
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
	dcy := doc.Call("getElementById", "gen-env-dcy")
	mode := doc.Call("getElementById", "gen-env-mode")
	astack := doc.Call("getElementById", "gen-env-astack")
	dstack := doc.Call("getElementById", "gen-env-dstack")
	mstack := doc.Call("getElementById", "gen-env-mstack")
	if !atk.Truthy() || !astack.Truthy() {
		return
	}
	// Both knobs go through the descriptor path: it owns the LED format, typed
	// entry, wheel nudge and reset, which the local wire helper below used to
	// duplicate for the two of them.
	//
	// LEDStep 10 rather than the Step of 1 is what keeps these reading whole
	// milliseconds. ledDecimals works from step × fineRatio, so a step of 1
	// asks for one decimal — right for a knob whose fine disc trims between
	// steps, and noise on a value that is only ever a whole number of
	// milliseconds. LEDStep is the field for saying so, and this preserves
	// exactly what the module showed before.
	adoptDescControl(ControlDesc{
		ID: "gen-env-atk", Label: "atk", Min: 1, Max: 2000, Step: 1, Def: 10,
		LEDID: "gen-env-atk-led", ResetID: "rst-gen-env-atk", LEDStep: 10,
	})
	adoptDescControl(ControlDesc{
		ID: "gen-env-dcy", Label: "dcy", Min: 1, Max: 5000, Step: 1, Def: 300,
		LEDID: "gen-env-dcy-led", ResetID: "rst-gen-env-dcy", LEDStep: 10,
	})
	astack.Call("appendChild", makeKnob(atk, js.Undefined(), true, false, true))
	dstack.Call("appendChild", makeKnob(dcy, js.Undefined(), true, false, true))
	mstack.Call("appendChild", singleSelectorKnob(mode, []string{"off", "rpt"}, 50))
	// The mode ring, like the two knobs beside it. genEnvTick reads the select
	// every frame rather than a cached mode, so there is no SelectApply to
	// write: putting the value back IS applying it.
	adoptDescControl(ControlDesc{
		ID: "gen-env-mode", Label: "mode", IsSelect: true, SelectDef: "off",
		ResetID: "rst-gen-env-mode",
	})
}
