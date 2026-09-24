//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"math"
)

// The attractor integrator state and its reset / divergence machinery — the
// live heart of the instrument. Camera, GL, DOM-ref and debug state live in
// camera_js.go / glstate_js.go / domrefs_js.go / stats_js.go.

// ── Attractor state ──────────────────────────────────────────────────────────

// simulation is the running model: the integrator's state in single and
// double precision, the trail it writes, how fast it advances, and the offset
// that centers it.
type simulation struct {
	x, y, z float32

	// x64,y64,z64 are the double-precision integrator state used by the shared
	// RK4 loop (integrate3D). Kept across frames so sensitive Sprott systems
	// don't get rounded off their attractor into divergence each frame; seeded
	// from the initial condition in resetAttractorState.
	x64, y64, z64 float64
	steps         int
	vertBuf       []float32 // pre-allocated vertex buffer (stride 4: x,y,z,t)
	speedSteps    int
	speedScale    float32 // dt multiplier for sub-1 speeds

	// centerOffset is computed after warmup frames and then held stable
	centerOffset [3]float32
	centerReady  bool
	centerWarmup int
}

var sim = simulation{
	x:          0.1,
	y:          0.5,
	z:          -0.6,
	steps:      20000,
	vertBuf:    make([]float32, 20000*4),
	speedSteps: 1,
	speedScale: 1.0,
}

func resetAttractorState() {
	sim.reseed()
	// Hyper-Rössler gets a warmup onto its attractor for EXPLICIT resets
	// (mode entry, reset buttons) — but NOT from checkDiverged: with
	// divergent parameters the warmup itself diverges, and re-running its
	// 60k steps on every diverging sub-step froze the page for minutes
	// (found by the demo recorder, bisected to hyperrossler param edits).
	if run.selectedMode == "hyperrossler" {
		hyperRosslerWarmup()
		hyperPrimed = true
	}
}

// reseed restores the integrator state to the mode's initial
// condition WITHOUT any warmup — safe to call from the per-step divergence
// guard, where the current parameters may make every trajectory blow up.
func (s *simulation) reseed() {
	if ic, ok := dynamics.InitCond[run.selectedMode]; ok {
		s.x, s.y, s.z = ic[0], ic[1], ic[2]
	} else {
		s.x, s.y, s.z = 0.1, 0.5, -0.6
	}
	s.x64, s.y64, s.z64 = float64(s.x), float64(s.y), float64(s.z)
	integ3DMode = ""  // force integrate3D to re-seed x64 from the IC
	ring.invalidate() // ring trail re-primes from the fresh state
	twin.invalidate() // twin pair re-seeds ε apart from the fresh state
	sect.invalidate() // section scatter restarts from the fresh state
	bif.invalidate()  // bifurcation re-sweeps (source params may have changed)
	mapInvalidate()   // a map orbit is only meaningful for the params that made it
	lyap.invalidate() // and so is its Lyapunov exponent
	// The live exponent restarts for the same reason, and it needs saying
	// separately: lyap.invalidate re-runs the Analysis module's on-demand
	// measurement, which is a different accumulation with a different clock.
	lyapLive.invalidate()
	// Hyper-Rössler's hidden 4th state; start it on-attractor for that mode,
	// zero otherwise (harmless — only that mode reads it).
	if run.selectedMode == "hyperrossler" {
		dynamics.HyperW = dynamics.HyperW0
	} else {
		dynamics.HyperW = 0
	}
	custom.w = 0
	custom.t = 0
	s.centerReady = false
	s.centerWarmup = 0
}

// checkDiverged returns true and resets state if the attractor has diverged (NaN or >1e6).
// checkDiverged resets the integrator state if it has blown up. The bound
// is well above every attractor's normal extent (tens of units) but low
// enough to catch an off-screen blow-up promptly — so an over-modulated
// attractor recovers on its own once the offending depth is dialed back,
// rather than drifting invisibly at huge coordinates.
func (s *simulation) checkDiverged() {
	const lim = 1e4
	if s.x != s.x || s.y != s.y || s.z != s.z || s.x > lim || s.x < -lim || s.y > lim || s.y < -lim || s.z > lim || s.z < -lim {
		s.reseed()
	}
}

// applySpeedLog converts a log10 slider value into speedSteps and speedScale.
// Slider range -2..2 maps to effective speed 0.01..100.
// Values >= 1: sub-step (speedSteps=N, speedScale=1.0).
// Values < 1: scale dt down (speedSteps=1, speedScale=fraction).
func (s *simulation) applySpeedLog(logVal float64) {
	speed := math.Pow(10, logVal)
	if speed >= 1.0 {
		s.speedSteps = int(speed + 0.5)
		s.speedScale = 1.0
	} else {
		s.speedSteps = 1
		s.speedScale = float32(speed)
	}
	// (The Speed LED is owned by its ControlDesc — speedDisplayVal is the
	// shared slider→display mapping, so LED and engine can't disagree.)
}

// speedDisplayVal maps the log slider's raw value to the effective speed
// multiplier the LED shows: whole sub-step counts at ≥1 (matching what the
// engine actually runs), the dt fraction below 1.
func speedDisplayVal(logVal float64) float64 {
	speed := math.Pow(10, logVal)
	if speed >= 1.0 {
		return float64(int(speed + 0.5))
	}
	return speed
}
