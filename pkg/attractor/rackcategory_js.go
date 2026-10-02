//go:build js && wasm

package attractor

import (
	"slices"

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

// categoryOffLabel is OFF: the category ring's position at the bottom, and the
// first of the model selector's on a bay that has no category ring.
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
	if dom.Doc.Call("getElementById", categoryHeadID(rackRows()[0])).Truthy() {
		return // already built
	}
	claimed := map[string]bool{}
	for _, label := range rackRows() {
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
	declarers := map[string][]string{} // which models declare each parameter
	for _, mode := range own {
		for _, p := range attractorParams[mode] {
			shared[p.ID]++
			declarers[p.ID] = append(declarers[p.ID], mode)
		}
	}

	// Build every cell first, and count what each generator owns, so the
	// packer works on the real shapes rather than on the declarations.
	cells := map[string][]js.Value{}
	steps := map[string]js.Value{}
	var sharedCells []js.Value
	var hidden []js.Value // set from another position's buttons (bankHidden)
	var gens []gentile.Spec
	var bare []string // models with no constants of their own
	for _, mode := range own {
		lone := -1 // the position of a switch still waiting for a partner
		for _, p := range attractorParams[mode] {
			if claimed[p.ID] {
				continue
			}
			claimed[p.ID] = true
			switch {
			case bankCategories[label] && bankHidden[p.ID]:
				hidden = append(hidden, buildCategoryParamCell(mode, p))
			case strings.HasSuffix(p.ID, "-dt"):
				steps[mode] = buildCategoryStepCell(mode, p)
			case shared[p.ID] > 1:
				sc := buildCategoryParamCell(mode, p)
				// In a bank it is a shared cell, shown for the models that
				// declare it and no others (see buildBankRow).
				if bankCategories[label] {
					sc.Call("setAttribute", "data-bank-for", strings.Join(declarers[p.ID], " "))
				}
				sharedCells = append(sharedCells, sc)
			// A bank position is one control, so switches are not paired there.
			case len(paramLabels[p.ID]) == 2 && lone >= 0 && !bankCategories[label]:
				cells[mode][lone] = pairSwitchCells(cells[mode][lone], buildCategoryParamCell(mode, p))
				lone = -1
			case bankCategories[label] && mode != own[0]:
				// Built when the model is first shown (banklazy_js.go); the
				// first model's are built now, for the bank's blanks to copy.
				cells[mode] = append(cells[mode], bankPlaceholder(mode, p))
			default:
				if len(paramLabels[p.ID]) == 2 {
					lone = len(cells[mode])
				}
				cells[mode] = append(cells[mode], buildCategoryParamCell(mode, p))
			}
		}
		if len(cells[mode]) == 0 {
			bare = append(bare, mode)
		}
		gens = append(gens, gentile.Spec{Mode: mode, Constants: len(cells[mode])})
	}

	// A bank category is one bay: the head and a bank of programmable
	// knobs, rather than a card per model. See buildBankModule.
	if bankCategories[label] {
		// With each model's constants, the selectors its own panel used to
		// carry (modelparts_js.go).
		for _, mode := range own {
			cells[mode] = append(cells[mode], modelSelectorCells(mode)...)
		}
		// And Model Out's knobs (sonify_js.go), at the top of the last column
		// for every model with equations or a trail to play (bankTail).
		var heard []string
		for _, m := range own {
			if isAttractorMode(m) {
				heard = append(heard, m)
			}
		}
		var mo []js.Value
		if len(heard) > 0 {
			for _, p := range modelOutParams {
				c := buildCategoryParamCell(heard[0], p)
				c.Call("setAttribute", "data-bank-for", strings.Join(heard, " "))
				c.Set("title", helpFor(p.ID))
				mo = append(mo, c)
			}
		}
		return buildBankRow(label, own, cells, steps, sharedCells, hidden, mo)
	}

	// How wide this category's heads are. Every bay spends catHeadCols on
	// the monitor, the rotary and the step; the FIRST one also carries the
	// parameters that belong to the category rather than to any of its
	// models, so it is wider. The packer is given the tighter figure, which
	// leaves the later bays a column of slack rather than letting the first
	// one overhang its rack.
	// With no step anywhere in the category, the first shared parameter
	// takes the step's position (see buildBayHead) and needs no column.
	inHead := len(sharedCells)
	if len(steps) == 0 && inHead > 0 {
		inHead--
	}
	headCols := catHeadCols + (inHead+catCellsPerCol-1)/catCellsPerCol
	mods := gentile.Pack(gens, catColsPerBay-headCols)

	// Modules into bays: as many as fit beside a head.
	type bayPlan struct {
		mods  []gentile.Module
		modes []string
	}
	plan := []bayPlan{{}}
	used := 0
	for _, m := range mods {
		cur := &plan[len(plan)-1]
		if used+m.Cols > catColsPerBay-headCols && len(cur.mods) > 0 {
			plan = append(plan, bayPlan{})
			cur, used, headCols = &plan[len(plan)-1], 0, catHeadCols
		}
		cur.mods = append(cur.mods, m)
		for _, t := range m.Tiles {
			cur.modes = append(cur.modes, t.Mode)
		}
		used += m.Cols
	}

	// A model with no constants has no module to be packed on, but it is
	// still one of the category's generators and has to be on a selector:
	// the Sprott systems have a step and nothing else, and a polyhedron has
	// nothing at all. Each goes in the bay of the model the catalog lists
	// before it, so Sprott A sits with the morph that moves between them,
	// and a category with no constants anywhere gets them all on its one bay.
	bayOfMode := map[string]int{}
	for i, b := range plan {
		for _, m := range b.modes {
			bayOfMode[m] = i
		}
	}
	prev, after := 0, ""
	for _, mode := range own {
		if i, ok := bayOfMode[mode]; ok {
			prev, after = i, mode
			continue
		}
		if !slices.Contains(bare, mode) {
			continue
		}
		b := &plan[prev]
		at := slices.Index(b.modes, after) + 1 // 0 when nothing precedes it
		b.modes = slices.Insert(b.modes, at, mode)
		bayOfMode[mode], after = prev, mode
	}

	var out []js.Value
	for bay, b := range plan {
		out = append(out, buildBayHead(label, bay, b.modes, steps, sharedCells, js.Value{}))
		sharedCells = nil // the first bay of a category carries them
		for _, m := range b.mods {
			out = append(out, buildGenPanel(label, m, cells))
		}
		for _, f := range modelFamilies {
			if slices.ContainsFunc(b.modes, func(m string) bool { return familyOf(m) == f }) {
				out = append(out, buildFamilyModule(label, f))
			}
		}
	}
	return out
}

// ── Families ────────────────────────────────────────────────────────────
//
// A family is a run of models that differ only in which member of a
// catalog they are: J. C. Sprott's nineteen simple systems, A to S. On the
// bay's MODEL knob they were nineteen positions of twenty-four, so reaching
// Chen meant turning past all of them. Now they are ONE position, and a
// lettered dial of their own picks which system that position plays.

// modelFamily is one such run.
type modelFamily struct {
	Key     string
	Label   string   // what the MODEL position reads
	Members []string // the models, in dial order
	Letters []string // the dial's ring, one per member

	sel     js.Value   // the family's own dial
	options []js.Value // the MODEL position that stands for it, per bay
	chosen  string     // the member the dial is set to
}

// modelFamilies is every family; Sprott's is derived from the catalog so a
// system added there joins the dial without being listed twice.
var modelFamilies = func() []*modelFamily {
	f := &modelFamily{Key: "sprott", Label: "Sprott A–S"}
	for _, g := range modeGroups {
		for _, m := range g.Keys {
			if len(m) == len("sprotta") && strings.HasPrefix(m, "sprott") && !slices.Contains(f.Members, m) {
				f.Members = append(f.Members, m)
				f.Letters = append(f.Letters, strings.ToUpper(m[len("sprott"):]))
			}
		}
	}
	return []*modelFamily{f}
}()

// familyOf is the family a model belongs to, or nil.
func familyOf(mode string) *modelFamily {
	for _, f := range modelFamilies {
		if slices.Contains(f.Members, mode) {
			return f
		}
	}
	return nil
}

// current is the member the family's MODEL position plays.
func (f *modelFamily) current() string {
	if f.chosen == "" && len(f.Members) > 0 {
		f.chosen = f.Members[0]
	}
	return f.chosen
}

// choose points the family at a member: its dial and every MODEL position
// that stands for it. It does not change what is running.
func (f *modelFamily) choose(mode string) {
	if !slices.Contains(f.Members, mode) {
		return
	}
	f.chosen = mode
	// The MODEL position reads which member it plays: that window is a
	// readout, so it may say "Sprott C" where the ring says nothing.
	text := strings.Fields(f.Label)[0] + " " + f.Letters[slices.Index(f.Members, mode)]
	for _, o := range f.options {
		o.Set("value", mode)
		o.Set("textContent", text)
	}
	if f.sel.Truthy() && f.sel.Get("value").String() != mode {
		f.sel.Set("value", mode)
		f.sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		f.sel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
}

// buildFamilyModule is the family's lettered dial, one slot beside the
// generators of the bay it is in. Turning it while a member is running
// switches to the member it lands on; otherwise it only sets which member
// the MODEL position will play.
func buildFamilyModule(label string, f *modelFamily) js.Value {
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid")
	at(buildFamilyCell(f), grid, 1, 1, 1, 1)
	return wrapCategoryModule(label, "fam-"+f.Key+"-module", categoryTag(f.Label), grid,
		docf("fam-module", "family", f.Label))
}

// buildFamilyCell is the family's dial as one control position: a card row
// mounts it as a module of its own, a bank as one of its selectors.
func buildFamilyCell(f *modelFamily) js.Value {
	sel := buildFamilySelect(f)
	cell := dom.Doc.Call("createElement", "div")
	cell.Set("className", "punit")
	cell.Set("title", docf("fam-cell", "family", f.Label))
	top := dom.Doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", "u-lbl")
	lbl.Set("textContent", "sys")
	top.Call("appendChild", lbl)
	cell.Call("appendChild", top)
	cell.Call("appendChild", sel)
	// Single letters, so the ring holds all nineteen: the fit rule's limit
	// is for words, which is what collides round a dial.
	cell.Call("appendChild", singleSelectorKnob(sel, f.Letters))
	return cell
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
		// The name goes on the first position that is a single control. A
		// switch pair has a label at the left of each half, and the name set
		// down that same edge ran through both of them.
		tagAt := 0
		for j := 0; j < t.N && j < len(own); j++ {
			if !own[j].Get("classList").Call("contains", "swpair").Bool() {
				tagAt = j
				break
			}
		}
		for j := 0; j < t.N && j < len(own); j++ {
			pos := t.Start + j
			col, row := m.ColRow(pos)
			c := own[j]
			c.Call("setAttribute", "data-mgroup", strconv.Itoa(i%3))
			markGroupEdges(c, m, t, pos)
			if j == tagAt {
				c.Call("appendChild", genGroupTag(name))
			}
			at(c, grid, 1+row, 1, 1+col, 1)
		}
	}
	return wrapCategoryModule(label, genModuleID(m), strings.Join(names, " · "), grid,
		docf("gen-unit", "count", strconv.Itoa(len(m.Tiles)), "names", strings.Join(names, ", ")))
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
//
// Given a bank's grid, the rotary and the step go to the head of the bank
// instead, at the top of its first column and at its foot, and the row
// under the monitor is the running model's equation (eqSlotID).
func buildBayHead(label string, bay int, modes []string, steps map[string]js.Value, extra []js.Value, bank js.Value) js.Value {
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid catgrid cathead")
	at(buildCategoryMonitor(label, bay), grid, 1, catMonitorRows, 1, catHeadCols)
	rotary, rotRow, rotCol := grid, 1+catMonitorRows, 1
	stepRow, stepCol := 1+catMonitorRows, 2
	if bank.Truthy() {
		rotary, rotRow, rotCol = bank, bankRotaryRow, 1
		stepRow, stepCol = bankStepRow, 1
		slot := dom.Doc.Call("createElement", "div")
		slot.Set("className", "eqslot")
		slot.Set("id", eqSlotID(label))
		mountModelReadouts(slot) // over the equation: modelparts_js.go
		at(slot, grid, 1+catMonitorRows, 1, 1, catHeadCols)
	}
	at(buildBayRotary(label, bay, modes), rotary, rotRow, 1, rotCol, 1)

	// Every step cell in the bay goes in the same position. Only one is ever
	// shown — the one belonging to the model the bay is set to — so one
	// position is what the panel needs, and a knob that re-ranges with the
	// selector is what it has always been from the front.
	stepped := false
	for _, m := range modes {
		if s, ok := steps[m]; ok {
			at(s, rotary, stepRow, 1, stepCol, 1)
			stepped = true
		}
	}

	// The category's own parameters — the ones more than one of its models
	// declare — go in columns of their own, on the first bay only. A bay
	// whose models have no step (a polyhedron is built, not integrated)
	// has that position free, and the first of them takes it rather than
	// opening a column beside an empty one.
	if !stepped && len(extra) > 0 {
		at(extra[0], grid, 1+catMonitorRows, 1, 2, 1)
		extra = extra[1:]
	}
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

// categoryOwnModes is the models filed under this row — a category, or every
// category of a merged row, in catalog order — whose constants it carries.
func categoryOwnModes(label string) []string {
	var out []string
	for _, cat := range rowCategories(label) {
		for _, mode := range categoryModes(cat) {
			if categoryOf(mode) == cat && !slices.Contains(out, mode) {
				out = append(out, mode)
			}
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
		tip = docf("cat-head", "what", what)
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
	// Which parameter, for the buttons beside it (rackbuttons_js.go).
	unit.Call("setAttribute", "data-param", p.ID)
	// WHICH MODEL's constant this is. A row carries up to fifty-five cells
	// and the label on one of them is a symbol: Chen's a and Lu's a are the
	// same letter over two different knobs, and only the model tells them
	// apart. The name is on the cell rather than in the label because the
	// label is one glyph wide by design.
	name := mode
	if info, ok := modeInfo[mode]; ok && info.Label != "" {
		name = info.Label
	}
	// And what it does, where the parameter has a sentence: the label is one
	// glyph or an abbreviation, and the cell's tooltip is where it is spelled
	// out (the legend display in a bank has none of its own).
	tip := name + " — " + p.Label
	if h := helpFor(p.ID); h != "" {
		tip = name + " — " + h
	}
	unit.Set("title", tip)
	return unit
}

// pairSwitchCells mounts two of a model's switches in one control position.
//
// A switch is a toggle and a word, and a whole position built for a knob
// leaves most of it blank: the globe's par and dir took two positions and
// with them a fourth column on the Globe · Torus panel. Stacked, they take
// one. Each half is still a cell of its own, so its label, reset, tooltip
// and live marking are what they were; the pair is only where it sits.
func pairSwitchCells(a, b js.Value) js.Value {
	pair := dom.Doc.Call("createElement", "div")
	pair.Set("className", "swpair")
	pair.Call("appendChild", a)
	pair.Call("appendChild", b)
	return pair
}

// buildCategoryStepCell is a model's integration step, for the position in
// the bay head that shows the step of whichever model the bay is set to.
//
// Labeled "dt", the same on every model's cell. It was labeled with the
// model's name, which made the legend change as the MODEL knob turned — and
// a legend is printed on the panel: only a readout may change. Which model's
// step it is, the MODEL knob and the monitor already say.
func buildCategoryStepCell(mode string, p paramDef) js.Value {
	unit := buildCategoryParamCell(mode, p)
	unit.Get("classList").Call("add", "stepcell")
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
		if slices.Contains(rackBays[i].Modes, mode) {
			return &rackBays[i]
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

	// The tooltip is the screen's and not the unit's: the screen is a part
	// with an address of its own (designate), and the switches under it
	// have theirs.
	bez := dom.Doc.Call("createElement", "span")
	bez.Set("className", "monbezel")
	bez.Set("title", doc("bay-monitor"))
	cv := dom.Doc.Call("createElement", "canvas")
	cv.Set("id", bayMonitorID(label, bay))
	cv.Set("width", catMonitorWidth)
	cv.Set("height", catMonitorHigh)
	bez.Call("appendChild", cv)
	// A bank bay's screen also carries the running model's Lyapunov
	// exponent, as a scope carries its measurements: a readout of the picture
	// on the picture, rather than a module of its own holding one number.
	if bankCategories[label] {
		ro := dom.Doc.Call("createElement", "span")
		ro.Set("className", "monread")
		ro.Set("id", bayMonitorID(label, bay)+"-lyap")
		bez.Call("appendChild", ro)
	}
	unit.Call("appendChild", bez)

	row := dom.Doc.Call("createElement", "span")
	row.Set("className", "monsw")
	row.Call("appendChild", screenSwitch(bayMonSwitchID(label, bay)))
	if bankCategories[label] {
		buildModelSwitches(row) // what each does is the model's: modelparts_js.go
	}
	unit.Call("appendChild", row)
	return unit
}

// screenSwitch is the monitor's power switch: the same part as the model's
// switches beside it, a dot-matrix legend over a switch, so the row under
// the screen reads as one row of switches.
func screenSwitch(id string) js.Value {
	lab := dom.Doc.Call("createElement", "label")
	lab.Set("className", "modsw")
	lab.Call("setAttribute", "data-no-drag", "")
	lab.Set("title", doc("bay-screen"))
	lab.Call("appendChild", dotDisplayN("scrn", false, legendChars))
	sw := dom.Doc.Call("createElement", "input")
	sw.Set("type", "checkbox")
	sw.Set("className", "sw")
	sw.Set("id", id)
	sw.Set("checked", true)
	lab.Call("appendChild", sw)
	return lab
}

// buildBayRotary is the cell that picks which of this bay's generators plays.
func buildBayRotary(label string, bay int, modes []string) js.Value {
	rackBays = append(rackBays, bayRecord{Label: label, N: bay, Modes: modes})

	cell := dom.Doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell catcell")
	cell.Call("setAttribute", "data-no-drag", "")
	cell.Set("title", doc("bay-model-cell"))

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
	sel.Set("title", doc("bay-model"))

	bay2 := dom.Doc.Call("createElement", "span")
	bay2.Set("className", "grp vmbay")
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "catsel")

	// A knob AND a list, which is what the Console's model selector was. A
	// dial alone is the wrong control for a long list: reaching the last of
	// them means dragging through all the others while the rack re-renders
	// at each. The list is how you go somewhere; the knob is how you walk.
	knob := selk.makeSelectorKnob(sel)
	var stack js.Value
	if cats := rowCategories(label); len(cats) > 1 {
		// A merged row: the outer ring picks the category, the inner ring the
		// model within it. The inner ring only ever holds one category's
		// models, so walking it is walking a short list, and the outer ring's
		// legends are printed because the categories never change.
		catSel := dom.Doc.Call("createElement", "select")
		catSel.Set("id", bayCatSelectID(label, bay))
		catSel.Set("title", doc("bay-category"))
		catSel.Get("style").Set("display", "none")
		// OFF is the category ring's, at the bottom, and the ring is endless
		// (selEndless): turned on past the last category it comes round to
		// OFF and on to the first. OFF was the inner ring's first position,
		// where it sat among the models as though it were one.
		catSel.Call("setAttribute", "data-endless", "")
		tags := []string{categoryOffLabel}
		off := dom.Doc.Call("createElement", "option")
		off.Set("value", "")
		off.Set("textContent", categoryOffLabel)
		catSel.Call("appendChild", off)
		for _, c := range cats {
			o := dom.Doc.Call("createElement", "option")
			o.Set("value", c)
			o.Set("textContent", c)
			catSel.Call("appendChild", o)
			tags = append(tags, categoryRingTag(c))
		}
		catSel.Set("value", cats[0])
		fillBayOptions(sel, modesOfCategory(modes, cats[0]), false)
		stack = stackKnobs(selk.makeSelectorKnob(catSel), knob)
		addSelectorLabels(stack, tags, catSel)
		stack.Call("setAttribute", "title", doc("bay-rings"))
		wrap.Call("appendChild", catSel)
		lb, nb := label, bay
		selOverflow[baySelectID(label, bay)] = func(dir int) { onBayModelOverflow(lb, nb, dir) }
		catSel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
			onBayCategory(lb, nb)
			return nil
		}))
	} else {
		fillBayOptions(sel, modes, true)
		stack = dom.Doc.Call("createElement", "span")
		stack.Set("className", "knobstack")
		stack.Call("setAttribute", "data-no-drag", "")
		knob.Get("classList").Call("add", "knob-ring")
		stack.Call("appendChild", knob)
	}
	wrap.Call("appendChild", stack)
	wrap.Call("appendChild", sel)
	// A merged bay's display opens every model in the bay, by category
	// (bayPicker); the knob's own select, one category's worth, is hidden.
	picker := sel
	if cats := rowCategories(label); len(cats) > 1 {
		picker = bayPicker(label, bay, sel, cats, modes)
		sel.Get("style").Set("display", "none")
		wrap.Call("appendChild", picker)
	}
	bay2.Call("appendChild", wrap)
	cell.Call("appendChild", bay2)
	attachSelMarquee(sel, picker)

	lb, nb := label, bay
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		onBayRotary(lb, nb)
		return nil
	}))
	return cell
}

// bayCatSelectID is a merged bay's category ring.
func bayCatSelectID(label string, n int) string { return bayID(label, n) + "-cat" }

// categoryRingTag is a category's name as the category ring prints it: four
// letters, which is what fits between two detents.
func categoryRingTag(cat string) string {
	r := []rune(strings.ToLower(categoryTag(cat)))
	return string(r[:min(4, len(r))])
}

// modesOfCategory is the models of modes filed under cat, in their order.
func modesOfCategory(modes []string, cat string) []string {
	var out []string
	for _, m := range modes {
		if categoryOf(m) == cat {
			out = append(out, m)
		}
	}
	return out
}

// fillBayOptions puts a bay selector's positions in: OFF when withOff (a bay
// with no category ring, which has nowhere else for it), then the models,
// a family standing as one position whose value is the member its dial is
// set to (see modelFamily).
func fillBayOptions(sel js.Value, modes []string, withOff bool) {
	sel.Set("innerHTML", "")
	for _, f := range modelFamilies {
		f.options = slices.DeleteFunc(f.options, func(o js.Value) bool { return !o.Get("parentNode").Truthy() })
	}
	add := func(value, text string) js.Value {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", value)
		o.Set("textContent", text)
		sel.Call("appendChild", o)
		return o
	}
	if withOff {
		add("", categoryOffLabel)
	}
	seen := map[*modelFamily]bool{}
	for _, m := range modes {
		if f := familyOf(m); f != nil {
			if !seen[f] {
				seen[f] = true
				o := add(f.current(), f.Label)
				o.Call("setAttribute", "data-family", f.Key)
				f.options = append(f.options, o)
				f.choose(f.current()) // the position reads its member
			}
			continue
		}
		add(m, modeLabel(m))
	}
}

// bayCatModel is the model each category of a merged bay was last set to, so
// turning the category ring away and back returns to it.
var bayCatModel = map[string]string{}

// onBayCategory follows a merged bay's category ring: the inner ring gets
// that category's models and the bay switches to the one it was last on.
func onBayCategory(label string, bay int) {
	if catRotarySyncing {
		return
	}
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(label, bay))
	sel := dom.Doc.Call("getElementById", baySelectID(label, bay))
	if !catSel.Truthy() || !sel.Truthy() {
		return
	}
	cat := catSel.Get("value").String()
	if cat == "" {
		// OFF, at the bottom of the ring, and it means off: the rack powers
		// down, which is what OFF on the model selector has always meant.
		setPowerState(false)
		setPowerSwitch(false)
		syncCategoryRotaries()
		return
	}
	var modes []string
	for _, r := range rackBays {
		if r.Label == label && r.N == bay {
			modes = modesOfCategory(r.Modes, cat)
		}
	}
	if len(modes) == 0 {
		return
	}
	fillBayOptions(sel, modes, false)
	m := bayCatModel[bayID(label, bay)+"/"+cat]
	if m == "" {
		m = modes[0]
		if f := familyOf(m); f != nil {
			m = f.current()
		}
	}
	sel.Set("value", m)
	sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	sel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
}

// onBayModelOverflow is a merged bay's model ring turned past its last model
// (dir +1) or before its first (-1): it goes on into the next category's first
// model, or the previous one's last, as the category ring follows. Past the
// last category, or back before the first, it comes to OFF, where the category
// ring has it, and from OFF on to the first model of the first category or the
// last of the last: the two rings walk one order.
func onBayModelOverflow(label string, bay int, dir int) {
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(label, bay))
	if !catSel.Truthy() {
		return
	}
	var bayModes []string
	for _, r := range rackBays {
		if r.Label == label && r.N == bay {
			bayModes = r.Modes
		}
	}
	cats := rowCategories(label)
	at := slices.Index(cats, catSel.Get("value").String())
	if at < 0 && dir < 0 {
		at = len(cats) // from OFF, backwards, to the last category
	}
	for range cats {
		at += dir
		if at < 0 || at >= len(cats) {
			catSel.Set("value", "") // OFF (onBayCategory powers down)
			catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			catSel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			return
		}
		modes := modesOfCategory(bayModes, cats[at])
		if len(modes) == 0 {
			continue
		}
		m := modes[0]
		if dir < 0 {
			m = modes[len(modes)-1]
		}
		if f := familyOf(m); f != nil {
			m = f.current()
		}
		// Where onBayCategory puts the model ring, as it would a category's
		// remembered model.
		bayCatModel[bayID(label, bay)+"/"+cats[at]] = m
		catSel.Set("value", cats[at])
		catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		catSel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return
	}
}

// setBayCategory turns a merged bay's category ring to cat without driving
// anything, and gives the inner ring that category's models.
func setBayCategory(b bayRecord, cat string) {
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(b.Label, b.N))
	if !catSel.Truthy() || catSel.Get("value").String() == cat {
		return
	}
	catSel.Set("value", cat)
	catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	catSel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	if sel := dom.Doc.Call("getElementById", baySelectID(b.Label, b.N)); sel.Truthy() {
		fillBayOptions(sel, modesOfCategory(b.Modes, cat), false)
	}
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
	bayCatModel[bayID(label, bay)+"/"+categoryOf(mode)] = mode
	setPowerState(true)
	setPowerSwitch(true)
	if mode == editMode() {
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
	// The model the panel is on: the running one, or the back layer's while
	// that is being edited (backlayer_js.go).
	m := editMode()
	setActiveCategory(m)
	// A running family member points its family at itself first, so the
	// MODEL position that stands for it has the value about to be set.
	if f := familyOf(m); f != nil {
		f.choose(m)
	}
	live := bayOf(m)
	if live != nil && m != "" && !run.stopped {
		bayModel[bayID(live.Label, live.N)] = m
	}
	for _, b := range rackBays {
		sel := dom.Doc.Call("getElementById", baySelectID(b.Label, b.N))
		if !sel.Truthy() {
			continue
		}
		// Powered down, every selector reads off: no bay is driving,
		// because nothing is being drawn.
		want := ""
		if live != nil && b.Label == live.Label && b.N == live.N && !run.stopped {
			want = m
			// A merged bay turns its category ring to the running model's
			// category first, so the inner ring holds the model to show.
			setBayCategory(b, categoryOf(m))
		}
		if want == "" {
			// A merged bay says OFF on its category ring, where OFF is; its
			// model ring, which has no OFF, points at nothing and reads off.
			if catSel := dom.Doc.Call("getElementById", bayCatSelectID(b.Label, b.N)); catSel.Truthy() {
				if catSel.Get("value").String() != "" {
					catSel.Set("value", "")
					catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
					catSel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
				}
				if sel.Get("selectedIndex").Int() >= 0 {
					sel.Set("selectedIndex", -1)
					sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
				}
				continue
			}
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
	// The model lists follow too: a select already on its model above was
	// skipped, and fired nothing for its list to follow.
	for _, follow := range bayPickerFollow {
		follow()
	}
	syncBankCells()
	lightLiveParamCells()
}

// lightLiveParamCells marks the cells of the model that is running.
//
// Two hundred and thirteen knobs are on the rack and three of them are doing
// something. Without this the rows are a reference rather than an instrument:
// you can read every model's settings and not see which ones are live.
func lightLiveParamCells() {
	// Nothing is lit while the rack is powered down, which is the same answer
	// the rotaries give: no model is running, so no front panel on the rack is
	// the one in the signal path. A bank's shared cell is the running model's
	// too if it is one of the models the cell serves. One pass in the page:
	// from here it was a few calls a cell over every model's cells.
	model := editMode()
	if run.stopped {
		model = ""
	}
	fastDOM().Call("lightLive", model)
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

// buildFamilySelect is the family's dial as a control: a select of its
// members, lettered, that switches to the member it lands on when a member is
// running and otherwise only sets which one the MODEL position will play.
// Mounted as a module of its own beside a card row, or as the inner ring of
// a bank bay's MODEL knob.
func buildFamilySelect(f *modelFamily) js.Value {
	sel := dom.Doc.Call("createElement", "select")
	sel.Set("id", "fam-"+f.Key)
	sel.Set("title", docf("fam-select", "family", f.Label))
	sel.Get("style").Set("display", "none")
	for i, m := range f.Members {
		o := dom.Doc.Call("createElement", "option")
		o.Set("value", m)
		o.Set("textContent", f.Letters[i])
		o.Set("title", modeLabel(m))
		sel.Call("appendChild", o)
	}
	sel.Set("value", f.current())
	f.sel = sel
	sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		if catRotarySyncing {
			return nil
		}
		m := sel.Get("value").String()
		f.choose(m)
		if em := editMode(); familyOf(em) == f && m != em && !run.stopped {
			if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
				ms.Set("value", m)
				ms.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			}
		}
		return nil
	}))

	return sel
}

// bayPickerFollow is each model list's way of catching up with its knob.
var bayPickerFollow = map[string]func(){}

// bayPicker is the list a merged bay's model display opens: every model in
// the bay under its category's name, the categories' names separators that
// cannot be chosen, and beside each model how many of the bay's controls it
// has (bankControls), so a model can be weighed before it is switched to.
// Choosing one turns both rings to it, as the knob would; sel is the knob's
// own select, which it follows.
func bayPicker(label string, bay int, sel js.Value, cats, modes []string) js.Value {
	p := dom.Doc.Call("createElement", "select")
	p.Set("id", baySelectID(label, bay)+"-all")
	p.Set("className", "selwin")
	p.Set("title", doc("bay-model-all"))
	// OFF first, as on the rings: the bay driving nothing.
	off := dom.Doc.Call("createElement", "option")
	off.Set("value", "")
	off.Set("textContent", categoryOffLabel)
	p.Call("appendChild", off)
	for _, c := range cats {
		g := dom.Doc.Call("createElement", "optgroup")
		g.Set("label", c)
		fillBayOptions(g, modesOfCategory(modes, c), false)
		p.Call("appendChild", g)
	}
	// Follows the knob, whichever moved it; a family's entry follows the
	// member it is on, and so does its count. syncCategoryRotaries calls it
	// too, for the moves that fire nothing.
	bayPickerCounts(p, false)
	follow := func() {
		bayPickerCounts(p, true)
		if sel.Truthy() && sel.Get("selectedIndex").Int() >= 0 {
			p.Set("value", sel.Get("value").String())
		} else {
			p.Set("value", "") // OFF
		}
	}
	follow()
	bayPickerFollow[p.Get("id").String()] = follow
	if sel.Truthy() {
		sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
			follow()
			return nil
		}))
	}
	p.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		onBayPick(label, bay, p.Get("value").String())
		return nil
	}))
	return p
}

// bayPickerCounts writes each model's control count after its name, the
// names padded to one width so the counts stand in a column. A family's
// entry is whichever member its dial is on, so its count can change; with
// families, only theirs are rewritten.
func bayPickerCounts(p js.Value, families bool) {
	opts := p.Call("querySelectorAll", "optgroup option")
	if !families {
		// The first time: every name kept, and the width they pad to.
		w := 0
		for i := range opts.Length() {
			o := opts.Index(i)
			name := o.Get("textContent").String()
			o.Call("setAttribute", "data-name", name)
			w = max(w, len([]rune(name)))
		}
		p.Call("setAttribute", "data-w", strconv.Itoa(w))
	} else {
		opts = p.Call("querySelectorAll", "option[data-family]")
	}
	w, _ := strconv.Atoi(p.Call("getAttribute", "data-w").String()) //nolint:errcheck // written just above, from an int
	for i := range opts.Length() {
		o := opts.Index(i)
		name := o.Call("getAttribute", "data-name").String()
		pad := strings.Repeat(" ", w-len([]rune(name))+2)
		o.Set("textContent", name+pad+strconv.Itoa(bankControls[o.Get("value").String()]))
	}
}

// onBayPick turns a merged bay to model m from its list: the category ring
// to m's category, which brings the model ring with it to m; or, for OFF,
// the category ring to OFF, which powers the rack down.
func onBayPick(label string, bay int, m string) {
	sel := dom.Doc.Call("getElementById", baySelectID(label, bay))
	catSel := dom.Doc.Call("getElementById", bayCatSelectID(label, bay))
	if !sel.Truthy() || !catSel.Truthy() {
		return
	}
	cat := ""
	if m != "" {
		cat = categoryOf(m)
		bayCatModel[bayID(label, bay)+"/"+cat] = m
	}
	if catSel.Get("value").String() != cat {
		catSel.Set("value", cat)
		catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		catSel.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return
	}
	sel.Set("value", m)
	sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
}
