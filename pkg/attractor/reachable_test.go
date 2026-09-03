package attractor

import (
	"strings"
	"testing"
)

// Every model has to be reachable by turning the two console knobs, and nothing
// else. That is a chain, and each link is a separate way to lose a model:
//
//	modeInfo  →  modeGroups  →  a category on the outer ring
//	                         →  a position on the inner ring
//
// TestEveryModeIsInAGroup already guards the first link. These guard the rest,
// which had gone unguarded — and one of them was already broken: the outer
// ring's tooltip was a hand-written list that never mentioned Maps, so the
// category added most recently was the one the control surface denied existed.
// It is generated now, and what is left to check is the label table it reads.

func TestEveryCategoryHasARingTag(t *testing.T) {
	for _, g := range Catalog() {
		tag, ok := catShortLabels[g.Label]
		if !ok {
			t.Errorf("category %q has no ring tag; it would print its full name around the knob", g.Label)
			continue
		}
		if tag == "" {
			t.Errorf("category %q has an empty ring tag", g.Label)
		}
	}
	// OFF is synthetic — it is not a catalog group — so it is checked apart.
	if catShortLabels[nestedOffCat] == "" {
		t.Error("the OFF position has no ring tag")
	}
}

func TestRingTagsAreDistinct(t *testing.T) {
	// Two categories with the same tag is two detents that read identically:
	// the model is still reachable, but only by turning past it and noticing.
	seen := map[string]string{}
	for cat, tag := range catShortLabels {
		if prev, dup := seen[tag]; dup {
			t.Errorf("categories %q and %q both print %q on the ring", prev, cat, tag)
		}
		seen[tag] = cat
	}
}

func TestRingTagsFitBetweenTheDetents(t *testing.T) {
	// The ring is a circle of a fixed size with one label per detent. Six
	// characters is what SCOPE and SOLID take; more overlaps its neighbors.
	const max = 6
	for cat, tag := range catShortLabels {
		if len(tag) > max {
			t.Errorf("category %q has a %d-character tag %q; the ring fits %d", cat, len(tag), tag, max)
		}
		if strings.TrimSpace(tag) != tag {
			t.Errorf("category %q has a padded tag %q", cat, tag)
		}
	}
}

func TestNoRingTagIsOrphaned(t *testing.T) {
	// The other direction: a tag for a category that no longer exists is a
	// detent that was renamed, and its models are now reachable only through
	// whatever the fallback prints.
	live := map[string]bool{nestedOffCat: true}
	for _, g := range Catalog() {
		live[g.Label] = true
	}
	for cat := range catShortLabels {
		if !live[cat] {
			t.Errorf("ring tag for %q, which is not a category any more", cat)
		}
	}
}

func TestEveryModelIsReachableFromTheKnobs(t *testing.T) {
	// The whole requirement, stated once: for every registered mode there is a
	// category on the outer ring, and the model is one of the positions the
	// inner ring turns through when that category is selected.
	//
	// The knobs are built from these groups at runtime — the outer ring from
	// the optgroups and the inner from their options — so a model in a group
	// with a tagged category is a model two turns away.
	reach := map[string]string{}
	for _, g := range Catalog() {
		if _, tagged := catShortLabels[g.Label]; !tagged {
			continue // reported by TestEveryCategoryHasARingTag
		}
		for _, m := range g.Models {
			if m.Label == "" {
				t.Errorf("%s/%s has no label; the inner ring would show a blank detent", g.Label, m.Key)
			}
			reach[m.Key] = g.Label
		}
	}
	for key, info := range modeInfo {
		if _, ok := reach[key]; !ok {
			t.Errorf("mode %q (%s) cannot be reached by turning the console knobs", key, info.Label)
		}
	}
	if len(reach) != len(modeInfo) {
		t.Errorf("%d models reachable, %d registered", len(reach), len(modeInfo))
	}
}
