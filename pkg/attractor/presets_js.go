//go:build js && wasm

package attractor

// The Presets module (Window > Presets): the front panel of the store in
// presets.go.
//
// Save writes serializeState() — the same string the address bar carries and
// the same one the patch bank stores — under the name in the field. Recall
// runs recallSerializedState, which is the patch bank's own restore path.
// Delete removes it. Nothing here knows what a state contains, which is the
// point: a control added to the permalink table is in every preset from then
// on, with nothing to add here.

import "syscall/js"

var presetOn bool

// presetStore reads the saved presets.
func presetStore() []preset {
	raw, ok := lsGet(presetStoreKey)
	if !ok {
		return nil
	}
	return decodePresets(raw)
}

func presetStoreWrite(ps []preset) { lsSet(presetStoreKey, encodePresets(ps)) }

// presetModuleVisible shows or hides the module.
func presetModuleVisible(on bool) {
	if sect := doc.Call("getElementById", "preset-module"); sect.Truthy() {
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
	sel := doc.Call("getElementById", "preset-list")
	if !sel.Truthy() {
		return
	}
	sel.Set("innerHTML", "")
	ps := presetStore()
	if len(ps) == 0 {
		opt := doc.Call("createElement", "option")
		opt.Set("value", "")
		opt.Set("textContent", "— none saved —")
		sel.Call("appendChild", opt)
		return
	}
	for _, p := range ps {
		opt := doc.Call("createElement", "option")
		opt.Set("value", p.Name)
		opt.Set("textContent", p.Name)
		sel.Call("appendChild", opt)
	}
	if _, ok := findPreset(ps, selected); ok {
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
	el := doc.Call("getElementById", "preset-name")
	if !el.Truthy() {
		return selectedMode
	}
	if n := cleanPresetName(el.Get("value").String()); n != "" {
		return n
	}
	return selectedMode
}

func wirePresetModule() {
	sw := doc.Call("getElementById", "preset-on")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
		presetOn = sw.Get("checked").Bool()
		presetModuleVisible(presetOn)
		if presetOn {
			refreshPresetList("")
		}
		return nil
	}))

	if b := doc.Call("getElementById", "preset-save"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			name := presetNameField()
			presetStoreWrite(putPreset(presetStore(), name, serializeState()))
			// Put the name in the field as well as the list: an unnamed save
			// used the model's name, and the panel should say which one it
			// picked rather than leaving the box empty over a preset that now
			// exists.
			if el := doc.Call("getElementById", "preset-name"); el.Truthy() {
				el.Set("value", name)
			}
			refreshPresetList(name)
			return nil
		}))
	}

	if b := doc.Call("getElementById", "preset-recall"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			sel := doc.Call("getElementById", "preset-list")
			if !sel.Truthy() {
				return nil
			}
			p, ok := findPreset(presetStore(), sel.Get("value").String())
			if !ok {
				return nil
			}
			recallSerializedState(p.State)
			// The recall runs onResetAll and then re-applies the snapshot,
			// which rebuilds the panel around this module. Nothing in that
			// path rewrites the list, so put it back, with the recalled preset
			// still chosen — the next thing anyone does is recall another one.
			refreshPresetList(p.Name)
			if el := doc.Call("getElementById", "preset-name"); el.Truthy() {
				el.Set("value", p.Name)
			}
			return nil
		}))
	}

	if b := doc.Call("getElementById", "preset-del"); b.Truthy() {
		b.Call("addEventListener", "click", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			sel := doc.Call("getElementById", "preset-list")
			if !sel.Truthy() {
				return nil
			}
			presetStoreWrite(deletePreset(presetStore(), sel.Get("value").String()))
			refreshPresetList("")
			return nil
		}))
	}

	// Picking from the list fills the name box, so Save over the same name is
	// one click away and Delete is obviously about the thing that is named.
	if sel := doc.Call("getElementById", "preset-list"); sel.Truthy() {
		sel.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			if el := doc.Call("getElementById", "preset-name"); el.Truthy() {
				el.Set("value", sel.Get("value").String())
			}
			return nil
		}))
	}

	refreshPresetList("")
}
