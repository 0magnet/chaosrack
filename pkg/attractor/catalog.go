package attractor

// The catalog is the mode registry read from outside the package: the same
// ordered groups the mode-selector knob turns through, with each model's
// label and description attached. cmd/uitool generates the README's model
// reference and captures its per-model images from this, so the documentation
// and the control surface cannot disagree about what exists — adding a mode
// to modeGroups puts it in the README, and a mode with no description fails
// catalog_test.go rather than shipping an empty section.

// CatalogModel is one model — one position of the model-selector knob.
type CatalogModel struct {
	Key         string    // hash/mode key, e.g. "lorenz"
	Label       string    // display label, e.g. "Lorenz"
	Class       ModeClass // behavior family
	Description string    // the info overlay's prose; equations after a blank line
}

// CatalogGroup is one category — one position of the category knob, and one
// optgroup of the plain <select>.
type CatalogGroup struct {
	Label  string
	Models []CatalogModel
}

// Catalog returns the mode selector's full layout in knob order. A key that
// appears in more than one group (xy in Scope and Audio, custom in Attractors
// and Custom) is repeated, because that is what the selector does.
func Catalog() []CatalogGroup {
	out := make([]CatalogGroup, 0, len(modeGroups))
	for _, g := range modeGroups {
		models := make([]CatalogModel, 0, len(g.Keys))
		for _, k := range g.Keys {
			info := modeInfo[k]
			models = append(models, CatalogModel{
				Key:         k,
				Label:       info.Label,
				Class:       info.Class,
				Description: attractorDescriptions[k],
			})
		}
		out = append(out, CatalogGroup{Label: g.Label, Models: models})
	}
	return out
}

// CatalogKeys returns every model key once, in catalog order — the list to
// iterate when capturing one image per model.
func CatalogKeys() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range Catalog() {
		for _, m := range g.Models {
			if !seen[m.Key] {
				seen[m.Key] = true
				out = append(out, m.Key)
			}
		}
	}
	return out
}

// String names the class for documentation.
func (c ModeClass) String() string {
	switch c {
	case ClassFlow3D:
		return "3-D flow"
	case ClassFlow4D:
		return "4-D flow"
	case ClassParametric:
		return "parametric"
	case ClassGeometry:
		return "geometry"
	case ClassAudio:
		return "audio"
	case ClassMap:
		return "discrete map"
	}
	return "unknown"
}

// nestedOffCat is the synthetic first outer-knob category: turning to it powers
// the model off (replacing the old PWR switch). It holds no models, and it is
// here rather than beside the knob so the label table below can name it.
const nestedOffCat = "OFF"

// catShortLabels maps a catalog group's label to the short tag printed around
// the outer selector knob's ring.
//
// The TABLE lives here, beside the catalog it labels and with no build tag,
// while the lookup that reads it stays with the knob — because everything worth
// checking about this is pure data: that EVERY category has an entry, that no
// two share a tag, and that each is short enough to fit between the detents. A
// category with no entry falls through to its own name, and
// "Sprott systems (1994)" wrapped around a knob ring is how that announces
// itself. reachable_test.go is the guard so it never has to.
var catShortLabels = map[string]string{
	"Attractors":            "ATTR",
	"Sprott systems (1994)": "SPRT",
	"Maps":                  "MAPS",
	"Scope":                 "SCOPE",
	"Polyhedra":             "POLY",
	"Geometry":              "GEO",
	"Sequences":             "SEQ",
	"Solids":                "SOLID",
	"Audio":                 "AUD",
	"Analysis":              "ANLY",
	"Custom":                "CUST",
	nestedOffCat:            "OFF",
}

// catTooltips say what each category IS, one per category, and are what the
// short tag around the knob ring carries as its tooltip.
//
// Beside catShortLabels for the reason catShortLabels is here: a dial label with
// no title of its own shows the KNOB's tooltip instead, so every one of the
// twelve tags explained the category knob and none explained itself. The table
// that had been written out beside the knob was missing Maps — the same
// omission, in the same category, that the generated ring tooltip was added to
// fix — because a second list of the categories is a second thing to forget.
// reachable_test.go guards this one the way it guards the tags.
var catTooltips = map[string]string{
	"Attractors":            "Attractors — chaotic flows (Lorenz, Rössler, Chua…) and your own equations",
	"Sprott systems (1994)": "Sprott systems — the twenty simple chaotic flows of J. C. Sprott, 1994",
	"Maps":                  "Maps — discrete iterated systems (Hénon, Ikeda, standard map…) rather than flows",
	"Scope":                 "Scope — Lissajous figures, the Graphic Artist and the XY oscilloscope",
	"Polyhedra":             "Polyhedra — the wireframe Platonic solids",
	"Geometry":              "Geometry — sphere, torus, globe and magnetosphere",
	"Sequences":             "Sequences — the Turtle Path: an integer sequence read as turn-and-step",
	"Solids":                "Solids — the STL viewer: a file from disk, or a built-in model",
	"Audio":                 "Audio — spectrogram, XY scope, FVF wobbulator and the Takens embedding",
	"Analysis":              "Analysis — the Bifurcation Explorer",
	"Custom":                "Custom — type your own differential equations",
	nestedOffCat:            "Power off — stop rendering and clear the display",
}
