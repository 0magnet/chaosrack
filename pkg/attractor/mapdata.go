package attractor

import (
	"math"
	"sort"
)

// Discrete maps — the family the catalog was missing.
//
// Everything else here is a FLOW (a vector field integrated forward) or a
// curve traced in time. A map is neither: there is no derivative and no dt,
// only x_{n+1} = f(x_n), and the picture is the set of iterates rather than a
// path through them. That difference is why they look nothing like the rest of
// the app — Hénon's attractor is a Cantor-set of filaments, not a tube — and
// why they are drawn as POINTS. Joining successive iterates with a line would
// be actively misleading: consecutive points of a chaotic map are far apart,
// so the line strip would draw a hairball that says nothing about the orbit.
//
// Untagged (no js build tag) so the equations are testable natively, the way
// flowdata.go and trajectory.go are. The render loop is in mapgen_js.go.
//
// The parameters are package vars rather than constants because the panel
// knobs write to them, exactly as the flow modes do.

// mapStep advances one iterate. Maps that are genuinely 2-D leave z alone;
// the third coordinate exists because the pipeline is 3-D and the figure can
// still be rotated, not because the map uses it.
type mapStep func(x, y, z float64) (float64, float64, float64)

// mapSys is one registered map.
type mapSys struct {
	step mapStep
	ic   [3]float64
	// orbits is how many trajectories to run at once. One is right for the
	// dissipative maps, whose orbits all fall onto the same attractor no
	// matter where they start. It is WRONG for an area-preserving map like
	// Chirikov's, which has no attractor at all: a single orbit there traces
	// one invariant curve and tells you nothing about the phase space, whose
	// whole interest is the coexistence of islands and chaotic sea. Those
	// need an ensemble spread over the plane.
	orbits int
	// seed places orbit i of n for an ensemble map; nil means use ic.
	seed func(i, n int) [3]float64
}

var mapSystems = map[string]mapSys{}

// registerMap adds a dissipative single-orbit map.
func registerMap(key string, ic [3]float64, step mapStep) {
	mapSystems[key] = mapSys{step: step, ic: ic, orbits: 1}
}

// registerEnsembleMap adds a map drawn as many orbits at once. ic is given
// explicitly rather than taken from seed(0, n): a seed function lays orbits
// out across the phase space and the first slot can easily land on a fixed
// point, which is exactly what happened here — the standard map seeded at
// θ=π, p=0 sits on an unstable fixed point and never moves.
func registerEnsembleMap(key string, ic [3]float64, orbits int, seed func(i, n int) [3]float64, step mapStep) {
	mapSystems[key] = mapSys{step: step, ic: ic, orbits: orbits, seed: seed}
}

// IsMap reports whether a mode key names a discrete map — a built-in one, or
// the Custom mode while its editor is in iterate flavor (see maporbit.go).
// This is the question anything that would otherwise INTEGRATE a system has to
// ask: there is no dt to integrate with here.
func IsMap(key string) bool { _, ok := mapSysFor(key); return ok }

// MapKeys lists the registered BUILT-IN maps in stable order. The typed map is
// not one of them: it has no catalog entry, no knobs of its own and no name.
func MapKeys() []string {
	out := make([]string, 0, len(mapSystems))
	for k := range mapSystems {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MapStep exposes a map's iteration for testing and for anything outside the
// render loop that wants to run the same system the screen is drawing.
func MapStep(key string) (func(x, y, z float64) (float64, float64, float64), [3]float64, bool) {
	m, ok := mapSysFor(key)
	if !ok {
		return nil, [3]float64{}, false
	}
	return m.step, m.ic, true
}

// ── parameters ───────────────────────────────────────────────────────────────

var (
	// Hénon (1976), the original "strange attractor from a simple map".
	henonA float32 = 1.4
	henonB float32 = 0.3

	// Ikeda (1979) — a model of light in a nonlinear optical cavity. 0.9, and
	// NOT the 0.918 often quoted: with t = 0.4 − 6/(1+x²+y²) there is a
	// periodic window across roughly 0.905..0.95 where the orbit settles onto a
	// fixed point and the mode draws a single dot.
	ikedaU float32 = 0.9

	// Clifford Pickover's trigonometric attractor.
	cliffordA float32 = -1.4
	cliffordB float32 = 1.6
	cliffordC float32 = 1.0
	cliffordD float32 = 0.7

	// Peter de Jong's, the same idea with a different pairing.
	dejongA float32 = 1.641
	dejongB float32 = 1.902
	dejongC float32 = 0.316
	dejongD float32 = 1.525

	// Gumowski–Mira, from CERN particle-beam studies (1980).
	miraA  float32 = 0.008
	miraB  float32 = 0.05
	miraMu float32 = -0.496

	// Tinkerbell.
	tinkA float32 = 0.9
	tinkB float32 = -0.6013
	tinkC float32 = 2.0
	tinkD float32 = 0.5

	// Chirikov's standard map — the canonical area-preserving map. K is the
	// kick strength; near 0.971635 the last invariant curve breaks and the
	// chaotic sea connects.
	stdK float32 = 0.971635
)

func init() {
	// Hénon: x' = 1 − a x² + y, y' = b x.
	registerMap("henon", [3]float64{0, 0, 0}, func(x, y, _ float64) (float64, float64, float64) {
		return 1 - float64(henonA)*x*x + y, float64(henonB) * x, 0
	})

	// Ikeda: a rotation by t(x,y) composed with a contraction and a shift.
	registerMap("ikeda", [3]float64{0.1, 0.1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		t := 0.4 - 6/(1+x*x+y*y)
		st, ct := math.Sin(t), math.Cos(t)
		u := float64(ikedaU)
		return 1 + u*(x*ct-y*st), u * (x*st + y*ct), 0
	})

	// Clifford: x' = sin(a y) + c cos(a x), y' = sin(b x) + d cos(b y).
	registerMap("clifford", [3]float64{0.1, 0.1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		a, b, c, d := float64(cliffordA), float64(cliffordB), float64(cliffordC), float64(cliffordD)
		return math.Sin(a*y) + c*math.Cos(a*x), math.Sin(b*x) + d*math.Cos(b*y), 0
	})

	// De Jong: x' = sin(a y) − cos(b x), y' = sin(c x) − cos(d y).
	registerMap("dejong", [3]float64{0.1, 0.1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		a, b, c, d := float64(dejongA), float64(dejongB), float64(dejongC), float64(dejongD)
		return math.Sin(a*y) - math.Cos(b*x), math.Sin(c*x) - math.Cos(d*y), 0
	})

	// Gumowski–Mira, with the recurrence's shared nonlinearity g.
	// From (1,0) this lands on a fixed point and draws a single dot; (1,1) is
	// on the figure.
	registerMap("mira", [3]float64{1, 1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		mu := float64(miraMu)
		g := func(v float64) float64 { return mu*v + 2*(1-mu)*v*v/(1+v*v) }
		a, b := float64(miraA), float64(miraB)
		nx := y + a*(1-b*y*y)*y + g(x)
		return nx, -x + g(nx), 0
	})

	// Tinkerbell.
	registerMap("tinkerbell", [3]float64{-0.72, -0.64, 0}, func(x, y, _ float64) (float64, float64, float64) {
		a, b, c, d := float64(tinkA), float64(tinkB), float64(tinkC), float64(tinkD)
		return x*x - y*y + a*x + b*y, 2*x*y + c*x + d*y, 0
	})

	// Chirikov standard map on the torus, as an ensemble: p' = p + K sin θ,
	// θ' = θ + p', both mod 2π. Seeded along a line of momenta so the islands
	// and the chaotic sea appear together.
	registerEnsembleMap("standardmap", [3]float64{2, 3, 0}, 96, func(i, n int) [3]float64 {
		// Offset by half a step so no orbit starts at p=0, which with θ=π is a
		// fixed point that would waste a slot on a stationary dot.
		return [3]float64{math.Pi, 2 * math.Pi * (float64(i) + 0.5) / float64(n), 0}
	}, func(th, p, _ float64) (float64, float64, float64) {
		p += float64(stdK) * math.Sin(th)
		th += p
		return wrapTau(th), wrapTau(p), 0
	})
}

// wrapTau folds a coordinate into [0, 2π). The standard map lives on a torus;
// without the fold the momentum of a chaotic orbit walks off to infinity and
// the figure is a diagonal smear instead of a phase portrait.
func wrapTau(v float64) float64 {
	const tau = 2 * math.Pi
	v = math.Mod(v, tau)
	if v < 0 {
		v += tau
	}
	return v
}
