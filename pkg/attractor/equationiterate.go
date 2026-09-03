package attractor

// The ITERATE flavor of the Custom equation engine: the typed expressions read
// as a discrete map instead of as a vector field.
//
//	flow:    x += dt * f(x, y, z)
//	iterate: x  =      f(x, y, z)
//
// That is a different SYSTEM, not a different integrator, and it is the reason
// the flavor exists at all: Henon's x' = 1 − 1.4x² + y is a fractal of
// filaments as a map and a slow drift to a fixed point as a flow at dt=0.005.
// Until this, the engine could only be told the second thing.
//
// The three expressions are evaluated against the PREVIOUS state and assigned
// together. Sequential assignment would be a different map — Gumowski-Mira in
// mapdata.go deliberately feeds its new x into its y, and gets a different
// figure for it — and simultaneous is the reading that matches how
// x_{n+1} = f(x_n) is written on paper.
//
// Untagged so the whole flavor is testable natively (equationiterate_test.go
// checks a typed Henon against the built-in one); the panel that drives it is
// in equation_js.go.

// iterateBlocker names what stops a compiled expression being run as a map, or
// returns "" if nothing does.
//
// Two things do, and both are rejected rather than quietly evaluated as zero,
// which would draw a different system than the one typed:
//
//   - w. The 4th state is a flow-only feature here. The map machinery is 3-D,
//     and carrying a hidden 4th state in a package var would break the
//     Lyapunov estimator outright: it runs two copies of the step side by side,
//     and they would share the hidden state and measure a system that does not
//     exist.
//   - t. A map has no time. There is nothing between iterate n and n+1 for t to
//     advance by, so an expression in t has no meaning to give it.
func iterateBlocker(e *Expr) string {
	switch {
	case e == nil:
		return ""
	case e.usesVar(3):
		return "w belongs to the flow flavor — iterate is 3-D (x, y, z)"
	case e.usesVar(4):
		return "t has no meaning in a map: there is no time between iterates"
	}
	return ""
}

// newIterateStep compiles the three expressions into the same mapStep shape
// the built-in maps register, so the typed system runs through the identical
// render loop, transient and escape guard.
//
// params holds one pointer slice per expression, aligned to that expression's
// Params, and is re-read on every iterate so a knob edit is live mid-flight —
// the same arrangement registerCustomSystem uses for the flow flavor.
//
// The returned closure owns its scratch and is therefore NOT re-entrant. Its
// two callers — the render loop and the Lyapunov readout, which steps a
// reference and a perturbed copy one after the other — are both on the single
// wasm thread and neither re-enters it.
func newIterateStep(exprs [3]*Expr, params [3][]*float32) mapStep {
	depth := 1
	for _, e := range exprs {
		if e != nil && len(e.rpn) > depth {
			depth = len(e.rpn)
		}
	}
	stack := make([]float64, depth+2)
	var vals [3][]float64
	for i, e := range exprs {
		if e != nil {
			vals[i] = make([]float64, len(e.Params))
		}
	}
	return func(x, y, z float64) (float64, float64, float64) {
		// w and t are zero and stay zero: iterateBlocker refused any expression
		// that reads them, so nothing here can see the difference.
		vars := [5]float64{x, y, z, 0, 0}
		var out [3]float64
		for i, e := range exprs {
			if e == nil {
				continue
			}
			for k, p := range params[i] {
				if p != nil {
					vals[i][k] = float64(*p)
				}
			}
			out[i] = e.Eval(vars, vals[i], stack)
		}
		return out[0], out[1], out[2]
	}
}
