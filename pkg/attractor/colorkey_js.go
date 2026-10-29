//go:build js && wasm

package attractor

import (
	"fmt"
	"math"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// The color key's view (colorkey.go): a small canvas at the top left of the
// stage, the part of the window the model is drawn in.
//
// On the HUD layer (panel.css's --z-hud), with the meters and the info
// overlay: above the model's canvas, which is otherwise all it would show.
//
// A canvas of its own and not text in the page, because a text write on this
// page repaints the rack (panelclock.go), and the key's live range changes as
// often as the sound does. It is drawn on the panel clock, and only when what
// it would show has changed.

// Its size, in CSS pixels.
const (
	colorKeyW = 232
	colorKeyH = 46
)

// colorKeyView is the key's canvas and what it last drew.
type colorKeyView struct {
	cv, g, sw js.Value
	shown     string   // what the canvas holds, so an unchanged key draws nothing
	at        [2]int   // where it was put, so an unmoved key writes no style
	px        []byte   // the bar's pixels, reused
	img       js.Value // their ImageData
	dpr       float64
	visible   bool
}

var ckey colorKeyView

func init() { pclock.onTick(ckey.tick) }

// tick draws the key if it is on and has changed: K on the MAP ring's P-unit.
func (k *colorKeyView) tick() {
	if !k.sw.Truthy() {
		k.sw = dom.Doc.Call("getElementById", "color-key-sw")
	}
	co := pageTraceColoring()
	key, ok := colorKeyFor(style.gradientSource, run.selectedMode, int(stereo.axesF+0.5), &acolor.AudioColor)
	if !ok || !k.sw.Truthy() || !k.sw.Get("checked").Bool() || co.cols == 1 {
		k.hide()
		return
	}
	if !k.cv.Truthy() && !k.make() {
		return
	}
	k.place()
	// The hue sweep turns with its phase every frame, so its key does too.
	sig := fmt.Sprintf("%+v|%d|%v|%v|%v|%v|%v|%v|%v", key, co.cols, co.reverse, co.freq, co.sft, co.phase, co.base, co.mid, co.top)
	if sig == k.shown {
		return
	}
	k.shown = sig
	k.draw(key, co)
}

// make puts the canvas on the page.
func (k *colorKeyView) make() bool {
	body := dom.Doc.Get("body")
	if !body.Truthy() {
		return false
	}
	k.dpr = math.Max(js.Global().Get("devicePixelRatio").Float(), 1)
	cv := dom.Doc.Call("createElement", "canvas")
	cv.Set("id", "color-key")
	cv.Set("width", int(colorKeyW*k.dpr))
	cv.Set("height", int(colorKeyH*k.dpr))
	st := cv.Get("style")
	st.Set("cssText", fmt.Sprintf("position:fixed;left:0;top:0;width:%dpx;height:%dpx;pointer-events:none;z-index:var(--z-hud);", colorKeyW, colorKeyH))
	body.Call("appendChild", cv)
	k.cv, k.g = cv, cv.Call("getContext", "2d")
	bw, bh := k.barSize()
	k.px = make([]byte, bw*bh*4)
	k.img = k.g.Call("createImageData", bw, bh)
	k.at = [2]int{-1, -1}
	return true
}

// place moves the key to the stage's top left, if the stage has moved.
func (k *colorKeyView) place() {
	w, h := js.Global().Get("innerWidth").Float(), js.Global().Get("innerHeight").Float()
	at := [2]int{int(stage.x0*w) + 12, int(stage.y0*h) + 12}
	if k.visible && at == k.at {
		return
	}
	st := k.cv.Get("style")
	if !k.visible {
		k.visible = true
		st.Set("display", "")
	}
	k.at = at
	st.Set("transform", fmt.Sprintf("translate(%dpx,%dpx)", at[0], at[1]))
}

// hide takes the key off the screen, and forgets what it showed so it is
// drawn afresh when it comes back.
func (k *colorKeyView) hide() {
	if k.cv.Truthy() && k.visible {
		k.cv.Get("style").Set("display", "none")
		k.visible, k.shown = false, ""
	}
}

// barSize is the color bar in device pixels.
func (k *colorKeyView) barSize() (int, int) {
	return int((colorKeyW - 16) * k.dpr), int(8 * k.dpr)
}

// draw paints the key: the source over the bar, the bar through the map as
// the trace has it, and what the bottom, middle and top mean under it.
func (k *colorKeyView) draw(key colorKey, co traceColoring) {
	g, d := k.g, k.dpr
	g.Call("setTransform", 1, 0, 0, 1, 0, 0)
	g.Call("clearRect", 0, 0, colorKeyW*d, colorKeyH*d)
	g.Call("setTransform", d, 0, 0, d, 0, 0)
	g.Set("fillStyle", "rgba(0,0,0,0.55)")
	g.Call("fillRect", 0, 0, colorKeyW, colorKeyH)

	g.Set("font", "11px monospace")
	g.Set("textBaseline", "top")
	g.Set("fillStyle", "#cfd8e3")
	g.Set("textAlign", "left")
	g.Call("fillText", key.Title, 8, 4)

	bw, bh := k.barSize()
	for x := range bw {
		c := co.colorOf(float32(x) / float32(max(bw-1, 1)))
		for y := range bh {
			o := (y*bw + x) * 4
			k.px[o], k.px[o+1], k.px[o+2], k.px[o+3] = byte255(c[0]), byte255(c[1]), byte255(c[2]), 255
		}
	}
	js.CopyBytesToJS(k.img.Get("data"), k.px)
	g.Call("putImageData", k.img, 8*d, 18*d)

	g.Set("font", "10px monospace")
	g.Set("fillStyle", "#9fb3c8")
	label := func(s, align string, x float64) {
		if s == "" {
			return
		}
		g.Set("textAlign", align)
		g.Call("fillText", s, x, 30)
	}
	label(key.Lo, "left", 8)
	label(key.Mid, "center", colorKeyW/2)
	label(key.Hi, "right", colorKeyW-8)
}
