package dynamics

// Data half of the Sprott catalog + Rössler hyperchaos: the equations, dts,
// and initial conditions, untagged so the native chaos guard integrates the
// EXACT systems the app renders (the js half — panel registration and the
// render loops — stays in sprottcases.go). See that file for citations.

// Case is one member of the Sprott catalog. deriv returns the time
// derivatives at (x,y,z); the coefficients are baked in (Sprott's systems are
// specific, not tunable families) while dt stays user-adjustable.
// Case is one of Sprott's nineteen systems.
//
// The fields are exported because the catalog and the info overlay describe
// these systems to the user: the label and the written-out equations are as
// much a part of "what this system is" as the vector field is.
type Case struct {
	Key   string // mode string, e.g. "sprottb"
	Name  string // dropdown label, e.g. "Sprott B"
	Eq    string // human-readable ODEs for the info overlay
	DT    float32
	IC    [3]float32
	Deriv Deriv
}

// SprottCases are the Sprott systems B through S, each with its equations,
// step and initial condition.
var SprottCases = []Case{
	{"sprottb", "Sprott B", "dx/dt = yz\ndy/dt = x − y\ndz/dt = 1 − xy", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y * z, x - y, 1 - x*y }},
	{"sprottc", "Sprott C", "dx/dt = yz\ndy/dt = x − y\ndz/dt = 1 − x²", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y * z, x - y, 1 - x*x }},
	{"sprottd", "Sprott D", "dx/dt = −y\ndy/dt = x + z\ndz/dt = xz + 3y²", 0.01, [3]float32{0.05, 0.05, 0.05},
		func(x, y, z float64) (float64, float64, float64) { return -y, x + z, x*z + 3*y*y }},
	{"sprotte", "Sprott E", "dx/dt = yz\ndy/dt = x² − y\ndz/dt = 1 − 4x", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y * z, x*x - y, 1 - 4*x }},
	{"sprottf", "Sprott F", "dx/dt = y + z\ndy/dt = −x + 0.5y\ndz/dt = x² − z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y + z, -x + 0.5*y, x*x - z }},
	{"sprottg", "Sprott G", "dx/dt = 0.4x + z\ndy/dt = xz − y\ndz/dt = −x + y", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 0.4*x + z, x*z - y, -x + y }},
	{"sprotth", "Sprott H", "dx/dt = −y + z²\ndy/dt = x + 0.5y\ndz/dt = x − z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -y + z*z, x + 0.5*y, x - z }},
	{"sprotti", "Sprott I", "dx/dt = −0.2y\ndy/dt = x + z\ndz/dt = x + y² − z", 0.01, [3]float32{0.05, 0.05, 0.05},
		func(x, y, z float64) (float64, float64, float64) { return -0.2 * y, x + z, x + y*y - z }},
	{"sprottj", "Sprott J", "dx/dt = 2z\ndy/dt = −2y + z\ndz/dt = −x + y + y²", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 2 * z, -2*y + z, -x + y + y*y }},
	{"sprottk", "Sprott K", "dx/dt = xy − z\ndy/dt = x − y\ndz/dt = x + 0.3z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return x*y - z, x - y, x + 0.3*z }},
	{"sprottl", "Sprott L", "dx/dt = y + 3.9z\ndy/dt = 0.9x² − y\ndz/dt = 1 − x", 0.01, [3]float32{-1, 0, 0},
		func(x, y, z float64) (float64, float64, float64) { return y + 3.9*z, 0.9*x*x - y, 1 - x }},
	{"sprottm", "Sprott M", "dx/dt = −z\ndy/dt = −x² − y\ndz/dt = 1.7 + 1.7x + y", 0.005, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -z, -x*x - y, 1.7 + 1.7*x + y }},
	{"sprottn", "Sprott N", "dx/dt = −2y\ndy/dt = x + z²\ndz/dt = 1 + y − 2z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -2 * y, x + z*z, 1 + y - 2*z }},
	{"sprotto", "Sprott O", "dx/dt = y\ndy/dt = x − z\ndz/dt = x + xz + 2.7y", 0.005, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return y, x - z, x + x*z + 2.7*y }},
	{"sprottp", "Sprott P", "dx/dt = 2.7y + z\ndy/dt = −x + y²\ndz/dt = x + y", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 2.7*y + z, -x + y*y, x + y }},
	{"sprottq", "Sprott Q", "dx/dt = −z\ndy/dt = x − y\ndz/dt = 3.1x + y² + 0.5z", 0.002, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -z, x - y, 3.1*x + y*y + 0.5*z }},
	{"sprottr", "Sprott R", "dx/dt = 0.9 − y\ndy/dt = 0.4 + z\ndz/dt = xy − z", 0.01, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return 0.9 - y, 0.4 + z, x*y - z }},
	{"sprotts", "Sprott S", "dx/dt = −x − 4y\ndy/dt = x + z²\ndz/dt = 1 + x", 0.002, [3]float32{0.1, 0.2, 0.3},
		func(x, y, z float64) (float64, float64, float64) { return -x - 4*y, x + z*z, 1 + x }},
}

// SprottDTs holds the live (user-adjustable) dt for each case; &SprottDTs[i]
// is the stable pointer the param slider binds to. CaseIndex maps a
// mode string to its index for dispatch. Both are VAR initializers (not an
// init() func) on purpose: sprottcases.go's init() takes &SprottDTs[i], and
// init() funcs run in file order (…cases.go before …data.go) while variable
// initialization is dependency-ordered and always precedes every init().
var (
	SprottDTs = func() []float32 {
		d := make([]float32, len(SprottCases))
		for i := range SprottCases {
			d[i] = SprottCases[i].DT
		}
		return d
	}()
	CaseIndex = func() map[string]int {
		m := make(map[string]int, len(SprottCases))
		for i := range SprottCases {
			m[SprottCases[i].Key] = i
		}
		return m
	}()
)

func init() {
	for i := range SprottCases {
		InitCond[SprottCases[i].Key] = SprottCases[i].IC
	}
}

// ── Rössler hyperchaos (4D) ───────────────────────────────────────────────
var (
	HyperDT float32 = 0.001
	HyperA  float32 = 0.25
	HyperB  float32 = 3.0
	HyperC  float32 = 0.5
	HyperD  float32 = 0.05
	HyperW  float32 // hidden fourth state; reset in resetAttractorState
)

// HyperW0 is the on-attractor seed for the hidden state: from w=0 the
// canonical parameters DIVERGE (t≈11 in float64 Euler — verified), and the
// render loop only appeared healthy because the divergence guard kept
// reseeding it. The literature IC for the 1979 system is (-10,-6,0,10).
const HyperW0 float32 = 10

// HyperScale shrinks the stored coordinates so this attractor's large
// natural extent (~120×120×230) fits the camera auto-fit like the others.
// Integration still runs in the true coordinates.
const HyperScale = 0.2

// HyperDeriv is THE hyper-Rössler vector field — the render loop, the flow
// registry (Model Out FLOW, ring beam) and the chaos guard all read it here.
func HyperDeriv(x, y, z, w float64) (float64, float64, float64, float64) {
	a, b, c, d := float64(HyperA), float64(HyperB), float64(HyperC), float64(HyperD)
	return -y - z, x + a*y + w, b + x*z, -c*z + d*w
}

func init() {
	InitCond["hyperrossler"] = [3]float32{-10, -6, 0}
	RegisterFlow4("hyperrossler", FlowSys4{
		Dt:    func() float64 { return float64(HyperDT) },
		F:     HyperDeriv,
		W:     func() float64 { return float64(HyperW) },
		SetW:  func(v float64) { HyperW = float32(v) },
		Scale: HyperScale,
		W0:    float64(HyperW0),
		Euler: true, // the hyper-Rössler render loop steps forward Euler
	})
}
