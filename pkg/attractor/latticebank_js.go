//go:build js && wasm

package attractor

import (
	"slices"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/lattice"
)

// The Lattice in bay 1.
//
// It was two things a bay apart: a Lattice MODEL, which drew the lattice's own
// solids, and a Lattice MODULE in bay 3, whose knobs said how the lattice drew
// and whose L drew whichever model was running through it instead. Now it is
// one position of the MODEL knob, and SRC, its first knob, says what is in it:
// one of its own solids, or any model the rack draws in three dimensions.
//
// THE MODEL IN THE LATTICE STILL RUNS AS ITSELF. Choosing Lorenz on SRC makes
// Lorenz the running model, with lat-on lit, exactly as L did: its sound, its
// equation, its section and its constants are Lorenz's, because everything in
// the rack that asks what the model is gets Lorenz. Only the MODEL knob says
// Lattice (rotaryMode), and the lattice's cells stand in the bank after
// Lorenz's own, so the model's knobs are where they always are and the
// lattice's are beside them. Its own solids are the Lattice model, as before,
// with lat-on off.
//
// THE CELLS LIVE IN THE LATTICE'S OWN BANK, after spin and tilt, as any model's
// selectors do (data-bank-of), so ctl, MIDI and a link reach them whatever is
// running. While the lattice draws another model they are moved, as Custom's
// are placed (bankCustomCells), to that model's free positions, and put back
// when it stops. A free position is an unassigned blank or a dormant one —
// the spectrogram's and the lens's knobs, which wait at the end of the bank
// for what is behind the model — and the lattice takes those in order from
// the first column the model's own cells leave. Nothing else is moved: a
// model's knobs keep their addresses in the lattice.

// latticeCells are the lattice's positions, SRC first.
var latticeCells = []string{
	"lattice-src-cell", "lattice-n-cell", "lattice-style-cell", "lattice-look-cell",
	"lattice-opac-cell", "lattice-stacks-cell", "lattice-turn-cell",
}

// latticeTookMark is on a position a lattice cell has taken; its value is the
// model it was taken from.
const latticeTookMark = "data-lattice-took"

// latticeSolidPrefix marks SRC's own solids, whose names (torus) are also
// models'.
const latticeSolidPrefix = "solid:"

// latticeShown reports whether the MODEL knob is on the Lattice: its own
// solid, or mode drawn by it.
func latticeShown(mode string) bool { return mode == "lattice" || latticeDraws(mode) }

// rotaryMode is the MODEL position mode is shown at.
func rotaryMode(mode string) string {
	if latticeDraws(mode) {
		return "lattice"
	}
	return mode
}

// latticeSources are the models SRC offers: every model the lattice can take
// a figure from, in the rack's order.
func latticeSources() []string {
	var out []string
	for _, g := range modeGroups {
		for _, m := range g.Keys {
			if m != "lattice" && !isTexturePlane(m) && !latticeNoFigure[m] && modeGenerate[m] != nil && !slices.Contains(out, m) {
				out = append(out, m)
			}
		}
	}
	return out
}

// wireLatticeSrc fills SRC and adopts it. The solids come first, by their
// own names; the models after them, by theirs.
func wireLatticeSrc() {
	sel := dom.Doc.Call("getElementById", "lattice-src")
	if !sel.Truthy() {
		return
	}
	add := func(value, text, title string) {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", value)
		o.Set("textContent", text)
		o.Set("title", title)
		sel.Call("appendChild", o)
	}
	for _, s := range lattice.ShapeNames() {
		add(latticeSolidPrefix+s, s, "the lattice's own "+s)
	}
	for _, m := range latticeSources() {
		add(m, modeLabel(m), modeLabel(m)+", drawn by the lattice")
	}
	def := latticeSolidPrefix + lattice.ShapeNames()[int(latticeSphere)]
	sel.Set("value", def)
	adoptDescControl(ControlDesc{ID: "lattice-src", Label: "src", IsSelect: true, SelectDef: def, SelectApply: chooseLatticeSrc})
}

// latticeLastSrc is what SRC was last on, which the MODEL knob returns to.
var latticeLastSrc string

// chooseLatticeSrc puts v in the lattice: a solid of its own, or a model.
func chooseLatticeSrc(v string) {
	if v == "" || v == latticeSrcNow() {
		return
	}
	latticeLastSrc = v
	if name, ok := strings.CutPrefix(v, latticeSolidPrefix); ok {
		if i := slices.Index(lattice.ShapeNames(), name); i >= 0 {
			applyParam("lattice-shape", strconv.Itoa(i))
		}
		setSwitch("lat-on", false)
		chooseModel("lattice")
		return
	}
	setSwitch("lat-on", true)
	chooseModel(v)
}

// enterLattice is the MODEL knob turned to the Lattice: back to what SRC was
// last on, or the lattice's own solid.
func enterLattice() {
	v := latticeLastSrc
	if v == "" {
		v = latticeSolidPrefix + lattice.ShapeNames()[pick(lat.shapeF, len(lattice.ShapeNames()))]
	}
	if _, solid := strings.CutPrefix(v, latticeSolidPrefix); solid || modeGenerate[v] == nil {
		setSwitch("lat-on", false)
		chooseModel("lattice")
		return
	}
	setSwitch("lat-on", true)
	chooseModel(v)
}

// chooseModel switches the running model to mode, as the MODEL knob does, or
// only resyncs the rack when it is already the one running (a switch of
// lat-on alone changes what the knob shows and what the bank holds).
func chooseModel(mode string) {
	if mode == editMode() {
		syncCategoryRotaries()
		buildParamPanel(mode)
		return
	}
	if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
		ms.Set("value", mode)
		dom.Fire(ms, "change")
	}
}

// latticeSrcNow is what SRC should read for the running model.
func latticeSrcNow() string {
	m := run.selectedMode
	if latticeDraws(m) {
		return m
	}
	return latticeSolidPrefix + lattice.ShapeNames()[pick(lat.shapeF, len(lattice.ShapeNames()))]
}

// showLatticeSrc points SRC at what is in the lattice, without choosing it.
func showLatticeSrc() {
	sel := dom.Doc.Call("getElementById", "lattice-src")
	if !sel.Truthy() {
		return
	}
	if v := latticeSrcNow(); sel.Get("value").String() != v {
		sel.Set("value", v)
		dom.Fire(sel, "input") // the knob and its display follow; change would choose it
	}
}

// latticeHomeMark is on a lattice cell away from the Lattice's own positions:
// where it stood, and the model it was shown for.
const latticeHomeMark = "data-lattice-home"

// clearLatticeBankCells puts the lattice's cells back at the Lattice's own
// positions and gives the positions they took back.
func clearLatticeBankCells() {
	if !dom.Doc.Truthy() {
		return
	}
	for _, id := range latticeCells {
		c := dom.Doc.Call("getElementById", id)
		if !c.Truthy() || !c.Call("hasAttribute", latticeHomeMark).Bool() {
			continue
		}
		home := strings.SplitN(c.Call("getAttribute", latticeHomeMark).String(), "|", 2)
		c.Call("removeAttribute", latticeHomeMark)
		c.Get("style").Set("gridArea", home[0])
		if len(home) == 2 {
			c.Call("setAttribute", "data-bank", home[1])
			c.Call("setAttribute", "data-mode", home[1])
		}
	}
	took := dom.Doc.Call("querySelectorAll", ".catbank ["+latticeTookMark+"]")
	for i := range took.Length() {
		b := took.Index(i)
		m := b.Call("getAttribute", latticeTookMark).String()
		b.Call("removeAttribute", latticeTookMark)
		b.Call("setAttribute", "data-bank-for", strings.TrimSpace(b.Call("getAttribute", "data-bank-for").String()+" "+m))
	}
}

// syncLatticeBank moves the lattice's cells after mode's own while the
// lattice draws mode, and puts them back at the Lattice's when it does not.
func syncLatticeBank(mode string) {
	clearLatticeBankCells()
	defer showLatticeSrc()
	if !latticeDraws(mode) {
		return
	}
	bank := dom.Doc.Call("getElementById", "bank-"+categorySlug(rowOf(mode))+"-module")
	if !bank.Truthy() {
		return
	}
	grid := bank.Call("querySelector", ".catbank")
	if !grid.Truthy() {
		return
	}
	// Each position as the grid spells it, which is how the cells standing
	// there are found: a probe placed there reads its own style back.
	last := (bankColumns-1)*catCellsPerCol - bankHeadCells
	spell := make([]string, last)
	probe := dom.Doc.Call("createElement", "div")
	for i := range spell {
		row, col := bankCell(i)
		at(probe, grid, row, 1, col, 1)
		spell[i] = probe.Get("style").Get("gridArea").String()
	}
	probe.Call("remove")
	// What mode shows at each position.
	shown := map[string][]js.Value{}
	cs := grid.Call("querySelectorAll", ":scope>*")
	for i := range cs.Length() {
		c := cs.Index(i)
		mine := c.Call("getAttribute", "data-bank").String() == mode
		if f := c.Call("getAttribute", "data-bank-for"); !mine && !f.IsNull() {
			mine = slices.Contains(strings.Fields(f.String()), mode)
		}
		if mine {
			k := c.Get("style").Get("gridArea").String()
			shown[k] = append(shown[k], c)
		}
	}
	// A position is free when all that stands there is unassigned, or a
	// dormant knob waiting for something behind the model.
	var dormant []js.Value
	for _, m := range homedModules {
		if m.dormant {
			dormant = append(dormant, homedCells[m.key]...)
		}
	}
	free := func(i int) bool {
		for _, c := range shown[spell[i]] {
			if !c.Get("classList").Call("contains", "bankblank").Bool() && !slices.ContainsFunc(dormant, c.Equal) {
				return false
			}
		}
		return true
	}
	// From the top of the column after the last position mode uses; then,
	// for a model whose own reach too far for all of them, the free
	// positions before it.
	start := 1
	for i := range last {
		if !free(i) {
			start = i + 1
		}
	}
	for (start+bankHeadCells)%catCellsPerCol != 0 {
		start++
	}
	var order []int
	for i := start; i < last; i++ {
		order = append(order, i)
	}
	for i := 1; i < min(start, last); i++ {
		order = append(order, i)
	}
	next := 0
	for _, i := range order {
		if next == len(latticeCells) {
			break
		}
		if !free(i) {
			continue
		}
		c := dom.Doc.Call("getElementById", latticeCells[next])
		next++
		if !c.Truthy() {
			continue
		}
		for _, b := range shown[spell[i]] {
			var rest []string
			for _, m := range strings.Fields(b.Call("getAttribute", "data-bank-for").String()) {
				if m != mode {
					rest = append(rest, m)
				}
			}
			b.Call("setAttribute", "data-bank-for", strings.Join(rest, " "))
			b.Call("setAttribute", latticeTookMark, mode)
		}
		c.Call("setAttribute", latticeHomeMark, c.Get("style").Get("gridArea").String()+"|"+c.Call("getAttribute", "data-bank").String())
		c.Call("setAttribute", "data-bank", mode)
		c.Call("setAttribute", "data-mode", mode)
		row, col := bankCell(i)
		at(c, grid, row, 1, col, 1)
	}
	syncBankCells()
	lightLiveParamCells()
}
