package attractor

import (
	"slices"
	"strings"
)

// A rack row per model category.
//
// The model knob used to be one control on the Console that changed what the
// whole instrument was, and the Parameters module was one panel that became
// whichever model's panel the knob had chosen. Everything else in the rack
// stayed put while the one thing the rack is FOR was swapped underneath it.
//
// That is not how a rack works. A rack holds its instruments; you patch the
// one you want to the output. So each model category gets a row of its own:
// a rotary picking which model in that category, that model's parameters
// beside it, and the row's own monitor. Every row keeps its selection and
// shows its knobs; the output-enable says which row reaches the display.
//
// Measured before building it, every category fits one 84 HP row. The widest
// single model in each, plus a rotary and a monitor:
//
//	Audio       1 + 7 + 2 = 10   (stereo is the widest model in the rack)
//	Sequences   1 + 4 + 2 =  7
//	Analysis    1 + 4 + 2 =  7
//	Attractors  1 + 3 + 2 =  6
//	Solids      1 + 3 + 2 =  6
//	Maps, Scope, Geometry, Custom             5
//	Polyhedra   1 + 0 + 2 =  3   (no model in it has a tunable constant)
//
// so the worst case is ten slots of twelve and there is room in every row
// for the scope to grow into.

// catPrefix marks a section as one of the per-category model rows, so the
// packer and the labels can tell them from the fixed sections without
// knowing the category list.
const catPrefix = "cat:"

// categorySection is the section key for a model category, derived from the
// category's own label rather than from a second list that could disagree
// with modeGroups.
func categorySection(label string) string { return catPrefix + categorySlug(label) }

// isCategorySection reports whether a section is a model-category row.
func isCategorySection(s string) bool { return strings.HasPrefix(s, catPrefix) }

// categorySlug is a label reduced to something usable as an id: lowercase,
// and anything that is not a letter or a digit becomes a dash. "Sprott
// systems (1994)" becomes "sprott-systems-1994".
func categorySlug(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	return b.String()
}

// modelCategories is the categories in the order the model selector offers
// them, which is the order their rows are stacked.
//
// Straight off modeGroups, so a category added there gets a row without a
// second list to keep in step — the mistake the mode registry was built to
// end, repeated one level up.
func modelCategories() []string {
	out := make([]string, 0, len(modeGroups))
	for _, g := range modeGroups {
		out = append(out, g.Label)
	}
	return out
}

// categoryModes is the models a category's rotary offers, in the order the
// selector lists them.
func categoryModes(label string) []string {
	for _, g := range modeGroups {
		if g.Label == label {
			return g.Keys
		}
	}
	return nil
}

// categoryOf is the category a model belongs to for the purpose of its row.
//
// A handful of modes are listed under two headings — the audio displays are
// both Scope and Audio, and the measurement modes are both Audio and
// Analysis — because they genuinely are both, and the selector offers them
// in both places. A model can only be IN one row, though, so the first
// listing wins: that is the category the mode was filed under before the
// duplicates were added, and it keeps the rows stable when a heading is
// reordered.
func categoryOf(mode string) string {
	for _, g := range modeGroups {
		if slices.Contains(g.Keys, mode) {
			return g.Label
		}
	}
	return ""
}

// ── Which row is the instrument ────────────────────────────────────────────
//
// A row per category is only half of it. Eleven rows each holding one rotary
// is eleven bays of eleven blank slots — the rack went to 17 bays and 63%
// blank panel the moment the rows were added, which is worse than the
// sparseness they were meant to fix.
//
// What fills a row is the model itself. The rack draws one model, so exactly
// one row at a time is an instrument rather than a selector: the row whose
// category the running model is in. Its rotary, that model's Parameters, the
// model's own panels and its monitor all belong in that row, together, which
// is Woodson & Conover's first rule for a panel (§2-132: a control belongs
// "close to the display which they affect") applied to the thing the whole
// rack is for.
//
// The other ten rows stay rotaries, and rotaries share a bay. See
// packBySection for that half.

// activeCategory is the ROW of the model currently driving the rack (see
// rackRows): its category, or the group that category is drawn in.
// Empty before the first model is chosen, which is the only time the model's
// panels have no row to go in.
var activeCategory string

// setActiveCategory records which row is the instrument. Called wherever the
// model changes, from whatever moved it.
func setActiveCategory(mode string) { activeCategory = rowOf(mode) }

// modelRowSection is the bay the running model's own panels belong in.
//
// Falls back to secModel — a bay of its own — when no category claims the
// mode, so a model missing from modeGroups still has somewhere to be rather
// than being filed under UTILITY with the presets.
func modelRowSection() string {
	if activeCategory == "" {
		return secModel
	}
	return categorySection(activeCategory)
}

// categoryTag is the category's name as it is printed over its rotary.
//
// The cell is one control column wide and the label reads across it, so the
// name has to be a name and not a citation. "Sprott systems (1994)" was a
// heading in the catalog and twenty-one characters over a knob, which is what
// this rule was written for; those systems are inside Attractors now, but a
// category whose name does not fit a knob can be added again tomorrow.
//
// Derived rather than tabulated. A table of short tags is what the Console's
// dial ring had, and a table is a second list of the categories to keep in
// step with modeGroups — which is the mistake the generated rows exist to
// end. A rule that is wrong for a future category is visible in the label;
// a table that is missing one is not.
//
//nolint:unused // called from rackcategory_js.go, which the native lint pass cannot see
func categoryTag(label string) string {
	if i := strings.IndexByte(label, '('); i > 0 {
		label = strings.TrimSpace(label[:i])
	}
	return label
}

// ── Rows ─────────────────────────────────────────────────────────────────
//
// A rack row used to be a category. It is a GROUP of categories now, where a
// group has been merged: the visual models — attractors, maps, solids,
// geometry, sequences — share one bay, one monitor and one bank of
// programmable controls (see rackbank_js.go), and the MODEL knob gains an
// outer ring that picks the category. A bank only needs as many controls as
// the biggest model it serves, so a model added to a bay costs nothing on the
// panel unless it is the new biggest.
//
// The categories themselves are unchanged — the catalog, the README and the
// model selector still list them — this only says which of them are drawn as
// one row.

// rowGroup is categories drawn as one row, under a name of its own.
type rowGroup struct {
	Label string
	Cats  []string
}

// rackRowGroups are the merged rows. A category in none of them is a row of
// its own.
var rackRowGroups = []rowGroup{
	// Every model, in one row: one MODEL knob whose outer ring picks the kind
	// and one bank it reprograms. It was two, Visual and Signal, each a head
	// with its own monitor and a bank of its own, and only one of the two was
	// ever running; one bank the size of the biggest model serves them all,
	// the same knobs turned to whichever model is playing.
	{Label: "Visual", Cats: []string{"Attractors", "Maps", "Solids", "Geometry", "Sequences", "Scope", "Embeddings", "Audio"}},
}

// rowOfCategory is the row a category is drawn in.
func rowOfCategory(cat string) string {
	for _, g := range rackRowGroups {
		if slices.Contains(g.Cats, cat) {
			return g.Label
		}
	}
	return cat
}

// rowOf is the row a model is drawn in, or "" for a model no category lists.
func rowOf(mode string) string {
	if c := categoryOf(mode); c != "" {
		return rowOfCategory(c)
	}
	return ""
}

// rowCategories is the categories a row draws, in catalog order.
//
//nolint:unused // called from rackcategory_js.go, which the native lint pass cannot see
func rowCategories(row string) []string {
	for _, g := range rackRowGroups {
		if g.Label == row {
			var out []string
			for _, c := range modelCategories() {
				if slices.Contains(g.Cats, c) {
					out = append(out, c)
				}
			}
			return out
		}
	}
	return []string{row}
}

// rackRows is every row, top to bottom: the categories in catalog order,
// each group standing where its first category would.
func rackRows() []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range modelCategories() {
		if r := rowOfCategory(c); !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}
