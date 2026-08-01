//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"
)

// The attractor integrator state and its reset / divergence machinery — the
// live heart of the instrument. Camera, GL, DOM-ref and debug state live in
// camera_js.go / glstate_js.go / domrefs_js.go / stats_js.go.

// ── Attractor state ──────────────────────────────────────────────────────────

var (
	x, y, z float32 = 0.1, 0.5, -0.6
	// x64,y64,z64 are the double-precision integrator state used by the shared
	// RK4 loop (integrate3D). Kept across frames so sensitive Sprott systems
	// don't get rounded off their attractor into divergence each frame; seeded
	// from the initial condition in resetAttractorState.
	x64, y64, z64 float64
	steps         int     = 20000
	vertBuf               = make([]float32, 20000*4) // pre-allocated vertex buffer (stride 4: x,y,z,t)
	speedSteps    int     = 1
	speedScale    float32 = 1.0 // dt multiplier for sub-1 speeds
	// centerOffset is computed after warmup frames and then held stable
	centerOffset [3]float32
	centerReady  bool
	centerWarmup int
)

// attractorDrawMode is the GL draw mode (LineStrip or Points) — set after glTypes.New.
var attractorDrawMode js.Value

// Persistent JS typed arrays — allocated once, reused every frame to avoid GC pressure.
var (
	jsVertUint8 js.Value // Uint8Array for CopyBytesToJS
	jsVertFloat js.Value // Float32Array view for bufferData
)

func resetAttractorState() {
	reseedAttractorState()
	// Hyper-Rössler gets a warmup onto its attractor for EXPLICIT resets
	// (mode entry, reset buttons) — but NOT from checkDiverged: with
	// divergent parameters the warmup itself diverges, and re-running its
	// 60k steps on every diverging sub-step froze the page for minutes
	// (found by the demo recorder, bisected to hyperrossler param edits).
	if selectedMode == "hyperrossler" {
		hyperRosslerWarmup()
		hyperPrimed = true
	}
}

// reseedAttractorState restores the integrator state to the mode's initial
// condition WITHOUT any warmup — safe to call from the per-step divergence
// guard, where the current parameters may make every trajectory blow up.
func reseedAttractorState() {
	if ic, ok := attractorInitCond[selectedMode]; ok {
		x, y, z = ic[0], ic[1], ic[2]
	} else {
		x, y, z = 0.1, 0.5, -0.6
	}
	x64, y64, z64 = float64(x), float64(y), float64(z)
	integ3DMode = "" // force integrate3D to re-seed x64 from the IC
	ringInvalidate() // ring trail re-primes from the fresh state
	twinInvalidate() // twin pair re-seeds ε apart from the fresh state
	sectInvalidate() // section scatter restarts from the fresh state
	bifInvalidate()  // bifurcation re-sweeps (source params may have changed)
	// Hyper-Rössler's hidden 4th state; start it on-attractor for that mode,
	// zero otherwise (harmless — only that mode reads it).
	if selectedMode == "hyperrossler" {
		hyperW = hyperW0
	} else {
		hyperW = 0
	}
	customW = 0
	customT = 0
	centerReady = false
	centerWarmup = 0
}

// checkDiverged returns true and resets state if the attractor has diverged (NaN or >1e6).
// checkDiverged resets the integrator state if it has blown up. The bound
// is well above every attractor's normal extent (tens of units) but low
// enough to catch an off-screen blow-up promptly — so an over-modulated
// attractor recovers on its own once the offending depth is dialed back,
// rather than drifting invisibly at huge coordinates.
func checkDiverged() {
	const lim = 1e4
	if x != x || y != y || z != z || x > lim || x < -lim || y > lim || y < -lim || z > lim || z < -lim {
		reseedAttractorState()
	}
}

// applySpeedLog converts a log10 slider value into speedSteps and speedScale.
// Slider range -2..2 maps to effective speed 0.01..100.
// Values >= 1: sub-step (speedSteps=N, speedScale=1.0).
// Values < 1: scale dt down (speedSteps=1, speedScale=fraction).
func applySpeedLog(logVal float64) {
	speed := math.Pow(10, logVal)
	if speed >= 1.0 {
		speedSteps = int(speed + 0.5)
		speedScale = 1.0
	} else {
		speedSteps = 1
		speedScale = float32(speed)
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
