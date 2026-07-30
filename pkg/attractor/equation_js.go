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

// customKnobRow builds a labeled hidden-slider + knob + number + reset that
// drives *target in [min,max], mirroring buildParamPanel's parameter cells.
func customKnobRow(label string, target *float32, min, max, step, def float32) js.Value {
	dec := decimalsForStep(step)
	fmtG := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	fmtF := func(v float32) string { return strconv.FormatFloat(float64(v), 'f', dec, 64) }

	cell := doc.Call("createElement", "span")
	cell.Set("className", "grp")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", labelIsSym(label))) // symbols keep case; words uppercase
	lbl.Set("textContent", label+" ")
	cell.Call("appendChild", lbl)

	slider := doc.Call("createElement", "input")
	slider.Set("type", "range")
	slider.Set("min", fmtG(min))
	slider.Set("max", fmtG(max))
	slider.Set("step", fmtG(step))
	slider.Set("value", fmtG(*target))
	slider.Set("style", "display:none;")

	num := doc.Call("createElement", "input")
	num.Set("type", "number")
	num.Set("min", fmtG(min))
	num.Set("max", fmtG(max))
	num.Set("step", fmtG(step))
	num.Set("value", fmtF(*target))
	num.Set("className", "numin")

	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			*target = float32(v)
			num.Set("value", strconv.FormatFloat(v, 'f', dec, 64))
			resetAttractorState()
		}
		return nil
	}))
	num.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if v, err := strconv.ParseFloat(num.Get("value").String(), 64); err == nil {
			*target = float32(v)
			slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
			resetAttractorState()
		}
		return nil
	}))

	rst := doc.Call("createElement", "button")
	rst.Set("className", "rst")
	rst.Set("textContent", "↺")
	rst.Set("title", "Reset "+label)
	rst.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		slider.Set("value", fmtG(def))
		slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return nil
	}))

	cell.Call("appendChild", slider)
	cell.Call("appendChild", makeKnob(slider, num, true, false, true))
	cell.Call("appendChild", num)
	cell.Call("appendChild", rst)
	return cell
}

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
		inp.Set("style", "width:300px;background:#0a1420;color:#cde;border:1px solid #345;font-family:monospace;font-size:12px;padding:2px 4px;")
		inp.Set("title", "Expression in x, y, z"+map[bool]string{true: ", w", false: ""}[customUseW]+", t; other letters become parameters")
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
		eqMod.Call("appendChild", hdr)
		body := doc.Call("createElement", "div")
		body.Set("className", "row")
		body.Call("appendChild", eqCol)
		eqMod.Call("appendChild", body)
		paramsSect.Get("parentNode").Call("insertBefore", eqMod, paramsSect)
	}

	// dt + detected parameter knobs (in the Parameters module). They go into
	// a punit-grid — the height-bounded column-wrap container — so many
	// params flow into extra columns and the width quantizer widens the
	// module. Appended loose to #params (display:contents) they bypassed both
	// and clipped against the module edges.
	grid := doc.Call("createElement", "div")
	grid.Set("className", "punit-grid")
	grid.Call("appendChild", customKnobRow("dt", &customDT, 0.0001, 0.05, 0.0001, 0.005))
	for _, name := range customParamList {
		ptr := customParamVal[name]
		if ptr == nil {
			continue
		}
		grid.Call("appendChild", customKnobRow(name, ptr, -10, 10, 0.01, 1))
	}
	paramsDiv.Call("appendChild", grid)
}

// ── Seeding the editor from a built-in ────────────────────────────────────

type builtinEq struct {
	eq     [4]string
	params map[string]float32
	useW   bool
	dt     float32
}

// builtinEquations gives parseable (explicit-operator) forms of the built-in
// systems so "Edit equations" can seed the editor. Not every mode is here;
// missing ones fall back to the default template.
var builtinEquations = map[string]builtinEq{
	"lorenz":        {[4]string{"sigma*(y - x)", "x*(rho - z) - y", "x*y - beta*z", ""}, map[string]float32{"sigma": 10, "rho": 28, "beta": 2.6667}, false, 0.005},
	"rossler":       {[4]string{"-(y + z)", "x + a*y", "b + z*(x - c)", ""}, map[string]float32{"a": 0.2, "b": 0.2, "c": 5.7}, false, 0.005},
	"thomas":        {[4]string{"-b*x + sin(y)", "-b*y + sin(z)", "-b*z + sin(x)", ""}, map[string]float32{"b": 0.208}, false, 0.05},
	"halvorsen":     {[4]string{"-a*x - 4*y - 4*z - y^2", "-a*y - 4*z - 4*x - z^2", "-a*z - 4*x - 4*y - x^2", ""}, map[string]float32{"a": 1.89}, false, 0.005},
	"chen":          {[4]string{"a*(y - x)", "(c - a)*x - x*z + c*y", "x*y - b*z", ""}, map[string]float32{"a": 35, "b": 3, "c": 28}, false, 0.0005},
	"aizawa":        {[4]string{"(z - b)*x - d*y", "d*x + (z - b)*y", "c + a*z - z^3/3 - (x^2 + y^2)*(1 + e*z) + f*z*x^3", ""}, map[string]float32{"a": 0.95, "b": 0.7, "c": 0.6, "d": 3.5, "e": 0.25, "f": 0.1}, false, 0.0052},
	"hyperrossler":  {[4]string{"-y - z", "x + a*y + w", "b + x*z", "-c*z + d*w"}, map[string]float32{"a": 0.25, "b": 3, "c": 0.5, "d": 0.05}, true, 0.001},
	"chua":          {[4]string{"alpha*(y - x - (m1*x + 0.5*(m0 - m1)*(abs(x + 1) - abs(x - 1))))", "x - y + z", "-beta*y", ""}, map[string]float32{"alpha": 15.6, "beta": 28.0, "m0": -1.143, "m1": -0.714}, false, 0.005},
	"dadras":        {[4]string{"y - p*x + q*y*z", "r*y - x*z + z", "s*x*y - e*z", ""}, map[string]float32{"p": 3, "q": 2.7, "r": 1.7, "s": 2, "e": 9}, false, 0.005},
	"rabinovich":    {[4]string{"y*(z - 1 + x^2) + gamma*x", "x*(3*z + 1 - x^2) + gamma*y", "-2*z*(alpha + x*y)", ""}, map[string]float32{"alpha": 1.1, "gamma": 0.87}, false, 0.001},
	"burkeshaw":     {[4]string{"-s*(x + y)", "-y - s*x*z", "s*x*y + v", ""}, map[string]float32{"s": 10, "v": 4.272}, false, 0.005},
	"lu":            {[4]string{"a*(y - x)", "c*y - x*z", "x*y - b*z", ""}, map[string]float32{"a": 36, "b": 3, "c": 20}, false, 0.005},
	"sprotta":       {[4]string{"y", "-x + y*z", "1 - y^2", ""}, nil, false, 0.01},
	"newtonleipnik": {[4]string{"-a*x + y + 10*y*z", "-x - 0.4*y + 5*x*z", "b*z - 5*x*y", ""}, map[string]float32{"a": 0.4, "b": 0.175}, false, 0.005},
	"sprott":        {[4]string{"y + a*x*y + x*z", "1 - b*x^2 + y*z", "x - x^2 - y^2", ""}, map[string]float32{"a": 1.9, "b": 2.0}, false, 0.005},
	// Sprott 1994 catalog (explicit operators)
	"sprottb": {[4]string{"y*z", "x - y", "1 - x*y", ""}, nil, false, 0.01},
	"sprottc": {[4]string{"y*z", "x - y", "1 - x^2", ""}, nil, false, 0.01},
	"sprottd": {[4]string{"-y", "x + z", "x*z + 3*y^2", ""}, nil, false, 0.01},
	"sprotte": {[4]string{"y*z", "x^2 - y", "1 - 4*x", ""}, nil, false, 0.01},
	"sprottf": {[4]string{"y + z", "-x + 0.5*y", "x^2 - z", ""}, nil, false, 0.01},
	"sprottg": {[4]string{"0.4*x + z", "x*z - y", "-x + y", ""}, nil, false, 0.01},
	"sprotth": {[4]string{"-y + z^2", "x + 0.5*y", "x - z", ""}, nil, false, 0.01},
	"sprotti": {[4]string{"-0.2*y", "x + z", "x + y^2 - z", ""}, nil, false, 0.01},
	"sprottj": {[4]string{"2*z", "-2*y + z", "-x + y + y^2", ""}, nil, false, 0.01},
	"sprottk": {[4]string{"x*y - z", "x - y", "x + 0.3*z", ""}, nil, false, 0.01},
	"sprottl": {[4]string{"y + 3.9*z", "0.9*x^2 - y", "1 - x", ""}, nil, false, 0.01},
	"sprottm": {[4]string{"-z", "-x^2 - y", "1.7 + 1.7*x + y", ""}, nil, false, 0.005},
	"sprottn": {[4]string{"-2*y", "x + z^2", "1 + y - 2*z", ""}, nil, false, 0.01},
	"sprotto": {[4]string{"y", "x - z", "x + x*z + 2.7*y", ""}, nil, false, 0.005},
	"sprottp": {[4]string{"2.7*y + z", "-x + y^2", "x + y", ""}, nil, false, 0.01},
	"sprottq": {[4]string{"-z", "x - y", "3.1*x + y^2 + 0.5*z", ""}, nil, false, 0.002},
	"sprottr": {[4]string{"0.9 - y", "0.4 + z", "x*y - z", ""}, nil, false, 0.01},
	"sprotts": {[4]string{"-x - 4*y", "x + z^2", "1 + x", ""}, nil, false, 0.002},
}

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
