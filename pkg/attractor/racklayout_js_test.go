//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strings"
	"syscall/js"
	"testing"
)

// fakeDoc is a document with nothing in it but the elements the test names —
// enough for the code that only ever calls getElementById. Node has no DOM at
// all, and `doc` is a package variable, so substituting one is the whole of
// what these tests need.
func fakeDoc(byID map[string]js.Value) js.Value {
	d := js.Global().Get("Object").New()
	d.Set("getElementById", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if v, ok := byID[args[0].String()]; ok {
			return v
		}
		return js.Null()
	}))
	return d
}

// fakeSwitch is a checkbox.
func fakeSwitch(checked bool) js.Value {
	el := js.Global().Get("Object").New()
	el.Set("checked", checked)
	return el
}

// withFakeDoc swaps the package's document for the duration of a test.
func withFakeDoc(t *testing.T, byID map[string]js.Value) {
	t.Helper()
	prev := dom.Swap(fakeDoc(byID))
	t.Cleanup(func() { dom.Swap(prev) })
}

// Which switches are on is read off the panel, in the list's order, and a
// switch the page does not have is not an error — the panel is built from one
// blob of markup but a host page may inject its own, and a missing element
// must mean "no such module", not a crash and no saved layout at all.
func TestOnConsoleModuleSwitchesReadsTheCheckedOnes(t *testing.T) {
	withFakeDoc(t, map[string]js.Value{
		"scope-on": fakeSwitch(true),
		// tpl-on absent entirely.
	})
	got := strings.Join(onConsoleModuleSwitches(), ",")
	if got != "scope-on" {
		t.Errorf("on switches came back %q, want %q", got, "scope-on")
	}
}

func TestOnConsoleModuleSwitchesNoneOn(t *testing.T) {
	byID := map[string]js.Value{}
	for _, id := range consoleModuleSwitches {
		byID[id] = fakeSwitch(false)
	}
	withFakeDoc(t, byID)
	if got := onConsoleModuleSwitches(); len(got) != 0 {
		t.Errorf("with every switch off, got %v", got)
	}
}

// Every module switch the saved layout carries must also be in the permalink
// table. They are the two ways of describing the same panel — one for this
// browser, one for a link — and a module that is in only one of them is a
// module that appears or vanishes depending on how the view was arrived at.
func TestPersistedModuleSwitchesAreAlsoShareable(t *testing.T) {
	inPerma := map[string]bool{}
	for _, c := range permaCtls {
		inPerma[c.id] = true
	}
	for _, id := range consoleModuleSwitches {
		if !inPerma[id] {
			t.Errorf("%q is saved to localStorage but is in no permalink row, so a link cannot carry it", id)
		}
	}
}

// A switch in the table with no element behind it restores nothing and
// serializes nothing, silently. The panel markup is a const in this package,
// so the check costs a substring search.
func TestPersistedModuleSwitchesExistInTheMarkup(t *testing.T) {
	for _, id := range consoleModuleSwitches {
		if !strings.Contains(controlsBody, `id="`+id+`"`) {
			t.Errorf("no element with id %q in the panel markup", id)
		}
	}
}

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
	if !strings.Contains(controlsBody, `maxlength="24"`) || presetNameMax != 24 {
		t.Errorf("the name field's maxlength and presetNameMax (%d) disagree", presetNameMax)
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
		"desk-module": true, "spectro-module": true, "pong-module": true,
		"stext-module": true, "smorph-module": true, "bounce-module": true,
		"stlfile-module": true, "termanim-module": true,
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
