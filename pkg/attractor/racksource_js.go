//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// The rack, as a front end sees it from inside.
//
// internal/rackcable is the same three methods over a wire: it asks a browser
// for window.rackctl and pays a round trip per read. This one is in the
// process the rack is in, so it reads the registry adoptDescControl filled and
// the elements the DOM panel's own knobs are bound to. Thirty lines against
// that file's two hundred, and the difference is all transport.

// inPageRack is a racktui.Source backed by this process.
type inPageRack struct{}

func (inPageRack) Rack() (string, error) {
	keys, cats, slots := rackModulesNow()
	return DrawRackFrom(keys, cats, slots, unitCapacitySlots(), bayMonitorSlots), nil
}

func (inPageRack) Controls() ([]racktui.Control, error) {
	reg := ControlRegistry()
	out := make([]racktui.Control, 0, len(reg))
	for _, in := range reg {
		c := racktui.Control{ControlInfo: in}
		if el := doc.Call("getElementById", in.ID); el.Truthy() {
			c.Value = el.Get("value").String()
			if el.Get("tagName").String() == "SELECT" {
				opts := el.Get("options")
				for i := 0; i < opts.Get("length").Int(); i++ {
					c.Options = append(c.Options, opts.Index(i).Get("value").String())
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// Set moves a control the way rackctl.set does, so the panel in the page and
// the cable from a host take exactly the same path into the rack.
func (inPageRack) Set(id, value string) error {
	el := doc.Call("getElementById", id)
	if !el.Truthy() {
		return errNoControl{id}
	}
	el.Set("value", value)
	ev := js.Global().Get("Event")
	for _, kind := range []string{"input", "change"} {
		opt := js.Global().Get("Object").New()
		opt.Set("bubbles", true)
		el.Call("dispatchEvent", ev.New(kind, opt))
	}
	return nil
}

type errNoControl struct{ id string }

func (e errNoControl) Error() string { return "the rack has no control " + strconv.Quote(e.id) }

// rackModulesNow reads the panel the way the drawing needs it: a name, a slot
// count, and the category a model card belongs to.
func rackModulesNow() (keys, cats []string, slots []int) {
	f := rackFrame()
	if !f.Truthy() {
		return nil, nil, nil
	}
	els := f.Call("querySelectorAll", ".sect")
	for i := 0; i < els.Get("length").Int(); i++ {
		m := els.Index(i)
		if !m.Call("querySelector", ".sect-hdr").Truthy() {
			continue
		}
		keys = append(keys, moduleKeyOf(m))
		slots = append(slots, moduleSlots(m))
		cat := ""
		if c := m.Call("getAttribute", "data-cat"); c.Truthy() {
			cat = c.String()
		}
		cats = append(cats, cat)
	}
	return keys, cats, slots
}
