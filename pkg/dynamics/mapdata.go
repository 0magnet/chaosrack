package dynamics

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

// MapStep advances one iterate. Maps that are genuinely 2-D leave z alone;
// the third coordinate exists because the pipeline is 3-D and the figure can
// still be rotated, not because the map uses it.
//
// It is the discrete-time counterpart of Deriv: a flow says how fast the state
// is changing and has to be integrated, a map says what the next state IS.
type MapStep func(x, y, z float64) (float64, float64, float64)

// MapSys is one registered map.
type MapSys struct {
	Step MapStep
	IC   [3]float64
	// Orbits is how many trajectories to run at once. One is right for the
	// dissipative maps, whose orbits all fall onto the same attractor no
	// matter where they start. It is WRONG for an area-preserving map like
	// Chirikov's, which has no attractor at all: a single orbit there traces
	// one invariant curve and tells you nothing about the phase space, whose
	// whole interest is the coexistence of islands and chaotic sea. Those
	// need an ensemble spread over the plane.
	//
	// Read it through Count rather than directly: unset means one.
	Orbits int
	// gen is the revision of a TYPED system (see customGen); zero for every
	// built-in map, which cannot be edited underneath a running orbit.
	gen int
	// Seed places orbit i of n for an ensemble map; nil means use IC.
	// Read it through Start, which applies that fallback.
	Seed func(i, n int) [3]float64
}

// Count is how many orbits this system runs at once, with the "unset means
// one" convention applied. The seeder and the render loop each used to clamp
// Orbits themselves, and two copies of a default are two chances to disagree
// about what an unset field means.
func (m MapSys) Count() int {
	if m.Orbits < 1 {
		return 1
	}
	return m.Orbits
}

// Start is where orbit i of n begins: the ensemble's seed function if there is
// one, and otherwise the system's single initial condition.
//
// This is also the right answer for an orbit that has to be RESTARTED mid-run
// after escaping, which is why it is a method rather than two lines inside the
// seeder — the render loop's escape guard had grown its own copy of the same
// choice, and a map whose seeding rule changed would have had to be fixed in
// both places or drawn one orbit from the wrong starting point.
func (m MapSys) Start(i, n int) [3]float64 {
	if m.Seed != nil {
		return m.Seed(i, n)
	}
	return m.IC
}

var mapSystems = map[string]MapSys{}

// RegisterMap adds a dissipative single-orbit map.
func RegisterMap(key string, ic [3]float64, step MapStep) {
	mapSystems[key] = MapSys{Step: step, IC: ic, Orbits: 1}
}

// RegisterEnsembleMap adds a map drawn as many orbits at once. ic is given
// explicitly rather than taken from seed(0, n): a seed function lays orbits
// out across the phase space and the first slot can easily land on a fixed
// point, which is exactly what happened here — the standard map seeded at
// θ=π, p=0 sits on an unstable fixed point and never moves.
func RegisterEnsembleMap(key string, ic [3]float64, orbits int, seed func(i, n int) [3]float64, step MapStep) {
	mapSystems[key] = MapSys{Step: step, IC: ic, Orbits: orbits, Seed: seed}
}

// IsMap reports whether a mode key names a discrete map — a built-in one, or
// the Custom mode while its editor is in iterate flavor (see maporbit.go).
// This is the question anything that would otherwise INTEGRATE a system has to
// ask: there is no dt to integrate with here.
func IsMap(key string) bool { _, ok := MapFor(key); return ok }

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

// The discrete maps' parameters.
var (
	// HenonA and HenonB are Hénon's (1976) parameters, the original "strange
	// attractor from a simple map".
	HenonA float32 = 1.4
	HenonB float32 = 0.3

	// IkedaU is the Ikeda map's (1979) parameter — a model of light in a
	// nonlinear optical cavity. 0.9, and NOT the 0.918 often quoted: with
	// t = 0.4 − 6/(1+x²+y²) there is a periodic window across roughly
	// 0.905..0.95 where the orbit settles onto a fixed point and the mode draws
	// a single dot.
	IkedaU float32 = 0.9

	// CliffordA through CliffordD are Clifford Pickover's trigonometric attractor.
	CliffordA float32 = -1.4
	CliffordB float32 = 1.6
	CliffordC float32 = 1.0
	CliffordD float32 = 0.7

	// DejongA through DejongD are Peter de Jong's, the same idea with a different
	// pairing.
	DejongA float32 = 1.641
	DejongB float32 = 1.902
	DejongC float32 = 0.316
	DejongD float32 = 1.525

	// MiraA, MiraB and MiraMu are Gumowski–Mira's, from CERN particle-beam studies
	// (1980).
	MiraA  float32 = 0.008
	MiraB  float32 = 0.05
	MiraMu float32 = -0.496

	// TinkA through TinkD are the Tinkerbell map's.
	TinkA float32 = 0.9
	TinkB float32 = -0.6013
	TinkC float32 = 2.0
	TinkD float32 = 0.5

	// StdK is the kick strength of Chirikov's standard map, the canonical
	// area-preserving map; near 0.971635 the last invariant curve breaks and the
	// chaotic sea connects.
	StdK float32 = 0.971635
)

func init() {
	// Hénon: x' = 1 − a x² + y, y' = b x.
	RegisterMap("henon", [3]float64{0, 0, 0}, func(x, y, _ float64) (float64, float64, float64) {
		return 1 - float64(HenonA)*x*x + y, float64(HenonB) * x, 0
	})

	// Ikeda: a rotation by t(x,y) composed with a contraction and a shift.
	RegisterMap("ikeda", [3]float64{0.1, 0.1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		t := 0.4 - 6/(1+x*x+y*y)
		st, ct := math.Sin(t), math.Cos(t)
		u := float64(IkedaU)
		return 1 + u*(x*ct-y*st), u * (x*st + y*ct), 0
	})

	// Clifford: x' = sin(a y) + c cos(a x), y' = sin(b x) + d cos(b y).
	RegisterMap("clifford", [3]float64{0.1, 0.1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		a, b, c, d := float64(CliffordA), float64(CliffordB), float64(CliffordC), float64(CliffordD)
		return math.Sin(a*y) + c*math.Cos(a*x), math.Sin(b*x) + d*math.Cos(b*y), 0
	})

	// De Jong: x' = sin(a y) − cos(b x), y' = sin(c x) − cos(d y).
	RegisterMap("dejong", [3]float64{0.1, 0.1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		a, b, c, d := float64(DejongA), float64(DejongB), float64(DejongC), float64(DejongD)
		return math.Sin(a*y) - math.Cos(b*x), math.Sin(c*x) - math.Cos(d*y), 0
	})

	// Gumowski–Mira, with the recurrence's shared nonlinearity g.
	// From (1,0) this lands on a fixed point and draws a single dot; (1,1) is
	// on the figure.
	RegisterMap("mira", [3]float64{1, 1, 0}, func(x, y, _ float64) (float64, float64, float64) {
		mu := float64(MiraMu)
		g := func(v float64) float64 { return mu*v + 2*(1-mu)*v*v/(1+v*v) }
		a, b := float64(MiraA), float64(MiraB)
		nx := y + a*(1-b*y*y)*y + g(x)
		return nx, -x + g(nx), 0
	})

	// Tinkerbell.
	RegisterMap("tinkerbell", [3]float64{-0.72, -0.64, 0}, func(x, y, _ float64) (float64, float64, float64) {
		a, b, c, d := float64(TinkA), float64(TinkB), float64(TinkC), float64(TinkD)
		return x*x - y*y + a*x + b*y, 2*x*y + c*x + d*y, 0
	})

	// Chirikov standard map on the torus, as an ensemble: p' = p + K sin θ,
	// θ' = θ + p', both mod 2π. Seeded along a line of momenta so the islands
	// and the chaotic sea appear together.
	RegisterEnsembleMap("standardmap", [3]float64{2, 3, 0}, 96, func(i, n int) [3]float64 {
		// Offset by half a step so no orbit starts at p=0, which with θ=π is a
		// fixed point that would waste a slot on a stationary dot.
		return [3]float64{math.Pi, 2 * math.Pi * (float64(i) + 0.5) / float64(n), 0}
	}, func(th, p, _ float64) (float64, float64, float64) {
		p += float64(StdK) * math.Sin(th)
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
