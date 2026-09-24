//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/skirt"
	"math"
	"strconv"
	"syscall/js"
)

// phColorCSS converts a phosphor's 0..1 emission color to a CSS rgb() string.
func phColorCSS(r, g, b float64) string {
	cl := func(x float64) int {
		v := int(x*255 + 0.5)
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		return v
	}
	return "rgb(" + strconv.Itoa(cl(r)) + "," + strconv.Itoa(cl(g)) + "," + strconv.Itoa(cl(b)) + ")"
}

// addPhosphorTraces rings the phosphor knob with one marker per phosphor: a
// short "CRT trace" streak in that phosphor's emission color, pointing inward
// toward the knob center and fading out — the fade length exaggerates the
// phosphor's persistence (a long-afterglow P33 leaves a long glowing tail; a
// crisp P31 a short one). "— none —" (index 0) gets no trace. Clicking a streak
// selects that phosphor.
func addPhosphorTraces(stack, sel js.Value) {
	if !stack.Truthy() {
		return
	}
	n := len(phosphors)
	if n < 2 {
		return
	}
	dial := dom.Doc.Call("createElement", "span")
	dial.Set("className", "knob-dial ph-dial")
	// Streaks live in the band OUTSIDE the knob ring: the outer end sits near the
	// cell edge and the trace points inward toward the knob, fading out. off is a
	// percentage of the dial half-size; ~43 puts the outer end just outside the
	// (smaller) knob ring, pointing inward.
	const off = 43.0
	for i := 1; i < n; i++ {
		p := phosphors[i]
		deg := -skirt.SweepDeg/2 + skirt.SweepDeg*float64(i)/float64(n-1)
		rad := deg * math.Pi / 180
		l, t := dialLabelPos(deg, off)
		col := phColorCSS(p.tr, p.tg, p.tb)
		persist := (p.kr + p.kg + p.kb) / 3 // 0..1, higher = longer afterglow
		length := 8 + persist*9             // px (kept short so it stays outside the knob)
		hold := 15 + persist*55             // % of streak that stays bright before fading
		phi := math.Atan2(math.Cos(rad), -math.Sin(rad)) * 180 / math.Pi
		s := dom.Doc.Call("createElement", "span")
		s.Set("className", "ph-trace clickable")
		s.Set("title", p.desc)
		st := s.Get("style")
		st.Set("left", l)
		st.Set("top", t)
		st.Set("width", strconv.FormatFloat(length, 'f', 1, 64)+"px")
		st.Set("transform", "translate(0,-50%) rotate("+strconv.FormatFloat(phi, 'f', 1, 64)+"deg)")
		st.Set("background", "linear-gradient(to right,"+col+","+col+" "+strconv.FormatFloat(hold, 'f', 0, 64)+"%,transparent)")
		st.Set("box-shadow", "0 0 5px "+col)
		idx := i
		s.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			sel.Set("selectedIndex", idx)
			sel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
			return nil
		}))
		dial.Call("appendChild", s)
	}
	stack.Call("insertBefore", dial, stack.Get("firstChild"))
	stack.Get("classList").Call("add", "has-dial")
}

// Selectable CRT phosphor for the scope modes (Lissajous / Graphic Artist).
// Each JEDEC "P" phosphor is approximated by its emission color plus a
// persistence: short-persistence phosphors clear almost instantly (crisp
// trace), long-persistence ones leave a glowing afterglow. UV/IR phosphors
// (e.g. P16) can't be shown, so they're omitted; two-layer cascade types (P7)
// use a per-channel decay so the trace changes color as it fades (blue flash
// → long green afterglow).
type phosphorSpec struct {
	name       string
	tr, tg, tb float64 // fresh-trace color (the excitation glow)
	// Per-channel retention per frame (0..1): the frame buffer is MULTIPLIED by
	// (kr,kg,kb) each frame, so higher = longer persistence. Different channels
	// let a two-layer phosphor decay through colors — e.g. P7's blue flash
	// dies fast while its green afterglow lingers.
	kr, kg, kb float64
	// desc is what this phosphor IS, and it is what the knob's readout and each
	// streak round the dial carry as their tooltip. It used to be the trailing
	// comment on each row — the right words, in the one place a user could never
	// see them.
	desc string
}

var phosphors = []phosphorSpec{
	{"— none —", 0, 0, 0, 1, 1, 1, "no phosphor — the trace keeps the palette's own colors and CRT mode is off"},
	{"P31 green", 0.35, 1.00, 0.45, 0.62, 0.62, 0.62, "P31 — the Tektronix standard: bright green, short to medium persistence"},
	{"P1 green", 0.45, 1.00, 0.28, 0.80, 0.80, 0.80, "P1 — willemite yellow-green, medium persistence (about 24 ms)"},
	{"P2 yel-green", 0.75, 1.00, 0.25, 0.90, 0.90, 0.90, "P2 — yellow-green, long persistence"},
	{"P3 amber", 1.00, 0.75, 0.15, 0.85, 0.85, 0.85, "P3 — yellow-amber, medium persistence: the classic oscilloscope tube"},
	{"P4 white", 0.95, 0.97, 1.00, 0.58, 0.58, 0.58, "P4 — television white, short persistence"},
	{"P11 blue", 0.30, 0.45, 1.00, 0.60, 0.60, 0.60, "P11 — photographic blue, short persistence: the tube built to expose film"},
	{"P7 blue→green", 0.45, 0.80, 1.00, 0.58, 0.95, 0.55, "P7 — two layers: a blue flash that dies fast over a green afterglow that lingers"},
	{"P39 green", 0.55, 1.00, 0.40, 0.94, 0.94, 0.94, "P39 — long-persistence green, about 150 ms"},
	{"P33 amber", 1.00, 0.50, 0.10, 0.985, 0.985, 0.985, "P33 — radar amber: the longest persistence of the set"},
}

// phosphorState is the CRT phosphor emulation: the phosphor chosen, its fade
// quad and the overlay.
type phosphorState struct {
	index int // 0 = off

	// crtMode: the selected phosphor paints ANY model (attractors too) as though
	// traced on that phosphor tube — its color + afterglow, plus scanline/vignette.
	// It's driven by the Phosphor knob: picking a phosphor turns it on, "— none —"
	// turns it off. (There is no separate CRT gradient source anymore.)
	crtMode    bool
	quadBuf    js.Value
	quadReady  bool
	crtOverlay js.Value
}

var phos phosphorState

// crtBeamLen is how many trajectory points the CRT beam draws per frame; the
// phosphor persistence (afterglow) turns that short advancing segment into the
// visible trail. Small = a tight moving dot with a long persistence tail.
var crtBeamLen = 600

// crtBeam reports whether to draw the short advancing-beam scope trace (a
// phosphor is selected on an attractor model). Scope modes (Lissajous / Graphic
// Artist) already draw a closed figure, so they keep their whole-curve draw.
//
// So do the three audio embeddings, and for a sharper reason than "it already
// looks right". The beam is implemented by shrinking `steps` around the
// generate call, which works because on an integrated model `steps` is how far
// to ADVANCE: fewer steps is a shorter arc, drawn at the same resolution, which
// is exactly a beam. On Takens, Stereo and Polar `steps` is not an advance at
// all — it is the vertex BUDGET for a window whose length comes from WIN — so
// shrinking it does not shorten the figure by one sample. It decimates it: at
// the default trail and an 85 ms window, 600 vertices means a stride of 28, and
// the Catmull-Rom then draws a smooth curve through samples 0.6 ms apart, which
// is below Nyquist for anything above about 860 Hz. The result is a figure
// whose high end is invented by the spline rather than measured, presented at
// the same confidence as the real one — and the phosphor's own accumulation
// hides it, because successive frames decimate at different offsets and pile up
// into something that looks complete.
func crtBeam() bool {
	return phos.active() && isAttractorMode(run.selectedMode) && !isAudioEmbedding(run.selectedMode)
}

// isAudioEmbedding names the modes whose trail is a window of the live audio
// rather than an integrated trajectory: the vertex budget is a resolution for
// them and a length for everything else, so anything that reaches for `steps`
// as a length has to ask.
func isAudioEmbedding(mode string) bool {
	return mode == "takens" || mode == "stereo" || mode == "polar" || mode == "waterfall"
}

// updateCRTDim dims the Controls a selected phosphor overrides (their color /
// trail is taken over by the phosphor). Which controls those are is a property
// of each Control (crtOverride), so this just iterates the model.
func (ph *phosphorState) updateCRTDim() {
	if len(panelModules) == 0 {
		buildControlModel()
	}
	for _, m := range panelModules {
		for _, c := range m.ctrls {
			c.applyCRTDim(ph.crtMode)
		}
	}
}

// isScopeMode reports the modes the phosphor look applies to (the ones that
// draw a scope-style trace through the attractor line pipeline).
func isScopeMode(mode string) bool {
	return mode == "lissajou" || mode == "graphicartist"
}

// crtLook reports whether the phosphor-tube look should be applied: either a
// native scope trace, or any model with the CRT source selected.
func (ph *phosphorState) crtLook() bool {
	return isScopeMode(run.selectedMode) || ph.crtMode
}

// active: a phosphor is selected AND we're drawing a CRT/scope look.
func (ph *phosphorState) active() bool {
	return ph.index > 0 && ph.index < len(phosphors) && ph.crtLook()
}

// applyPhosphorColor overrides the gradient with the selected phosphor's mono
// color. Called after the gradient uniforms are set in generateForMode.
func (ph *phosphorState) applyPhosphorColor() {
	p := phosphors[ph.index]
	glctx.GL.Call("uniform1i", gpu.u.gradientColors, 1) // monochrome
	glctx.GL.Call("uniform3f", gpu.u.baseColor, p.tr, p.tg, p.tb)
}

// updateCRTOverlay shows the scanline+vignette CRT overlay for scope modes and
// hides it otherwise. Called on every mode change (from buildParamPanel).
func (ph *phosphorState) updateCRTOverlay() {
	if !ph.crtOverlay.Truthy() {
		ph.crtOverlay = dom.Doc.Call("createElement", "div")
		ph.crtOverlay.Set("id", "crt-overlay")
		dom.Body.Call("appendChild", ph.crtOverlay)
	}
	if ph.crtLook() {
		ph.crtOverlay.Get("classList").Call("add", "on")
	} else {
		ph.crtOverlay.Get("classList").Call("remove", "on")
	}
}

// drawPhosphorFade multiplies the whole frame by the selected phosphor's
// per-channel retention (kr,kg,kb) each frame — an exponential decay instead of
// a hard clear, which is the phosphor afterglow. Because the decay is
// per-channel, a two-layer phosphor like P7 loses its blue flash quickly while
// its green afterglow lingers, so the trail shifts color as it fades. Reuses
// the xy scope's solid-color program with a fullscreen quad; the multiply is
// done with blendFunc(ZERO, SRC_COLOR). Leaves depth-test disabled; the caller
// re-enables it.
func (ph *phosphorState) drawPhosphorFade() {
	if !xy.ready {
		xy.initXY()
	}
	if !ph.quadReady {
		verts := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
		ph.quadBuf = glctx.GL.Call("createBuffer")
		glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, ph.quadBuf)
		glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(verts), glctx.Types.StaticDraw)
		ph.quadReady = true
	}
	p := phosphors[ph.index]
	ph.drawFadeQuad(float32(p.kr), float32(p.kg), float32(p.kb))
}

// drawFadeQuad multiplies the whole frame by a per-channel retention, which is
// what turns a cleared buffer into a decaying one. Split out of
// drawPhosphorFade so the xy scope's PERSIST knob can use it: an afterglow is
// an afterglow, and a second fullscreen multiply written separately would be a
// second thing to keep in step with whatever the blend state needs next.
//
// The caller is left with depth-testing DISABLED, as drawPhosphorFade's callers
// always were.
func (ph *phosphorState) drawFadeQuad(kr, kg, kb float32) {
	if !xy.ready {
		xy.initXY()
	}
	if !ph.quadReady {
		verts := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
		ph.quadBuf = glctx.GL.Call("createBuffer")
		glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, ph.quadBuf)
		glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(verts), glctx.Types.StaticDraw)
		ph.quadReady = true
	}
	glctx.GL.Call("disable", glctx.Types.DepthTest)
	glctx.GL.Call("enable", glctx.GL.Get("BLEND"))
	// dst_rgb = dst_rgb * src_rgb → multiply the frame by the retention color.
	glctx.GL.Call("blendFunc", glctx.GL.Get("ZERO"), glctx.GL.Get("SRC_COLOR"))
	glctx.GL.Call("useProgram", xy.program)
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, ph.quadBuf)
	glctx.GL.Call("enableVertexAttribArray", xy.aPos)
	glctx.GL.Call("vertexAttribPointer", xy.aPos, 2, glctx.Types.Float, false, 0, 0)
	glctx.GL.Call("uniform3f", xy.uColor, kr, kg, kb) // per-channel retention
	glctx.GL.Call("uniform1f", xy.uAlpha, 1)
	glctx.GL.Call("uniform2f", xy.uOffset, 0, 0)
	glctx.GL.Call("drawArrays", glctx.GL.Get("TRIANGLE_STRIP"), 0, 4)
	glctx.GL.Call("blendFunc", glctx.GL.Get("SRC_ALPHA"), glctx.GL.Get("ONE_MINUS_SRC_ALPHA"))
	glctx.GL.Call("disable", glctx.GL.Get("BLEND"))
}
