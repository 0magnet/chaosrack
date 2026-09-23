package attractor

import (
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/racklayout"
)

// Both new stores live under the same "wasmstuff-" prefix every other
// preference in this app uses — the dock edge, the interface size, the rack
// bay, the patch bank. The prefix is how they are found: it is what a host
// page embedding this panel would clear to reset it, and a key outside the
// family is one that survives that and then restores a rack nobody asked for.
func TestPersistenceKeysAreInTheFamily(t *testing.T) {
	for _, k := range []string{racklayout.LayoutKey, presetStoreKey} {
		if !strings.HasPrefix(k, "wasmstuff-") {
			t.Errorf("localStorage key %q is outside the wasmstuff- family", k)
		}
	}
	if racklayout.LayoutKey == presetStoreKey {
		t.Error("the layout and the presets would overwrite each other")
	}
}
