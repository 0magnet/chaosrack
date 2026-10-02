package attractor

import (
	"slices"
	"strings"

	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Which bay a module belongs in.
//
// A rack is not a shelf. Woodson & Conover's panel-layout procedure (Human
// Engineering Guide for Equipment Designers, 2nd ed., §2-131) starts by
// itemizing the components BY RELATED GROUP and only then arranging them,
// and §2-132 says the positions within a group are "determined by the
// sequence in which they are to be read, that is, the operator should be
// able to read in order of sequence from left to right or from the top of
// the panel to the bottom".
//
// chaosrack had neither. Modules sat in whatever order they were declared
// in and packed into units by width alone, so a bay held whatever happened
// to fit — an analyzer beside an oscillator beside the palette. The
// sections below are the blocks of manual/routing.md, in the order the
// signal travels through them, and a bay now holds one section.

// The sections. A unit holds as many as fit; a section too wide for a unit
// continues into the next.
const (
	secConsole = "console" // the master section: model choice, global acts, capture
	secMod     = "mod"     // routing: the Mixer, and the modulation matrix and its EQ
	secModel   = "model"   // the instrument proper and its per-mode panels
	secDisplay = "display" // how the picture is drawn: pose, color, grid, style
	secGen     = "gen"     // the signal sources: oscillators, keys, sequencers
	secUtility = "utility" // where a module nobody has placed lands, to be noticed
)

// The rack opens with the instrument: the model rows, with the running
// model's own panels after them, then the controls for how a model is drawn
// and the console. Below them is the auditory half, the generators that make
// signals, the sequencers that play them and the meters that measure them,
// headed by the scope, because it is both: a picture whose axes are two
// signals. Its tube and knobs come first in the GENERATORS bay, then the
// oscillators, which is where the thing the generators are most often
// patched into is.

// bayOpeningSections start a bay of their own rather than filling the end of
// the one before. GENERATORS, so the scope has a line to sit on and the
// oscillators are not packed into the model row's spare slots.
//
// METERING was one too, and is empty now: its four meters filled seven slots
// of a twelve-slot bay, while the Console and Generators bays each had five
// spare, so the meters went to those two and the rack lost a bay.
var bayOpeningSections = map[string]bool{secGen: true}

// bayLeadModules each start a bay: an instrument wide enough to be one, the
// keyboard's synthesizer, the keyboard and the tone matrix, rather than two
// things half-way along a row of generators.
var bayLeadModules = map[string]bool{"synth": true, "keys": true, "matrix": true}

// sectionOrder is every bay in the order the rack reads, top to bottom, until
// somebody moves one by its screws (bayorder.go):
//
//	the model rows, then the running model's own panels: the instrument first
//	DISPLAY: the capture monitor and what places and colors the picture
//	the console
//	GENERATORS (the scope's bay, with two of the meters), then ROUTING,
//	UTILITY
//
// Computed rather than written out, so a category added to modeGroups gets
// its row with no second list to keep in step.
var sectionOrder = buildSectionOrder()

func buildSectionOrder() []string {
	var out []string
	for _, c := range rackRows() {
		out = append(out, categorySection(c))
	}
	return append(out, secModel, secDisplay, secConsole, secGen, secMod, secUtility)
}

// sectionTitleOf is what is silkscreened on a bay, including the model
// rows, whose name is the category's own.
func sectionTitleOf(section string) string {
	if t, ok := sectionTitle[section]; ok {
		return t
	}
	if isCategorySection(section) {
		for _, c := range rackRows() {
			if categorySection(c) == section {
				return strings.ToUpper(c)
			}
		}
	}
	return ""
}

// sectionTitle is what is silkscreened on the bay.
var sectionTitle = map[string]string{
	secConsole: "CONSOLE",
	secMod:     "ROUTING",
	secModel:   "MODEL",
	secDisplay: "DISPLAY",
	secGen:     "GENERATORS",
	secUtility: "UTILITY",
}

// moduleSections maps a module's key — its header text, lowercased, which is
// what rack-go names it by — to the bay it belongs in.
//
// A table rather than a class on each element because the grouping is a
// statement about the INSTRUMENT, not about the markup: it is the same
// grouping manual/routing.md draws, and the two should be read together.
// A module missing from here is caught by a test rather than quietly
// landing in whatever bay it was declared next to.
var moduleSections = map[string]string{
	"console": secConsole,
	// The capture monitor leads the display bay: it carries a
	// screen, so it leads a bay wherever it goes, and at the left of that bay
	// it is the first thing read there. See moduleOrder.
	"record": secDisplay,
	// Saving and recalling the whole rack is the Console's job.
	"presets": secConsole,
	// Timing measures the INSTRUMENT, not the signal: how fast the rack
	// draws. (The Lyapunov readout, which said whether the running model is
	// chaotic, is on the Visual head now, with the model.) A global fact, at
	// the top where it is found without hunting.
	"timing": secConsole,
	// How the whole rack looks: interface size, knob faces, LED color, the
	// rack's metalwork. About the instrument, not about the picture.
	"style": secConsole,

	// The meters, together in the Console bay: the scope bay above the
	// generators is its tube, its knobs and the oscillators now, and the
	// meters that shared a bay with the test signals moved here, where the
	// Console and Timing already were (moduleOrder).
	"counter":       secConsole,
	"distortion":    secConsole,
	"loudness":      secConsole,
	"wow & flutter": secConsole,

	// The rack-wide patch panel: any audio feature to any knob, on either
	// side of the line. Global like the Console it sits beside.

	// The model's own controls (the demo models' panels are in its bank now).
	// secModel means "part of the instrument rather than of the rack", and
	// moduleSection turns that into the running model's category row.
	"parameters": secModel,
	"equation":   secModel,

	"grid":            secDisplay,
	"view":            secDisplay,
	"display":         secDisplay,
	"layers · colors": secDisplay,
	"spectro":         secDisplay,

	// Everything that MAKES a signal, under the scope that draws one.
	"scope 1": secGen,
	"scope 2": secGen,
	"scope 3": secGen,
	"scope 4": secGen,
	"gen 1":   secGen,
	"gen 2":   secGen,
	"gen 3":   secGen,
	"gen 4":   secGen,
	// The keyboard's synthesizer, a bay of its own above it (synth_js.go).
	"synth":        secGen,
	"string":       secGen,
	"fm":           secGen,
	"wave · noise": secGen,
	"filter":       secGen,
	"amp":          secGen,
	"keys":         secGen,
	"matrix":       secGen,

	// The routing bay: the Mixer, where every sound goes, and the Mod matrix,
	// where the rack's signal goes to the controls. Side by side, because
	// the one makes the signal the other reads.
	"mixer": secMod,
	"mod":   secMod,
}

// moduleSection is the bay a module belongs in. A module nobody has placed
// goes to UTILITY rather than to the front: an unplaced module is a mistake
// to notice, and the last bay is where it will be noticed without being in
// the way of the signal path.
func moduleSection(key string) string {
	if s, ok := moduleSections[key]; ok {
		// The model's own panels go wherever the model is, which is its
		// category's row. secModel in the table is a statement about what the
		// module IS — part of the instrument rather than of the rack — and
		// modelRowSection turns that into which row it is in today.
		if s == secModel {
			return modelRowSection()
		}
		return s
	}
	// A category row names itself: its module's header IS the category, so
	// there is nothing to write in the table above and nothing that can
	// disagree with modeGroups.
	for _, c := range rackRows() {
		if strings.EqualFold(c, key) {
			return categorySection(c)
		}
	}
	return secUtility
}

// sectionRank is a section's place in the stack, for sorting modules into
// the factory order.
func sectionRank(section string) int {
	for i, s := range sectionOrder {
		if s == section {
			return i
		}
	}
	return len(sectionOrder)
}

// The packer itself now lives in pkg/racksurface, which knows how to pack a
// rack and nothing about this one. What stays here is the part that IS about
// this one: which sections exist, what they are called, and which module is
// in which.
//
// It moved because there were two layouts. The page packed modules into bays
// with these rules and the terminal panel wrapped the same modules to the
// terminal's width, so the two surfaces disagreed about what a bay even was.
// One copy, used by both, is the fix — and aliases rather than wrappers, so
// every caller here reads as it did.
type packItem = racksurface.Item

type sectionRun = racksurface.Run

func unitSection(items []packItem, idx []int) string { return racksurface.SectionOf(items, idx) }

func packBySection(items []packItem, capacity int, monitor map[string]int) [][]int {
	return racksurface.Pack(markBayLeads(items), capacity, monitor)
}

// markBayLeads is the items with this rack's own breaks marked: the first
// module with any width in each of bayOpeningSections, and every one of
// bayLeadModules. Here rather than where the items are read off the page, so
// the page and every drawing of the rack (rackascii.go) pack the same bays.
func markBayLeads(items []packItem) []packItem {
	out := slices.Clone(items)
	opened := map[string]bool{}
	for i, it := range out {
		if bayLeadModules[it.Key] {
			out[i].Break = true
		}
		if bayOpeningSections[it.Section] && !opened[it.Section] && it.Slots > 0 {
			opened[it.Section] = true
			out[i].Break = true
		}
	}
	return out
}

func sectionRuns(items []packItem, idx []int) []sectionRun { return racksurface.Runs(items, idx) }

// bayMonitorSlots is what each section's built-in monitor occupies.
//
// Empty for now, and deliberately: the packer honors it and the invariant is
// tested, but a section only appears here once there is a monitor built to
// put in its bays. Filling it is what puts a screen on the left of every row;
// doing it one section at a time is what keeps that from being one change
// that moves every panel in the rack.
var bayMonitorSlots = map[string]int{}

// sectionOrderOf is the order the packer takes items in: section by section
// in sectionOrder, and anything in no known section last.
//
// Within a section the running model's own panels (a module the table files
// under secModel, such as Parameters or Loader) come after the category's
// own modules. Document order put Custom's Parameters BEFORE the Custom
// head, so the module packed into the bay before its head and the head
// started a row of its own.
func sectionOrderOf(items []packItem) []int {
	order := make([]int, 0, len(items))
	for _, s := range sectionOrder {
		for _, panel := range []bool{false, true} {
			from := len(order)
			for i, it := range items {
				if it.Section == s && (moduleSections[it.Key] == secModel) == panel {
					order = append(order, i)
				}
			}
			slices.SortStableFunc(order[from:], func(a, b int) int {
				return moduleRank(items[a].Key) - moduleRank(items[b].Key)
			})
		}
	}
	// Anything whose section is not in the order at all still gets placed,
	// at the end: losing a module is worse than putting it last.
	for i, it := range items {
		if sectionRank(it.Section) >= len(sectionOrder) {
			order = append(order, i)
		}
	}
	return order
}

// moduleOrder is the order within a section where it matters more than the
// order the modules were declared in. The display bay opens the rack: the
// capture monitor, then what places the model on screen (its pose, its
// position, how many views, how it is drawn), then how it is colored.
//
// The Console bay reads left to right from the rack itself (Console, Style,
// Presets) to the meters, with Timing, the rack's own meter, last. (Model
// Out, the model as sound, is in the model's own bank now: modelOutParams.)
//
// The scope bays are each scope beside the generator it starts on, and
// they are listed so they come before Keys and the Matrix, which open bays
// of their own.
//
// Anything not listed keeps its declared order, after these.
var moduleOrder = []string{
	"record", "view", "grid", "display", "layers · colors",
	"console", "style", "presets", "counter", "distortion", "loudness", "wow & flutter", "timing",
	"scope 1", "gen 1", "scope 2", "gen 2", "scope 3", "gen 3", "scope 4", "gen 4",
	"synth", "string", "fm", "wave · noise", "filter", "amp",
	"mixer", "mod",
}

// moduleRank is a module's place in moduleOrder, or after all of it.
func moduleRank(key string) int {
	if i := slices.Index(moduleOrder, key); i >= 0 {
		return i
	}
	return len(moduleOrder)
}
