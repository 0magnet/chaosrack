package attractor

import (
	"strings"
	"testing"
)

// Every category the selector offers gets a row, and no second list can
// disagree with modeGroups about what the categories are.
func TestEveryCategoryHasARow(t *testing.T) {
	cats := modelCategories()
	if len(cats) != len(modeGroups) {
		t.Fatalf("%d category rows for %d groups in the selector", len(cats), len(modeGroups))
	}
	for i, g := range modeGroups {
		if cats[i] != g.Label {
			t.Errorf("row %d is %q, the selector's group %d is %q", i, cats[i], i, g.Label)
		}
	}
}

// A section key has to survive being an element id and a CSS class.
func TestACategorySectionIsUsableAsAnID(t *testing.T) {
	for _, c := range modelCategories() {
		s := categorySection(c)
		if !isCategorySection(s) {
			t.Errorf("%q does not read as a category section", s)
		}
		slug := strings.TrimPrefix(s, catPrefix)
		if slug == "" {
			t.Errorf("category %q reduced to an empty slug", c)
		}
		for _, r := range slug {
			ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
			if !ok {
				t.Errorf("category %q gives slug %q, which contains %q", c, slug, r)
			}
		}
		if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") || strings.Contains(slug, "--") {
			t.Errorf("category %q gives a malformed slug %q", c, slug)
		}
	}
}

// The awkward one, and the reason the slug is derived rather than written
// out: a heading with punctuation and a year in it.
func TestASlugFlattensPunctuationAndYears(t *testing.T) {
	if got := categorySlug("Sprott systems (1994)"); got != "sprott-systems-1994" {
		t.Errorf("got %q, want %q", got, "sprott-systems-1994")
	}
	if got := categorySlug("Attractors"); got != "attractors" {
		t.Errorf("got %q, want %q", got, "attractors")
	}
}

// Two categories must not collapse to one row, which is what a slug
// collision would do — silently, with one row's models unreachable.
func TestNoTwoCategoriesShareARow(t *testing.T) {
	seen := map[string]string{}
	for _, c := range modelCategories() {
		s := categorySection(c)
		if prev, ok := seen[s]; ok {
			t.Errorf("%q and %q both give section %q", prev, c, s)
		}
		seen[s] = c
	}
}

// Every model is reachable from exactly one row's rotary, and that row
// exists. A model in no category is a model with no knob to select it.
func TestEveryModelIsOnSomeRowsRotary(t *testing.T) {
	rows := map[string]bool{}
	for _, c := range modelCategories() {
		rows[c] = true
	}
	for key := range modeInfo {
		c := categoryOf(key)
		if c == "" {
			t.Errorf("model %q is in no category, so no rotary offers it", key)
			continue
		}
		if !rows[c] {
			t.Errorf("model %q is in category %q, which has no row", key, c)
		}
	}
}

// A model listed under two headings belongs to the first of them. The audio
// displays are filed under both Scope and Audio on purpose; the row has to
// pick one, and picking the first keeps it stable.
func TestADoublyListedModelTakesItsFirstCategory(t *testing.T) {
	// Find one that really is listed twice, so the test tracks the data
	// rather than asserting a mode name that may move.
	count := map[string]int{}
	first := map[string]string{}
	for _, g := range modeGroups {
		for _, k := range g.Keys {
			count[k]++
			if count[k] == 1 {
				first[k] = g.Label
			}
		}
	}
	found := false
	for k, n := range count {
		if n < 2 {
			continue
		}
		found = true
		if got := categoryOf(k); got != first[k] {
			t.Errorf("%q is listed in %d groups; categoryOf says %q, first listing is %q", k, n, got, first[k])
		}
	}
	if !found {
		t.Skip("no model is listed in two groups any more")
	}
}

// A rotary needs something to select. A category with no models is a row
// with a knob that does nothing.
func TestEveryCategoryOffersAtLeastOneModel(t *testing.T) {
	for _, c := range modelCategories() {
		if len(categoryModes(c)) == 0 {
			t.Errorf("category %q has no models", c)
		}
	}
	if got := categoryModes("no such category"); got != nil {
		t.Errorf("an unknown category returned %v, want nil", got)
	}
}
