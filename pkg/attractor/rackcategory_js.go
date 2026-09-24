//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/gentile"
	"strconv"
	"strings"
	"syscall/js"
)

// One rack row per model category.
//
// A row is a whole instrument: its own monitor on the left, the rotary that
// picks which of its models is playing, and then the parameters of EVERY
// model in the category — not only the one running. See rackcategory.go for
// why the rows exist.
//
// Every model's knobs, always. That is the point of a rack: an instrument's
// front panel does not appear when you patch it in and vanish when you patch
// it out. It also makes the rows the size of what they hold, which nothing
// else here managed — a row per category with only the running model's panel
// on it was eleven rows of blank panel and one that was half full.
//
// Measured, the union of parameters per category:
//
//	Attractors 76   Scope 50   Audio 40   Maps 19
//	Geometry   13   Sequences 10   Analysis 4   Polyhedra 1
//	Solids      0   Custom     0                  = 213
//
// Attractors is 76 because the Sprott systems were folded into it; it was 55
// and they were 21. The total is unchanged — the same knobs, one heading
// fewer — and it is the widest category by some way, which is what the
// continuation modules below are for.
//
// Three of those are wider than an 84 HP bay, so those categories continue
// into a second module and the section flows into the next bay, which is
// what a section wider than a bay has always done here. A module cannot
// overflow a bay, which is why the continuation is a module and not a
// wrapped grid.
//
// A parameter has ONE knob in the rack. Its id is what the MIDI map, the
// permalink and Reset All address it by, so a second element with the same
// id is not a second view of a value, it is a bug in three features at once.
// Two consequences: the per-mode Parameters module stopped building knobs
// (see buildParamPanel), and where a parameter is shared by models in two
// categories the first row to claim it builds it — one variable, one knob.
//
// The rotaries INTERLOCK, the mechanism the Rhythm module's preset tabs use
// and for the same reason: the rack draws one model, so one row is driving
// it and the rest are showing what they would play. Choosing a model on any
// row puts every other row's rotary to OFF. Choosing OFF on the row that IS
// driving powers the rack down — which is what OFF on the model selector has
// always meant here, it being the first detent of the Console's old category
// knob. It used to snap back instead, and a detent you cannot reach is a
// broken control whatever the argument for it.

// categoryOffLabel is the rotary's first position.
const categoryOffLabel = "off"

// The row's geometry, in grid columns. A bay is twelve columns, a column
// holds three control positions, and a bay's head is two of those columns:
// the monitor across the top two rows, the model rotary and the step knob
// side by side underneath it. That is six positions with nothing blank in
// them, where the head used to be four columns to show three controls.
const (
	catCellsPerCol = 3
	catColsPerBay  = 12
	catHeadCols    = 2
	catMonitorRows = 2
	// The screen fills the two columns it is given, less the bezel.
	catMonitorWidth = 232
	catMonitorHigh  = 250
)

// catRotarySyncing guards the interlock against its own writes: putting the
// other rotaries to OFF dispatches change events, and without this each one
// would try to re-drive the interlock.
var catRotarySyncing bool

// catParamControls are the Controls of the category rows' parameter cells.
//
// Separate from paramControls, which buildParamPanel clears on every mode
// change: these are built once and outlive every rebuild, and clearing them
// would take the whole rack's knobs out of the control model. Kept so
// findBuiltControl can still match a cell to the Control the builder made.
var catParamControls []*Control

// buildCategoryModules creates every row, once.
//
// Inserted before the Parameters module, which is where the running model's
// readouts go and therefore where a reader looks after choosing one.
func buildCategoryModules() {
	if !dom.Doc.Truthy() {
		return
	}
	host := dom.Doc.Call("getElementById", "params-module")
	if !host.Truthy() {
		return
	}
	parent := host.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	if dom.Doc.Call("getElementById", categoryHeadID(modelCategories()[0])).Truthy() {
		return // already built
	}
	claimed := map[string]bool{}
	for _, label := range modelCategories() {
		for _, mod := range buildCategoryRow(label, claimed) {
			parent.Call("insertBefore", mod, host)
		}
	}
	syncCategoryRotaries()
}

// A CATEGORY IS A CAGE OF CARDS.
//
// Each model that has constants of its own is a card with its NAME on it —
// Lorenz, Chua, Aizawa — holding that model's constants and nothing else.
// The category's own module sits at the head of the run with the monitor,
// the rotary that chooses which card is playing, the step size, and any
// control that belongs to the category rather than to one of its models.
// The bay silkscreen marks the whole run.
//
// This replaces one wide grid per category with a silkscreened band over
// each model's columns. The band was the right idea and the wrong object: a
// group of controls with a name over it, sharing a panel with eleven other
// groups, IS a card — so it should be one, and then it has an honest width,
// it flows across bays the way any section too wide for one always has, and
// it can be dragged and put away like everything else in the rack.
//
// It also ends the continuation modules. "Attractors 2" existed because a
// module cannot overflow a bay and a category's grid could; no equipment has
// a panel called that. Fourteen cards called after fourteen systems is what
// the thing actually is.
//
// Two kinds of parameter do NOT go on a card:
//
//   - The step size. dt is not a constant of any system, it is how finely
//     the integrator walks it, so it belongs with the category's controls.
//     Every model's step cell is built and stacked in one place and the head
//     shows the one the row is set to. The Sprott systems are the case that
//     proves it: all nineteen have a step and nothing else, so on cards they
//     would be nineteen one-knob panels.
//
//   - A parameter several models declare. poly-op is the Conway operator
//     applied to whichever Platonic seed is chosen and takens-smooth is
//     shared by three of the Scope displays; neither is a property of any one
//     model, and putting it on the first card that happened to claim it would
//     say it was.

// buildCategoryRow builds one category's modules: its head, then a card for
// every model in it that has constants of its own.
//
// claimed is shared across rows so a parameter that two categories' models
// both declare is built once, by the first row that reaches it.
// buildCategoryRow builds one category's bays.
//
// A bay is the unit that means something here: one monitor, one selector
// choosing among the generators in THAT bay, and the generator modules
// themselves. A category takes as many bays as its generators need, and the
// silkscreen names the category over every one of them.
//
// claimed is shared across categories so a parameter that two of their
// models both declare is built once, by the first that reaches it.
func buildCategoryRow(label string, claimed map[string]bool) []js.Value {
	own := categoryOwnModes(label)

	// Which parameters are the category's rather than a model's: the ones
	// more than one of its models declares. poly-op is the Conway operator
	// applied to whichever seed is chosen; it is not a property of the
	// tetrahedron just because that is the first card that could claim it.
	shared := map[string]int{}
	for _, mode := range own {
		for _, p := range attractorParams[mode] {
			shared[p.ID]++
		}
	}

	// Build every cell first, and count what each generator owns, so the
	// packer works on the real shapes rather than on the declarations.
	cells := map[string][]js.Value{}
	steps := map[string]js.Value{}
	var sharedCells []js.Value
	var gens []gentile.Spec
	for _, mode := range own {
		for _, p := range attractorParams[mode] {
			if claimed[p.ID] {
				continue
			}
			claimed[p.ID] = true
			switch {
			case strings.HasSuffix(p.ID, "-dt"):
				steps[mode] = buildCategoryStepCell(mode, p)
			case shared[p.ID] > 1:
				sharedCells = append(sharedCells, buildCategoryParamCell(mode, p))
			default:
				cells[mode] = append(cells[mode], buildCategoryParamCell(mode, p))
			}
		}
		gens = append(gens, gentile.Spec{Mode: mode, Constants: len(cells[mode])})
	}

	// How wide this category's heads are. Every bay spends catHeadCols on
	// the monitor, the rotary and the step; the FIRST one also carries the
	// parameters that belong to the category rather than to any of its
	// models, so it is wider. The packer is given the tighter figure, which
	// leaves the later bays a column of slack rather than letting the first
	// one overhang its rack.
	headCols := catHeadCols + (len(sharedCells)+catCellsPerCol-1)/catCellsPerCol
	mods := gentile.Pack(gens, catColsPerBay-headCols)

	// Modules into bays: as many as fit beside a head.
	var out []js.Value
	bay, used := 0, 0
	var pending []js.Value
	var pendingModes []string
	flush := func() {
		if len(pending) == 0 && bay > 0 {
			return
		}
		head := buildBayHead(label, bay, pendingModes, steps, sharedCells)
		sharedCells = nil // the first bay of a category carries them
		out = append(out, head)
		out = append(out, pending...)
		pending, pendingModes, used = nil, nil, 0
		bay++
		headCols = catHeadCols
	}
	for _, m := range mods {
		if used+m.Cols > catColsPerBay-headCols && len(pending) > 0 {
			flush()
		}
		pending = append(pending, buildGenPanel(label, m, cells))
		for _, t := range m.Tiles {
			pendingModes = append(pendingModes, t.Mode)
		}
		used += m.Cols
	}
	flush()
	return out
}

// buildGenPanel is one hardware unit: up to three generators sharing one
// panel, each holding a run of control positions filled in reading order.
//
// A generator is identified three ways, because it has to be: its positions
// carry its ground tint, an outline is drawn round the run whatever shape it
// came out, and the first position is tagged with the model's name. That is
// Woodson & Conover's answer for a group inside a panel rather than a panel
// per group (§2-133, "marked outlines around each group... area color
// patterning"), and unlike a header strip along the top it still works for a
// generator that sits along the bottom row.
func buildGenPanel(label string, m gentile.Module, cells map[string][]js.Value) js.Value {
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid catgen")

	var names []string
	for i, t := range m.Tiles {
		name := modeLabel(t.Mode)
		names = append(names, categoryTag(name))
		own := cells[t.Mode]
		for j := 0; j < t.N && j < len(own); j++ {
			pos := t.Start + j
			col, row := m.ColRow(pos)
			c := own[j]
			c.Call("setAttribute", "data-mgroup", strconv.Itoa(i%3))
			markGroupEdges(c, m, t, pos)
			if j == 0 {
				c.Call("appendChild", genGroupTag(name))
			}
			at(c, grid, 1+row, 1, 1+col, 1)
		}
	}
	return wrapCategoryModule(label, genModuleID(m), strings.Join(names, " · "), grid,
		"One unit, "+strconv.Itoa(len(m.Tiles))+" generator(s): "+strings.Join(names, ", ")+
			".\n\nA module is a hardware unit — the same inputs and outputs as any other, "+
			"dedicated to what is printed on it. A unit per generator is honest about the "+
			"models and wrong about the hardware, because most of them are a knob wide. A "+
			"panel is three control rows deep, so generators share one when between them "+
			"they fill it; one that fills its own columns keeps them.")
}

// markGroupEdges draws the outline: a border on every side of a position
// where the neighbor is not part of the same generator.
//
// Computed per position rather than drawn as a box, because a run is not
// always a rectangle — five constants in a two-column panel are the top two
// rows and one below — and an outline that only knew how to be a rectangle
// would have to round up to one, which is the blank panel this is avoiding.
func markGroupEdges(c js.Value, m gentile.Module, t gentile.Tile, pos int) {
	in := func(p int) bool { return p >= t.Start && p < t.Start+t.N }
	col, _ := m.ColRow(pos)
	cl := c.Get("classList")
	if col == 0 || !in(pos-1) {
		cl.Call("add", "ge-l")
	}
	if col == m.Cols-1 || !in(pos+1) {
		cl.Call("add", "ge-r")
	}
	if !in(pos - m.Cols) {
		cl.Call("add", "ge-t")
	}
	if !in(pos + m.Cols) {
		cl.Call("add", "ge-b")
	}
}

// genModuleID names a generator module by the models on it.
func genModuleID(m gentile.Module) string {
	parts := make([]string, 0, len(m.Tiles))
	for _, t := range m.Tiles {
		parts = append(parts, categorySlug(t.Mode))
	}
	return "gen-" + strings.Join(parts, "-") + "-module"
}

// genGroupTag is the model's name, silkscreened in the corner of the first
// control position of its run.
func genGroupTag(name string) js.Value {
	t := dom.Doc.Call("createElement", "span")
	t.Set("className", "gtag")
	t.Set("textContent", name)
	t.Set("title", name)
	return t
}

// buildBayHead is the bay's own panel: its monitor, the selector that picks
// among the generators in this bay, and the step size.
//
// Two columns, six positions, nothing blank. The monitor takes the top two
// rows across both columns — it is a screen, so the space it is given is
// space it uses — and the rotary and the step knob sit side by side under
// it. It was four columns for the same three controls, because the step had
// a position reserved for every model in the bay while showing one: they are
// stacked in ONE position now, which is what the panel always looked like.
func buildBayHead(label string, bay int, modes []string, steps map[string]js.Value, extra []js.Value) js.Value {
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid cathead")
	at(buildCategoryMonitor(label, bay), grid, 1, catMonitorRows, 1, catHeadCols)
	at(buildBayRotary(label, bay, modes), grid, 1+catMonitorRows, 1, 1, 1)

	// Every step cell in the bay goes in the same position. Only one is ever
	// shown — the one belonging to the model the bay is set to — so one
	// position is what the panel needs, and a knob that re-ranges with the
	// selector is what it has always been from the front.
	for _, m := range modes {
		if s, ok := steps[m]; ok {
			at(s, grid, 1+catMonitorRows, 1, 2, 1)
		}
	}

	// The category's own parameters — the ones more than one of its models
	// declare — go in columns of their own, on the first bay only.
	for i, c := range extra {
		at(c, grid, 1+i%catCellsPerCol, 1, 1+catHeadCols+i/catCellsPerCol, 1)
	}

	title := label
	if bay > 0 {
		title = label + " " + strconv.Itoa(bay+1)
	}
	mod := wrapCategoryModule(label, categoryHeadID(label)+"-"+strconv.Itoa(bay), title, grid, "")
	// Declared on the module, because the rack packer works on the DOM and
	// has no idea a category was ever divided into bays. Without it the
	// packer refills the bays by width and this panel lands wherever it
	// fits, which for nine of the eleven heads was not the left of a row.
	mod.Call("setAttribute", bayHeadAttr, "1")
	return mod
}

// categoryOwnModes is the models filed under this category — the ones whose
// cards and constants the row carries.
func categoryOwnModes(label string) []string {
	var out []string
	for _, mode := range categoryModes(label) {
		if categoryOf(mode) == label {
			out = append(out, mode)
		}
	}
	return out
}

// categoryHeadID is the row.s head panel.
func categoryHeadID(label string) string { return "cat-" + categorySlug(label) + "-module" }

// wrapCategoryModule puts a grid in a module of its own.
func wrapCategoryModule(label, id, title string, grid js.Value, tip string) js.Value {
	mod := dom.Doc.Call("createElement", "div")
	mod.Set("className", "sect catmodule")
	mod.Set("id", id)
	// The bay this module is in, declared rather than looked up from the
	// header: a card is named after its model, and the Analysis category
	// collides by name with the Analysis meter.
	mod.Call("setAttribute", "data-cat", label)

	h := dom.Doc.Call("createElement", "div")
	h.Set("className", "sect-hdr")
	h.Set("textContent", title)
	if tip == "" {
		what := catTooltips[label]
		if what == "" {
			what = label
		}
		tip = what + "\n\nThe head of the row: the monitor, the knob that chooses which of " +
			"this category's models is playing, its integration step, and anything that " +
			"belongs to the category rather than to one model. Each model with constants " +
			"of its own has a card of its own further along the row."
	}
	h.Set("title", tip)
	mod.Call("appendChild", h)
	mod.Call("appendChild", grid)
	return mod
}

// belongs to so the row can light the ones that are running.
func buildCategoryParamCell(mode string, p paramDef) js.Value {
	before := len(paramControls)
	unit := buildParamUnit(mode, p)
	// The Controls the builder just made belong to the rack for good, not to
	// the next panel rebuild, which clears paramControls.
	if len(paramControls) > before {
		catParamControls = append(catParamControls, paramControls[before:]...)
		paramControls = paramControls[:before]
	}
	unit.Call("setAttribute", "data-mode", mode)
	// WHICH MODEL's constant this is. A row carries up to fifty-five cells
	// and the label on one of them is a symbol: Chen's a and Lu's a are the
	// same letter over two different knobs, and only the model tells them
	// apart. The name is on the cell rather than in the label because the
	// label is one glyph wide by design.
	name := mode
	if info, ok := modeInfo[mode]; ok && info.Label != "" {
		name = info.Label
	}
	unit.Set("title", name+" — "+p.Label)
	return unit
}

// buildCategoryStepCell is a model's integration step, for the block that
// gathers every model's.
//
// Labeled by MODEL, not by parameter. Inside a block already headed "step"
// the useful word on a cell is which system's step it is; "dt" written on
// every one of them is exactly what made the row read as one knob repeated.
func buildCategoryStepCell(mode string, p paramDef) js.Value {
	unit := buildCategoryParamCell(mode, p)
	unit.Get("classList").Call("add", "stepcell")
	if l := unit.Call("querySelector", ".u-lbl"); l.Truthy() {
		l.Set("textContent", categoryTag(modeLabel(mode)))
		// The model name is words, and a cell label is set upright down the
		// left edge, which is right for "dt" and wrong for "Newton-Leipnik".
		l.Get("classList").Call("add", "wordlbl")
	}
	return unit
}

// ── Bays ──────────────────────────────────────────────────────────────────
//
// A bay is the unit of selection now, not a category. It has one monitor and
// one selector, and the selector offers the generators mounted in that bay.
// A category with more generators than a bay holds simply takes more bays,
// and every one of them is silkscreened with the category's name.
//
// That is what a rack of this kind looks like: a row of units with a screen
// and a selector at the left of it, and the selector switches between what
// is IN that row. It also fixes the thing a category-wide selector could not
// — with fourteen attractors on one knob, reaching the last of them means
// turning past thirteen.

// bayRecord is one bay: which category it belongs to, its index within that
// category, and the generators mounted in it.
type bayRecord struct {
	Label string
	N     int
	Modes []string
}

// racksBays is every bay in the rack, in the order they are built.
var rackBays []bayRecord

// bayID names a bay's elements.
func bayID(label string, n int) string { return "bay-" + categorySlug(label) + "-" + strconv.Itoa(n) }

// bayMonitorID, bayMonSwitchID and baySelectID are the bay's screen, the
// switch that powers it, and the selector that picks what it plays.
func bayMonitorID(label string, n int) string   { return bayID(label, n) + "-mon" }
func bayMonSwitchID(label string, n int) string { return bayMonitorID(label, n) + "-on" }
func baySelectID(label string, n int) string    { return bayID(label, n) + "-sel" }

// bayOf is the bay a model is mounted in, or nil.
func bayOf(mode string) *bayRecord {
	for i := range rackBays {
		for _, m := range rackBays[i].Modes {
			if m == mode {
				return &rackBays[i]
			}
		}
	}
	return nil
}

// bayModel is what each bay is set to, remembered while another bay drives.
//
// The selectors interlock — the rack draws one model — so a bay that is not
// driving reads OFF and cannot say which of its generators it is on. It
// still has one: it is what comes back when the bay is turned on, and it is
// what that bay's step cell and monitor caption follow.
var bayModel = map[string]string{}

// bayModelOf is what a bay is set to: what it was last turned to, or its
// first generator before it has been turned at all.
func bayModelOf(b bayRecord) string {
	if m := bayModel[bayID(b.Label, b.N)]; m != "" {
		return m
	}
	if len(b.Modes) > 0 {
		return b.Modes[0]
	}
	return ""
}

// buildCategoryMonitor is a bay's screen: the model, while this bay is the
// one driving the rack, and standing by when another is.
func buildCategoryMonitor(label string, bay int) js.Value {
	unit := dom.Doc.Call("createElement", "span")
	unit.Set("className", "punit monunit")
	unit.Call("setAttribute", "data-no-drag", "")
	unit.Set("title", "Monitor — this bay's screen. It shows the model while this bay is the "+
		"one driving the rack, and stands by when another is: only one model is drawn, so "+
		"only one screen can carry a picture. The switch under it cuts the copy out of the "+
		"drawing buffer that the picture costs.")

	bez := dom.Doc.Call("createElement", "span")
	bez.Set("className", "monbezel")
	cv := dom.Doc.Call("createElement", "canvas")
	cv.Set("id", bayMonitorID(label, bay))
	cv.Set("width", catMonitorWidth)
	cv.Set("height", catMonitorHigh)
	bez.Call("appendChild", cv)
	unit.Call("appendChild", bez)

	lab := dom.Doc.Call("createElement", "label")
	lab.Set("className", "grp")
	lab.Get("style").Set("cursor", "pointer")
	sw := dom.Doc.Call("createElement", "input")
	sw.Set("type", "checkbox")
	sw.Set("className", "sw")
	sw.Set("id", bayMonSwitchID(label, bay))
	sw.Set("checked", true)
	lab.Call("appendChild", sw)
	lab.Call("appendChild", dom.Doc.Call("createTextNode", " Screen"))
	unit.Call("appendChild", lab)
	return unit
}

// buildBayRotary is the cell that picks which of this bay's generators plays.
func buildBayRotary(label string, bay int, modes []string) js.Value {
	rackBays = append(rackBays, bayRecord{Label: label, N: bay, Modes: modes})

	cell := dom.Doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell catcell")
	cell.Call("setAttribute", "data-no-drag", "")
	cell.Set("title", "Model — which of the generators in THIS bay is playing. Off means "+
		"another bay is driving the rack; every bay keeps what it was set to either way.")

	top := dom.Doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", "plabel")
	lbl.Set("textContent", "model")
	top.Call("appendChild", lbl)
	cell.Call("appendChild", top)

	sel := dom.Doc.Call("createElement", "select")
	sel.Set("id", baySelectID(label, bay))
	sel.Set("className", "selwin")
	sel.Set("title", "Model — pick one of this bay's generators, or turn the knob above it")
	add := func(value, text string) {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", value)
		o.Set("textContent", text)
		sel.Call("appendChild", o)
	}
	add("", categoryOffLabel)
	for _, m := range modes {
		add(m, modeLabel(m))
	}

	bay2 := dom.Doc.Call("createElement", "span")
	bay2.Set("className", "grp vmbay")
	// A knob AND a list, which is what the Console's model selector was. A
	// dial alone is the wrong control for a long list: reaching the last of
	// them means dragging through all the others while the rack re-renders
	// at each. The list is how you go somewhere; the knob is how you walk.
	stack := dom.Doc.Call("createElement", "span")
	stack.Set("className", "knobstack")
	stack.Call("setAttribute", "data-no-drag", "")
	knob := selk.makeSelectorKnob(sel)
	knob.Get("classList").Call("add", "knob-ring")
	stack.Call("appendChild", knob)

	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "catsel")
	wrap.Call("appendChild", stack)
	wrap.Call("appendChild", sel)
	bay2.Call("appendChild", wrap)
	cell.Call("appendChild", bay2)
	attachSelMarquee(sel, "#7fe0a0")

	lb, nb := label, bay
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
		onBayRotary(lb, nb)
		return nil
	}))
	return cell
}

// onBayRotary drives the model from a bay's own selector.
func onBayRotary(label string, bay int) {
	if catRotarySyncing {
		return
	}
	sel := dom.Doc.Call("getElementById", baySelectID(label, bay))
	if !sel.Truthy() {
		return
	}
	mode := sel.Get("value").String()
	if mode == "" {
		// OFF, and it means off: the rack powers down, which is what OFF on
		// a model selector has always meant here.
		setPowerState(false)
		setPowerSwitch(false)
		syncCategoryRotaries()
		return
	}
	bayModel[bayID(label, bay)] = mode
	setPowerState(true)
	setPowerSwitch(true)
	if mode == selectedMode {
		syncCategoryRotaries()
		return
	}
	ms := dom.Doc.Call("getElementById", "mode-select")
	if !ms.Truthy() {
		return
	}
	ms.Set("value", mode)
	ms.Call("dispatchEvent", js.Global().Get("Event").New("change"))
}

// setPowerSwitch moves the Console's Power switch without firing it, so the
// switch and the selectors always say the same thing about whether the rack
// is running.
func setPowerSwitch(on bool) {
	if sw := dom.Doc.Call("getElementById", "power-sw"); sw.Truthy() {
		sw.Set("checked", on)
	}
}

// syncCategoryRotaries puts every bay's selector where the current model
// says it should be: the bay holding it shows it, every other shows OFF.
//
// Called after any change of model, from wherever: a permalink, a preset,
// the jam performer, another bay's selector.
func syncCategoryRotaries() {
	if !dom.Doc.Truthy() {
		return
	}
	catRotarySyncing = true
	defer func() { catRotarySyncing = false }()
	setActiveCategory(selectedMode)
	live := bayOf(selectedMode)
	if live != nil && selectedMode != "" && !stopped {
		bayModel[bayID(live.Label, live.N)] = selectedMode
	}
	for _, b := range rackBays {
		sel := dom.Doc.Call("getElementById", baySelectID(b.Label, b.N))
		if !sel.Truthy() {
			continue
		}
		// Powered down, every selector reads off: no bay is driving,
		// because nothing is being drawn.
		want := ""
		if live != nil && b.Label == live.Label && b.N == live.N && !stopped {
			want = selectedMode
		}
		if sel.Get("value").String() == want {
			continue
		}
		sel.Set("value", want)
		// Both events. The knob's pointer follows 'input'; the marquee
		// readout follows 'change', and without it a bay put to off still
		// reads out the model it used to be on. Dispatching 'change' is safe
		// because catRotarySyncing is what stops the interlock answering its
		// own writes.
		sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		sel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	syncStepCells()
	lightLiveParamCells()
}

// syncStepCells shows one step cell per bay: the one belonging to the model
// that bay is set to.
func syncStepCells() {
	if !dom.Doc.Truthy() {
		return
	}
	want := map[string]bool{}
	for _, b := range rackBays {
		if m := bayModelOf(b); m != "" {
			want[m] = true
		}
	}
	cells := dom.Doc.Call("querySelectorAll", ".stepcell[data-mode]")
	for i := 0; i < cells.Get("length").Int(); i++ {
		c := cells.Index(i)
		if want[c.Call("getAttribute", "data-mode").String()] {
			c.Get("style").Set("display", "")
			continue
		}
		c.Get("style").Set("display", "none")
	}
}

// lightLiveParamCells marks the cells of the model that is running.
//
// Two hundred and thirteen knobs are on the rack and three of them are doing
// something. Without this the rows are a reference rather than an instrument:
// you can read every model's settings and not see which ones are live.
func lightLiveParamCells() {
	cells := dom.Doc.Call("querySelectorAll", ".punit[data-mode]")
	for i := 0; i < cells.Get("length").Int(); i++ {
		c := cells.Index(i)
		// Nothing is lit while the rack is powered down, which is the same
		// answer the rotaries give: no model is running, so no front panel
		// on the rack is the one in the signal path.
		live := !stopped && c.Call("getAttribute", "data-mode").String() == selectedMode
		c.Get("classList").Call("toggle", "live", live)
	}
}

// ── Putting a whole row away ───────────────────────────────────────────────
//
// Every model's knobs, always, is 213 parameter cells and 11 screens, and
// that is most of the panel's cost: the DOM went from 3748 nodes to 10966
// when the rows were filled, the relayout sweep from 0.30ms to 2.1ms, and
// the panel from about a tenth of the frame budget to about a fifth.
//
// A switch per row is the answer a rack already has for this. Taking a row
// out is display:none on its modules, which costs the browser nothing to
// keep — no layout, no paint, no measurement — so a rack cut down to the
// three categories somebody actually uses is as cheap as the rack was
// before the rows were filled. The models in a row that is out still play:
// this is putting the front panel away, not unplugging the instrument.
//
// The switches are on the CONSOLE and not on the rows, because a switch
// that goes away with the thing it hides cannot bring it back.

// rowHiddenKey is where the put-away rows are remembered. Its own record
// rather than the rack layout's: that one lists which switches are ON, so an
// empty record means everything off, and the right default here is that
// every row is in the rack.
const rowHiddenKey = "wasmstuff-rackrows-out"

// rowSwitchID is a category's row switch.
func rowSwitchID(label string) string { return "row-" + categorySlug(label) + "-sw" }

// buildRowSwitches fills the Console's Rows group, one switch per category.
func buildRowSwitches() {
	host := dom.Doc.Call("getElementById", "row-switches")
	if !host.Truthy() {
		return
	}
	out := readHiddenRows()
	for _, label := range modelCategories() {
		lab := dom.Doc.Call("createElement", "label")
		lab.Set("className", "grp")
		lab.Get("style").Set("cursor", "pointer")
		what := catTooltips[label]
		if what == "" {
			what = label
		}
		lab.Set("title", what+"\n\nIn the rack, or put away. A row that is out costs nothing "+
			"to keep — no layout, no paint, no measurement — which is what the switch is "+
			"for: every model's knobs, always, is 213 controls and most of the panel's "+
			"cost. The models in a row that is out still play; this puts the front panel "+
			"away, not the instrument.")
		sw := dom.Doc.Call("createElement", "input")
		sw.Set("type", "checkbox")
		sw.Set("className", "sw")
		sw.Set("id", rowSwitchID(label))
		sw.Set("checked", !out[label])
		lab.Call("appendChild", sw)
		lab.Call("appendChild", dom.Doc.Call("createTextNode", " "+categoryTag(label)))
		host.Call("appendChild", lab)

		sw.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			applyRowVisibility()
			saveHiddenRows()
			quantizeModuleWidths() // the rack is a different size now
			return nil
		}))
	}
	applyRowVisibility()
}

// applyRowVisibility puts each row in or out to match its switch.
func applyRowVisibility() {
	for _, label := range modelCategories() {
		sw := dom.Doc.Call("getElementById", rowSwitchID(label))
		in := !sw.Truthy() || sw.Get("checked").Bool()
		mods := dom.Doc.Call("querySelectorAll", "[data-cat]")
		for i := 0; i < mods.Get("length").Int(); i++ {
			m := mods.Index(i)
			if m.Call("getAttribute", "data-cat").String() != label {
				continue
			}
			if in {
				m.Get("style").Set("display", "")
			} else {
				m.Get("style").Set("display", "none")
			}
		}
	}
}

// readHiddenRows is the set of categories that were put away.
func readHiddenRows() map[string]bool {
	out := map[string]bool{}
	v, ok := lsGet(rowHiddenKey)
	if !ok || v == "" {
		return out
	}
	for _, s := range strings.Split(v, ",") {
		if s != "" {
			out[s] = true
		}
	}
	return out
}

// saveHiddenRows records it, by label, so a category renamed comes back
// rather than staying out under a name nothing matches.
func saveHiddenRows() {
	var out []string
	for _, label := range modelCategories() {
		sw := dom.Doc.Call("getElementById", rowSwitchID(label))
		if sw.Truthy() && !sw.Get("checked").Bool() {
			out = append(out, label)
		}
	}
	lsSet(rowHiddenKey, strings.Join(out, ","))
}

// at places an element in a module's grid: a row, a starting column, and
// how many of each it spans.
//
// Explicit, not flowed. The grid's own auto-flow fills three cells and moves
// right, which is right for one generator's panel and useless for three:
// each would start wherever the last one ended, and a tile with a name over
// it has to start where the name does.
func at(el, grid js.Value, row, rowSpan, col, colSpan int) {
	st := el.Get("style")
	st.Set("gridRow", strconv.Itoa(row)+" / span "+strconv.Itoa(rowSpan))
	st.Set("gridColumn", strconv.Itoa(col)+" / span "+strconv.Itoa(colSpan))
	grid.Call("appendChild", el)
}
