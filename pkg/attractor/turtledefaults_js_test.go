//go:build js && wasm

package attractor

import "testing"

// TurtleDefaults, which a renderer without the page draws from, are the
// page's own.
func TestTurtleDefaultsAreThePages(t *testing.T) {
	def := map[string]float32{}
	for _, p := range attractorParams["turtle"] {
		def[p.ID] = p.Def
	}
	for i, id := range TurtleKnobs {
		if d, ok := def[id]; !ok || d != TurtleDefaults[i] {
			t.Errorf("%s: TurtleDefaults has %v, the page %v", id, TurtleDefaults[i], d)
		}
	}
}
