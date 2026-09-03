package attractor

// The rack layout, as a thing that can be written down.
//
// A rack is arranged: modules are taken out, put back, and dragged into the
// order that suits whoever is using it. None of that survived a reload, which
// made the arranging pointless — every visit started from the factory order
// with every module in, and the first thing anyone did was take out the six
// they were not using. Again.
//
// This is the record. The panel already persists to localStorage for the dock
// edge, the interface size and the rack bay (see layout_js.go and
// rackhandles_js.go), so this is one more key in the same store rather than a
// second mechanism.
//
// Untagged so the encoding and the ordering arithmetic can be tested on the
// host, without a DOM. The bugs in this kind of code are all in the ordering —
// what happens to a module the saved order has never heard of, what happens to
// a saved module that no longer exists — and none of them need a browser.

import "strings"

// rackLayoutKey is where the arrangement lives, alongside wasmstuff-dock,
// wasmstuff-kscale and wasmstuff-handles.
const rackLayoutKey = "wasmstuff-racklayout"

// consoleModuleSwitches are the Console checkboxes that put a module in or out
// of the rack by their own means rather than through the rack's switch list.
//
// They are separate because these modules start hidden, and the rack skips a
// hidden module when it builds its switches (rack_js.go's modulePinned) — a
// module nobody can see has nothing to toggle. So the module owns a switch in
// the Console's Window column instead, and that switch is the thing to persist.
//
// The rack bay's "handles-on" is deliberately NOT here: the bay is the frame
// the modules sit in, not a module, and it already persists under its own key
// through setRackBay / restoreRackBay.
var consoleModuleSwitches = []string{
	"patch-on",
	"analysis-on",
	"counter-on",
	"keys-on",
	"tm-on",
	"rhythm-on",
	"tpl-on",
	"preset-on",
}

// rackLayout is the panel's arrangement: which modules are in it, in what
// order, and which of the Console's own module switches are on.
type rackLayout struct {
	Order    []string // module keys, left to right, as the rack reports them
	Hidden   []string // module keys taken out through a rack switch
	Switches []string // ids of the Console module switches that are ON
}

// The record is "field=a,b,c" joined by semicolons — legible in a devtools
// storage inspector, which matters for a preference nobody can see any other
// way, and extensible: decode ignores a field it does not know, so a record
// written by a newer build does not poison an older one.
const (
	layoutFieldSep = ";"
	layoutListSep  = ","
)

// encode renders the layout for localStorage.
func (l rackLayout) encode() string {
	return strings.Join([]string{
		"order=" + joinLayoutList(l.Order),
		"hidden=" + joinLayoutList(l.Hidden),
		"switches=" + joinLayoutList(l.Switches),
	}, layoutFieldSep)
}

// decodeRackLayout parses a record. Anything malformed yields an empty layout
// for that field rather than an error: a preference that cannot be read is a
// preference that was never set, and refusing to boot over it would be worse
// than starting from the factory order.
func decodeRackLayout(s string) rackLayout {
	var l rackLayout
	for _, field := range strings.Split(s, layoutFieldSep) {
		kv := strings.SplitN(field, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "order":
			l.Order = splitLayoutList(kv[1])
		case "hidden":
			l.Hidden = splitLayoutList(kv[1])
		case "switches":
			l.Switches = splitLayoutList(kv[1])
		}
	}
	return l
}

// joinLayoutList drops anything carrying a separator.
//
// Module keys are header text lowercased and switch names are element ids, so
// today none of them can contain a comma or a semicolon. Today is not a
// guarantee: a module called "Gen X, Y" would otherwise write a record that
// read back as two modules, one of them named " y", and the rack would spend
// every subsequent boot restoring a module that does not exist.
func joinLayoutList(items []string) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || strings.ContainsAny(it, layoutFieldSep+layoutListSep+"=") {
			continue
		}
		out = append(out, it)
	}
	return strings.Join(out, layoutListSep)
}

func splitLayoutList(s string) []string {
	var out []string
	for _, it := range strings.Split(s, layoutListSep) {
		if it = strings.TrimSpace(it); it != "" {
			out = append(out, it)
		}
	}
	return out
}

// modulePinnedFrom decides whether a module gets a show/hide switch, from the
// three facts that decide it. Separated from the DOM so the rule can be stated
// and tested rather than inferred from three chained conditions.
//
//   - neverSwitched: the Console, or a module a MODE owns by class. No switch,
//     whatever else is true — a switch that could hide the switches is a door
//     that locks from the outside, and one that contradicts the mode is a way
//     to get stuck.
//   - rackHidden: the RACK put this away, through the switch itself. It must
//     keep its switch, because that switch is the only way back.
//   - displayNone: it is not on screen and the rack is not why. Something else
//     took it away and has its reasons.
//
// The middle case is the one that was missing, and it cost the module. A hidden
// module is display:none for exactly the reason the rack hid it, and restore
// hides before the switches are built, so "hidden" read as "pinned" and the
// switch was never made: hide a module, reload, and it was gone for good.
func modulePinnedFrom(neverSwitched, rackHidden, displayNone bool) bool {
	if neverSwitched {
		return true
	}
	if rackHidden {
		return false
	}
	return displayNone
}

// mergeModuleOrder is the saved order applied to the modules that actually
// exist: saved ones first, in the saved order, then everything else in the
// order it is already in.
//
// Both halves matter and both are about a rack that has changed since it was
// saved. A module named in the record but no longer built has to be skipped,
// or SetOrder is handed a key it cannot find. A module that exists but was
// never saved — the app shipped a new one since — has to land at the END:
// rack-go's SetOrder appends every key it is given, so anything left out of
// the list stays where it was and ends up in FRONT of the whole rack. A new
// module would have shoved the Console out of the first slot, which is the one
// module whose position anybody depends on.
func mergeModuleOrder(saved, present []string) []string {
	have := make(map[string]bool, len(present))
	for _, k := range present {
		have[k] = true
	}
	out := make([]string, 0, len(present))
	placed := make(map[string]bool, len(present))
	for _, k := range saved {
		if have[k] && !placed[k] {
			placed[k] = true
			out = append(out, k)
		}
	}
	for _, k := range present {
		if !placed[k] {
			placed[k] = true
			out = append(out, k)
		}
	}
	return out
}

// ringLabelsFit reports whether these option names can sit round a dial.
//
// A parameter cell is a third of a module wide, so the ring has room for a
// handful of short words and nothing more. Past either bound the labels run
// into each other and the dial stops being readable — which matters more here
// than it looks, because a named setting hides its numeric LED on the grounds
// that the ring IS the readout.
//
// selectorKnobReadout is the answer past that point, and says so in its own
// doc: "for selectors that have too many options or too-long labels for a ring
// of labels around the dial". The Phosphor, Backdrop, Skin and Desk-style knobs
// all take it; so do the twenty-odd terminal demos, whose names are words.
//
//nolint:unused // called from panelbuild_js.go and paramrings_js_test.go, both js-tagged, which the native lint pass cannot see
func ringLabelsFit(labels []string) bool {
	const maxRingLabels = 8
	const maxRingLabelRunes = 5
	if len(labels) > maxRingLabels {
		return false
	}
	for _, l := range labels {
		if len([]rune(l)) > maxRingLabelRunes {
			return false
		}
	}
	return true
}
