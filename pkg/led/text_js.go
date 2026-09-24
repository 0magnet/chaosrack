//go:build js && wasm

package led

import "syscall/js"

// Readouts puts text on LED readouts, skipping a write that would not change
// what one is showing. The zero value is ready to use.
//
// The meters latch on their own clocks — loudness every 200 ms, distortion
// every 400, wow and flutter every 500, the counter once a gate — and each
// latch wrote textContent on every LED in its panel whether the reading had
// moved or not. Setting textContent replaces the node's data and invalidates
// style and paint for that element even when the new string equals the old
// one, and these are the most heavily styled elements on the panel: each LED
// carries a text-shadow glow.
//
// With nothing playing into the rack — which is the state it sits in most of
// the time, and the state it was in when this was found — EVERY one of those
// writes is the identical string of dashes. Six LEDs five times a second from
// the loudness panel alone, about fifty a second across the four meters, all
// of them repainting a glowing readout to say exactly what it already said.
//
// So a readout remembers what it is showing and a write that would not change
// it does not happen — the knob rule, hold it on this side because the draw
// runs sixty times a second, applied to the writes instead of the reads.
type Readouts struct {
	showing map[string]string
}

// Set writes s to a readout, unless it is already showing s. The key names
// the readout; it is not the element id because the caller has the element
// already and a Get("id") would cost more than the map lookup saves.
func (r *Readouts) Set(key string, el js.Value, s string) {
	if !el.Truthy() {
		return
	}
	if was, ok := r.showing[key]; ok && was == s {
		return
	}
	if r.showing == nil {
		r.showing = map[string]string{}
	}
	r.showing[key] = s
	el.Set("textContent", s)
}

// Forget drops what the readouts were showing, for a panel that has just been
// rebuilt: the elements are new, so what the old ones displayed says nothing
// about them, and a memory left in place would leave a fresh LED blank until
// its reading happened to change.
func (r *Readouts) Forget() {
	clear(r.showing)
}
