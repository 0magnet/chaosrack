package attractor

// Data half of the Sprott catalog + Rössler hyperchaos: the equations, dts,
// and initial conditions, untagged so the native chaos guard integrates the
// EXACT systems the app renders (the js half — panel registration and the
// render loops — stays in sprott_cases.go). See that file for citations.

// sprottCase is one member of the Sprott catalog. deriv returns the time
// derivatives at (x,y,z); the coefficients are baked in (Sprott's systems are
// specific, not tunable families) while dt stays user-adjustable.
type sprottCase struct {
	key   string // mode string, e.g. "sprottb"
	name  string // dropdown label, e.g. "Sprott B"
	eq    string // human-readable ODEs for the info overlay
	dt    float32
	ic    [3]float32
	deriv func(x, y, z float64) (dx, dy, dz float64)
}

var sprottCases = []sprottCase{
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

// sprottDTs holds the live (user-adjustable) dt for each case; &sprottDTs[i]
// is the stable pointer the param slider binds to. sprottCaseIndex maps a
// mode string to its index for dispatch.
var (
	sprottDTs       []float32
	sprottCaseIndex = map[string]int{}
)

func init() {
	sprottDTs = make([]float32, len(sprottCases))
	for i := range sprottCases {
		sprottDTs[i] = sprottCases[i].dt
		sprottCaseIndex[sprottCases[i].key] = i
		attractorInitCond[sprottCases[i].key] = sprottCases[i].ic
	}
}

// ── Rössler hyperchaos (4D) ───────────────────────────────────────────────
var (
	hyperDT float32 = 0.001
	hyperA  float32 = 0.25
	hyperB  float32 = 3.0
	hyperC  float32 = 0.5
	hyperD  float32 = 0.05
	hyperW  float32 // hidden fourth state; reset in resetAttractorState
)

// hyperW0 is the on-attractor seed for the hidden state: from w=0 the
// canonical parameters DIVERGE (t≈11 in float64 Euler — verified), and the
// render loop only appeared healthy because the divergence guard kept
// reseeding it. The literature IC for the 1979 system is (-10,-6,0,10).
const hyperW0 float32 = 10

// hyperScale shrinks the stored coordinates so this attractor's large
// natural extent (~120×120×230) fits the camera auto-fit like the others.
// Integration still runs in the true coordinates.
const hyperScale = 0.2

// hyperDeriv is THE hyper-Rössler vector field — the render loop, the flow
// registry (Model Out FLOW, ring beam) and the chaos guard all read it here.
func hyperDeriv(x, y, z, w float64) (float64, float64, float64, float64) {
	a, b, c, d := float64(hyperA), float64(hyperB), float64(hyperC), float64(hyperD)
	return -y - z, x + a*y + w, b + x*z, -c*z + d*w
}

func init() {
	attractorInitCond["hyperrossler"] = [3]float32{-10, -6, 0}
	registerFlow4("hyperrossler", flowSys4{
		dt:    func() float64 { return float64(hyperDT) },
		f:     hyperDeriv,
		w:     func() float64 { return float64(hyperW) },
		setW:  func(v float64) { hyperW = float32(v) },
		scale: hyperScale,
	})
}
