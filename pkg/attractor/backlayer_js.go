//go:build js && wasm

package attractor

import (
	"image/color"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/colorspace"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
)

// backLayer is the backdrop's own coloring (backlayer.go), and whether the
// panel is pointed at it.
type backLayer struct {
	pal layerPalette // the backdrop's palette, while the panel is not on it

	// editing is whether the Back switch has the panel on the backdrop. While
	// it does, the color globals and the controls ARE the backdrop's palette,
	// and the model's is held in front.
	editing bool
	front   layerPalette

	drawing bool // inside the backdrop's pass (renderBackgroundVisual)

	// lut is the palette as a 256×1 texture, for recoloring a picture by its
	// brightness in the textured shader; lutOf is the palette it holds.
	lut   js.Value
	lutOf layerPalette
	lutJS js.Value
}

var back = backLayer{pal: defaultBackPalette}

// backLUTUnit is the texture unit the backdrop's palette is bound to: not 0,
// which the picture itself is on, and not the trace's colormap unit.
const backLUTUnit = 2

// currentPalette is the palette the color globals hold now.
func currentPalette() layerPalette {
	return layerPalette{
		src: style.gradientSource, cols: style.gradientColors,
		base: style.baseColor, mid: style.midColor, top: style.topColor,
		freq: style.gradientFreq, shift: gradientShift,
	}
}

// install puts p into the color globals, and nothing else.
func (p layerPalette) install() {
	style.gradientSource, style.gradientColors = p.src, p.cols
	style.baseColor, style.midColor, style.topColor = p.base, p.mid, p.top
	style.gradientFreq, gradientShift = p.freq, p.shift
}

// show puts p onto the Colors module's controls, each through its own input
// event, so its knob and readout follow and its handler writes p into the
// globals and the shader — the way a view's focus puts its colors up.
func (p layerPalette) show() {
	setSelectQuiet("gradient-source", p.src)
	setSelectQuiet("gradient-colors", p.cols)
	for id, c := range map[string][3]float32{"color-base": p.base, "color-mid": p.mid, "color-top": p.top} {
		if el := dom.Doc.Call("getElementById", id); el.Truthy() {
			el.Set("value", colorspace.Hex(c))
			el.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
	}
	for id, v := range map[string]float32{"rainbow-freq": p.freq, "palette-shift": p.shift} {
		if el := dom.Doc.Call("getElementById", id); el.Truthy() {
			el.Set("value", strconv.FormatFloat(float64(v), 'g', -1, 32))
			el.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
	}
	p.install() // whatever a handler did not write
	updateGradientUI()
}

// backIn puts the backdrop's palette in for its pass and returns what puts
// the panel's back. While the panel is on the backdrop there is nothing to
// swap: the globals are its palette already.
func (b *backLayer) backIn() func() {
	b.drawing = true
	if b.editing {
		return func() { b.drawing = false }
	}
	held := currentPalette()
	b.pal.install()
	return func() {
		held.install()
		b.drawing = false
	}
}

// frontIn puts the model's own palette in for its draw while the panel holds
// the backdrop's, and returns what puts the panel's back. The swatches reach
// the shader only when they change, so they are uploaded here each frame
// while it is needed.
func (b *backLayer) frontIn() func() {
	if !b.editing {
		return func() {}
	}
	held := currentPalette()
	b.front.install()
	uploadSwatches()
	return func() {
		held.install()
		uploadSwatches()
	}
}

// uploadSwatches sends the three swatches in the globals to the trace shader.
func uploadSwatches() {
	if !gpu.ready {
		return
	}
	glctx.GL.Call("useProgram", gpu.program)
	glctx.GL.Call("uniform3f", gpu.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
	glctx.GL.Call("uniform3f", gpu.u.midColor, style.midColor[0], style.midColor[1], style.midColor[2])
	glctx.GL.Call("uniform3f", gpu.u.topColor, style.topColor[0], style.topColor[1], style.topColor[2])
}

// recolors reports whether the backdrop being drawn now takes its palette
// over its own colors.
func (b *backLayer) recolors() bool {
	return b.drawing && backdropRecolors(bgVisual, style.gradientSource)
}

// xyColor is the xy scope's trace color as a backdrop in its palette: the
// top of the map, the brightest thing it has to say.
func (b *backLayer) xyColor() ([3]float32, bool) {
	if !b.recolors() {
		return [3]float32{}, false
	}
	return rgbOf(mapColorAt(1)), true
}

func rgbOf(c color.Color) [3]float32 {
	r, g, bl, _ := c.RGBA()
	return [3]float32{float32(r) / 0xffff, float32(g) / 0xffff, float32(bl) / 0xffff}
}

// bindLUT readies the palette texture for a textured draw and says whether
// the draw should read it. Rebuilt only when the palette changes.
func (b *backLayer) bindLUT() bool {
	if !b.recolors() || bgVisual == "spectrogram" { // its columns are colored as they are written
		return false
	}
	p := currentPalette()
	if b.lut.IsUndefined() {
		b.lut = glctx.GL.Call("createTexture")
		b.lutJS = js.Global().Get("Uint8Array").New(colormap.Texels * 4)
		b.lutOf.cols = -1
	}
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0").Int()+backLUTUnit)
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), b.lut)
	if p != b.lutOf {
		px := make([]byte, colormap.Texels*4)
		for i := range colormap.Texels {
			c := rgbOf(mapColorAt(float64(i) / float64(colormap.Texels-1)))
			px[4*i], px[4*i+1], px[4*i+2], px[4*i+3] = byte(c[0]*255), byte(c[1]*255), byte(c[2]*255), 255
		}
		js.CopyBytesToJS(b.lutJS, px)
		t2d := glctx.GL.Get("TEXTURE_2D")
		glctx.GL.Call("texImage2D", t2d, 0, glctx.GL.Get("RGBA"), colormap.Texels, 1, 0, glctx.GL.Get("RGBA"), glctx.GL.Get("UNSIGNED_BYTE"), b.lutJS)
		for _, kv := range [][2]string{{"TEXTURE_MIN_FILTER", "LINEAR"}, {"TEXTURE_MAG_FILTER", "LINEAR"}, {"TEXTURE_WRAP_S", "CLAMP_TO_EDGE"}, {"TEXTURE_WRAP_T", "CLAMP_TO_EDGE"}} {
			glctx.GL.Call("texParameteri", t2d, glctx.GL.Get(kv[0]), glctx.GL.Get(kv[1]))
		}
		b.lutOf = p
	}
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
	return true
}

// editMode is the model the panel shows: the backdrop while Back is on, the
// running model otherwise. Every backdrop with colors is also a model, so the
// MODEL knob and the bank show its own controls.
func editMode() string {
	if back.editing {
		return bgVisual
	}
	return run.selectedMode
}

// setEditing points the Colors module (and the MODEL knob and the bank) at
// the backdrop, or back at the model. One panel, two layers.
func (b *backLayer) setEditing(on bool) {
	if on && !flatBackdrop(bgVisual) {
		on = false // nothing behind with colors of its own
	}
	if sw := dom.Doc.Call("getElementById", "edit-back"); sw.Truthy() {
		sw.Set("checked", on)
	}
	if on == b.editing {
		return
	}
	if on {
		b.front = currentPalette()
		b.editing = true
		b.pal.show()
	} else {
		b.pal = currentPalette()
		b.editing = false
		b.front.show()
		uploadSwatches()
	}
	if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
		ms.Set("value", editMode())
	}
	syncCategoryRotaries()
	syncBankCells()
	if !on {
		perma.syncPermalinkNow() // held while the panel showed the backdrop
	}
}

// chooseBackdrop is the MODEL knob while the panel is on the backdrop: a
// backdrop it can be becomes the backdrop, and anything else is refused.
func chooseBackdrop(mode string) {
	if flatBackdrop(mode) && mode != bgVisual {
		if bv := dom.Doc.Call("getElementById", "bg-visual"); bv.Truthy() {
			bv.Set("value", mode)
			bv.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
		ms.Set("value", editMode())
	}
	syncCategoryRotaries()
	syncBankCells()
}

// onBackdropChoice follows the BEHIND knob: the Back switch dims where there
// is nothing behind with colors, and turns off there.
func onBackdropChoice(kind string) {
	if !flatBackdrop(kind) {
		back.setEditing(false)
	} else if back.editing {
		syncCategoryRotaries()
		syncBankCells()
	}
	if sw := dom.Doc.Call("getElementById", "edit-back"); sw.Truthy() {
		if l := sw.Call("closest", "label"); l.Truthy() {
			l.Get("classList").Call("toggle", "layer-dim", !flatBackdrop(kind))
		}
	}
}

// wireBackLayer hooks up the Back switch.
func wireBackLayer() {
	sw := dom.Doc.Call("getElementById", "edit-back")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", dom.FuncOf(func(js.Value, []js.Value) any {
		back.setEditing(sw.Get("checked").Bool())
		return nil
	}))
	onBackdropChoice(bgVisual)
}

// permaString is the backdrop's palette for a link, or "" at its default.
func (b *backLayer) permaString() string {
	p := b.pal
	if b.editing {
		p = currentPalette()
	}
	if p == defaultBackPalette {
		return ""
	}
	return p.String()
}

// applyPerma takes the backdrop's palette from a link.
func (b *backLayer) applyPerma(s string) {
	if p, ok := parseLayerPalette(s); ok {
		if b.editing {
			p.show()
		} else {
			b.pal = p
		}
	}
}

// frontValue is what a Colors control would read for the MODEL while the
// panel shows the backdrop's palette, in the form a link writes it — so a
// link, a preset or a patch memory taken then still describes the model.
func (b *backLayer) frontValue(id string) (string, bool) {
	if !b.editing {
		return "", false
	}
	p := b.front
	hex := func(c [3]float32) string { return strings.TrimPrefix(colorspace.Hex(c), "#") }
	f := func(x float32) string { return strconv.FormatFloat(float64(x), 'g', -1, 32) }
	switch id {
	case "gradient-source":
		return strconv.Itoa(p.src), true
	case "gradient-colors":
		return strconv.Itoa(p.cols), true
	case "color-base":
		return hex(p.base), true
	case "color-mid":
		return hex(p.mid), true
	case "color-top":
		return hex(p.top), true
	case "rainbow-freq":
		return f(p.freq), true
	case "palette-shift":
		return f(p.shift), true
	}
	return "", false
}
