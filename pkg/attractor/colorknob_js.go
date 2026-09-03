//go:build js && wasm

package attractor

import (
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

// hexRGB64 wraps the existing float32 hexToRGB as float64 for the HSV math.
func hexRGB64(hex string) (r, g, b float64) {
	r32, g32, b32 := hexToRGB(hex)
	return float64(r32), float64(g32), float64(b32)
}

func rgbToHex(r, g, b float64) string {
	cl := func(x float64) int64 {
		v := int64(math.Round(x * 255))
		if v < 0 {
			v = 0
		} else if v > 255 {
			v = 255
		}
		return v
	}
	h := "0123456789abcdef"
	to := func(v int64) string { return string([]byte{h[v>>4], h[v&0xf]}) }
	return "#" + to(cl(r)) + to(cl(g)) + to(cl(b))
}

// rgb2hsv: h in 0..360, s/v in 0..1.
func rgb2hsv(r, g, b float64) (h, s, v float64) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	v = mx
	d := mx - mn
	if mx > 0 {
		s = d / mx
	}
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// hsv2rgb: h 0..360, s/v 0..1.
func hsv2rgb(h, s, v float64) (r, g, b float64) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

// buildColorKnob returns a concentric color knob wired two-way to a
// <input type=color>, styled to stay in character with the rest of the panel:
// the OUTER ring turns Hue and is ringed by a rainbow spectrum dial (in place
// of tick marks); the INNER ring turns Level — a black → pure-hue → white shade
// axis — ringed by a matching gradient dial that recolors with the hue. The
// native swatch (shown above, current color) stays the source of truth for
// arbitrary colors; the knob is the analog way to dial one in.
// colorSyncing guards against the knob→swatch→knob loop.
var colorSyncing bool

// levelToSV maps the 0..100 Level axis to HSV saturation/value: 0 = black,
// 50 = pure saturated hue, 100 = white.
func levelToSV(l float64) (s, v float64) {
	if l <= 50 {
		return 1, l / 50
	}
	return 1 - (l-50)/50, 1
}

// svToLevel is the inverse used when an arbitrary swatch color is picked; it is
// approximate for muted colors (the swatch, not the knob, is authoritative).
func svToLevel(s, v float64) float64 {
	if v < 0.999 {
		return v * 50
	}
	return 50 + (1-s)*50
}

// makeHueKnob builds a full-360° continuous rotary over slider (a 0..360 hue
// range): the pointer points straight at the hue's position on the rainbow
// ring, and dragging spins it all the way around with no dead zone (hue is
// cyclic). Returns a bare .knob so it stacks as a clean outer ring with a
// visible pointer.
func makeHueKnob(slider js.Value) js.Value {
	knob := doc.Call("createElement", "div")
	knob.Set("className", "knob knobb hueknob")
	knob.Call("setAttribute", "data-no-drag", "")
	knob.Set("title", "Hue — turn all the way around the spectrum")
	ptr := doc.Call("createElement", "i")
	ptr.Set("className", "knob-ptr")
	knob.Call("appendChild", ptr)

	update := func() {
		h, _ := strconv.ParseFloat(slider.Get("value").String(), 64) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		ptr.Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.FormatFloat(h, 'f', 1, 64)+"deg)")
	}
	update()
	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { update(); return nil }))

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
	knob.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		dragging = true
		knob.Call("setPointerCapture", e.Get("pointerId"))
		setFromEvent(e)
		return nil
	}))
	knob.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if dragging {
			setFromEvent(a[0])
		}
		return nil
	}))
	rel := trackedFuncOf(func(this js.Value, a []js.Value) interface{} { dragging = false; return nil })
	knob.Call("addEventListener", "pointerup", rel)
	knob.Call("addEventListener", "pointercancel", rel)
	knob.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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

func buildColorKnob(colorInput js.Value) js.Value {
	mkRange := func(max int, val float64) js.Value {
		r := doc.Call("createElement", "input")
		r.Set("type", "range")
		r.Set("min", "0")
		r.Set("max", strconv.Itoa(max))
		r.Set("step", "1")
		r.Set("value", strconv.FormatFloat(val, 'f', 0, 64))
		r.Set("style", "display:none")
		return r
	}
	r0, g0, b0 := hexRGB64(colorInput.Get("value").String())
	h0, s0, v0 := rgb2hsv(r0, g0, b0)
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
		d := doc.Call("createElement", "div")
		d.Set("className", "ck-dial "+cls)
		stack.Call("insertBefore", d, stack.Get("firstChild"))
		return d
	}
	levDial := addColorDial("ck-level") // inner ring, recolors with hue
	addColorDial("ck-hue")              // outer full-360° rainbow ring
	setHueCol := func(h float64) {
		levDial.Get("style").Call("setProperty", "--hue-col", rgbToHex(hsv2rgb(h, 1, 1)))
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
		colorInput.Set("value", rgbToHex(hsv2rgb(h, s, v)))
		colorInput.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		colorSyncing = false
		setHueCol(h)
	}
	for _, rng := range []js.Value{hueR, levR} {
		rng.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { apply(); return nil }))
	}
	// External swatch pick → turn the knobs (and recolor the level dial) to match.
	syncFromSwatch := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if colorSyncing {
			return nil
		}
		r, g, b := hexRGB64(colorInput.Get("value").String())
		h, s, v := rgb2hsv(r, g, b)
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

	wrap := doc.Call("createElement", "span")
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
		ci := doc.Call("getElementById", id)
		if !ci.Truthy() {
			continue
		}
		holder := doc.Call("getElementById", "ck-"+id)
		if !holder.Truthy() {
			continue
		}
		holder.Call("appendChild", buildColorKnob(ci))
		// LED readout of the HTML color (hex) below the knob, pinned to the cell
		// bottom so it doesn't shift the knob's centring.
		if cell := holder.Get("parentNode"); cell.Truthy() {
			hex := doc.Call("createElement", "span")
			hex.Set("className", "led pal-hex")
			ci := ci
			upd := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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
