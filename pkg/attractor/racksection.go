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

// packBySection assigns modules to units, one section per unit.
//
// Within a section it is the same fill-and-overflow as before — cards go in
// left to right until the next does not fit. Between sections it always
// breaks, even with room to spare, because a bay that holds the tail of the
// analysis rack and the head of the modulation rack is a bay whose label
// would have to be a lie.
func packBySection(items []packItem, capacity int) [][]int {
	if capacity < 1 {
		capacity = 1
	}
	var units [][]int
	var cur []int
	used := 0
	section := ""
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
		if len(cur) > 0 && it.Section != section {
			flush()
		}
		section = it.Section
		if w > capacity {
			// Too big for any unit. Its own, overhanging — visible, which is
			// the right outcome for a thing that genuinely does not fit.
			flush()
			units = append(units, []int{i})
			section = ""
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
