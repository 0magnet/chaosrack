package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// Every map must be in the catalog, and every catalog entry claiming to be a
// map must actually be one. The two lists drifting apart is how a mode ends
// up unreachable.
func TestMapsAreInTheCatalogAsMaps(t *testing.T) {
	inCatalog := map[string]bool{}
	for _, g := range Catalog() {
		for _, m := range g.Models {
			if m.Class == ClassMap {
				inCatalog[m.Key] = true
				if !dynamics.IsMap(m.Key) {
					t.Errorf("%q is cataloged as a map but has no registered step function", m.Key)
				}
				if m.Description == "" {
					t.Errorf("%q has no description", m.Key)
				}
			}
		}
	}
	for _, k := range dynamics.MapKeys() {
		if !inCatalog[k] {
			t.Errorf("map %q is registered but not in the catalog — nothing can select it", k)
		}
	}
	if len(inCatalog) < 7 {
		t.Errorf("only %d maps cataloged", len(inCatalog))
	}
}
