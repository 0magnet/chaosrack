//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/equation"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/implicit"
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
	// modDefs are the constants the equations name, as parameters: what the
	// Mod matrix routes to and the modulator drives (modParams).
	modDefs []paramDef
	eq      [4]string
	useW    bool
	iterate bool // flavor: false = flow (derivatives), true = discrete map
	// surface is the third flavor: the first expression is F(x, y, z), and
	// what is drawn is the surface F = 0, by its contours (pkg/implicit),
	// across the cube extent wide either side of the center.
	surface   bool
	extent    float32
	surfKey   string  // what the drawn surface was built from
	surfDraft bool    // the drawing is the coarse one made while a knob moves
	surfAt    float64 // when the surface last changed, ms
	surfFit   float32 // the extent the camera was last fitted to; 0 until a surface is drawn
	expr      [4]*equation.Expr
	dt        float32
	paramVal  map[string]*float32
	paramList []string // union of params across the active expressions
	err       string
	stack     []float64 // reused eval scratch
	t         float64   // running t for time-dependent systems
	w         float32   // 4th state when useW
}

var custom = customEquation{
	eq:       [4]string{"sigma*(y - x)", "x*(rho - z) - y", "x*y - beta*z", "-w"},
	dt:       0.005,
	extent:   1.5,
	paramVal: map[string]*float32{},
}

// eqLabel names row i of the editor in the current flavor. A map's rows are
// not derivatives and must not be labeled as though they were: x' = 1 − ax² + y
// is Henon, dx/dt = 1 − ax² + y is something else entirely.
func (c *customEquation) eqLabel(i int) string {
	if c.surface {
		return "F"
	}
	if c.iterate {
		return [4]string{"x'", "y'", "z'", "w'"}[i]
	}
	return [4]string{"dx/dt", "dy/dt", "dz/dt", "dw/dt"}[i]
}

// flavorW reports whether the 4th state is in play: iterate is 3-D, so
// the w equation is not compiled there even when the toggle is left on (which
// keeps a typed dw/dt safe across a flavor round-trip).
func (c *customEquation) flavorW() bool { return c.useW && !c.iterate && !c.surface }

// Seed the default template's parameters (Lorenz) so Custom mode shows a real
// attractor immediately, before any editing or seeding.
func init() {
	for n, v := range map[string]float32{"sigma": 10, "rho": 28, "beta": 2.6667} {
		vv := v
		custom.paramVal[n] = &vv
	}
}

// parseCustom (re)compiles the equation strings, refreshes the parameter list
// (keeping existing values), and records any parse error in custom.err.
func (c *customEquation) parseCustom() {
	// Deferred, so that the error returns below re-publish too: they used to
	// return without touching the registry, which left the PREVIOUS system
	// registered — Model Out FLOW would keep sonifying equations that were no
	// longer on screen, and now a flavor switch would leave a map registered as
	// a flow. Withdrawing is as much the job as registering.
	defer c.registerCustomSystem()
	c.err = ""
	c.surfFit = 0 // a surface compiled anew is framed anew (generateSurface)
	seen := map[string]bool{}
	var order []string
	maxRPN := 1
	for i := range 4 {
		c.expr[i] = nil
		if i == 3 && !c.flavorW() {
			continue
		}
		if i > 0 && c.surface {
			continue // a surface is one function; the other lines are kept, not compiled
		}
		e, err := equation.ParseExpr(c.eq[i])
		if err != nil {
			c.err = c.eqLabel(i) + ": " + err.Error()
			return
		}
		if c.iterate {
			if why := equation.IterateBlocker(e); why != "" {
				c.err = c.eqLabel(i) + ": " + why
				return
			}
		}
		c.expr[i] = e
		if e.StackNeed() > maxRPN {
			maxRPN = e.StackNeed()
		}
		for _, p := range e.Params {
			if !seen[p] {
				seen[p] = true
				order = append(order, p)
			}
		}
	}
	for _, p := range order {
		if _, ok := c.paramVal[p]; !ok {
			v := float32(1)
			c.paramVal[p] = &v
		}
	}
	c.paramList = order
	c.stack = make([]float64, maxRPN+2)
}

// paramPtrs binds one pointer slice per expression, aligned to that
// expression's Params. Binding the pointers once and dereferencing per step is
// what keeps knob edits live without a map lookup in the hot loop.
func (c *customEquation) paramPtrs(exprs []*equation.Expr) [][]*float32 {
	out := make([][]*float32, len(exprs))
	for i, e := range exprs {
		if e == nil {
			continue
		}
		out[i] = make([]*float32, len(e.Params))
		for k, p := range e.Params {
			out[i][k] = c.paramVal[p]
		}
	}
	return out
}

// registerCustomSystem publishes the compiled equations to the registry that
// matches the FLAVOR — and, as much the point, withdraws them from the other.
//
// Flow flavor goes to dynamics.FlowSystems4, so Model Out FLOW and the ring beam
// integrate the SAME system the renderer draws.
//
// Iterate flavor deliberately does NOT. Everything downstream of dynamics.FlowSystems4
// does one thing with what it finds there: steps it with dt. For a map that
// produces a different system — x' = 1 − 1.4x² + y read as a derivative at
// dt = 0.005 is a slow crawl to a fixed point, not the fractal on the screen —
// so Model Out FLOW would sonify a system nobody typed, the Poincare section
// would hunt for crossings of a trajectory that does not exist (a map has no
// path between iterates to cross anything), and the Lyapunov readout would
// print a per-TIME exponent for a system with no time. Being absent from the
// registry is a shape the consumers already handle: dynamics.FlowFor4 misses and they
// fall back to scanning the drawn trail. The map registry takes it instead,
// which is how IsMap/MapStep steer LyapunovFor to its per-iterate branch.
func (c *customEquation) registerCustomSystem() {
	dynamics.Unregister4(dynamics.CustomKey)
	dynamics.ClearCustomMap()
	if c.err != "" || c.expr[0] == nil || c.surface {
		return // a surface is drawn, not run: nothing to register
	}
	if c.iterate {
		pp := c.paramPtrs(c.expr[:3])
		dynamics.SetCustomMap(equation.NewIterateStep(
			[3]*equation.Expr{c.expr[0], c.expr[1], c.expr[2]},
			[3][]*float32{pp[0], pp[1], pp[2]}))
		return
	}
	c.registerCustomFlow()
}

// registerCustomFlow publishes the flow flavor. The closure keeps its own eval
// scratch (everything runs on the one JS thread, but the audio callback must
// not share generateCustom's stack mid-frame) and re-reads parameter values
// each call so knob edits are live.
func (c *customEquation) registerCustomFlow() {
	exprs := c.expr
	useW := c.flavorW()
	stack := make([]float64, len(c.stack))
	pv := [4][]float64{}
	pp := c.paramPtrs(exprs[:])
	for i := range 4 {
		if exprs[i] != nil {
			pv[i] = make([]float64, len(exprs[i].Params))
		}
	}
	dynamics.RegisterFlow4(dynamics.CustomKey, dynamics.FlowSys4{
		Dt:    func() float64 { return float64(c.dt) },
		Euler: true, // generateCustom integrates with forward Euler
		F: func(x, y, z, w float64) (float64, float64, float64, float64) {
			vars := [5]float64{x, y, z, w, c.t}
			eval := func(i int) float64 {
				if exprs[i] == nil {
					return 0
				}
				for k, ptr := range pp[i] {
					if ptr != nil {
						pv[i][k] = float64(*ptr)
					}
				}
				return exprs[i].Eval(vars, pv[i], stack)
			}
			dx, dy, dz := eval(0), eval(1), eval(2)
			dw := 0.0
			if useW {
				dw = eval(3)
			}
			return dx, dy, dz, dw
		},
		W:           func() float64 { return float64(c.w) },
		SetW:        func(v float64) { c.w = float32(v) },
		Interpreted: true,
	})
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
	var key strings.Builder
	key.WriteString(c.eq[0])
	key.WriteString(strconv.FormatFloat(float64(c.extent), 'g', -1, 32))
	var pv []float64
	if c.expr[0] != nil {
		for _, p := range c.expr[0].Params {
			v := 0.0
			if ptr := c.paramVal[p]; ptr != nil {
				v = float64(*ptr)
			}
			pv = append(pv, v)
			key.WriteString("|" + strconv.FormatFloat(v, 'g', -1, 64))
		}
	}
	now := js.Global().Get("performance").Call("now").Float()
	changed := key.String() != c.surfKey
	due := c.surfDraft && now-c.surfAt > surfaceSettleMs
	if !changed && !due && gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	slices, res := surfaceSlices, surfaceRes
	if changed {
		c.surfKey, c.surfAt = key.String(), now
		slices, res = draftSlices, draftRes
	}
	c.surfDraft = changed
	var v []float32
	if c.err == "" && c.expr[0] != nil {
		e := c.expr[0]
		stack := make([]float64, e.StackNeed()+2)
		v = implicit.Contours(func(x, y, z float64) float64 {
			return e.Eval([5]float64{x, y, z}, pv, stack)
		}, float64(c.extent), slices, res)
	}
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
	if paramsSect := paramsDiv.Call("closest", ".sect"); paramsSect.Truthy() {
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
	// In the row's bank, with every other model's constants (bankCustomCells),
	// and the Parameters module put away if that leaves it with nothing.
	if bankCustomCells(defs) {
		if paramsDiv.Get("childElementCount").Int() == 0 {
			showParamsModule(false)
		}
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
