//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

// Background visualizer: render a live audio display (scrolling spectrogram or
// xy/Lissajous scope) BEHIND the current attractor / geometry model, so the
// attractor floats over a reactive backdrop. It is a layer, not a mode — the
// spectrogram and xy MODES still exist (they replace the model); this draws the
// same visuals as a full-screen background under a normal model instead.
//
// Driven from renderLoop: after the frame is cleared and before the attractor
// is generated, renderBackgroundVisual() paints the backdrop with depth-testing
// off (a flat 2D layer); the attractor then draws on top with depth on.

// bgVisual selects the background layer: "" (off) | "spectrogram" | "xy".
var bgVisual string

// bgVisualActive reports whether a background visualizer is enabled AND the
// current model is a normal one (not itself a visualizer mode, which would
// already fill the screen).
func bgVisualActive() bool {
	if bgVisual == "" {
		return false
	}
	// Water is in the same selector because that is where a hand looks for it,
	// but it is not a backdrop: it is drawn THROUGH the finished frame rather
	// than behind it, so the pre-pass must not clear for it or try to paint it.
	// See drawWaterLens, called after the model instead.
	if bgVisual == "water" {
		return false
	}
	if isSpectroSurface(run.selectedMode) || isAudioMode(run.selectedMode) {
		return false
	}
	return true
}

// setBackgroundVisual switches the background layer. Turning one on ensures the
// shared audio source is live so samples start flowing immediately.
func setBackgroundVisual(kind string) {
	bgVisual = kind
	if bgVisual != "" {
		aud.ensureAudioSource()
	}
}

// renderBackgroundVisual paints the selected backdrop onto the (already
// cleared) color buffer with depth off. Leaves depth test disabled; the caller
// re-enables it before drawing the attractor.
//
// In the backdrop's own palette (backlayer_js.go), which the spectrogram's
// columns are written in and the other pictures are recolored by.
func renderBackgroundVisual(nowMs float64) {
	defer back.backIn()()
	switch bgVisual {
	case "xy":
		xy.drawXYScope(false) // no self-clear — layer onto the current buffer
	case "spectrogram":
		drawSpectrogramBackground(nowMs)
	case "desk":
		drawDeskBackground()
	case "terminal":
		drawTerminalBackground()
	case "termanim":
		drawTermAnimBackground()
	}
}

// drawSpectrogramBackground keeps the scrolling spectrogram texture current and
// draws it face-on filling the canvas (regardless of the Fill switch), with no
// clear, so it sits behind the model.
func drawSpectrogramBackground(nowMs float64) {
	if !spect.ready {
		spect.initSpectrogram()
	}
	aud.ensureAudioSource()
	spect.updateSpectrogramTexture(nowMs)
	offset := float32(spect.texCol) / float32(spectTexW)
	// Force the full-screen face-on placement for the duration of this draw
	// (the background always fills; the Fill switch only governs the MODE).
	savedFill := spect.fill
	spect.fill = true
	glctx.GL.Call("disable", glctx.Types.DepthTest)
	texp.drawTexturedPlane(spect.texture, offset)
	spect.fill = savedFill
}

// syncLayersModule dims the two layer controls the current model cannot take.
//
// The Layers module answers one question three ways — where a second picture
// goes relative to the model: BEHIND it (the backdrop), ON it (the spectrogram
// skin), or AS it (Fill, which turns the spectrogram/FVF plane into the whole
// screen). Backdrop applies to any ordinary model; the other two do not.
//
// Skin needs a surface to paint on, which is what ModeInfo.Skin records, and
// Fill only means anything when the spectrogram or FVF IS the model. Left
// undimmed they were two switches that did nothing, with no way to tell that
// apart from a feature that was broken — the same complaint the Persist switch
// had against the Fore knob, answered the same way and with the same class.
func syncLayersModule(mode string) {
	// The skin selector is a P-unit, so it dims by the cell.
	dimIn := func(id, wrapper string, applies bool) {
		el := dom.Doc.Call("getElementById", id)
		if !el.Truthy() {
			return
		}
		w := el.Call("closest", wrapper)
		if !w.Truthy() {
			return
		}
		w.Get("classList").Call("toggle", "layer-dim", !applies)
	}
	dimIn("skin-visual", ".punit", isSkinnable(mode))
	// Fill is BEHIND's F button now: the button dims.
	dimTrioButton("bg-visual", 0, !isSpectroSurface(mode))
}
