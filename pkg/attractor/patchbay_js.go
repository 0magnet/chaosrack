//go:build js && wasm

package attractor

// The PATCHBAY module (Window > Patchbay): the analog-computer patching
// surface, in two halves.
//
// Pin matrix — an EMS-Synthi-style routing grid: rows are the modulation
// sources (stereo / left / right energy), columns are every routable
// destination of the current mode (its float parameters plus the view/motion
// targets). A pin toggles the route (default depth 0.4); the mouse wheel on a
// lit pin adjusts depth. It reads and writes the SAME paramMods map as the
// per-parameter MOD knobs — two views of one routing state, so edits in
// either stay consistent (the panel rebuilds on pin edits).
//
// Program bank — 8 patch-memory slots. STO arms store mode: STO then a slot
// saves the CURRENT full state (the permalink serialization); a plain click
// recalls a slot by resetting to defaults and re-applying its snapshot (the
// URL hash updates too, so a recalled patch is immediately shareable).
// Persisted in localStorage.

import (
	"strconv"
	"strings"
	"syscall/js"
)

var (
	patchOn     bool
	patchStoArm bool
)

const patchSlots = 8
const patchStoreKey = "wasmstuff-patchbank"

type matrixDest struct {
	id, label string
}

// matrixDests lists the current mode's routable destinations.
func matrixDests(mode string) []matrixDest {
	var out []matrixDest
	for _, pd := range attractorParams[mode] {
		if decimalsForStep(pd.Step) == 0 {
			continue // integer settings aren't modulatable (same rule as applyAudioModulation)
		}
		out = append(out, matrixDest{pd.ID, pd.Label})
	}
	for _, vt := range viewModTargets {
		out = append(out, matrixDest{vt.id, vt.label})
	}
	return out
}

// patchBank returns the stored snapshots (always patchSlots entries).
func patchBank() []string {
	bank := make([]string, patchSlots)
	raw, ok := lsGet(patchStoreKey)
	if !ok {
		return bank
	}
	parts := strings.Split(raw, "\x1f")
	for i := 0; i < len(parts) && i < patchSlots; i++ {
		bank[i] = parts[i]
	}
	return bank
}

func patchBankStore(bank []string) { lsSet(patchStoreKey, strings.Join(bank, "\x1f")) }

// recallSerializedState resets to defaults, then re-applies the snapshot.
//
// The ONE restore path for a stored view, shared by the patch bank's numbered
// slots and the Presets module's named ones. They differ in how a snapshot is
// found, not in what putting one back means, and a second copy of this would
// be a second set of answers to "does recalling change the mode" and "does the
// URL follow".
func recallSerializedState(snapshot string) {
	if snapshot == "" {
		return
	}
	// CHECKED BEFORE ANYTHING IS RESET, and that order is the whole of this.
	// onResetAll below puts every control back to its default; it used to run
	// first, so recalling a slot this build cannot read reset the live view and
	// then restored nothing over it. The user is left with neither the patch
	// they asked for nor the view they had. Measured: the URL went from
	// "#thomas&pb=1" to "#thomas" — the Patchbay switching ITSELF off, in the
	// middle of a click on the Patchbay.
	//
	// A mode this build does not have is how that happens: a patch stored by an
	// older build, or one where the model was called something else. The rest of
	// the snapshot cannot be trusted to still mean the same thing either, so the
	// recall is refused whole rather than half-applied — and said out loud,
	// because a slot that does nothing is indistinguishable from a broken one.
	if gone, refused := recallRefusedMode(snapshot, knownMode); refused {
		showAudioStatus("that patch was saved for \"" + gone +
			"\", which this build does not have — nothing was changed")
		return
	}
	mode := hashModeOf(snapshot)
	onResetAll(js.Undefined(), nil)
	if knownMode(mode) && mode != selectedMode {
		if sel := doc.Call("getElementById", "mode-select"); sel.Truthy() {
			sel.Set("value", mode)
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	// Apply the snapshot STRING directly — the mode-change dispatch above
	// resyncs the permalink, so location.hash can't be the carrier here.
	applyStateFrom("#" + snapshot)
	syncKnobs()
	buildParamPanel(selectedMode)
	syncPermalinkNow() // canonicalize the URL to the recalled state
}

// buildPatchbayModule (re)creates the PATCHBAY module. Called from
// buildParamPanel so the matrix columns track the current mode.
func buildPatchbayModule(paramsSect js.Value) {
	if old := doc.Call("getElementById", "patch-module"); old.Truthy() {
		old.Get("parentNode").Call("removeChild", old)
	}
	if !patchOn || !paramsSect.Truthy() {
		return
	}
	mod := doc.Call("createElement", "div")
	mod.Set("className", "sect")
	mod.Set("id", "patch-module")
	hdr := doc.Call("createElement", "div")
	hdr.Set("className", "sect-hdr")
	hdr.Set("textContent", "Patchbay")
	hdr.Set("title", "Patchbay — pin-matrix audio routing (sources × destinations) and the 8-slot patch memory bank")
	mod.Call("appendChild", hdr)
	body := doc.Call("createElement", "div")
	body.Set("className", "row")

	// ── Program bank ──
	bankRow := doc.Call("createElement", "div")
	bankRow.Set("className", "grp")
	bank := patchBank()
	sto := doc.Call("createElement", "button")
	sto.Set("className", "pslot")
	sto.Set("textContent", "STO")
	sto.Set("title", "Store mode — press STO, then a slot, to save the current patch there. Plain slot click recalls.")
	refreshSto := func() {
		if patchStoArm {
			sto.Get("classList").Call("add", "sto")
		} else {
			sto.Get("classList").Call("remove", "sto")
		}
	}
	sto.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		patchStoArm = !patchStoArm
		refreshSto()
		return nil
	}))
	bankRow.Call("appendChild", sto)
	for i := 0; i < patchSlots; i++ {
		i := i
		b := doc.Call("createElement", "button")
		b.Set("className", "pslot")
		if bank[i] != "" {
			b.Get("classList").Call("add", "full")
		}
		b.Set("textContent", strconv.Itoa(i+1))
		b.Set("title", "Patch memory "+strconv.Itoa(i+1)+" — click to recall; STO first to store the current patch")
		b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			bank := patchBank()
			if patchStoArm {
				bank[i] = serializeState()
				patchBankStore(bank)
				patchStoArm = false
				refreshSto()
				b.Get("classList").Call("add", "full")
				return nil
			}
			recallSerializedState(bank[i])
			return nil
		}))
		bankRow.Call("appendChild", b)
	}
	body.Call("appendChild", bankRow)

	// ── Pin matrix ──
	dests := matrixDests(selectedMode)
	if len(dests) > 0 {
		rows := []struct{ label, ch string }{{"ST", "mono"}, {"L", "L"}, {"R", "R"}}
		grid := doc.Call("createElement", "div")
		grid.Set("className", "mxgrid")
		grid.Get("style").Set("gridTemplateColumns", "repeat("+strconv.Itoa(len(dests)+1)+",auto)")
		grid.Call("appendChild", doc.Call("createElement", "span")) // corner
		for _, d := range dests {
			cl := doc.Call("createElement", "span")
			cl.Set("className", "mxcol")
			cl.Set("textContent", d.label)
			cl.Set("title", "Destination: "+d.label)
			grid.Call("appendChild", cl)
		}
		for _, row := range rows {
			row := row
			rl := doc.Call("createElement", "span")
			rl.Set("className", "mxlbl")
			rl.Set("textContent", row.label)
			rl.Set("title", "Source: "+row.label+" channel energy (loudest band unless EQ bands are painted on the MOD knob)")
			grid.Call("appendChild", rl)
			for _, d := range dests {
				d := d
				pin := doc.Call("createElement", "span")
				pin.Set("className", "mxpin")
				m := paramMods[d.id]
				on := m.channel == row.ch && m.level != 0
				if on {
					pin.Get("classList").Call("add", "on")
					pin.Get("style").Set("opacity", strconv.FormatFloat(0.45+0.55*float64(m.level), 'f', 2, 64))
				}
				pin.Set("title", row.label+" → "+d.label+" — click to toggle, wheel to set depth (needs Audio mod on)")
				pin.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
					m := paramMods[d.id]
					if m.channel == row.ch && m.level != 0 {
						m.channel = ""
					} else {
						m.channel = row.ch
						if m.level == 0 {
							m.level = 0.4
						}
					}
					paramMods[d.id] = m
					syncPermalinkNow()
					buildParamPanel(selectedMode) // resync MOD knobs + this matrix
					return nil
				}))
				pin.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
					e := a[0]
					e.Call("preventDefault")
					m := paramMods[d.id]
					if m.channel != row.ch {
						return nil
					}
					dl := float32(0.05)
					if e.Get("deltaY").Float() > 0 {
						dl = -dl
					}
					m.level = clampF(m.level+dl, 0, 1)
					paramMods[d.id] = m
					pin.Get("style").Set("opacity", strconv.FormatFloat(0.45+0.55*float64(m.level), 'f', 2, 64))
					pin.Set("title", row.label+" → "+d.label+" — depth "+strconv.FormatFloat(float64(m.level), 'f', 2, 64))
					syncPermalinkNow()
					return nil
				}))
				grid.Call("appendChild", pin)
			}
		}
		body.Call("appendChild", grid)
	}

	mod.Call("appendChild", body)
	paramsSect.Get("parentNode").Call("insertBefore", mod, paramsSect.Get("nextSibling"))
}
