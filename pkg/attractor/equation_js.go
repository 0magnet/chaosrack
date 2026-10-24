//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/glctx"
)

// Custom mode: user-editable attractor equations. The three (optionally four)
// expressions are parsed by the engine in equation.go; any free identifier
// becomes a knobbed parameter. The same compiled forms drive the integrator
// here, so a typed system behaves like any built-in — and is shareable via the
// permalink. This is the representation the schematic view will later render
// from.
//
// Two FLAVORS, because the same expressions can mean two different systems:
//
//	flow    (default): they are derivatives, integrated — x += dt·f(x,y,z)
//	iterate:           they ARE the next state — x = f(x,y,z)
//
// The iterate flavor is a discrete map and behaves like the built-in ones: no
// dt, drawn as points, transient discarded on reseed, and — the part that is
// not cosmetic — NOT published to the flow registry. The machinery for it is
// in equationiterate.go, untagged so it can be tested off the browser.

// customEquation is the Custom mode: the equations as typed, compiled and
// running.
type customEquation struct {
	CustomSystem // the equations, compiled and registered (customsystem.go)
	// modDefs are the constants the equations name, as parameters: what the
	// Mod matrix routes to and the modulator drives (modParams).
	modDefs   []paramDef
	surfKey   string  // what the drawn surface was built from
	surfDraft bool    // the drawing is the coarse one made while a knob moves
	surfAt    float64 // when the surface last changed, ms
	surfFit   float32 // the extent the camera was last fitted to; 0 until a surface is drawn
}

var custom = customEquation{CustomSystem: *NewCustomSystem()}

// parseCustom is CustomSystem's, and a surface compiled anew is framed anew
// (generateSurface).
func (c *customEquation) parseCustom() {
	c.surfFit = 0
	c.CustomSystem.parseCustom()
}

// generateCustom runs whichever flavor is selected: forward Euler over the
// derivatives, exactly like the built-in attractors, or — in iterate flavor —
// the shared discrete-map loop, which is where the points draw mode, the
// discarded transient and the escape-reseed already live.
func (c *customEquation) generateCustom() {
	if c.surface {
		c.generateSurface()
		return
	}
	if c.err != "" || c.expr[0] == nil {
		// Nothing valid to run — leave the last frame on screen.
		gpu.uploadVerticesOnly(sim.vertBuf[:sim.steps*4], mapDrawMode(dynamics.CustomKey), sim.steps)
		return
	}
	if c.iterate {
		generateMap(dynamics.CustomKey)
		return
	}
	// Per-frame snapshot of each expression's parameter values (aligned to
	// its own Params slice), so the hot loop does no map lookups.
	var pv [4][]float64
	for i := range 4 {
		if c.expr[i] == nil {
			continue
		}
		s := make([]float64, len(c.expr[i].Params))
		for k, p := range c.expr[i].Params {
			if ptr := c.paramVal[p]; ptr != nil {
				s[k] = float64(*ptr)
			}
		}
		pv[i] = s
	}
	dt := float64(c.dt) * float64(sim.speedScale)
	stack := c.stack
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	if c.rk4 {
		c.generateCustomRK4(pv, dt, vertices, invN)
		return
	}
	sub := effSubSteps(sim.speedSteps, sim.steps, frameBudgetInterpreted)
	for i := range sim.steps {
		for range sub {
			vars := [5]float64{float64(sim.x), float64(sim.y), float64(sim.z), float64(c.w), c.t}
			dx := c.expr[0].Eval(vars, pv[0], stack)
			dy := 0.0
			if c.expr[1] != nil {
				dy = c.expr[1].Eval(vars, pv[1], stack)
			}
			dz := 0.0
			if c.expr[2] != nil {
				dz = c.expr[2].Eval(vars, pv[2], stack)
			}
			sim.x += float32(dt * dx)
			sim.y += float32(dt * dy)
			sim.z += float32(dt * dz)
			if c.useW && c.expr[3] != nil {
				c.w += float32(dt * c.expr[3].Eval(vars, pv[3], stack))
			}
			c.t += dt
			sim.checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = sim.x, sim.y, sim.z, float32(i)*invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}

// generateCustomRK4 is generateCustom's flow stepped with RK4, in float64:
// the state lives in sim.x64..z64 as integrate3D's does, picked up from the
// float32 state whenever something else (a drag, the Euler loop) has moved
// it. A step evaluates the equations four times, so it gets a quarter of the
// budget.
func (c *customEquation) generateCustomRK4(pv [4][]float64, dt float64, vertices []float32, invN float32) {
	stack := c.stack
	useW := c.useW && c.expr[3] != nil
	eval := func(s [4]float64, t float64) [4]float64 {
		vars := [5]float64{s[0], s[1], s[2], s[3], t}
		var d [4]float64
		for k := range 3 {
			if c.expr[k] != nil {
				d[k] = c.expr[k].Eval(vars, pv[k], stack)
			}
		}
		if useW {
			d[3] = c.expr[3].Eval(vars, pv[3], stack)
		}
		return d
	}
	add := func(s, k [4]float64, h float64) [4]float64 {
		return [4]float64{s[0] + h*k[0], s[1] + h*k[1], s[2] + h*k[2], s[3] + h*k[3]}
	}
	s := [4]float64{sim.x64, sim.y64, sim.z64, float64(c.w)}
	if float32(s[0]) != sim.x || float32(s[1]) != sim.y || float32(s[2]) != sim.z {
		s[0], s[1], s[2] = float64(sim.x), float64(sim.y), float64(sim.z)
	}
	sub := effSubSteps(sim.speedSteps, sim.steps, frameBudgetInterpreted/4)
	for i := range sim.steps {
		for range sub {
			k1 := eval(s, c.t)
			k2 := eval(add(s, k1, dt/2), c.t+dt/2)
			k3 := eval(add(s, k2, dt/2), c.t+dt/2)
			k4 := eval(add(s, k3, dt), c.t+dt)
			for k := range 4 {
				s[k] += dt / 6 * (k1[k] + 2*k2[k] + 2*k3[k] + k4[k])
			}
			c.t += dt
			sim.x64, sim.y64, sim.z64 = s[0], s[1], s[2]
			sim.x, sim.y, sim.z = float32(s[0]), float32(s[1]), float32(s[2])
			c.w = float32(s[3])
			sim.checkDiverged() // a reseed puts both forms back at the start
			s = [4]float64{sim.x64, sim.y64, sim.z64, float64(c.w)}
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = sim.x, sim.y, sim.z, float32(i)*invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}

// surfaceSlices and surfaceRes are how finely a surface is drawn: planes
// across each axis, and grid cells along each plane. Enough to read a solid's
// facets and a gyroid's weave (3 × 18 × 57² ≈ 175,000 evaluations). While a
// knob is moving it is drawn at the draft figures, a fifth of the work, and
// at these once it has been still for surfaceSettleMs: the way a scope or a
// synth's display keeps up with a hand and then fills in.
const (
	surfaceSlices, surfaceRes = 18, 56
	draftSlices, draftRes     = 10, 32
	surfaceSettleMs           = 250
)

// generateSurface draws F(x,y,z) = 0 by its contours, rebuilding only when the
// function, a parameter or the extent has changed since the last build, or
// when a draft drawn while it was changing is due its full detail.
func (c *customEquation) generateSurface() {
	key, pv := c.SurfaceKey()
	now := js.Global().Get("performance").Call("now").Float()
	changed := key != c.surfKey
	due := c.surfDraft && now-c.surfAt > surfaceSettleMs
	if !changed && !due && gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	slices, res := surfaceSlices, surfaceRes
	if changed {
		c.surfKey, c.surfAt = key, now
		slices, res = draftSlices, draftRes
	}
	c.surfDraft = changed
	v := c.SurfaceLines(pv, slices, res)
	idx := make([]uint16, len(v)/3)
	for i := range idx {
		idx[i] = uint16(i) //nolint:gosec // implicit.Contours stops short of 65535 vertices
	}
	gpu.staticDirty = true
	gpu.uploadBuffersIndexed(v, idx, glctx.Types.Line)
	// Framed and colored for the region drawn, which is known by construction:
	// the flow flavor before it left the camera fitted to its trail (fifty
	// times the size) and the gradient spread over that trail's bounds, so a
	// surface switched back on came up as a speck in one color. The gradient
	// range is given with the trail's center offset added back, because
	// setGradientRange takes it off again and the surface is not offset.
	e := c.extent
	o := sim.centerOffset
	gpu.setGradientRange(-e+o[0], e+o[0], -e+o[1], e+o[1], -e+o[2], e+o[2])
	if c.surfFit != e {
		c.surfFit = e
		dist := fitDistFor(e * 1.8) // the region's corners reach √3 of its half-width
		view.initDist, view.defaultDist = dist, dist
		view.updateViewMatrix()
	}
}

// seedCustomSurface loads F into the editor as a surface: what editing the
// Polyhedron's equation does.
func (c *customEquation) seedCustomSurface(f string) {
	c.surface, c.iterate, c.useW = true, false, false
	c.eq[0] = f
	c.paramVal = map[string]*float32{}
	c.t, c.w = 0, 0
	c.parseCustom()
}

// ── Custom-mode control panel ─────────────────────────────────────────────

// buildCustomPanel renders the equation editor into #params: three/four
// equation fields, a 4D toggle, a dt knob, a parse-error line, and a knob per
// detected parameter. Called from buildParamPanel when mode == "custom".
func (c *customEquation) buildCustomPanel(paramsDiv js.Value) {
	c.parseCustom()

	eqCol := dom.Doc.Call("createElement", "span")
	eqCol.Set("className", "pcell")
	eqCol.Set("style", "gap:2px;")

	makeEqField := func(i int) js.Value {
		row := dom.Doc.Call("createElement", "span")
		row.Set("className", "grp")
		lbl := dom.Doc.Call("createElement", "span")
		lbl.Set("textContent", c.eqLabel(i)+" =")
		lbl.Set("style", "color:#8cf;min-width:44px;")
		inp := dom.Doc.Call("createElement", "input")
		inp.Set("type", "text")
		// A control of the rack, so a panel in a terminal and a rack without the
		// page can type the system too. Its module is the Equation module's,
		// not the bank's: the bank's region is where its controls are, and this
		// one is under the monitor.
		inp.Set("id", "custom-eq-"+strconv.Itoa(i))
		registerControl(ControlDesc{ID: "custom-eq-" + strconv.Itoa(i), Label: c.eqLabel(i), IsText: true}, "equation")
		inp.Set("value", c.eq[i])
		inp.Set("spellcheck", false)
		inp.Set("style", "width:180px;background:#0a1420;color:#cde;border:1px solid #345;font-family:monospace;font-size:12px;padding:2px 4px;")
		vars := "x, y, z" + map[bool]string{true: ", w", false: ""}[c.flavorW()] +
			map[bool]string{true: "", false: ", t"}[c.iterate || c.surface]
		what := map[bool]string{true: "the NEXT value of " + c.eqLabel(i)[:1], false: c.eqLabel(i)}[c.iterate]
		if c.surface {
			what = "F, the surface's function: the surface is where it is zero"
		}
		inp.Set("title", what+" — expression in "+vars+"; any other letters become knobbed parameters (e / pi / tau are constants)")
		// Commit on change (blur/Enter) to avoid rebuilding mid-keystroke.
		dom.On(inp, "change", func(this js.Value, a []js.Value) any {
			c.eq[i] = inp.Get("value").String()
			resetAttractorState()
			buildParamPanel("custom") // reparse + refresh param knobs
			perma.syncPermalinkNow()
			return nil
		})
		row.Call("appendChild", lbl)
		row.Call("appendChild", inp)
		return row
	}
	eqCol.Call("appendChild", makeEqField(0))
	if !c.surface { // a surface is one function: its other lines are kept, not shown
		eqCol.Call("appendChild", makeEqField(1))
		eqCol.Call("appendChild", makeEqField(2))
	}
	if c.flavorW() {
		eqCol.Call("appendChild", makeEqField(3))
	}

	// The error line. The flavor and 4D switches are the Visual head's
	// programmable switches while Custom runs (modelparts_js.go: customSwitches).
	if c.err != "" {
		errSpan := dom.Doc.Call("createElement", "span")
		errSpan.Set("className", "grp")
		errSpan.Set("textContent", "⚠ "+c.err)
		errSpan.Set("style", "color:#f86;font-size:11px;")
		eqCol.Call("appendChild", errSpan)
	}

	// The equation editor lives in its own "Equation" module, before Parameters
	// (which holds the detected parameter knobs).
	if paramsSect := dom.Doc.Call("getElementById", "params-module"); paramsSect.Truthy() {
		if old := dom.Doc.Call("getElementById", "eqn-module"); old.Truthy() {
			old.Get("parentNode").Call("removeChild", old)
		}
		eqMod := dom.Doc.Call("createElement", "div")
		eqMod.Set("className", "sect")
		eqMod.Set("id", "eqn-module")
		hdr := dom.Doc.Call("createElement", "div")
		hdr.Set("className", "sect-hdr")
		hdr.Set("textContent", "Equation")
		hdrTip := doc("equation")
		if c.iterate {
			hdrTip = doc("equation.map")
		}
		if c.surface {
			hdrTip = doc("equation.surface")
		}
		hdr.Set("title", hdrTip)
		eqMod.Call("appendChild", hdr)
		body := dom.Doc.Call("createElement", "div")
		body.Set("className", "row")
		body.Call("appendChild", eqCol)
		eqMod.Call("appendChild", body)
		mountEquation(eqMod, paramsSect)
	}

	// dt + detected parameter knobs, built with the SAME buildParamUnit
	// anatomy as every other mode's Parameters module (label · LED · knob ·
	// step · reset, aligned by the punit grid) — the old bespoke rows
	// misaligned and skipped the role-tooltip pass.
	//
	// No dt knob in iterate flavor: a map has no timestep, and a knob that
	// changes nothing is worse than a missing one.
	var defs []paramDef
	switch {
	case c.surface:
		// How far the surface is drawn, either side of the center.
		defs = append(defs, paramDef{"custom-extent", "size", &c.extent, 1.5, 0.2, 10, 0.1})
	case !c.iterate:
		defs = append(defs, paramDef{"custom-dt", "dt", &c.dt, 0.005, 0.0001, 0.05, 0.0001})
	}
	for _, name := range c.paramList {
		if ptr := c.paramVal[name]; ptr != nil {
			defs = append(defs, paramDef{"custom-" + name, name, ptr, 1, -10, 10, 0.01})
		}
	}
	c.modDefs = defs
	buildModMatrix(defs)
	// In the row's bank, with every other model's constants (bankCustomCells).
	if bankCustomCells(defs) {
		return
	}
	grid := dom.Doc.Call("createElement", "div")
	grid.Set("className", "punit-grid")
	for _, d := range defs {
		grid.Call("appendChild", buildParamUnit(run.selectedMode, d))
	}
	paramsDiv.Call("appendChild", grid)
}

// ── Seeding the editor from a built-in ────────────────────────────────────

// seedCustomFromMode loads the given built-in's equations into the editor
// (falling back to the current default template if unknown) and returns
// "custom" so the caller can switch modes.
func (c *customEquation) seedCustomFromMode(mode string) {
	be, ok := builtinEquations[mode]
	if !ok {
		return // keep whatever's already in the editor
	}
	c.eq = be.eq
	c.useW = be.useW
	c.dt = be.dt
	// Every seed in the table is a FLOW (the guard in chaos_test.go checks each
	// one against the mode's vector field), so seeding leaves the iterate
	// flavor: read as a map, Lorenz's dx/dt is not Lorenz.
	c.iterate, c.surface = false, false
	c.paramVal = map[string]*float32{}
	for name, val := range be.params {
		v := val
		c.paramVal[name] = &v
	}
	c.t = 0
	c.w = 0
	c.parseCustom()
}

// serializeCustom appends the custom equations + params to the permalink.
func (c *customEquation) serializeCustom(b *strings.Builder) {
	// The flavor first: it decides what the expressions MEAN, and a link that
	// restored Henon's equations as a flow would restore a different system.
	// Omitted when false, like every other control at its default.
	if c.iterate {
		b.WriteString("&cit=1")
	}
	if c.surface {
		b.WriteString("&csf=1&cex=" + permaFmt(c.extent))
	}
	b.WriteString("&eq=")
	n := 3
	if c.useW {
		n = 4
	}
	parts := make([]string, n)
	for i := range n {
		parts[i] = jsEncodeURI(c.eq[i])
	}
	b.WriteString(strings.Join(parts, ";"))
	for _, name := range c.paramList {
		if ptr := c.paramVal[name]; ptr != nil {
			b.WriteString("&cp." + name + "=" + permaFmt(*ptr))
		}
	}
	if !c.iterate && !c.surface {
		b.WriteString("&cdt=" + permaFmt(c.dt))
		if c.rk4 {
			b.WriteString("&crk=1")
		}
	}
}

// applyCustomEq / applyCustomParam restore permalinked custom state.
func (c *customEquation) applyCustomEq(val string) {
	parts := strings.Split(val, ";")
	c.useW = len(parts) >= 4
	for i := range 4 {
		if i < len(parts) {
			c.eq[i] = jsDecodeURI(parts[i])
		}
	}
	c.parseCustom()
}

func (c *customEquation) applyCustomParam(name, val string) {
	if v, err := strconv.ParseFloat(val, 32); err == nil {
		f := float32(v)
		c.paramVal[name] = &f
	}
}

func jsEncodeURI(s string) string {
	return js.Global().Call("encodeURIComponent", s).String()
}
func jsDecodeURI(s string) string {
	return js.Global().Call("decodeURIComponent", s).String()
}

// ── The Equation module in a bank bay ─────────────────────────────────────

// buildEquationView is the Equation module for a built-in model in a bank
// bay: the same editor Custom has, holding the running model's equations.
//
// It is always there, and always four lines — dw/dt stays, empty, on a 3-D
// system — so turning the MODEL knob changes what the lines say and not the
// size of the panel. Editing a line is how a built-in becomes your own: the
// editor is seeded from the model, the edit is applied, and the rack switches
// to Custom running it.
func (c *customEquation) buildEquationView(mode string, paramsDiv js.Value) {
	if old := dom.Doc.Call("getElementById", "eqn-module"); old.Truthy() {
		old.Get("parentNode").Call("removeChild", old)
	}
	paramsSect := paramsDiv.Call("closest", ".sect")
	if !paramsSect.Truthy() {
		return
	}
	be, known := builtinEquations[mode]
	// The Polyhedron's lines are its symbol, its surface as F(x,y,z) = 0,
	// its counts and what has been done to it (polyEquationLines), and they
	// follow its knobs (refreshPolyEquation, by the ids below).
	var polyLines [4][2]string
	polyRun := false
	if mode == "polyhedron" {
		polyLines, polyRun = polyEquationLines()
	}
	col := dom.Doc.Call("createElement", "span")
	col.Set("className", "pcell")
	col.Set("style", "gap:2px;")
	for i, v := range []string{"x", "y", "z", "w"} {
		row := dom.Doc.Call("createElement", "span")
		row.Set("className", "grp")
		lbl := dom.Doc.Call("createElement", "span")
		lbl.Set("textContent", "d"+v+"/dt =")
		lbl.Set("style", "color:#8cf;min-width:44px;")
		inp := dom.Doc.Call("createElement", "input")
		inp.Set("type", "text")
		inp.Set("spellcheck", false)
		inp.Set("className", "eqview")
		inp.Set("id", "eqv-"+strconv.Itoa(i))
		inp.Call("setAttribute", "data-model", mode)
		if mode == "polyhedron" {
			lbl.Set("textContent", polyLines[i][0])
			inp.Set("value", polyLines[i][1])
			if i != 1 || !polyRun {
				inp.Set("disabled", true)
				inp.Set("title", doc("equation.readonly"))
			} else {
				inp.Set("title", doc("equation.solid"))
				dom.On(inp, "change", func(js.Value, []js.Value) any {
					c.seedCustomSurface(inp.Get("value").String())
					if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
						ms.Set("value", "custom")
						dom.Fire(ms, "change")
					}
					return nil
				})
			}
			row.Call("appendChild", lbl)
			row.Call("appendChild", inp)
			col.Call("appendChild", row)
			continue
		}
		if known {
			inp.Set("value", be.eq[i])
		}
		if !known || (i == 3 && !be.useW) {
			inp.Set("disabled", true)
			if known {
				inp.Set("title", docf("equation.no-w", "v", v, "mode", modeLabel(mode)))
			} else {
				inp.Set("title", docf("equation.not-system", "v", v, "mode", modeLabel(mode)))
			}
		} else {
			n := i
			inp.Set("title", docf("equation.line", "v", v, "mode", modeLabel(mode)))
			dom.On(inp, "change", func(js.Value, []js.Value) any {
				c.seedCustomFromMode(mode)
				c.eq[n] = inp.Get("value").String()
				c.parseCustom()
				if ms := dom.Doc.Call("getElementById", "mode-select"); ms.Truthy() {
					ms.Set("value", "custom")
					dom.Fire(ms, "change")
				}
				return nil
			})
		}
		row.Call("appendChild", lbl)
		row.Call("appendChild", inp)
		col.Call("appendChild", row)
	}

	eqMod := dom.Doc.Call("createElement", "div")
	eqMod.Set("className", "sect")
	eqMod.Set("id", "eqn-module")
	hdr := dom.Doc.Call("createElement", "div")
	hdr.Set("className", "sect-hdr")
	hdr.Set("textContent", "Equation")
	hdr.Set("title", doc("equation.running"))
	eqMod.Call("appendChild", hdr)
	body := dom.Doc.Call("createElement", "div")
	body.Set("className", "row")
	body.Call("appendChild", col)
	eqMod.Call("appendChild", body)
	mountEquation(eqMod, paramsSect)
}

// mountEquation puts the Equation panel where the running model's row keeps a
// place for it, under the bay's monitor (buildBayHead), and anywhere else as
// a module of its own before Parameters.
//
// Under the monitor it is part of the head rather than a module in the rack:
// no .sect, so the rack does not pack it as one, and its header's tooltip
// goes on the panel itself.
func mountEquation(eqMod, paramsSect js.Value) {
	slot := dom.Doc.Call("getElementById", eqSlotID(rowOf(run.selectedMode)))
	if !slot.Truthy() {
		paramsSect.Get("parentNode").Call("insertBefore", eqMod, paramsSect)
		return
	}
	eqMod.Set("className", "eqnmounted")
	if hdr := eqMod.Call("querySelector", ".sect-hdr"); hdr.Truthy() {
		eqMod.Set("title", hdr.Get("title"))
		hdr.Call("remove")
	}
	slot.Call("appendChild", eqMod)
}

// customIntCell is the bank position that chooses the flow's integrator:
// built once in the #model-parts holder and placed by bankCustomCells while
// Custom is a flow.
const customIntCell = "custom-int-cell"

// wireIntegratorCell adopts the integrator selector: euler or rk4.
func (c *customEquation) wireIntegratorCell() {
	adoptDescControl(ControlDesc{
		ID: "custom-int-select", Label: "intg", IsSelect: true, SelectDef: "0", ResetID: "rst-custom-int-select",
		SelectApply: func(v string) { c.setIntegrator(v == "1") },
	})
}

// setIntegrator steps the flow with RK4 or forward Euler, from the start: a
// different integrator is, for a chaotic flow, a different trajectory.
func (c *customEquation) setIntegrator(rk4 bool) {
	if c.rk4 == rk4 {
		return
	}
	c.SetRK4(rk4)
	resetAttractorState()
	perma.syncPermalinkNow()
}

// showIntegrator turns the selector to the integrator in force, which a link
// may have set without it.
func (c *customEquation) showIntegrator() {
	sel := dom.Doc.Call("getElementById", "custom-int-select")
	if !sel.Truthy() {
		return
	}
	want := map[bool]string{true: "1", false: "0"}[c.rk4]
	if sel.Get("value").String() != want {
		sel.Set("value", want)
		dom.Fire(sel, "change") // the knob follows; setIntegrator has nothing to do
	}
}
