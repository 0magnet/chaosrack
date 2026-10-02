//go:build js && wasm

package attractor

import (
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/preset"
)

// The Presets module's own furniture, for the same reason: every id
// presets_js.go looks up has to be in the markup that is supposed to provide
// it, or the module builds with dead buttons and says nothing about it.
func TestPresetModuleMarkupHasItsControls(t *testing.T) {
	for _, id := range []string{
		"preset-module", "preset-name", "preset-list",
		"preset-save", "preset-recall", "preset-del",
	} {
		if !strings.Contains(controlsBody, `id="`+id+`"`) {
			t.Errorf("no element with id %q in the panel markup", id)
		}
	}
	// And it starts VISIBLE. It used to start put away behind a Console
	// switch, along with seven other modules; those switches are gone, so a
	// module that still shipped hidden would be one nothing could reveal.
	if strings.Contains(controlsBody, `id="preset-module" style="display:none"`) {
		t.Error("the Presets module still starts hidden, and nothing can bring it back")
	}
	// The name field's maxlength and the store's cap have to agree, or a name
	// typed to the limit of the field comes back from storage shorter than the
	// one on screen and Save stops finding the preset it just wrote.
	if !strings.Contains(controlsBody, `maxlength="24"`) || preset.NameMax != 24 {
		t.Errorf("the name field's maxlength and preset.NameMax (%d) disagree", preset.NameMax)
	}
}

// A module ships hidden only if something can still bring it back.
//
// Eight modules used to start with display:none because the Console carried a
// switch for each. Those switches are gone — a module is in the rack — so a
// module that still shipped hidden would be one nothing reveals: present in
// the markup, absent from the rack, and unreachable from any control.
//
// What may still start hidden is a module the MODEL owns. Those are revealed
// by choosing the model whose front panel they are, which is a control that
// exists and cannot be removed, so they are listed here by name rather than
// by rule.
func TestOnlyModelOwnedModulesShipHidden(t *testing.T) {
	modelOwned := map[string]bool{
		"spectro-module": true, // the spectrogram as a layer rather than the model
	}
	rest := controlsBody
	for {
		i := strings.Index(rest, ` style="display:none"`)
		if i < 0 {
			break
		}
		// The id is the last one declared before this attribute.
		head := rest[:i]
		j := strings.LastIndex(head, `id="`)
		id := ""
		if j >= 0 {
			if k := strings.Index(head[j+4:], `"`); k >= 0 {
				id = head[j+4 : j+4+k]
			}
		}
		if strings.HasSuffix(id, "-module") && !modelOwned[id] {
			t.Errorf("module %q starts hidden, and no switch is left to bring it back", id)
		}
		rest = rest[i+1:]
	}
}
