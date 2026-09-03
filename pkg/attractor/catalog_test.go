package attractor

import (
	"strings"
	"testing"
)

// The catalog is what the README is generated from, so a mode that is missing
// its label or its prose would ship as an empty documentation section. Catch
// it here instead.
func TestCatalogIsComplete(t *testing.T) {
	groups := Catalog()
	if len(groups) == 0 {
		t.Fatal("catalog is empty")
	}
	for _, g := range groups {
		if g.Label == "" {
			t.Error("group with no label")
		}
		if len(g.Models) == 0 {
			t.Errorf("group %q has no models", g.Label)
		}
		for _, m := range g.Models {
			if m.Label == "" {
				t.Errorf("%s/%s: no label — missing from modeInfo?", g.Label, m.Key)
			}
			if strings.TrimSpace(m.Description) == "" {
				t.Errorf("%s/%s: no description — add one to descriptions.go", g.Label, m.Key)
			}
		}
	}
}

// Every registered mode must be reachable from the selector, or it exists
// only as a URL hash nobody can find.
func TestEveryModeIsInAGroup(t *testing.T) {
	inGroup := map[string]bool{}
	for _, k := range CatalogKeys() {
		inGroup[k] = true
	}
	for key := range modeInfo {
		if !inGroup[key] {
			t.Errorf("mode %q is registered but in no modeGroups entry", key)
		}
	}
}
