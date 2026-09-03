package attractor

import (
	"math"
	"testing"
)

func evalStr(t *testing.T, s string, vars [5]float64, params map[string]float64) float64 {
	t.Helper()
	e, err := ParseExpr(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	pv := make([]float64, len(e.Params))
	for i, p := range e.Params {
		pv[i] = params[p]
	}
	return e.Eval(vars, pv, make([]float64, len(e.rpn)+1))
}

func TestExpr(t *testing.T) {
	cases := []struct {
		s      string
		vars   [5]float64
		params map[string]float64
		want   float64
	}{
		{"1+2*3", [5]float64{}, nil, 7},
		{"(1+2)*3", [5]float64{}, nil, 9},
		{"2^3^2", [5]float64{}, nil, 512}, // right-assoc
		{"-3+2", [5]float64{}, nil, -1},
		{"-(3+2)", [5]float64{}, nil, -5},
		{"-x^2", [5]float64{3, 0, 0, 0, 0}, nil, -9},            // unary binds looser than ^
		{"-x^2 - y", [5]float64{0.3, -0.7, 0, 0, 0}, nil, 0.61}, // the sprottm seed shape
		{"(-x)^2", [5]float64{3, 0, 0, 0, 0}, nil, 9},
		{"2x", [5]float64{3, 0, 0, 0, 0}, nil, 6},                                       // implicit mult (number)
		{"x y", [5]float64{3, 4, 0, 0, 0}, nil, 12},                                     // implicit mult (space)
		{"y*z", [5]float64{0, 4, 5, 0, 0}, nil, 20},                                     // Sprott B term (explicit)
		{"1-x*y", [5]float64{2, 3, 0, 0, 0}, nil, -5},                                   // Sprott B dz
		{"x^2", [5]float64{3, 0, 0, 0, 0}, nil, 9},                                      // power
		{"3y^2", [5]float64{0, 2, 0, 0, 0}, nil, 12},                                    // Sprott D term
		{"sin(0)+cos(0)", [5]float64{}, nil, 1},                                         // funcs
		{"a*x+b", [5]float64{5, 0, 0, 0, 0}, map[string]float64{"a": 2, "b": 1}, 11},    // params
		{"sigma*(y-x)", [5]float64{1, 4, 0, 0, 0}, map[string]float64{"sigma": 10}, 30}, // lorenz dx
	}
	for _, c := range cases {
		got := evalStr(t, c.s, c.vars, c.params)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%q = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestExprParams(t *testing.T) {
	e, err := ParseExpr("a*x + b*y - z")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b"}
	if len(e.Params) != len(want) {
		t.Fatalf("params = %v, want %v", e.Params, want)
	}
	for i := range want {
		if e.Params[i] != want[i] {
			t.Fatalf("params = %v, want %v", e.Params, want)
		}
	}
}

func TestExprErrors(t *testing.T) {
	for _, s := range []string{"1+", "(1+2", "1)*2", "*3", "sin(", "1 2 +"} {
		if _, err := ParseExpr(s); err == nil {
			// "1 2 +" becomes 1*2+ which is malformed → want error
			t.Errorf("expected error for %q", s)
		}
	}
}
