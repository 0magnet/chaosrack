package attractor

import "testing"

// The Custom mode compiles text somebody typed into an attractor's
// derivatives, and this fuzzes that path for panics.
//
// It is worth fuzzing rather than spot-checking because of where the parser
// runs. A panic here is not a rejected equation and an error message: Go
// compiled to wasm ends the PROGRAM on a panic, so a string that gets through
// tokenize and into Eval with the stack one short would take the whole page
// down — canvas frozen, every control dead, nothing said. Rejecting bad input
// is this parser's ordinary job; the thing that must never happen is
// ACCEPTING something it then cannot evaluate.
//
// ParseExpr guards that with evalCheck, a dry run over the RPN that proves the
// arity works out and the stack lands on exactly one value. This asks whether
// that guard is actually airtight, over inputs nobody would think to write.
func FuzzParseExprNeverPanics(f *testing.F) {
	for _, seed := range []string{
		"", " ", "x", "10*y - x", "x*(rho - z) - y", "x y", "2x", "3(x+1)",
		"sin(x)+cos(y)^2", "-x", "--x", "x^y^z", "a*b*c", "pi*tau/e",
		"(", ")", "()", "(()", "x+", "+x", "*", "^", "1/0", "log(0)",
		"sqrt(-1)", "floor(x)", "sign(-t)", "w+t", "abs(", "sin", "sin()",
		"1e309", "0x10", "1.2.3", "..", "1e", "1e+", "x@y", "x!y",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		e, err := ParseExpr(src)
		if err != nil {
			return // refusing input is the parser doing its job
		}
		if e == nil {
			t.Fatalf("ParseExpr(%q) returned no expression and no error", src)
		}
		// Accepted, so it must evaluate. The contract is a scratch stack of at
		// least len(rpn) and one value per named parameter; give exactly that,
		// because a caller that gave less is a different bug and this test is
		// about the parser keeping its side of the bargain.
		stack := make([]float64, len(e.rpn)+1)
		params := make([]float64, len(e.Params))
		for i := range params {
			params[i] = 0.5
		}
		vars := [5]float64{0.7, -1.3, 2.9, 0.0, 1.1}
		_ = e.Eval(vars, params, stack)

		// And with the awkward values a real integration will reach: zero
		// (division, log), and something enormous.
		_ = e.Eval([5]float64{0, 0, 0, 0, 0}, params, stack)
		big := [5]float64{1e308, -1e308, 1e-308, 0, 1e308}
		_ = e.Eval(big, params, stack)
	})
}
