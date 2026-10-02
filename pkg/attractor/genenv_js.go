//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/dom"
)

// Each generator's envelope — the Complex Sound Generator's shaper, one per
// oscillator and folded into its level cell rather than a module of its own:
// ATTACK and DECAY are two rings round the LEVEL knob, and the env button
// beside them repeats attack → decay continuously, a shaped tremolo; dark,
// the level is steady. Paired with the noise waveform this is the classic
// chuff-chuff / siren / steam-train territory the SN76477 was sold on.
//
// It shapes the generator wherever it goes, the rack's signal as well as the
// speakers, as a capture of the speakers would: audiosrc.FuncGen steps it by
// the sample and genEnvTick drives the speakers' gain by the audio clock,
// both from audiosrc.EnvAt.

// genEnvTick runs every frame from the render loop and steers each heard
// generator's envelope gain along its ramps (smoothed by setTargetAtTime so
// the 60 Hz stepping never zippers).
func genEnvTick() {
	if !gen.running {
		return
	}
	now := gen.ctx.Get("currentTime").Float()
	for i, o := range genOscs {
		if !gen.env[i].Truthy() {
			continue
		}
		v := 1.0
		if on, atk, dcy := genEnvOf(o); on {
			v = audiosrc.EnvAt(now, atk, dcy)
		}
		gen.env[i].Get("gain").Call("setTargetAtTime", v, now, 0.005)
	}
}

// genEnvOf reads a generator's envelope off its controls: whether it
// repeats, and its attack and decay in seconds.
func genEnvOf(o genOscSpec) (on bool, atk, dcy float64) {
	if m := dom.Doc.Call("getElementById", o.id+"-env"); m.Truthy() {
		on = m.Get("value").String() == "rpt"
	}
	atk = fgFloat(dom.Doc.Call("getElementById", o.id+"-atk")) / 1000
	dcy = fgFloat(dom.Doc.Call("getElementById", o.id+"-dcy")) / 1000
	return on, atk, dcy
}

// genEnvApply pushes a generator's envelope to the rack's signal.
func genEnvApply(o genOscSpec) {
	on, atk, dcy := genEnvOf(o)
	aud.fg().SetEnv(o.idx, on, atk, dcy)
}

// genLevelStack is a generator's level cell: the LEVEL knob, and under it
// the envelope's ATTACK and DECAY on mini knobs
// (minis_js.go). They were rings round the level on one shaft, and three
// rings on a knob a hand's width across could not be told apart.
//
// The cell has one display. It reads the level, and while ATTACK or DECAY
// is being turned it reads that instead, its legend saying which.
func genLevelStack(o genOscSpec, lvl js.Value) js.Value {
	atk := dom.Doc.Call("getElementById", o.id+"-atk")
	dcy := dom.Doc.Call("getElementById", o.id+"-dcy")
	knob := makeKnob(lvl, js.Undefined(), true, false, true)
	col := miniRow(miniKnob(atk, "A", false, true), miniKnob(dcy, "D", false, true))
	if cell := lvl.Call("closest", ".pcell"); cell.Truthy() {
		cell.Call("appendChild", col)
	}
	loan := lendReadout(col, dom.Doc.Call("getElementById", o.id+"-lvl-led"), func() {
		// The level's own control rewrites its display.
		dom.Fire(lvl, "input")
	})
	ms := func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
	adoptDescControl(ControlDesc{
		ID: o.id + "-atk", Label: "atk", Min: 1, Max: 2000, Step: 1, Def: 10,
		PermaKey: o.key("a"),
		ResetID:  "rst-" + o.id + "-lvl",
		Apply: func(v float64) {
			genEnvApply(o)
			loan.show("atk", ms(v))
		},
	})
	adoptDescControl(ControlDesc{
		ID: o.id + "-dcy", Label: "dcy", Min: 1, Max: 5000, Step: 1, Def: 300,
		PermaKey: o.key("d"),
		ResetID:  "rst-" + o.id + "-lvl",
		Apply: func(v float64) {
			genEnvApply(o)
			loan.show("dcy", ms(v))
		},
	})
	adoptDescControl(ControlDesc{
		ID: o.id + "-env", Label: "env", IsSelect: true, SelectDef: "off",
		PermaKey: o.key("e"),
		ResetID:  "rst-" + o.id + "-lvl",
		SelectApply: func(string) {
			genEnvApply(o)
			lightTrios(o.id + "-env")
		},
	})
	genEnvApply(o)
	return knob
}

// The env button, one column (trioColumn) like spk and solo.
func init() {
	for _, o := range genOscs {
		env := o.id + "-env"
		trioPrograms[env] = trioProgram{
			keys: []string{"env"},
			help: []string{"envelope: lit, Gen " + o.letter() + "'s level repeats attack then decay, a shaped tremolo; dark, it is steady"},
			press: func(int) {
				v := "rpt"
				if s := dom.Doc.Call("getElementById", env); s.Truthy() && s.Get("value").String() == "rpt" {
					v = "off"
				}
				setSelect(env, v)
			},
			lit: func() int {
				if s := dom.Doc.Call("getElementById", env); s.Truthy() && s.Get("value").String() == "rpt" {
					return 0
				}
				return -1
			},
		}
	}
}
