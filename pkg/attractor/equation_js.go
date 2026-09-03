//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"
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

var (
	customEq        = [4]string{"sigma*(y - x)", "x*(rho - z) - y", "x*y - beta*z", "-w"}
	customUseW      bool
	customIterate   bool // flavor: false = flow (derivatives), true = discrete map
	customExpr      [4]*Expr
	customDT        float32  = 0.005
	customParamVal           = map[string]*float32{}
	customParamList []string // union of params across the active expressions
	customErr       string
	customStack     []float64 // reused eval scratch
	customT         float64   // running t for time-dependent systems
	customW         float32   // 4th state when useW
)

// eqLabel names row i of the editor in the current flavor. A map's rows are
// not derivatives and must not be labeled as though they were: x' = 1 − ax² + y
// is Henon, dx/dt = 1 − ax² + y is something else entirely.
func eqLabel(i int) string {
	if customIterate {
		return [4]string{"x'", "y'", "z'", "w'"}[i]
	}
	return [4]string{"dx/dt", "dy/dt", "dz/dt", "dw/dt"}[i]
}

// customFlavorW reports whether the 4th state is in play: iterate is 3-D, so
// the w equation is not compiled there even when the toggle is left on (which
// keeps a typed dw/dt safe across a flavor round-trip).
func customFlavorW() bool { return customUseW && !customIterate }

// Seed the default template's parameters (Lorenz) so Custom mode shows a real
// attractor immediately, before any editing or seeding.
func init() {
	for n, v := range map[string]float32{"sigma": 10, "rho": 28, "beta": 2.6667} {
		vv := v
		customParamVal[n] = &vv
	}
}

// parseCustom (re)compiles the equation strings, refreshes the parameter list
// (keeping existing values), and records any parse error in customErr.
func parseCustom() {
	// Deferred, so that the error returns below re-publish too: they used to
	// return without touching the registry, which left the PREVIOUS system
	// registered — Model Out FLOW would keep sonifying equations that were no
	// longer on screen, and now a flavor switch would leave a map registered as
	// a flow. Withdrawing is as much the job as registering.
	defer registerCustomSystem()
	customErr = ""
	seen := map[string]bool{}
	var order []string
	maxRPN := 1
	for i := 0; i < 4; i++ {
		customExpr[i] = nil
		if i == 3 && !customFlavorW() {
			continue
		}
		e, err := ParseExpr(customEq[i])
		if err != nil {
			customErr = eqLabel(i) + ": " + err.Error()
			return
		}
		if customIterate {
			if why := iterateBlocker(e); why != "" {
				customErr = eqLabel(i) + ": " + why
				return
			}
		}
		customExpr[i] = e
		if len(e.rpn) > maxRPN {
			maxRPN = len(e.rpn)
		}
		for _, p := range e.Params {
			if !seen[p] {
				seen[p] = true
				order = append(order, p)
			}
		}
	}
	for _, p := range order {
		if _, ok := customParamVal[p]; !ok {
			v := float32(1)
			customParamVal[p] = &v
		}
	}
	customParamList = order
	customStack = make([]float64, maxRPN+2)
}

// paramPtrs binds one pointer slice per expression, aligned to that
// expression's Params. Binding the pointers once and dereferencing per step is
// what keeps knob edits live without a map lookup in the hot loop.
func paramPtrs(exprs []*Expr) [][]*float32 {
	out := make([][]*float32, len(exprs))
	for i, e := range exprs {
		if e == nil {
			continue
		}
		out[i] = make([]*float32, len(e.Params))
		for k, p := range e.Params {
			out[i][k] = customParamVal[p]
		}
	}
	return out
}

// registerCustomSystem publishes the compiled equations to the registry that
// matches the FLAVOR — and, as much the point, withdraws them from the other.
//
// Flow flavor goes to flowSystems4, so Model Out FLOW and the ring beam
// integrate the SAME system the renderer draws.
//
// Iterate flavor deliberately does NOT. Everything downstream of flowSystems4
// does one thing with what it finds there: steps it with dt. For a map that
// produces a different system — x' = 1 − 1.4x² + y read as a derivative at
// dt = 0.005 is a slow crawl to a fixed point, not the fractal on the screen —
// so Model Out FLOW would sonify a system nobody typed, the Poincare section
// would hunt for crossings of a trajectory that does not exist (a map has no
// path between iterates to cross anything), and the Lyapunov readout would
// print a per-TIME exponent for a system with no time. Being absent from the
// registry is a shape the consumers already handle: flowFor4 misses and they
// fall back to scanning the drawn trail. The map registry takes it instead,
// which is how IsMap/MapStep steer LyapunovFor to its per-iterate branch.
func registerCustomSystem() {
	delete(flowSystems4, customModeKey)
	clearCustomMap()
	if customErr != "" || customExpr[0] == nil {
		return
	}
	if customIterate {
		pp := paramPtrs(customExpr[:3])
		setCustomMap(newIterateStep(
			[3]*Expr{customExpr[0], customExpr[1], customExpr[2]},
			[3][]*float32{pp[0], pp[1], pp[2]}))
		return
	}
	registerCustomFlow()
}

// registerCustomFlow publishes the flow flavor. The closure keeps its own eval
// scratch (everything runs on the one JS thread, but the audio callback must
// not share generateCustom's stack mid-frame) and re-reads parameter values
// each call so knob edits are live.
func registerCustomFlow() {
	exprs := customExpr
	useW := customFlavorW()
	stack := make([]float64, len(customStack))
	pv := [4][]float64{}
	pp := paramPtrs(exprs[:])
	for i := 0; i < 4; i++ {
		if exprs[i] != nil {
			pv[i] = make([]float64, len(exprs[i].Params))
		}
	}
	registerFlow4(customModeKey, flowSys4{
		dt:    func() float64 { return float64(customDT) },
		euler: true, // generateCustom integrates with forward Euler
		f: func(x, y, z, w float64) (float64, float64, float64, float64) {
			vars := [5]float64{x, y, z, w, customT}
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
		w:           func() float64 { return float64(customW) },
		setW:        func(v float64) { customW = float32(v) },
		interpreted: true,
	})
}

// generateCustom runs whichever flavor is selected: forward Euler over the
// derivatives, exactly like the built-in attractors, or — in iterate flavor —
// the shared discrete-map loop, which is where the points draw mode, the
// discarded transient and the escape-reseed already live.
func generateCustom() {
	if customErr != "" || customExpr[0] == nil {
		// Nothing valid to run — leave the last frame on screen.
		uploadVerticesOnly(vertBuf[:steps*4], mapDrawMode(customModeKey), steps)
		return
	}
	if customIterate {
		generateMap(customModeKey)
		return
	}
	// Per-frame snapshot of each expression's parameter values (aligned to
	// its own Params slice), so the hot loop does no map lookups.
	var pv [4][]float64
	for i := 0; i < 4; i++ {
		if customExpr[i] == nil {
			continue
		}
		s := make([]float64, len(customExpr[i].Params))
		for k, p := range customExpr[i].Params {
			if ptr := customParamVal[p]; ptr != nil {
				s[k] = float64(*ptr)
			}
		}
		pv[i] = s
	}
	dt := float64(customDT) * float64(speedScale)
	stack := customStack
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	sub := effSubSteps(speedSteps, steps, frameBudgetInterpreted)
	for i := 0; i < steps; i++ {
		for s := 0; s < sub; s++ {
			vars := [5]float64{float64(x), float64(y), float64(z), float64(customW), customT}
			dx := customExpr[0].Eval(vars, pv[0], stack)
			dy := 0.0
			if customExpr[1] != nil {
				dy = customExpr[1].Eval(vars, pv[1], stack)
			}
			dz := 0.0
			if customExpr[2] != nil {
				dz = customExpr[2].Eval(vars, pv[2], stack)
			}
			x += float32(dt * dx)
			y += float32(dt * dy)
			z += float32(dt * dz)
			if customUseW && customExpr[3] != nil {
				customW += float32(dt * customExpr[3].Eval(vars, pv[3], stack))
			}
			customT += dt
			checkDiverged()
		}
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = x, y, z, float32(i)*invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}

// ── Custom-mode control panel ─────────────────────────────────────────────

// buildCustomPanel renders the equation editor into #params: three/four
// equation fields, a 4D toggle, a dt knob, a parse-error line, and a knob per
// detected parameter. Called from buildParamPanel when mode == "custom".
func buildCustomPanel(paramsDiv js.Value) {
	parseCustom()

	eqCol := doc.Call("createElement", "span")
	eqCol.Set("className", "pcell")
	eqCol.Set("style", "gap:2px;")

	makeEqField := func(i int) js.Value {
		row := doc.Call("createElement", "span")
		row.Set("className", "grp")
		lbl := doc.Call("createElement", "span")
		lbl.Set("textContent", eqLabel(i)+" =")
		lbl.Set("style", "color:#8cf;min-width:44px;")
		inp := doc.Call("createElement", "input")
		inp.Set("type", "text")
		inp.Set("value", customEq[i])
		inp.Set("spellcheck", false)
		inp.Set("style", "width:180px;background:#0a1420;color:#cde;border:1px solid #345;font-family:monospace;font-size:12px;padding:2px 4px;")
		vars := "x, y, z" + map[bool]string{true: ", w", false: ""}[customFlavorW()] +
			map[bool]string{true: "", false: ", t"}[customIterate]
		what := map[bool]string{true: "the NEXT value of " + eqLabel(i)[:1], false: eqLabel(i)}[customIterate]
		inp.Set("title", what+" — expression in "+vars+"; any other letters become knobbed parameters (e / pi / tau are constants)")
		// Commit on change (blur/Enter) to avoid rebuilding mid-keystroke.
		inp.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			customEq[i] = inp.Get("value").String()
			resetAttractorState()
			buildParamPanel("custom") // reparse + refresh param knobs
			syncPermalinkNow()
			return nil
		}))
		row.Call("appendChild", lbl)
		row.Call("appendChild", inp)
		return row
	}
	eqCol.Call("appendChild", makeEqField(0))
	eqCol.Call("appendChild", makeEqField(1))
	eqCol.Call("appendChild", makeEqField(2))
	if customFlavorW() {
		eqCol.Call("appendChild", makeEqField(3))
	}

	// Flavor + 4D toggles, then the error line.
	ctlRow := doc.Call("createElement", "span")
	ctlRow.Set("className", "grp")

	// makeSwitch is the shared anatomy of both toggles: label · checkbox · text,
	// committing through a reparse so the registry, the labels and the knobs all
	// change together.
	makeSwitch := func(text, title string, on bool, set func(bool)) js.Value {
		lbl := doc.Call("createElement", "label")
		lbl.Set("className", "grp")
		lbl.Set("style", "cursor:pointer;color:#8cf;")
		chk := doc.Call("createElement", "input")
		chk.Set("type", "checkbox")
		chk.Set("className", "sw")
		chk.Set("title", title)
		chk.Set("checked", on)
		chk.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			set(chk.Get("checked").Bool())
			// Reparse before the rebuild: the panel's mode-scoped syncs run
			// ahead of buildCustomPanel, and IsMap("custom") has to be true by
			// the time syncMapExtras asks — otherwise a switch to iterate poses
			// the (plane) figure face-on one rebuild late.
			parseCustom()
			resetAttractorState()
			buildParamPanel("custom")
			syncPermalinkNow()
			return nil
		}))
		lbl.Call("appendChild", chk)
		txt := doc.Call("createElement", "span")
		txt.Set("textContent", " "+text)
		lbl.Call("appendChild", txt)
		return lbl
	}

	ctlRow.Call("appendChild", makeSwitch("iterate",
		"iterate — read the expressions as a discrete MAP (x = f(x,y,z)) instead of as derivatives to integrate (x += dt·f). No dt, no path between iterates, so it draws as points. Type 1 - 1.4x^2 + y and 0.3x for Henon.",
		customIterate, func(v bool) { customIterate = v }))
	if !customIterate {
		// A map has no hidden 4th state here: the 3-D map machinery cannot carry
		// one, and the Lyapunov estimator runs two copies of the step side by
		// side, which a package-var w would have them share. The typed dw/dt is
		// kept, just not compiled, so flipping back restores it.
		ctlRow.Call("appendChild", makeSwitch("4D (w)",
			"4D — add a fourth state variable w with its own dw/dt equation (hidden from the 3D plot, fed back through the others)",
			customUseW, func(v bool) { customUseW = v }))
	}
	if customErr != "" {
		errSpan := doc.Call("createElement", "span")
		errSpan.Set("textContent", "⚠ "+customErr)
		errSpan.Set("style", "color:#f86;font-size:11px;margin-left:8px;")
		ctlRow.Call("appendChild", errSpan)
	}
	eqCol.Call("appendChild", ctlRow)

	// The equation editor lives in its own "Equation" module, before Parameters
	// (which holds the detected parameter knobs).
	if paramsSect := paramsDiv.Call("closest", ".sect"); paramsSect.Truthy() {
		if old := doc.Call("getElementById", "eqn-module"); old.Truthy() {
			old.Get("parentNode").Call("removeChild", old)
		}
		eqMod := doc.Call("createElement", "div")
		eqMod.Set("className", "sect eqnmodule")
		eqMod.Set("id", "eqn-module")
		hdr := doc.Call("createElement", "div")
		hdr.Set("className", "sect-hdr")
		hdr.Set("textContent", "Equation")
		hdrTip := "Equation — the editable system: one derivative expression per state variable; commits on Enter/blur"
		if customIterate {
			hdrTip = "Equation — the editable system: one expression per state variable giving its NEXT value (a discrete map); commits on Enter/blur"
		}
		hdr.Set("title", hdrTip)
		eqMod.Call("appendChild", hdr)
		body := doc.Call("createElement", "div")
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
	if !customIterate {
		defs = append(defs, paramDef{"custom-dt", "dt", &customDT, 0.005, 0.0001, 0.05, 0.0001})
	}
	for _, name := range customParamList {
		if ptr := customParamVal[name]; ptr != nil {
			defs = append(defs, paramDef{"custom-" + name, name, ptr, 1, -10, 10, 0.01})
		}
	}
	grid := doc.Call("createElement", "div")
	gc := "punit-grid"
	if len(defs) > 3 {
		gc += " two-col"
	}
	grid.Set("className", gc)
	for _, d := range defs {
		grid.Call("appendChild", buildParamUnit(d))
	}
	paramsDiv.Call("appendChild", grid)
}

// ── Seeding the editor from a built-in ────────────────────────────────────

// seedCustomFromMode loads the given built-in's equations into the editor
// (falling back to the current default template if unknown) and returns
// "custom" so the caller can switch modes.
func seedCustomFromMode(mode string) {
	be, ok := builtinEquations[mode]
	if !ok {
		return // keep whatever's already in the editor
	}
	customEq = be.eq
	customUseW = be.useW
	customDT = be.dt
	// Every seed in the table is a FLOW (the guard in chaos_test.go checks each
	// one against the mode's vector field), so seeding leaves the iterate
	// flavor: read as a map, Lorenz's dx/dt is not Lorenz.
	customIterate = false
	customParamVal = map[string]*float32{}
	for name, val := range be.params {
		v := val
		customParamVal[name] = &v
	}
	customT = 0
	customW = 0
	parseCustom()
}

// serializeCustom appends the custom equations + params to the permalink.
func serializeCustom(b *strings.Builder) {
	// The flavor first: it decides what the expressions MEAN, and a link that
	// restored Henon's equations as a flow would restore a different system.
	// Omitted when false, like every other control at its default.
	if customIterate {
		b.WriteString("&cit=1")
	}
	b.WriteString("&eq=")
	n := 3
	if customUseW {
		n = 4
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = jsEncodeURI(customEq[i])
	}
	b.WriteString(strings.Join(parts, ";"))
	for _, name := range customParamList {
		if ptr := customParamVal[name]; ptr != nil {
			b.WriteString("&cp." + name + "=" + permaFmt(*ptr))
		}
	}
	if !customIterate {
		b.WriteString("&cdt=" + permaFmt(customDT))
	}
}

// applyCustomEq / applyCustomParam restore permalinked custom state.
func applyCustomEq(val string) {
	parts := strings.Split(val, ";")
	customUseW = len(parts) >= 4
	for i := 0; i < 4; i++ {
		if i < len(parts) {
			customEq[i] = jsDecodeURI(parts[i])
		}
	}
	parseCustom()
}

func applyCustomParam(name, val string) {
	if v, err := strconv.ParseFloat(val, 32); err == nil {
		f := float32(v)
		customParamVal[name] = &f
	}
}

func jsEncodeURI(s string) string {
	return js.Global().Call("encodeURIComponent", s).String()
}
func jsDecodeURI(s string) string {
	return js.Global().Call("decodeURIComponent", s).String()
}
