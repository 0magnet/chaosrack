//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The controls of something switched on elsewhere, or of a model's layer:
// the section's plane, the Turtle's weight, the spectrogram's analysis, the
// water lens. They are positions in the model bank (bay 1), in the room a
// model's own constants leave: a bay of modules of their own, dark under
// most models, said nothing of what brought them into play.
//
// Two kinds. The section's plane and the Turtle's weight are their models':
// they stand after the constants of the flows and of Turtle, and their
// switch, ON, is on their first position, so they show with the model, their
// readouts blank until it is lit (cell-off). The spectrogram's and the
// lens's come with what is behind, or on, any model, and stand at the same
// positions for every model, against Model Out's column: dormant — dark,
// like the unassigned positions around them, and not to be turned — until
// BEHIND, SKIN or the model brings them in. A model whose own constants
// reach that far goes without them (bankGroupSlots).
//
// Each is the one home of its controls. The Poincaré model's plane and the
// spectrogram's analysis are also those models' constants, and building them
// twice gave two elements the same id — the second one's knob drove the
// first one's slider. The models' own cells leave them out (homedParamIDs).

// homedModule is one of them: its entry, its controls, the models it can
// stand in the bank for, whether it is dormant rather than blank while out
// of play, and whether it is in play under model mode.
type homedModule struct {
	key     string
	docKey  string // its entry, which heads its first position's tooltip
	params  func() []paramDef
	models  func(own []string) []string
	dormant bool
	applies func(mode string) bool
}

var homedModules = []homedModule{
	{"section", "sect-module", func() []paramDef { return sectPlaneParams }, func(own []string) []string {
		var out []string
		for _, m := range own {
			if _, flow := dynamics.FlowFor4(m); flow || m == "poincare" {
				out = append(out, m)
			}
		}
		return out
	}, false, func(mode string) bool {
		_, flow := dynamics.FlowFor4(mode)
		return mode == "poincare" || (sect.on && flow)
	}},
	{"physics", "phys-module", func() []paramDef { return turtlePhysParams }, func([]string) []string { return []string{"turtle"} },
		false, func(string) bool { return physOn() }},
	{"water", "water-module", func() []paramDef { return attractorParams["water"] }, func(own []string) []string { return own },
		true, func(string) bool { return waterActive() }},
	{"spectro", "spectro", func() []paramDef { return spectParams }, func(own []string) []string { return own },
		true, func(mode string) bool {
			return isSpectroSurface(mode) || (skin.source == "spectrogram" && isSkinnable(mode)) || bgVisual == "spectrogram"
		}},
}

// homedCells are each homed module's cells, as the bank built them.
var homedCells = map[string][]js.Value{}

// homedParamIDs are the controls the bank holds for them, which the models'
// own cells leave out.
func homedParamIDs() map[string]bool {
	ids := map[string]bool{}
	for _, m := range homedModules {
		for _, p := range m.params() {
			ids[p.ID] = true
		}
	}
	return ids
}

// homedBankGroups builds each module's cells for the bank of models own, and
// says where they go: after the own cells of their models, or (the dormant
// ones) against the end of the bank.
func homedBankGroups(own []string) ([]bankGroup, [][]js.Value) {
	var groups []bankGroup
	var cells [][]js.Value
	for _, m := range homedModules {
		var cs []js.Value
		for _, p := range m.params() {
			cs = append(cs, homedCell(p))
		}
		// What the run is, and what brings it into play, on its first
		// position: the bank has no header to say it.
		if len(cs) > 0 && doc(m.docKey) != "" {
			cs[0].Set("title", doc(m.docKey)+"\n\n"+cs[0].Get("title").String())
		}
		homedCells[m.key] = cs
		groups = append(groups, bankGroup{Size: len(cs), Models: m.models(own), Tail: m.dormant})
		cells = append(cells, cs)
	}
	return groups, cells
}

// placeHomedGroups puts each group's cells into the bank's positions for the
// models it serves (bankGroupSlots), as shared cells (data-bank-for); a cell
// no model can show is kept hidden in the bank, still a control for reset,
// the permalink and MIDI. used is how far each model's own cells reach.
func placeHomedGroups(used map[string]int, cells map[string][]js.Value, groups []bankGroup, gcells [][]js.Value) (orphans []js.Value) {
	end := (bankColumns-1)*catCellsPerCol - bankHeadCells
	// A run after own cells starts at the top of a column, so it reads as a
	// group of its own rather than as more of the model's constants.
	top := func(p int) int {
		for {
			if r, _ := bankCell(p); r == 1 || p >= end {
				return p
			}
			p++
		}
	}
	starts, served := bankGroupSlots(used, groups, end, top)
	for i, cs := range gcells {
		if len(served[i]) == 0 {
			orphans = append(orphans, cs...)
			continue
		}
		for j, c := range cs {
			c.Call("setAttribute", "data-bank-for", strings.Join(served[i], " "))
			p := starts[i] + j
			for _, m := range served[i] {
				for len(cells[m]) <= p {
					cells[m] = append(cells[m], js.Undefined())
				}
				cells[m][p] = c
			}
		}
	}
	return orphans
}

// buildHomedModules wires the switches on their first positions: Physics's
// own, and the section's, which LINE's S is the other face of.
func buildHomedModules() {
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

// syncHomedModules sets each module's cells for model mode: blank readouts
// (cell-off), or dormant, while it is out of play; and the Lattice module's,
// which draws only for its model or with L lit. Then it relights the
// switches on them.
func syncHomedModules(mode string) {
	for _, m := range homedModules {
		out := !m.applies(mode)
		cls := "cell-off"
		if m.dormant {
			cls = "dormant"
		}
		for _, c := range homedCells[m.key] {
			c.Get("classList").Call("toggle", cls, out)
		}
	}
	if host := dom.Doc.Call("getElementById", "lattice-bank"); host.Truthy() {
		off := mode != "lattice" && !lat.figOn
		cs := host.Call("querySelectorAll", ".punit")
		for i := range cs.Length() {
			cs.Index(i).Get("classList").Call("toggle", "cell-off", off)
		}
	}
	lightTrios("turtle-grav")
	lightTrios("sect-axis")
}
