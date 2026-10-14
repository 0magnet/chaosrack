package attractor

import (
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/equation"
	"github.com/0magnet/chaosrack/pkg/implicit"
)

// The Custom model's system, shared by the page (equation_js.go, whose file
// comment says what the flavors are) and a native rack: the equations as
// typed, compiled, and published to the registry that matches their flavor,
// so whatever draws or runs "custom" runs what was typed.

// CustomSystem is the equations as typed, compiled and running.
type CustomSystem struct {
	eq      [4]string
	useW    bool
	iterate bool // flavor: false = flow (derivatives), true = discrete map
	// surface is the third flavor: the first expression is F(x, y, z), and
	// what is drawn is the surface F = 0, by its contours (pkg/implicit),
	// across the cube extent wide either side of the center.
	surface   bool
	extent    float32
	expr      [4]*equation.Expr
	dt        float32
	paramVal  map[string]*float32
	paramList []string // union of params across the active expressions
	err       string
	stack     []float64 // reused eval scratch
	t         float64   // running t for time-dependent systems
	w         float32   // 4th state when useW
}

// NewCustomSystem is the default template, Lorenz, with its parameters
// seeded so Custom shows a real attractor before any editing.
func NewCustomSystem() *CustomSystem {
	c := &CustomSystem{
		eq:       [4]string{"sigma*(y - x)", "x*(rho - z) - y", "x*y - beta*z", "-w"},
		dt:       0.005,
		extent:   1.5,
		paramVal: map[string]*float32{},
	}
	for n, v := range map[string]float32{"sigma": 10, "rho": 28, "beta": 2.6667} {
		vv := v
		c.paramVal[n] = &vv
	}
	return c
}

// eqLabel names row i of the editor in the current flavor. A map's rows are
// not derivatives and must not be labeled as though they were: x' = 1 − ax² + y
// is Henon, dx/dt = 1 − ax² + y is something else entirely.
func (c *CustomSystem) eqLabel(i int) string {
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
func (c *CustomSystem) flavorW() bool { return c.useW && !c.iterate && !c.surface }

// parseCustom (re)compiles the equation strings, refreshes the parameter list
// (keeping existing values), and records any parse error in custom.err.
func (c *CustomSystem) parseCustom() {
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
func (c *CustomSystem) paramPtrs(exprs []*equation.Expr) [][]*float32 {
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
func (c *CustomSystem) registerCustomSystem() {
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
func (c *CustomSystem) registerCustomFlow() {
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

// SurfaceKey is what the surface flavor's drawing is built from — F, the
// extent and F's parameter values — and those values, aligned to F's
// parameters: a drawing is kept until its key changes.
func (c *CustomSystem) SurfaceKey() (string, []float64) {
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
	return key.String(), pv
}

// SurfaceLines draws the surface F = 0 by its contours, across the cube
// extent either side of the center, slices planes across each axis of res
// cells: x,y,z triples, in pairs for line segments. None while F does not
// compile.
func (c *CustomSystem) SurfaceLines(pv []float64, slices, res int) []float32 {
	if c.err != "" || c.expr[0] == nil {
		return nil
	}
	e := c.expr[0]
	stack := make([]float64, e.StackNeed()+2)
	return implicit.Contours(func(x, y, z float64) float64 {
		return e.Eval([5]float64{x, y, z}, pv, stack)
	}, float64(c.extent), slices, res)
}

// Compile compiles the equations and publishes them to the registry their
// flavor belongs in.
func (c *CustomSystem) Compile() { c.parseCustom() }

// Params are the constants the equations name, as the knobs custom-NAME.
func (c *CustomSystem) Params() []string { return c.paramList }

// SetParam sets a constant, live: the registered system reads it each step.
func (c *CustomSystem) SetParam(name string, v float32) {
	if p := c.paramVal[name]; p != nil {
		*p = v
	}
}

// SetDt sets the flow flavor's step.
func (c *CustomSystem) SetDt(v float32) {
	if v > 0 {
		c.dt = v
	}
}

// Flavor is which of the three the equations are read as, and whether the
// fourth state is in play.
func (c *CustomSystem) Flavor() (iterate, surface, useW bool) { return c.iterate, c.surface, c.useW }

// SetFlavor reads the equations as a flow, a map (iterate) or a surface,
// one at a time, with or without the fourth state, and compiles them so.
func (c *CustomSystem) SetFlavor(iterate, surface, useW bool) {
	if surface {
		iterate = false
	}
	c.iterate, c.surface, c.useW = iterate, surface, useW
	c.parseCustom()
}

// Err is why the equations do not compile, or "".
func (c *CustomSystem) Err() string { return c.err }

// Extent is the surface flavor's half-width.
func (c *CustomSystem) Extent() float32 { return c.extent }

// Param is a constant's value, if the equations name it.
func (c *CustomSystem) Param(name string) (float32, bool) {
	if p := c.paramVal[name]; p != nil {
		return *p, true
	}
	return 0, false
}

// Dt is the flow flavor's step.
func (c *CustomSystem) Dt() float32 { return c.dt }
