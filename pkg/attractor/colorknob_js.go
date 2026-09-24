//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// Color knobs: an analog way to dial a gradient color that keeps the module in
// character with the rest of the panel. Each color swatch (<input type=color>)
// gets a concentric knob beneath it: the outer ring turns Hue (ringed by a
// rainbow spectrum dial), the inner ring turns Level — a black → pure-hue →
// white shade axis (ringed by a matching gradient dial). Turning either updates
// the swatch (and the shader, via the swatch's existing input handler); picking
// a color with the native swatch turns the knobs to match.

// colorSyncing guards against the knob→swatch→knob loop.
var colorSyncing bool

// makeHueKnob builds a full-360° continuous rotary over slider (a 0..360 hue
// range): the pointer points straight at the hue's position on the rainbow
// ring, and dragging spins it all the way around with no dead zone (hue is
// cyclic). Returns a bare .knob so it stacks as a clean outer ring with a
// visible pointer.
func makeHueKnob(slider js.Value) js.Value {
	knob := dom.Doc.Call("createElement", "span")
	knob.Set("className", "knob knobb hueknob")
	knob.Call("setAttribute", "data-no-drag", "")
	knob.Set("title", "Hue — turn all the way around the spectrum")
	ptr := dom.Doc.Call("createElement", "i")
	ptr.Set("className", "knob-ptr")
	knob.Call("appendChild", ptr)

	update := func() {
		h, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		ptr.Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.FormatFloat(h, 'f', 1, 64)+"deg)")
	}
	update()
	slider.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any { update(); return nil }))

	dragging := false
	setFromEvent := func(e js.Value) {
		r := knob.Call("getBoundingClientRect")
		cx := r.Get("left").Float() + r.Get("width").Float()/2
		cy := r.Get("top").Float() + r.Get("height").Float()/2
		ang := math.Atan2(e.Get("clientY").Float()-cy, e.Get("clientX").Float()-cx)*180/math.Pi + 90
		for ang < 0 {
			ang += 360
		}
		ang = math.Mod(ang, 360)
		slider.Set("value", strconv.FormatFloat(ang, 'f', 0, 64))
		slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	knob.Call("addEventListener", "pointerdown", dom.FuncOf(func(this js.Value, a []js.Value) any {
		e := a[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		dragging = true
		knob.Call("setPointerCapture", e.Get("pointerId"))
		setFromEvent(e)
		return nil
	}))
	knob.Call("addEventListener", "pointermove", dom.FuncOf(func(this js.Value, a []js.Value) any {
		if dragging {
			setFromEvent(a[0])
		}
		return nil
	}))
	rel := dom.FuncOf(func(this js.Value, a []js.Value) any { dragging = false; return nil })
	knob.Call("addEventListener", "pointerup", rel)
	knob.Call("addEventListener", "pointercancel", rel)
	knob.Call("addEventListener", "wheel", dom.FuncOf(func(this js.Value, a []js.Value) any {
		e := a[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		h, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		if e.Get("deltaY").Float() < 0 {
			h += 6
		} else {
			h -= 6
		}
		for h < 0 {
			h += 360
		}
		h = math.Mod(h, 360)
		slider.Set("value", strconv.FormatFloat(h, 'f', 0, 64))
		slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return nil
	}))
	return knob
}

// buildColorKnob returns a concentric color knob wired two-way to a
// <input type=color>, styled to stay in character with the rest of the panel:
// the OUTER ring turns Hue and is ringed by a rainbow spectrum dial (in place
// of tick marks); the INNER ring turns Level — a black → pure-hue → white shade
// axis — ringed by a matching gradient dial that recolors with the hue. The
// native swatch (shown above, current color) stays the source of truth for
// arbitrary colors; the knob is the analog way to dial one in.
func buildColorKnob(colorInput js.Value) js.Value {
	mkRange := func(max int, val float64) js.Value {
		r := dom.Doc.Call("createElement", "input")
		r.Set("type", "range")
		r.Set("min", "0")
		r.Set("max", strconv.Itoa(max))
		r.Set("step", "1")
		r.Set("value", strconv.FormatFloat(val, 'f', 0, 64))
		r.Set("style", "display:none")
		return r
	}
	h0, s0, v0 := knobHSV(colorInput.Get("value").String())
	hueR := mkRange(360, h0)
	levR := mkRange(100, svToLevel(s0, v0))

	// Name every part of this knob from the swatch it drives (single source: the
	// <input type=color>'s title), so the start / middle / end / background knobs
	// are each identifiable instead of a generic "Color knob".
	name := colorInput.Get("title").String()
	if name == "" {
		name = "Color"
	}
	levR.Set("title", name+" — Level (black → color → white)")
	hueKnob := makeHueKnob(hueR) // full-360° rainbow rotary
	hueKnob.Set("title", name+" — Hue: turn all the way around the spectrum")
	levKnob := makeKnob(levR, js.Undefined(), false, false, false)
	stack := stackKnobs(hueKnob, levKnob)
	stack.Set("title", name+" knob — outer ring = Hue (rainbow scale), inner ring = Level (black → color → white)")

	// Gradient dials behind the knobs (drawn as conic rings, no tick marks).
	addColorDial := func(cls string) js.Value {
		d := dom.Doc.Call("createElement", "span")
		d.Set("className", "ck-dial "+cls)
		stack.Call("insertBefore", d, stack.Get("firstChild"))
		return d
	}
	levDial := addColorDial("ck-level") // inner ring, recolors with hue
	addColorDial("ck-hue")              // outer full-360° rainbow ring
	setHueCol := func(h float64) {
		levDial.Get("style").Call("setProperty", "--hue-col", knobHex(h, 1, 1))
	}
	setHueCol(h0)

	apply := func() {
		if colorSyncing {
			return
		}
		h, _ := strconv.ParseFloat(hueR.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		l, _ := strconv.ParseFloat(levR.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		s, v := levelToSV(l)
		colorSyncing = true
		colorInput.Set("value", knobHex(h, s, v))
		colorInput.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		colorSyncing = false
		setHueCol(h)
	}
	for _, rng := range []js.Value{hueR, levR} {
		rng.Call("addEventListener", "input", dom.FuncOf(func(this js.Value, a []js.Value) any { apply(); return nil }))
	}
	// External swatch pick → turn the knobs (and recolor the level dial) to match.
	syncFromSwatch := dom.FuncOf(func(this js.Value, a []js.Value) any {
		if colorSyncing {
			return nil
		}
		h, s, v := knobHSV(colorInput.Get("value").String())
		colorSyncing = true
		hueR.Set("value", strconv.FormatFloat(h, 'f', 0, 64))
		levR.Set("value", strconv.FormatFloat(svToLevel(s, v), 'f', 0, 64))
		hueR.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		levR.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		colorSyncing = false
		setHueCol(h)
		return nil
	})
	colorInput.Call("addEventListener", "change", syncFromSwatch)
	colorInput.Call("addEventListener", "input", syncFromSwatch)

	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "colorknob")
	wrap.Call("appendChild", hueR)
	wrap.Call("appendChild", levR)
	wrap.Call("appendChild", stack)
	return wrap
}

// attachColorKnobs adds a Hue/Level color knob under each palette swatch (start
// / mid / end / bg), dropping it into that swatch's dedicated #ck-<id> holder so
// it lands beneath the swatch in the templated palette cell.
func attachColorKnobs() {
	for _, id := range []string{"color-base", "color-mid", "color-top", "color-bg"} {
		ci := dom.Doc.Call("getElementById", id)
		if !ci.Truthy() {
			continue
		}
		holder := dom.Doc.Call("getElementById", "ck-"+id)
		if !holder.Truthy() {
			continue
		}
		holder.Call("appendChild", buildColorKnob(ci))
		// LED readout of the HTML color (hex) below the knob, pinned to the cell
		// bottom so it doesn't shift the knob's centering.
		if cell := holder.Get("parentNode"); cell.Truthy() {
			hex := dom.Doc.Call("createElement", "span")
			hex.Set("className", "led pal-hex")
			ci := ci
			upd := dom.FuncOf(func(this js.Value, a []js.Value) any {
				v := ci.Get("value").String()
				hex.Set("textContent", strings.ToUpper(strings.TrimPrefix(v, "#")))
				return nil
			})
			ci.Call("addEventListener", "input", upd)
			upd.Invoke()
			cell.Call("appendChild", hex)
		}
	}
}
