//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// The modules whose controls come into play with something switched on
// elsewhere: the lattice drawing a model, a spectrogram on screen, the
// section over a flow, the water lens, the Turtle's weight. Out of play their
// readouts are blank (cell-off), which says that they do nothing but not what
// would make them do something; that was in their tooltips alone. So each
// carries a lamp at the right of its header, lit while it is in play, and
// beside it while it is out a legend naming what brings it in. The lamp's
// tooltip says it in full (play.<module> in the manual).
//
// Outside the header, not in it: a module is known by its header's text
// (moduleSections, the manual), which a lamp in it would change.

// playModule is one of them: the module, the grid of its cells, the legend,
// and whether it is in play under model mode.
type playModule struct {
	module string
	cells  string
	legend string
	on     func(mode string) bool
}

var playModules = []playModule{
	{"lattice-module", "lattice-bank", "model · ROWS L", func(mode string) bool { return mode == "lattice" || lat.figOn }},
	{"spectro-module", "spectro-params", "a spectrogram", homedApplies("spectro-params")},
	{"section-module", "section-params", "ON · flow", homedApplies("section-params")},
	{"water-module", "water-params", "BEHIND: water", homedApplies("water-params")},
	{"physics-module", "physics-params", "ON · Turtle", homedApplies("physics-params")},
}

// homedApplies is the homed module host's test of whether it applies.
func homedApplies(host string) func(string) bool {
	return func(mode string) bool {
		for _, m := range homedModules {
			if m.host == host {
				return m.applies(mode)
			}
		}
		return false
	}
}

// syncPlayModules blanks the readouts of each module out of play under mode,
// and lights each one's lamp, or shows its legend.
func syncPlayModules(mode string) {
	for _, p := range playModules {
		mod := dom.Doc.Call("getElementById", p.module)
		if !mod.Truthy() {
			continue
		}
		on := p.on(mode)
		if host := dom.Doc.Call("getElementById", p.cells); host.Truthy() {
			cs := host.Call("querySelectorAll", ".punit")
			for i := range cs.Length() {
				cs.Index(i).Get("classList").Call("toggle", "cell-off", !on)
			}
		}
		lamp := playLamp(mod, p)
		lamp.Get("classList").Call("toggle", "lit", on)
		mod.Get("classList").Call("toggle", "in-play", on)
	}
}

// playLamp is module mod's lamp and legend, made the first time.
func playLamp(mod js.Value, p playModule) js.Value {
	if l := mod.Call("querySelector", ":scope>.play-lamp"); l.Truthy() {
		return l
	}
	lg := dom.Doc.Call("createElement", "span")
	lg.Set("className", "play-legend")
	lg.Set("textContent", p.legend)
	lamp := dom.Doc.Call("createElement", "span")
	lamp.Set("className", "play-lamp")
	if t := doc("play." + p.module); t != "" {
		lamp.Set("title", t)
		lg.Set("title", t)
	}
	mod.Get("classList").Call("add", "has-play")
	mod.Call("appendChild", lg)
	mod.Call("appendChild", lamp)
	return lamp
}
