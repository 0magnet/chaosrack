//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// meterDisplays are the measurement modules' displays, looked up once each.
var meterDisplays = map[string]js.Value{}

// heldReadouts are what the meters latched since the panel clock last ticked,
// by display: the latest text for each.
var heldReadouts = map[string]string{}

func init() { pclock.onTick(writeHeldReadouts) }

// showMeterReadouts writes what a module's displays say (meterreadouts.go)
// into them, on the panel's clock (panelclock.go). Each meter latches on its
// own period — loudness every 200 ms, distortion every 400 — and wrote on
// whatever frame that fell on, so the four of them dirtied the panel on frames
// of their own. A latch between ticks is written on the next one, at most
// 250 ms on; what each measures, and over what window, is unchanged.
func showMeterReadouts(rs []Readout) {
	for _, ro := range rs {
		if pclock.due {
			writeMeterReadout(ro.ID, ro.Text)
		} else {
			heldReadouts[ro.ID] = ro.Text
		}
	}
}

// writeHeldReadouts writes what was held for this tick.
func writeHeldReadouts() {
	for id, text := range heldReadouts {
		writeMeterReadout(id, text)
	}
	clear(heldReadouts)
}

func writeMeterReadout(id, text string) {
	el, ok := meterDisplays[id]
	if !ok {
		el = dom.Doc.Call("getElementById", id)
		meterDisplays[id] = el
	}
	owed.readouts.Set(id, el, text)
}

// markMeterDisplays tags a module's displays as readings (data-meter), so the
// catalog marks them for a rack without the page to write into.
func markMeterDisplays(rs []Readout) {
	for _, ro := range rs {
		if el := dom.Doc.Call("getElementById", ro.ID); el.Truthy() {
			el.Call("setAttribute", "data-meter", "")
		}
	}
}
