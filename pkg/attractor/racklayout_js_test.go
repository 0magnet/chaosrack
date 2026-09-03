//go:build js && wasm

package attractor

import (
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
	prev := doc
	doc = fakeDoc(byID)
	t.Cleanup(func() { doc = prev })
}

// Which switches are on is read off the panel, in the list's order, and a
// switch the page does not have is not an error — the panel is built from one
// blob of markup but a host page may inject its own, and a missing element
// must mean "no such module", not a crash and no saved layout at all.
func TestOnConsoleModuleSwitchesReadsTheCheckedOnes(t *testing.T) {
	withFakeDoc(t, map[string]js.Value{
		"analysis-on": fakeSwitch(true),
		"keys-on":     fakeSwitch(false),
		"tm-on":       fakeSwitch(true),
		// patch-on, counter-on, tpl-on, preset-on absent entirely.
	})
	got := strings.Join(onConsoleModuleSwitches(), ",")
	if got != "analysis-on,tm-on" {
		t.Errorf("on switches came back %q, want %q", got, "analysis-on,tm-on")
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
		"preset-save", "preset-recall", "preset-del", "preset-on",
	} {
		if !strings.Contains(controlsBody, `id="`+id+`"`) {
			t.Errorf("no element with id %q in the panel markup", id)
		}
	}
	// It starts put away, like every other module that answers to a Window
	// switch. Shipped visible it would be in everybody's rack whether they
	// wanted it or not.
	if !strings.Contains(controlsBody, `id="preset-module" style="display:none"`) {
		t.Error("the Presets module does not start hidden")
	}
	// The name field's maxlength and the store's cap have to agree, or a name
	// typed to the limit of the field comes back from storage shorter than the
	// one on screen and Save stops finding the preset it just wrote.
	if !strings.Contains(controlsBody, `maxlength="24"`) || presetNameMax != 24 {
		t.Errorf("the name field's maxlength and presetNameMax (%d) disagree", presetNameMax)
	}
}
