//go:build js && wasm

package attractor

// The patch memories: 8 numbered snapshots of the whole rack, on the Presets
// module. STO arms store mode: STO then a slot saves the CURRENT full state
// (the permalink serialization); a plain click recalls a slot by resetting to
// defaults and re-applying its snapshot (the URL hash updates too, so a
// recalled patch is immediately shareable). Persisted in localStorage.
//
// They were the Patchbay's, beside its pin matrix. The matrix is the Mod
// module's now (modmatrix_js.go), and the Patchbay went with it.

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strconv"
	"strings"
	"syscall/js"
)

var patchStoArm bool

const patchSlots = 8
const patchStoreKey = "wasmstuff-patchbank"

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
		aud.showAudioStatus("that patch was saved for \"" + gone +
			"\", which this build does not have — nothing was changed")
		return
	}
	mode := hashModeOf(snapshot)
	onResetAll(js.Undefined(), nil)
	if knownMode(mode) && mode != run.selectedMode {
		if sel := dom.Doc.Call("getElementById", "mode-select"); sel.Truthy() {
			sel.Set("value", mode)
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	// Apply the snapshot STRING directly — the mode-change dispatch above
	// resyncs the permalink, so location.hash can't be the carrier here.
	perma.applyStateFrom("#" + snapshot)
	syncKnobs()
	buildParamPanel(run.selectedMode)
	perma.syncPermalinkNow() // canonicalize the URL to the recalled state
}

// buildPatchBank (re)builds the patch memories' cell on the Presets module.
func buildPatchBank() {
	presets := dom.Doc.Call("querySelector", "#preset-module > .row")
	if !presets.Truthy() {
		return
	}
	if old := dom.Doc.Call("getElementById", "patch-bank-cell"); old.Truthy() {
		old.Call("remove")
	}
	bankRow := dom.Doc.Call("createElement", "div")
	bankRow.Set("className", "pbank")
	bank := patchBank()
	sto := dom.Doc.Call("createElement", "button")
	sto.Set("className", "pslot")
	sto.Set("textContent", "STO")
	sto.Set("title", doc("patch-sto"))
	refreshSto := func() {
		if patchStoArm {
			sto.Get("classList").Call("add", "sto")
		} else {
			sto.Get("classList").Call("remove", "sto")
		}
	}
	sto.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
		patchStoArm = !patchStoArm
		refreshSto()
		return nil
	}))
	bankRow.Call("appendChild", sto)
	for i := range patchSlots {
		b := dom.Doc.Call("createElement", "button")
		b.Set("className", "pslot")
		if bank[i] != "" {
			b.Get("classList").Call("add", "full")
		}
		b.Set("textContent", strconv.Itoa(i+1))
		b.Set("title", docf("patch-slot", "n", strconv.Itoa(i+1)))
		b.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) any {
			bank := patchBank()
			if patchStoArm {
				bank[i] = perma.serializeState()
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
	cell := dom.Doc.Call("createElement", "span")
	cell.Set("className", "pcell axcol vmcell gen-cell")
	cell.Set("id", "patch-bank-cell")
	cell.Set("title", doc("patch-bank-cell"))
	cell.Call("appendChild", bankRow)
	presets.Call("appendChild", cell)
}
