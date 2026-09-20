package attractor

import "strings"

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
//	Sprott, Maps, Scope, Geometry, Custom     5
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
		for _, k := range g.Keys {
			if k == mode {
				return g.Label
			}
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

// activeCategory is the category of the model currently driving the rack.
// Empty before the first model is chosen, which is the only time the model's
// panels have no row to go in.
var activeCategory string

// setActiveCategory records which row is the instrument. Called wherever the
// model changes, from whatever moved it.
func setActiveCategory(mode string) { activeCategory = categoryOf(mode) }

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
// name has to be a name and not a citation: "Sprott systems (1994)" is a
// heading in the catalog and twenty-one characters over a knob. The year is
// what identifies the paper, not the category, and it is still in the cell's
// tooltip where the sentence about the category is.
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
