//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// The meters' readout labels, on character displays.
//
// Timing, Distortion, Loudness and Wow & Flutter are columns of readouts, each
// named over its number. The names were printed; on the bank they are dot
// matrix displays, which is what a front panel that says what a readout is
// reading uses, and these say the same kind of thing.
//
// Each display is the same part as the LED under it — the same height, the
// same width, the same bezel — so a column reads as pairs of equal windows.
// The name is written from the left, the way a character module writes, and
// the number under it runs to the right, so the two ends of a pair say which
// is which without a printed word between them.

// readoutLabelModules are the modules whose readout labels are displays.
var readoutLabelModules = []string{"timing-module", "thd-module", "lufs-module", "wf-module"}

// readoutLabelChars is every label display's size: a full display, the width
// of the readout it names.
const readoutLabelChars = dispFullChars

// dotReadoutLabels puts a display where each printed readout label was.
func dotReadoutLabels() {
	for _, id := range readoutLabelModules {
		ls := dom.Doc.Call("querySelectorAll", "#"+id+" .ledlbl")
		for i := range ls.Length() {
			l := ls.Index(i)
			text := strings.TrimSpace(l.Get("textContent").String())
			if text == "" {
				continue
			}
			d := dotDisplayN(text, false, readoutLabelChars)
			d.Get("classList").Call("add", "ledlbl", "dmdlbl")
			d.Call("setAttribute", "aria-label", text)
			l.Call("replaceWith", d)
			// A typed field under a display is the display's width like an
			// LED is, not the width of its digits (sizeLEDField): the CSS
			// stretches it, and the inline width would win.
			if f := d.Get("nextElementSibling"); f.Truthy() && f.Get("classList").Call("contains", "numin").Bool() {
				f.Get("style").Set("width", "")
			}
		}
	}
}

// A column of readouts holds what Timing's does: nine, two to a group and the
// last alone. Distortion and Wow & Flutter have fewer, and the rest of their
// column is spare positions — a dark display over a dark LED, the way the
// bank shows a position with nothing assigned — so a column is always the
// same part, full height, and a new reading has somewhere to go.
const (
	readoutColumnPairs = 9
	readoutGroupPairs  = 2
)

// fillReadoutColumns tops up every readout column (.readcol) with spares.
func fillReadoutColumns() {
	cols := dom.Doc.Call("querySelectorAll", ".readcol")
	for i := range cols.Length() {
		fillReadoutColumn(cols.Index(i))
	}
}

// fillReadoutColumn adds spare pairs to col until it holds readoutColumnPairs,
// filling its last group before starting another.
func fillReadoutColumn(col js.Value) {
	n := col.Call("querySelectorAll", ".led").Length()
	grp := col.Get("lastElementChild")
	for ; n < readoutColumnPairs; n++ {
		if !grp.Truthy() || grp.Call("querySelectorAll", ".led").Length() >= readoutGroupPairs {
			grp = dom.Doc.Call("createElement", "span")
			grp.Set("className", "grp vmbay")
			col.Call("appendChild", grp)
		}
		grp.Call("appendChild", spareReadout())
		led := dom.Doc.Call("createElement", "span")
		led.Set("className", "led spare")
		led.Set("title", doc("ro.spare"))
		grp.Call("appendChild", led)
	}
}

// spareReadout is a spare position's label: a display with nothing on it.
func spareReadout() js.Value {
	d := dotDisplayN("", false, readoutLabelChars)
	d.Get("classList").Call("add", "ledlbl", "dmdlbl", "spare")
	return d
}
