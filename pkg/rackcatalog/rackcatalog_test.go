package rackcatalog

import "testing"

// TestTheCatalogIsAWholeRack fails on a capture that came back short: one
// taken from a page still building had a dozen modules missing, and nothing
// that reads the catalog minded. A change to the rack that really does add or
// remove a module or a bay changes these numbers with it.
func TestTheCatalogIsAWholeRack(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	const modules, bays = 34, 8
	if len(c.Modules) != modules {
		t.Errorf("%d modules, want %d", len(c.Modules), modules)
	}
	if len(c.Manual) != bays {
		t.Errorf("%d bays in the manual, want %d", len(c.Manual), bays)
	}
	if len(c.Controls) < 500 {
		t.Errorf("only %d controls", len(c.Controls))
	}
	if len(c.Banks) < 75 {
		t.Errorf("only %d models' banks", len(c.Banks))
	}
	for model, b := range c.Banks {
		if len(b.Ctls) < 4 || len(b.Items) == 0 {
			t.Errorf("%s: a bank of %d controls and %d items", model, len(b.Ctls), len(b.Items))
		}
	}
}
