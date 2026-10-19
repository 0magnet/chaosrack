//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// genOutCell finishes a generator's out cell: a display in its top row that
// reads the waveform, and under the wave knob the OUT mini knob, which
// borrows that display while it is turned (lendReadout), as a level's ATTACK
// and DECAY borrow the level's. OUT is a view of the Mixer's pins
// (genout.go): turning it pins the generator, and pins set on the Mixer turn
// it (mixShowGenOut). It has no key in a link of its own: the Mixer's carries
// the pins it reads.
func genOutCell(o genOscSpec, cell, wave, solo js.Value) {
	top := cell.Call("querySelector", ".punit-top")
	out := dom.Doc.Call("getElementById", o.id+"-out")
	if !top.Truthy() || !out.Truthy() {
		return
	}
	win := dotDisplayN(genWaveName(wave), false, dispFullChars)
	win.Set("id", o.id+"-wave-read")
	top.Call("appendChild", win)
	row := miniRow(miniKnob(out, "O", true, false))
	cell.Call("insertBefore", row, solo)
	loan := lendReadout(row, win, func() { setDotText(win, genWaveName(wave)) })
	adoptDescControl(ControlDesc{
		ID: o.id + "-out", Label: "out", Min: 0, Max: GenOutCount - 1, Step: 1, Def: GenOutOff,
		Apply: func(v float64) {
			mixSetGenOut(o.idx, clampIdx(int(v+0.5), GenOutCount))
			loan.show("out", GenOutNames[mixGenOut(o.idx)])
		},
	})
}

// genWaveName is what the waveform knob is on, as its ring names it.
func genWaveName(wave js.Value) string {
	if i := wave.Get("selectedIndex").Int(); i >= 0 {
		return wave.Get("options").Index(i).Get("textContent").String()
	}
	return ""
}

// genShowWave puts generator o's waveform on its out cell's display.
func genShowWave(o genOscSpec) {
	win := dom.Doc.Call("getElementById", o.id+"-wave-read")
	wave := dom.Doc.Call("getElementById", o.id+"-wave")
	if win.Truthy() && wave.Truthy() {
		setDotText(win, genWaveName(wave))
	}
}
