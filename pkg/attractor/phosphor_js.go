//go:build js && wasm

package attractor

import (
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
	dial := doc.Call("createElement", "div")
	dial.Set("className", "knob-dial ph-dial")
	// Streaks live in the band OUTSIDE the knob ring: the outer end sits near the
	// cell edge and the trace points inward toward the knob, fading out. off is a
	// percentage of the dial half-size; ~43 puts the outer end just outside the
	// (smaller) knob ring, pointing inward.
	const off = 43.0
	for i := 1; i < n; i++ {
		p := phosphors[i]
		deg := -knobSweepDeg/2 + knobSweepDeg*float64(i)/float64(n-1)
		rad := deg * math.Pi / 180
		l, t := dialLabelPos(deg, off)
		col := phColorCSS(p.tr, p.tg, p.tb)
		persist := (p.kr + p.kg + p.kb) / 3 // 0..1, higher = longer afterglow
		length := 8 + persist*9             // px (kept short so it stays outside the knob)
		hold := 15 + persist*55             // % of streak that stays bright before fading
		phi := math.Atan2(math.Cos(rad), -math.Sin(rad)) * 180 / math.Pi
		s := doc.Call("createElement", "div")
		s.Set("className", "ph-trace clickable")
		s.Set("title", p.name+" — CRT phosphor trace (color + persistence)")
		st := s.Get("style")
		st.Set("left", l)
		st.Set("top", t)
		st.Set("width", strconv.FormatFloat(length, 'f', 1, 64)+"px")
		st.Set("transform", "translate(0,-50%) rotate("+strconv.FormatFloat(phi, 'f', 1, 64)+"deg)")
		st.Set("background", "linear-gradient(to right,"+col+","+col+" "+strconv.FormatFloat(hold, 'f', 0, 64)+"%,transparent)")
		st.Set("box-shadow", "0 0 5px "+col)
		idx := i
		s.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
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
}

var phosphors = []phosphorSpec{
	{"— none —", 0, 0, 0, 1, 1, 1},
	{"P31 green", 0.35, 1.00, 0.45, 0.62, 0.62, 0.62},     // Tek standard, bright green, short–medium
	{"P1 green", 0.45, 1.00, 0.28, 0.80, 0.80, 0.80},      // willemite yellow-green, medium (~24 ms)
	{"P2 yel-green", 0.75, 1.00, 0.25, 0.90, 0.90, 0.90},  // yellow-green, long
	{"P3 amber", 1.00, 0.75, 0.15, 0.85, 0.85, 0.85},      // yellow-amber, medium (classic scope)
	{"P4 white", 0.95, 0.97, 1.00, 0.58, 0.58, 0.58},      // TV white, short
	{"P11 blue", 0.30, 0.45, 1.00, 0.60, 0.60, 0.60},      // photographic blue, short
	{"P7 blue→green", 0.45, 0.80, 1.00, 0.58, 0.95, 0.55}, // blue flash (short) → green afterglow (long)
	{"P39 green", 0.55, 1.00, 0.40, 0.94, 0.94, 0.94},     // long-persistence green (~150 ms)
	{"P33 amber", 1.00, 0.50, 0.10, 0.985, 0.985, 0.985},  // very long persistence (radar amber)
}

var phosphorIdx int // 0 = off

// crtMode: the selected phosphor paints ANY model (attractors too) as though
// traced on that phosphor tube — its color + afterglow, plus scanline/vignette.
// It's driven by the Phosphor knob: picking a phosphor turns it on, "— none —"
// turns it off. (There is no separate CRT gradient source anymore.)
var crtMode bool

// crtBeamLen is how many trajectory points the CRT beam draws per frame; the
// phosphor persistence (afterglow) turns that short advancing segment into the
// visible trail. Small = a tight moving dot with a long persistence tail.
var crtBeamLen = 600

// crtBeam reports whether to draw the short advancing-beam scope trace (a
// phosphor is selected on an attractor model). Scope modes (Lissajous / Graphic
// Artist) already draw a closed figure, so they keep their whole-curve draw.
func crtBeam() bool {
	return phosphorActive() && isAttractorMode(selectedMode)
}

// updateCRTDim dims the Controls a selected phosphor overrides (their color /
// trail is taken over by the phosphor). Which controls those are is a property
// of each Control (crtOverride), so this just iterates the model.
func updateCRTDim() {
	if len(panelModules) == 0 {
		buildControlModel()
	}
	for _, m := range panelModules {
		for _, c := range m.ctrls {
			c.applyCRTDim(crtMode)
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
func crtLook() bool {
	return isScopeMode(selectedMode) || crtMode
}

// phosphorActive: a phosphor is selected AND we're drawing a CRT/scope look.
func phosphorActive() bool {
	return phosphorIdx > 0 && phosphorIdx < len(phosphors) && crtLook()
}

// applyPhosphorColor overrides the gradient with the selected phosphor's mono
// color. Called after the gradient uniforms are set in generateForMode.
func applyPhosphorColor() {
	p := phosphors[phosphorIdx]
	gl.Call("uniform1i", uGradientColorsLoc, 1) // monochrome
	gl.Call("uniform3f", uBaseColorLoc, p.tr, p.tg, p.tb)
}

var (
	phosphorQuadBuf   js.Value
	phosphorQuadReady bool
	crtOverlay        js.Value
)

// updateCRTOverlay shows the scanline+vignette CRT overlay for scope modes and
// hides it otherwise. Called on every mode change (from buildParamPanel).
func updateCRTOverlay() {
	if !crtOverlay.Truthy() {
		crtOverlay = doc.Call("createElement", "div")
		crtOverlay.Set("id", "crt-overlay")
		body.Call("appendChild", crtOverlay)
	}
	if crtLook() {
		crtOverlay.Get("classList").Call("add", "on")
	} else {
		crtOverlay.Get("classList").Call("remove", "on")
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
func drawPhosphorFade() {
	if !xyReady {
		initXY()
	}
	if !phosphorQuadReady {
		verts := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
		phosphorQuadBuf = gl.Call("createBuffer")
		gl.Call("bindBuffer", glTypes.ArrayBuffer, phosphorQuadBuf)
		gl.Call("bufferData", glTypes.ArrayBuffer, SliceToTypedArray(verts), glTypes.StaticDraw)
		phosphorQuadReady = true
	}
	p := phosphors[phosphorIdx]
	gl.Call("disable", glTypes.DepthTest)
	gl.Call("enable", gl.Get("BLEND"))
	// dst_rgb = dst_rgb * src_rgb → multiply the frame by the retention color.
	gl.Call("blendFunc", gl.Get("ZERO"), gl.Get("SRC_COLOR"))
	gl.Call("useProgram", xyProgram)
	gl.Call("bindBuffer", glTypes.ArrayBuffer, phosphorQuadBuf)
	gl.Call("enableVertexAttribArray", xyAPos)
	gl.Call("vertexAttribPointer", xyAPos, 2, glTypes.Float, false, 0, 0)
	gl.Call("uniform3f", xyUColor, p.kr, p.kg, p.kb) // per-channel retention
	gl.Call("uniform1f", xyUAlpha, 1)
	gl.Call("drawArrays", gl.Get("TRIANGLE_STRIP"), 0, 4)
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("disable", gl.Get("BLEND"))
}
