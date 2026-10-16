//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The modules that hold the controls of something switched on elsewhere, or
// of a model's layer: the section's plane, the water lens, the Turtle's
// weight, the spectrogram's analysis. They are in the rack for every model
// and whatever is switched on, as every other module is — a module that
// appears with a switch moves every module after it in its bay, and a native
// rack, which reads the rack's parts off one picture of it, never saw them.
// Where one does not apply its cells keep their legends and blank their
// readouts (cell-off, panel.css), and stay turnable: a knob set before
// the lens is on is the lens the switch turns on.
//
// Each is the one home of its controls. The Poincaré model's plane and the
// spectrogram's analysis were also that model's constants in the bank, and a
// module beside it building them again gave two elements the same id once
// the model had been shown — the second one's knob drove the first one's
// slider. The bank leaves them to the module (homedParamIDs).

// homedModule is one of them: the grid its cells go in, its controls, and
// whether they apply under model mode.
type homedModule struct {
	host    string
	params  func() []paramDef
	applies func(mode string) bool
}

var homedModules = []homedModule{
	{"section-params", func() []paramDef { return sectPlaneParams }, func(mode string) bool {
		_, flow := dynamics.FlowFor4(mode)
		return mode == "poincare" || (sect.on && flow)
	}},
	{"water-params", func() []paramDef { return attractorParams["water"] }, func(string) bool { return waterActive() }},
	{"physics-params", func() []paramDef { return turtlePhysParams }, func(string) bool { return physOn() }},
	{"spectro-params", func() []paramDef { return spectParams }, func(mode string) bool {
		return isSpectroSurface(mode) || (skin.source == "spectrogram" && isSkinnable(mode)) || bgVisual == "spectrogram"
	}},
}

// homedParamIDs are the controls the modules hold, which the bank leaves out.
func homedParamIDs() map[string]bool {
	ids := map[string]bool{}
	for _, m := range homedModules {
		for _, p := range m.params() {
			ids[p.ID] = true
		}
	}
	return ids
}

// buildHomedModules fills each module's grid, once: with the bank, outside any
// panel build, so their listeners live as long as the page.
func buildHomedModules() {
	for _, m := range homedModules {
		host := dom.Doc.Call("getElementById", m.host)
		if !host.Truthy() || host.Get("childElementCount").Int() > 0 {
			continue
		}
		grid := dom.Doc.Call("createElement", "div")
		grid.Set("className", "punit-grid")
		for _, p := range m.params() {
			grid.Call("appendChild", homedCell(p))
		}
		host.Call("appendChild", grid)
	}
	// The switches on their first cells: Physics's own, and the section's,
	// which LINE's S is the other face of.
	trioPrograms["turtle-grav"] = switchTrio("phys-sw", "trio.turtle-grav=0")
	trioPrograms["sect-axis"] = switchTrio("sect-sw", "trio.sect-axis=0")
	wireTrios()
	syncTrios()
	syncHomedModules(run.selectedMode)
}

// switchTrio is a position's top button as the face of the hidden switch sw.
func switchTrio(sw, help string) trioProgram {
	return trioProgram{
		keys:     []string{"ON", "", ""},
		switches: []string{sw},
		help:     []string{doc(help)},
		press:    func(int) { setSwitch(sw, !checkedOn(sw)) },
		lits:     func() []bool { return []bool{checkedOn(sw), false, false} },
		drive:    []string{sw},
	}
}

// homedCell is p's cell as a bank's position is made (newPUnit), but no
// model's: a model's position blanks while another model runs, and these
// answer to their switches instead (syncHomedModules).
func homedCell(p paramDef) js.Value {
	before := len(paramControls)
	unit := buildParamUnitAs("", p, true)
	// The rack's for good, not the next panel rebuild's (newPUnit's reason).
	if len(paramControls) > before {
		catParamControls = append(catParamControls, paramControls[before:]...)
		paramControls = paramControls[:before]
	}
	unit.Call("setAttribute", "data-param", p.ID)
	if h := helpFor(p.ID); h != "" {
		unit.Set("title", h)
	}
	bankPosition(unit)
	return unit
}

// syncHomedModules blanks the readouts of each module that does not apply under
// mode, and relights the switches on them.
func syncHomedModules(mode string) {
	for _, m := range homedModules {
		host := dom.Doc.Call("getElementById", m.host)
		if !host.Truthy() {
			continue
		}
		blank := !m.applies(mode)
		cs := host.Call("querySelectorAll", ".punit")
		for i := range cs.Length() {
			cs.Index(i).Get("classList").Call("toggle", "cell-off", blank)
		}
	}
	lightTrios("turtle-grav")
	lightTrios("sect-axis")
}
