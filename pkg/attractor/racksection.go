package attractor

import (
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
// sections below are the blocks of docs/signal-flow.md, in the order the
// signal travels through them, and a bay now holds one section.

// The sections, in signal order. A unit holds one of them; a section too
// wide for a unit continues into the next, and a new section always starts
// a fresh one — a bay with two sections in it is the thing this fixes.
const (
	secConsole = "console" // the master section: model choice and global acts
	secInput   = "input"   // what is being fed in, and the test generator
	secAnalyze = "analyze" // the metering bay
	secMod     = "mod"     // the CV matrix and its sources
	secModel   = "model"   // the instrument proper and its per-mode panels
	secDisplay = "display" // deflection, color, the tube, capture
	secOutput  = "output"  // the generators and the output bus
	secUtility = "utility" // presets, template: about the rack, not in it
)

// fixedSectionOrder is the order of the bays that are not model rows: in
// at the top, measured, routed, then the models, then displayed and out at
// the bottom.
//
// secModel is where the per-category model rows are spliced in. It still
// names a section of its own because the mode-owned panels that have not
// been given to a category row yet land there.
var fixedSectionOrder = []string{
	secConsole, secInput, secAnalyze, secMod, secModel, secDisplay, secOutput, secUtility,
}

// sectionOrder is fixedSectionOrder with one row per model category spliced
// in at the model position. Computed rather than written out, so a category
// added to modeGroups gets its row with no second list to keep in step.
var sectionOrder = buildSectionOrder()

func buildSectionOrder() []string {
	out := make([]string, 0, len(fixedSectionOrder)+len(modeGroups))
	for _, s := range fixedSectionOrder {
		if s == secModel {
			for _, c := range modelCategories() {
				out = append(out, categorySection(c))
			}
		}
		out = append(out, s)
	}
	return out
}

// sectionTitleOf is what is silkscreened on a bay, including the model
// rows, whose name is the category's own.
func sectionTitleOf(section string) string {
	if t, ok := sectionTitle[section]; ok {
		return t
	}
	if isCategorySection(section) {
		for _, c := range modelCategories() {
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
	secInput:   "INPUT",
	// METERING and not ANALYSIS: there is now an Analysis model CATEGORY with
	// a row of its own — the recurrence, transfer and waterfall displays — and
	// two bays silkscreened ANALYSIS meaning different things is worse than
	// either name alone. This bay holds meters: loudness, distortion, wow and
	// flutter, the counter, the Lyapunov readout.
	secAnalyze: "METERING",
	secMod:     "MODULATION",
	secModel:   "GENERATOR",
	secDisplay: "DISPLAY",
	secOutput:  "OUTPUT",
	secUtility: "UTILITY",
}

// moduleSections maps a module's key — its header text, lowercased, which is
// what rack-go names it by — to the bay it belongs in.
//
// A table rather than a class on each element because the grouping is a
// statement about the INSTRUMENT, not about the markup: it is the same
// grouping docs/signal-flow.md draws, and the two should be read together.
// A module missing from here is caught by a test rather than quietly
// landing in whatever bay it was declared next to.
var moduleSections = map[string]string{
	"console": secConsole,
	// The capture monitor opens the rack. It carries a screen, so it leads a
	// bay wherever it goes, and at the end of DISPLAY that bay held little
	// else; at the top it is the rack's own output monitor beside the Console,
	// the first thing on the left edge where a rack is read from.
	"record": secConsole,

	"test": secInput,

	"analysis": secAnalyze,
	// Not secAnalyze: the metering bay measures the SIGNAL — loudness,
	// distortion, speed stability, frequency. This one measures the
	// INSTRUMENT, which is a global fact about the rack and belongs with the
	// other global ones, at the top where it can be found without hunting.
	"timing":        secConsole,
	"loudness":      secAnalyze,
	"distortion":    secAnalyze,
	"wow & flutter": secAnalyze,
	"counter":       secAnalyze,

	"envelope": secMod,
	"patchbay": secMod,

	// The model, and the per-mode front panels that are its own controls.
	// secModel means "part of the instrument rather than of the rack", and
	// moduleSection turns that into the running model's category row.
	"parameters": secModel,
	"patch":      secModel,
	"scoreboard": secModel,
	"banner":     secModel,
	"launcher":   secModel,
	"loader":     secModel,
	"animation":  secModel,

	"grid":     secDisplay,
	"view":     secDisplay,
	"position": secDisplay,
	"display":  secDisplay,
	"colors":   secDisplay,
	"palette":  secDisplay,
	"style":    secDisplay,
	"layers":   secDisplay,
	"spectro":  secDisplay,
	"desk":     secDisplay,

	"gen x":     secOutput,
	"gen y":     secOutput,
	"gen z":     secOutput,
	"keys":      secOutput,
	"matrix":    secOutput,
	"rhythm":    secOutput,
	"model out": secOutput,

	"presets":  secUtility,
	"template": secUtility,
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
	for _, c := range modelCategories() {
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
	return racksurface.Pack(items, capacity, monitor)
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
			for i, it := range items {
				if it.Section == s && (moduleSections[it.Key] == secModel) == panel {
					order = append(order, i)
				}
			}
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
