//go:build js && wasm

package attractor

// The Presets module (Window > Presets): the front panel of the store in
// pkg/preset.
//
// Save writes serializeState() — the same string the address bar carries and
// the same one the patch bank stores — under the name in the field. Recall
// runs recallSerializedState, which is the patch bank's own restore path.
// Delete removes it. Nothing here knows what a state contains, which is the
// point: a control added to the permalink table is in every preset from then
// on, with nothing to add here.

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/preset"
)

// presetStore reads the saved presets.
func presetStore() preset.List {
	raw, ok := lsGet(preset.StoreKey)
	if !ok {
		return nil
	}
	return preset.Decode(raw)
}

func presetStoreWrite(ps preset.List) { lsSet(preset.StoreKey, ps.Encode()) }

// presetModuleVisible shows or hides the module.
func presetModuleVisible(on bool) {
	if sect := dom.Doc.Call("getElementById", "preset-module"); sect.Truthy() {
		if on {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	quantizeModuleWidths()
}

// refreshPresetList rebuilds the <select>, leaving `selected` chosen when it
// still exists.
func refreshPresetList(selected string) {
	sel := dom.Doc.Call("getElementById", "preset-list")
	if !sel.Truthy() {
		return
	}
	sel.Set("innerHTML", "")
	ps := presetStore()
	if len(ps) == 0 {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", "")
		opt.Set("textContent", "— none saved —")
		sel.Call("appendChild", opt)
		return
	}
	for _, p := range ps {
		opt := dom.Doc.Call("createElement", "option")
		opt.Set("value", p.Name)
		opt.Set("textContent", p.Name)
		sel.Call("appendChild", opt)
	}
	if _, ok := ps.Find(selected); ok {
		sel.Set("value", selected)
	}
}

// presetNameField is what the name box currently says, cleaned.
//
// An empty field falls back to the current model's key rather than doing
// nothing. A Save button that silently declines is indistinguishable from a
// broken one, and the model is the most useful thing a view can be filed
// under when nobody has said otherwise. Saving over a name replaces it, so a
// second unnamed save from the same model updates that preset instead of
// making "lorenz (2)".
func presetNameField() string {
	el := dom.Doc.Call("getElementById", "preset-name")
	if !el.Truthy() {
		return selectedMode
	}
	if n := preset.CleanName(el.Get("value").String()); n != "" {
		return n
	}
	return selectedMode
}

func wirePresetModule() {
	// Always in the rack. The Console's module switches are gone, so there is
	// no state in which this module is absent, and the flag that used to mean
	// "switched in" is simply true. It is SET rather than the module's setter
	// being called: the setter is the switch's behavior — it opens an audio
	// graph and takes a context lease — and booting must not do that. What
	// the module DOES is its own transport control.
	presetModuleVisible(true)
	refreshPresetList("")

	if b := dom.Doc.Call("getElementById", "preset-save"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			name := presetNameField()
			presetStoreWrite(presetStore().Put(name, perma.serializeState()))
			// Put the name in the field as well as the list: an unnamed save
			// used the model's name, and the panel should say which one it
			// picked rather than leaving the box empty over a preset that now
			// exists.
			if el := dom.Doc.Call("getElementById", "preset-name"); el.Truthy() {
				el.Set("value", name)
			}
			refreshPresetList(name)
			return nil
		}))
	}

	if b := dom.Doc.Call("getElementById", "preset-recall"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			sel := dom.Doc.Call("getElementById", "preset-list")
			if !sel.Truthy() {
				return nil
			}
			p, ok := presetStore().Find(sel.Get("value").String())
			if !ok {
				return nil
			}
			recallSerializedState(p.State)
			// The recall runs onResetAll and then re-applies the snapshot,
			// which rebuilds the panel around this module. Nothing in that
			// path rewrites the list, so put it back, with the recalled preset
			// still chosen — the next thing anyone does is recall another one.
			refreshPresetList(p.Name)
			if el := dom.Doc.Call("getElementById", "preset-name"); el.Truthy() {
				el.Set("value", p.Name)
			}
			return nil
		}))
	}

	if b := dom.Doc.Call("getElementById", "preset-del"); b.Truthy() {
		b.Call("addEventListener", "click", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			sel := dom.Doc.Call("getElementById", "preset-list")
			if !sel.Truthy() {
				return nil
			}
			presetStoreWrite(presetStore().Delete(sel.Get("value").String()))
			refreshPresetList("")
			return nil
		}))
	}

	// Picking from the list fills the name box, so Save over the same name is
	// one click away and Delete is obviously about the thing that is named.
	if sel := dom.Doc.Call("getElementById", "preset-list"); sel.Truthy() {
		sel.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) interface{} {
			if el := dom.Doc.Call("getElementById", "preset-name"); el.Truthy() {
				el.Set("value", sel.Get("value").String())
			}
			return nil
		}))
	}

	refreshPresetList("")
}
