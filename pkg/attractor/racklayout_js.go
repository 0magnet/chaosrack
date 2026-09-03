//go:build js && wasm

package attractor

// Reading and writing the rack layout, which is the only part of it that needs
// a browser. The record itself, and the ordering arithmetic, are in
// racklayout.go where they can be tested without one.

import "syscall/js"

// readRackLayout loads the saved arrangement, or an empty one.
func readRackLayout() rackLayout {
	v, ok := lsGet(rackLayoutKey)
	if !ok {
		return rackLayout{}
	}
	return decodeRackLayout(v)
}

// saveRackLayout writes the arrangement as it stands.
//
// Called from the rack's own OnReorder / OnVisibility callbacks and from each
// Console module switch, so every way a module can move or disappear ends up
// here and there is no arrangement that is only half-remembered.
func saveRackLayout() {
	r := ensureRack()
	if r == nil {
		return
	}
	l := rackLayout{
		Order:    r.Order(),
		Hidden:   r.HiddenKeys(),
		Switches: onConsoleModuleSwitches(),
	}
	lsSet(rackLayoutKey, l.encode())
}

// onConsoleModuleSwitches is which of the Console's module switches are on.
func onConsoleModuleSwitches() []string {
	var out []string
	for _, id := range consoleModuleSwitches {
		if sw := doc.Call("getElementById", id); sw.Truthy() && sw.Get("checked").Bool() {
			out = append(out, id)
		}
	}
	return out
}

// restoreRackLayout puts the modules back where they were.
//
// Must run BEFORE buildModuleSwitches: the rack builds each switch checked or
// not from its own hidden set, so a module restored as hidden afterward would
// come back with its switch saying it was in.
func restoreRackLayout() {
	r := ensureRack()
	if r == nil {
		return
	}
	l := readRackLayout()
	if len(l.Order) > 0 {
		r.SetOrder(mergeModuleOrder(l.Order, r.Order()))
	}
	r.SetHidden(restorableHidden(l.Hidden))
}

// restorableHidden drops any module the panel offers no switch for.
//
// Hide() refuses to put a pinned module away, but SetHidden — the RESTORE path
// — does not consult Pinned at all, so what the UI will not let you do to the
// Console a stored record can still do to it. A record naming the Console (an
// older build's key, a hand edit in devtools, a future one that pins something
// new) would hide the module the switches live in, leaving no way to bring
// anything back and no way to undo it, on this load or any later one.
//
// Dropping the key is the whole fix: an arrangement that cannot be undone is
// not an arrangement worth restoring.
func restorableHidden(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if moduleNeverSwitched(k) {
			continue
		}
		out = append(out, k)
	}
	return out
}

// restoreConsoleModuleSwitches puts back the Console switches for the modules
// that start hidden — Analysis, Counter, Keys, Matrix, Rhythm, Patchbay,
// Template, Presets.
//
// The timing is the whole of it, and it is why this is not folded into
// restoreRackLayout. It has to run:
//
//   - AFTER capturePermaDefaults, or the permalink would record a restored
//     switch as that control's pristine value and then omit it from the hash.
//     Turn Keys on, copy the link, and the person you sent it to would get no
//     Keys module — the state was there in the panel and missing from the only
//     thing that describes it.
//
//   - BEFORE applyStateFromHash, so a shared link WINS over this browser's own
//     preference. A link is somebody describing a view on purpose; the saved
//     layout is only where this browser left its furniture.
//
// The asymmetry that follows is real and is the price of the hash carrying
// only what differs from a default: a link whose sender had Keys off says
// nothing about Keys, so a local preference for Keys on survives it. Making
// the hash carry every control to close that would cost every shared link its
// length, for a case that is a module being present rather than a view being
// wrong.
func restoreConsoleModuleSwitches() {
	l := readRackLayout()
	on := make(map[string]bool, len(l.Switches))
	for _, id := range l.Switches {
		on[id] = true
	}
	for _, id := range consoleModuleSwitches {
		sw := doc.Call("getElementById", id)
		if !sw.Truthy() || sw.Get("checked").Bool() == on[id] {
			continue
		}
		sw.Set("checked", on[id])
		// Through the switch's own listener, which is what actually reveals
		// the module and starts whatever it runs. Setting .checked alone gives
		// a panel that says a module is in while the rack has no such module.
		sw.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	}
}

// wireConsoleModuleSwitchSaves records the Console module switches whenever
// one is flipped. Registered after the restore above so the restore's own
// dispatch does not write the record it just read.
func wireConsoleModuleSwitchSaves() {
	for _, id := range consoleModuleSwitches {
		sw := doc.Call("getElementById", id)
		if !sw.Truthy() {
			continue
		}
		sw.Call("addEventListener", "change", trackedFuncOf(func(js.Value, []js.Value) interface{} {
			saveRackLayout()
			return nil
		}))
	}
}
