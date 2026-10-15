//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// meterDisplays are the measurement modules' displays, looked up once each.
var meterDisplays = map[string]js.Value{}

// showMeterReadouts writes what a module's displays say (meterreadouts.go)
// into them.
func showMeterReadouts(rs []Readout) {
	for _, ro := range rs {
		el, ok := meterDisplays[ro.ID]
		if !ok {
			el = dom.Doc.Call("getElementById", ro.ID)
			meterDisplays[ro.ID] = el
		}
		owed.readouts.Set(ro.ID, el, ro.Text)
	}
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
