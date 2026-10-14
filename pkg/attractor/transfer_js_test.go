//go:build js && wasm

package attractor

import "testing"

// The SHOW dial names every layout Transfer draws.
func TestTheTransferDialNamesEveryLayout(t *testing.T) {
	if len(xfShowNames) != xfShowLayouts || len(xfShowRing) != xfShowLayouts {
		t.Errorf("%d names and %d ring labels for %d layouts", len(xfShowNames), len(xfShowRing), xfShowLayouts)
	}
}
