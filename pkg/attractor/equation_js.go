//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/equation"
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
	eq        [4]string
	useW      bool
	iterate   bool // flavor: false = flow (derivatives), true = discrete map
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
	paramVal: map[string]*float32{},
}

// eqLabel names row i of the editor in the current flavor. A map's rows are
// not derivatives and must not be labeled as though they were: x' = 1 − ax² + y
// is Henon, dx/dt = 1 − ax² + y is something else entirely.
func (c *customEquation) eqLabel(i int) string {
	if c.iterate {
		return [4]string{"x'", "y'", "z'", "w'"}[i]
	}
	return [4]string{"dx/dt", "dy/dt", "dz/dt", "dw/dt"}[i]
}

// flavorW reports whether the 4th state is in play: iterate is 3-D, so
// the w equation is not compiled there even when the toggle is left on (which
// keeps a typed dw/dt safe across a flavor round-trip).
func (c *customEquation) flavorW() bool { return c.useW && !c.iterate }

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
	seen := map[string]bool{}
	var order []string
	maxRPN := 1
	for i := 0; i < 4; i++ {
		c.expr[i] = nil
		if i == 3 && !c.flavorW() {
			continue
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
	if c.err != "" || c.expr[0] == nil {
		return
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
	for i := 0; i < 4; i++ {
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
	for i := 0; i < 4; i++ {
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
	for i := 0; i < sim.steps; i++ {
		for s := 0; s < sub; s++ {
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
			map[bool]string{true: "", false: ", t"}[c.iterate]
		what := map[bool]string{true: "the NEXT value of " + c.eqLabel(i)[:1], false: c.eqLabel(i)}[c.iterate]
		inp.Set("title", what+" — expression in "+vars+"; any other letters become knobbed parameters (e / pi / tau are constants)")
		// Commit on change (blur/Enter) to avoid rebuilding mid-keystroke.
		inp.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			c.eq[i] = inp.Get("value").String()
			resetAttractorState()
			buildParamPanel("custom") // reparse + refresh param knobs
			perma.syncPermalinkNow()
			return nil
		}))
		row.Call("appendChild", lbl)
		row.Call("appendChild", inp)
		return row
	}
	eqCol.Call("appendChild", makeEqField(0))
	eqCol.Call("appendChild", makeEqField(1))
	eqCol.Call("appendChild", makeEqField(2))
	if c.flavorW() {
		eqCol.Call("appendChild", makeEqField(3))
	}

	// Flavor + 4D toggles, then the error line.
	ctlRow := dom.Doc.Call("createElement", "span")
	ctlRow.Set("className", "grp")

	// makeSwitch is the shared anatomy of both toggles: label · checkbox · text,
	// committing through a reparse so the registry, the labels and the knobs all
	// change together.
	makeSwitch := func(text, title string, on bool, set func(bool)) js.Value {
		lbl := dom.Doc.Call("createElement", "label")
		lbl.Set("className", "grp")
		lbl.Set("style", "cursor:pointer;color:#8cf;")
		chk := dom.Doc.Call("createElement", "input")
		chk.Set("type", "checkbox")
		chk.Set("className", "sw")
		chk.Set("title", title)
		chk.Set("checked", on)
		chk.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			set(chk.Get("checked").Bool())
			// Reparse before the rebuild: the panel's mode-scoped syncs run
			// ahead of buildCustomPanel, and IsMap("custom") has to be true by
			// the time syncMapExtras asks — otherwise a switch to iterate poses
			// the (plane) figure face-on one rebuild late.
			c.parseCustom()
			resetAttractorState()
			buildParamPanel("custom")
			perma.syncPermalinkNow()
			return nil
		}))
		lbl.Call("appendChild", chk)
		txt := dom.Doc.Call("createElement", "span")
		txt.Set("textContent", " "+text)
		lbl.Call("appendChild", txt)
		return lbl
	}

	ctlRow.Call("appendChild", makeSwitch("iterate",
		"iterate — read the expressions as a discrete MAP (x = f(x,y,z)) instead of as derivatives to integrate (x += dt·f). No dt, no path between iterates, so it draws as points. Type 1 - 1.4x^2 + y and 0.3x for Henon.",
		c.iterate, func(v bool) { c.iterate = v }))
	if !c.iterate {
		// A map has no hidden 4th state here: the 3-D map machinery cannot carry
		// one, and the Lyapunov estimator runs two copies of the step side by
		// side, which a package-var w would have them share. The typed dw/dt is
		// kept, just not compiled, so flipping back restores it.
		ctlRow.Call("appendChild", makeSwitch("4D (w)",
			"4D — add a fourth state variable w with its own dw/dt equation (hidden from the 3D plot, fed back through the others)",
			c.useW, func(v bool) { c.useW = v }))
	}
	if c.err != "" {
		errSpan := dom.Doc.Call("createElement", "span")
		errSpan.Set("textContent", "⚠ "+c.err)
		errSpan.Set("style", "color:#f86;font-size:11px;margin-left:8px;")
		ctlRow.Call("appendChild", errSpan)
	}
	eqCol.Call("appendChild", ctlRow)

	// The equation editor lives in its own "Equation" module, before Parameters
	// (which holds the detected parameter knobs).
	if paramsSect := paramsDiv.Call("closest", ".sect"); paramsSect.Truthy() {
		if old := dom.Doc.Call("getElementById", "eqn-module"); old.Truthy() {
			old.Get("parentNode").Call("removeChild", old)
		}
		eqMod := dom.Doc.Call("createElement", "div")
		eqMod.Set("className", "sect eqnmodule")
		eqMod.Set("id", "eqn-module")
		hdr := dom.Doc.Call("createElement", "div")
		hdr.Set("className", "sect-hdr")
		hdr.Set("textContent", "Equation")
		hdrTip := "Equation — the editable system: one derivative expression per state variable; commits on Enter/blur"
		if c.iterate {
			hdrTip = "Equation — the editable system: one expression per state variable giving its NEXT value (a discrete map); commits on Enter/blur"
		}
		hdr.Set("title", hdrTip)
		eqMod.Call("appendChild", hdr)
		body := dom.Doc.Call("createElement", "div")
		body.Set("className", "row")
		body.Call("appendChild", eqCol)
		eqMod.Call("appendChild", body)
		paramsSect.Get("parentNode").Call("insertBefore", eqMod, paramsSect)
	}

	// dt + detected parameter knobs, built with the SAME buildParamUnit
	// anatomy as every other mode's Parameters module (label · LED · knob ·
	// step · reset, aligned by the punit grid) — the old bespoke rows
	// misaligned and skipped the role-tooltip pass.
	//
	// No dt knob in iterate flavor: a map has no timestep, and a knob that
	// changes nothing is worse than a missing one.
	var defs []paramDef
	if !c.iterate {
		defs = append(defs, paramDef{"custom-dt", "dt", &c.dt, 0.005, 0.0001, 0.05, 0.0001})
	}
	for _, name := range c.paramList {
		if ptr := c.paramVal[name]; ptr != nil {
			defs = append(defs, paramDef{"custom-" + name, name, ptr, 1, -10, 10, 0.01})
		}
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
	c.iterate = false
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
	b.WriteString("&eq=")
	n := 3
	if c.useW {
		n = 4
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = jsEncodeURI(c.eq[i])
	}
	b.WriteString(strings.Join(parts, ";"))
	for _, name := range c.paramList {
		if ptr := c.paramVal[name]; ptr != nil {
			b.WriteString("&cp." + name + "=" + permaFmt(*ptr))
		}
	}
	if !c.iterate {
		b.WriteString("&cdt=" + permaFmt(c.dt))
	}
}

// applyCustomEq / applyCustomParam restore permalinked custom state.
func (c *customEquation) applyCustomEq(val string) {
	parts := strings.Split(val, ";")
	c.useW = len(parts) >= 4
	for i := 0; i < 4; i++ {
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
