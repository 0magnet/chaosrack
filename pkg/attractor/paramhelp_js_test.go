package attractor

import "testing"

// Every knob a model puts on the Parameters module says what it does. One with
// no help is described by the manual as its label and its range ("dt, 0.001 to
// 0.05"), and 72 of them were, unnoticed, until this was written.
func TestEveryModelParameterIsDescribed(t *testing.T) {
	n := 0
	for _, k := range CatalogKeys() {
		for _, p := range attractorParams[k] {
			n++
			if helpFor(p.ID) == "" {
				t.Errorf("%s: %s (%s) has no help: add p.%s to manual/parameters.md", k, p.ID, p.Label, p.ID)
			}
		}
	}
	if n < 200 {
		t.Errorf("only %d parameters: attractorParams is not filled", n)
	}
}
