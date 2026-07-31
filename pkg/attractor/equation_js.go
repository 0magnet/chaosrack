//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"syscall/js"
)

// Custom mode: user-editable attractor equations. The three (optionally four)
// derivative expressions are parsed by the engine in equation.go; any free
// identifier becomes a knobbed parameter. The same compiled forms drive the
// integrator here, so a typed system behaves like any built-in — and is
// shareable via the permalink. This is the representation the schematic view
// will later render from.

var (
	customEq        = [4]string{"sigma*(y - x)", "x*(rho - z) - y", "x*y - beta*z", "-w"}
	customUseW      bool
	customExpr      [4]*Expr
	customDT        float32  = 0.005
	customParamVal           = map[string]*float32{}
	customParamList []string // union of params across the active expressions
	customErr       string
	customStack     []float64 // reused eval scratch
	customT         float64   // running t for time-dependent systems
	customW         float32   // 4th state when useW
)

func eqLabel(i int) string {
	return [4]string{"dx/dt", "dy/dt", "dz/dt", "dw/dt"}[i]
}

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
	customErr = ""
	seen := map[string]bool{}
	var order []string
	maxRPN := 1
	for i := 0; i < 4; i++ {
		customExpr[i] = nil
		if i == 3 && !customUseW {
			continue
		}
		e, err := ParseExpr(customEq[i])
		if err != nil {
			customErr = eqLabel(i) + ": " + err.Error()
			return
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
	registerCustomFlow()
}

// registerCustomFlow publishes the compiled equations to the 4D flow registry
// so Model Out FLOW and the ring beam integrate the SAME system the renderer
// draws. The closure keeps its own eval scratch (everything runs on the one
// JS thread, but the audio callback must not share generateCustom's stack
// mid-frame) and re-reads parameter values each call so knob edits are live.
func registerCustomFlow() {
	if customErr != "" || customExpr[0] == nil {
		delete(flowSystems4, "custom")
		return
	}
	exprs := customExpr
	useW := customUseW
	stack := make([]float64, len(customStack))
	pv := [4][]float64{}
	pp := [4][]*float32{}
	for i := 0; i < 4; i++ {
		if exprs[i] == nil {
			continue
		}
		pv[i] = make([]float64, len(exprs[i].Params))
		pp[i] = make([]*float32, len(exprs[i].Params))
		for k, p := range exprs[i].Params {
			pp[i][k] = customParamVal[p]
		}
	}
	registerFlow4("custom", flowSys4{
		dt: func() float64 { return float64(customDT) },
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

// generateCustom integrates the compiled expressions with forward Euler,
// exactly like the built-in attractors.
func generateCustom() {
	if customErr != "" || customExpr[0] == nil {
		// Nothing valid to integrate — leave the last frame on screen.
		uploadVerticesOnly(vertBuf[:steps*4], attractorDrawMode, steps)
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
		inp.Set("title", eqLabel(i)+" — expression in x, y, z"+map[bool]string{true: ", w", false: ""}[customUseW]+", t; any other letters become knobbed parameters (e / pi / tau are constants)")
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
	if customUseW {
		eqCol.Call("appendChild", makeEqField(3))
	}

	// 4D (w) toggle + error line
	ctlRow := doc.Call("createElement", "span")
	ctlRow.Set("className", "grp")
	wlbl := doc.Call("createElement", "label")
	wlbl.Set("className", "grp")
	wlbl.Set("style", "cursor:pointer;color:#8cf;")
	wchk := doc.Call("createElement", "input")
	wchk.Set("type", "checkbox")
	wchk.Set("className", "sw")
	wchk.Set("title", "4D — add a fourth state variable w with its own dw/dt equation (hidden from the 3D plot, fed back through the others)")
	wchk.Set("checked", customUseW)
	wchk.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		customUseW = wchk.Get("checked").Bool()
		resetAttractorState()
		buildParamPanel("custom")
		syncPermalinkNow()
		return nil
	}))
	wlbl.Call("appendChild", wchk)
	wtxt := doc.Call("createElement", "span")
	wtxt.Set("textContent", " 4D (w)")
	wlbl.Call("appendChild", wtxt)
	ctlRow.Call("appendChild", wlbl)
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
		hdr.Set("title", "Equation — the editable system: one derivative expression per state variable; commits on Enter/blur")
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
	defs := []paramDef{{"custom-dt", "dt", &customDT, 0.005, 0.0001, 0.05, 0.0001}}
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
	b.WriteString("&cdt=" + permaFmt(customDT))
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
