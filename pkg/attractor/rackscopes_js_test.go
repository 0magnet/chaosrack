//go:build js && wasm

package attractor

import "testing"

// Each scope's inputs have a link key of their own.
func TestScopeInputKeysAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for n := range rackScopeCount {
		for ch := range 2 {
			k := scopeInKey(n, ch)
			if seen[k] {
				t.Errorf("key %s twice", k)
			}
			seen[k] = true
		}
	}
	if scopeInKey(1, 1) != "si2b" {
		t.Errorf("scopeInKey(1, 1) = %s", scopeInKey(1, 1))
	}
}
