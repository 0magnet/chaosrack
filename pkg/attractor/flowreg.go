package attractor

// Flow-derivative registry: one place that knows each flow mode's vector
// field, so anything that wants to integrate the SAME system the renderer
// draws (Model Out's FLOW sonification, and eventually others) reads it from
// here instead of duplicating equations that could drift.
//
// Sources, in priority order:
//   - flowSystems: the classic bespoke-loop modes register their deriv here
//     (each classic file defines its deriv ONCE and both its render loop and
//     this registry use it).
//   - integrate3D capture: the shared RK4 loop stashes the (dt, deriv) it was
//     last called with, which covers every integrate3D mode (Sprott B–S,
//     Rabinovich, Lü, …) with zero per-mode wiring.
//
// Modes in neither (4D equation modes with w/t state, parametric curves,
// geometry) simply aren't flows here — callers fall back to trail scanning.

type flowDeriv func(x, y, z float64) (float64, float64, float64)

type flowSys struct {
	dt func() float64 // the mode's BASE dt (live: reads the param var)
	f  flowDeriv
}

var flowSystems = map[string]flowSys{}

// classicSys is the native-float32 form of a classic system, used by the ONE
// shared render loop (generateClassic) — no float64 round-trips in the hot
// path, unlike the flowSystems wrapper the audio integrator uses.
type classicSys struct {
	dt *float32
	f  func(x, y, z float32) (float32, float32, float32)
}

var classicSystems = map[string]classicSys{}

// registerFlow adds a classic float32 system to BOTH registries: native form
// for the render loop, float64-wrapped for the audio integrator. dt is
// captured by pointer so param edits are live.
func registerFlow(mode string, dt *float32, f func(x, y, z float32) (float32, float32, float32)) {
	classicSystems[mode] = classicSys{dt: dt, f: f}
	flowSystems[mode] = flowSys{
		dt: func() float64 { return float64(*dt) },
		f: func(x, y, z float64) (float64, float64, float64) {
			a, b, c := f(float32(x), float32(y), float32(z))
			return float64(a), float64(b), float64(c)
		},
	}
}

// integrate3D capture — refreshed every frame the shared RK4 loop runs.
var (
	flowCapMode string
	flowCapDT   float64
	flowCapF    flowDeriv
)

// flowFor returns the vector field + dt for a mode, if it is a known 3D flow.
func flowFor(mode string) (flowSys, bool) {
	if s, ok := flowSystems[mode]; ok {
		return s, true
	}
	if mode != "" && mode == flowCapMode && flowCapF != nil {
		dt := flowCapDT
		return flowSys{dt: func() float64 { return dt }, f: flowCapF}, true
	}
	return flowSys{}, false
}

// attractorInitCond seeds each mode's integrator (modes not listed use the
// generic fallback in resetAttractorState / defaultInitCond). Lives untagged
// with the registry so the native chaos test starts from the SAME state the
// app does.
var attractorInitCond = map[string][3]float32{
	"chua":       {0.1, 0.0, 0.0},
	"rabinovich": {-1.0, 0.0, 0.5},
	"burkeshaw":  {0.6, 0.0, 0.0},
	"chen":       {-3.0, 2.0, 20.0},
	"sprott":     {0.63, 0.47, -0.54},
	"thomas":     {1.0, 0.0, 0.0},
	"halvorsen":  {-1.48, -1.51, 2.04},
}

// defaultInitCond is the fallback for modes without an attractorInitCond row
// (must match resetAttractorState's else branch).
var defaultInitCond = [3]float32{0.1, 0.5, -0.6}

func initCondFor(mode string) [3]float32 {
	if ic, ok := attractorInitCond[mode]; ok {
		return ic
	}
	return defaultInitCond
}

// effSubSteps caps the per-frame integration work so the Speed knob can
// never freeze the page: at long trail lengths the requested sub-steps are
// clamped to keep steps×substeps within a frame budget (visual speed tops
// out instead of blocking the main thread for seconds — the "page
// unresponsive" class found by the demo recorder, and the likely cause of
// the one fuzzer FROZEN hit in hyperrossler). Budgets differ by engine cost:
// compiled vector fields are ~10× cheaper per step than the interpreted
// equation engine.
func effSubSteps(requested, points, budget int) int {
	if requested <= 1 || points <= 0 {
		return requested
	}
	max := budget / points
	if max < 1 {
		max = 1
	}
	if requested > max {
		return max
	}
	return requested
}

const (
	frameBudgetCompiled    = 4_000_000 // classic Euler / integrate3D RK4 steps per frame
	frameBudgetInterpreted = 400_000   // equation-engine AST evaluations per frame (per expression set)
)

func init() {
	registerFlow("lorenz", &lorenzDT, lorenzDeriv)
	registerFlow("rossler", &rosslerDT, rosslerDeriv)
	registerFlow("chua", &chuaDT, chuaDeriv)
	registerFlow("aizawa", &aizawaDT, aizawaDeriv)
	registerFlow("sprott", &sprottDT, sprottDeriv)
	registerFlow("thomas", &thomasDT, thomasDeriv)
	registerFlow("halvorsen", &halvorsenDT, halvorsenDeriv)
	registerFlow("chen", &chenDT, chenDeriv)
	registerFlow("dadras", &dadrasDT, dadrasDeriv)
	registerFlow("burkeshaw", &burkeDT, burkeShawDeriv)
}
