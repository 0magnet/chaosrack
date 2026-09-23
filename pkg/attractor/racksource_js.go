//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/racksurface"
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

// Modules is what the frame holds, measured off the laid-out panel and then
// grouped into bays by the same rule the rack itself uses — so the terminal
// cannot put a module in a bay the page would not.
//
// The heights are left at zero: how tall a module needs to be depends on how
// its controls are drawn, and that is the renderer's to decide.
func (inPageRack) Modules() ([]racksurface.Item, int, error) {
	keys, cats, slots := rackModulesNow()
	return RackItemsFrom(keys, cats, slots, nil), unitCapacitySlots(), nil
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

// CellAspect measures the terminal the panel is drawing on, so the dials come
// out round.
//
// The one thing a panel in a page can do that a panel on a host terminal
// cannot. A terminal does not report its font — no escape sequence, no termios
// field — so a panel over a cable has to assume the 1:2 that console fonts
// usually are. Here the terminal is an element, and the browser will measure
// it: at a nine-pixel font xterm-go's cell is 5.39 x 12.8, an aspect of 2.37,
// which is far enough from 2 to draw every knob as a visible ellipse.
//
// Measured with a hidden span in the terminal's own font rather than read off
// xterm-go, because what matters is the shape the glyphs are actually drawn
// at, and that is a question for the font engine.
func (inPageRack) CellAspect() float64 {
	host := termHost
	if s := deskTerminalEl(); s.Truthy() {
		host = s
	}
	if !host.Truthy() {
		return 0
	}
	st := js.Global().Get("getComputedStyle").Invoke(host)
	span := doc.Call("createElement", "span")
	span.Get("style").Set("cssText",
		"position:absolute;visibility:hidden;white-space:pre;font-family:"+
			st.Get("fontFamily").String()+";font-size:"+st.Get("fontSize").String())
	span.Set("textContent", "XXXXXXXXXX")
	doc.Get("body").Call("appendChild", span)
	r := span.Call("getBoundingClientRect")
	w, h := r.Get("width").Float()/10, r.Get("height").Float()
	span.Call("remove")
	if w <= 0 || h <= 0 {
		return 0
	}
	return h / w
}

// deskTerminalEl is the terminal a desk window is showing, if there is one.
// The panel is usually typed into that rather than into the Terminal model's
// own shell, and the two can be at different zooms.
func deskTerminalEl() js.Value {
	els := doc.Call("querySelectorAll", ".xterm")
	for i := els.Get("length").Int() - 1; i >= 0; i-- {
		el := els.Index(i)
		// Not the Terminal model's own shell, which is parked off screen: the
		// panel is usually typed into a desk window, and the two can be at
		// different zooms. contains rather than Equal because termHost is the
		// container the session was mounted on and .xterm is inside it.
		if termHost.Truthy() && termHost.Call("contains", el).Bool() {
			continue
		}
		return el
	}
	return js.Value{}
}
