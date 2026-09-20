package attractor

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

// sectionOrder is the order the bays are stacked, which is the order the
// signal moves: in at the top, measured, routed, generated, displayed, out
// at the bottom.
var sectionOrder = []string{
	secConsole, secInput, secAnalyze, secMod, secModel, secDisplay, secOutput, secUtility,
}

// sectionTitle is what is silkscreened on the bay.
var sectionTitle = map[string]string{
	secConsole: "CONSOLE",
	secInput:   "INPUT",
	secAnalyze: "ANALYSIS",
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

	"test": secInput,

	"analysis":      secAnalyze,
	"loudness":      secAnalyze,
	"distortion":    secAnalyze,
	"wow & flutter": secAnalyze,
	"counter":       secAnalyze,

	"envelope": secMod,
	"patchbay": secMod,

	// The model, and the per-mode front panels that are its own controls.
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
	"record":   secDisplay,
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
		return s
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

// packItem is one module as the packer sees it: how many slots it needs and
// which bay it belongs in.
type packItem struct {
	Slots   int
	Section string
}

// unitSection is the bay a packed unit belongs to, which is the section of
// whatever is in it. Empty for an empty unit.
func unitSection(items []packItem, idx []int) string {
	for _, i := range idx {
		if i >= 0 && i < len(items) {
			return items[i].Section
		}
	}
	return ""
}

// packBySection assigns modules to bays, fitting as many sections into a
// bay as will go.
//
// It used to break at every section, which made the labels honest and the
// rack empty: ten bays, twenty-six modules and 69 blank slots of 120 — 58%
// of the rack was blank panel. Most sections are nowhere near 84 HP wide,
// and a bay per section spends a whole row on a section holding one module.
//
// So a bay carries several sections now, each marked over its own span.
// That is what a real 84 HP row looks like — several functional groups
// sharing it — and it is Woodson & Conover's own answer for identifying a
// group WITHIN a row rather than by giving it a row: "adequate spacing of
// display or control groups... marked outlines around each group... area
// color patterning" (§2-133). Only the break rule changed; a section still
// never interleaves with another, because groupBySection has already made
// each one contiguous.
func packBySection(items []packItem, capacity int) [][]int {
	if capacity < 1 {
		capacity = 1
	}
	var units [][]int
	var cur []int
	used := 0
	flush := func() {
		if len(cur) > 0 {
			units = append(units, cur)
			cur, used = nil, 0
		}
	}
	for i, it := range items {
		w := it.Slots
		if w < 0 {
			w = 0
		}
		if w > capacity {
			// Too big for any bay. Its own, overhanging — visible, which is
			// the right outcome for a thing that genuinely does not fit.
			flush()
			units = append(units, []int{i})
			continue
		}
		if used+w > capacity && len(cur) > 0 {
			flush()
		}
		cur = append(cur, i)
		used += w
	}
	flush()
	return units
}

// sectionRun is one section's stretch inside a bay: where it starts among
// the bay's modules, and how many of them it covers.
type sectionRun struct {
	Section string
	From    int // index into the unit's own module list
	Count   int
}

// sectionRuns splits a bay into the sections it carries, in order.
//
// A bay with three groups in it needs three labels, each over the modules
// it names — one label on a bay holding three sections would be a label
// that is two-thirds wrong.
func sectionRuns(items []packItem, idx []int) []sectionRun {
	var out []sectionRun
	for n, i := range idx {
		sec := ""
		if i >= 0 && i < len(items) {
			sec = items[i].Section
		}
		if len(out) > 0 && out[len(out)-1].Section == sec {
			out[len(out)-1].Count++
			continue
		}
		out = append(out, sectionRun{Section: sec, From: n, Count: 1})
	}
	return out
}
