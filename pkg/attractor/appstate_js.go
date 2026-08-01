//go:build js && wasm

package attractor

import (
	_ "embed"
	"math"
	"strconv"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// ── Debug stats ─────────────────────────────────────────────────────────────

var (
	debugEnabled   bool
	frameCount     int
	frameTotalMs   float32
	frameMinMs     float32 = 999
	frameMaxMs     float32
	lastFrameStart float32
)

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

// ── Camera / view state ──────────────────────────────────────────────────────

var (
	initCameraDist                     float32 = 100
	defaultCameraDist                  float32 = 100
	rotationX, rotationY, rotationZ    float32
	rotationX1, rotationY1, rotationZ1 float32
	movMatrix                          mgl32.Mat4
	tmark                              float32

	// Absolute orientation angles (radians) — the source of truth for the
	// model's pose. The "digital-potentiometer" rotation knobs set these
	// directly (position); the X/Y/Z rate sliders and auto-rotate advance
	// them over time. movMatrix is rebuilt from them every frame (see
	// rebuildModelMatrix), so a held angle stays put and a spinning one is
	// just the angle marching. Mirrors Glen's 3D projective unit, whose
	// front-panel pots set absolute X/Y angles shown on 7-seg displays.
	angleX, angleY, angleZ float32
)

// ── Color state ──────────────────────────────────────────────────────────────

var (
	baseColor = [3]float32{1.0, 0.0, 0.0}
	topColor  = [3]float32{0.0, 0.0, 1.0}
	midColor  = [3]float32{0.0, 1.0, 0.0}
	bgColor   = [3]float32{0.0, 0.0, 0.0}
)

// ── Interaction state ────────────────────────────────────────────────────────

// rebindParamWheel re-wires wheel-on-input listeners to every range/
// number input inside the params div. Set in Run(); called from
// buildParamPanel after it rebuilds the panel's children.
var rebindParamWheel func()

// Cached slider values. Read once at startup, then refreshed from
// the DOM only on the slider's input event (which fires on user
// interaction OR our synthetic dispatch from wheel-on-input). The
// renderLoop reads these instead of calling parseFloat per frame —
// saves ~16 JS roundtrips/frame across the 4 fixed sliders.
var (
	cachedZoom float32
	cachedPanX float32
	cachedPanY float32
	cachedRotX float32
	cachedRotY float32
	cachedRotZ float32
)

// staticGeomDirty is set true when a non-attractor mode's geometry
// needs re-upload to the GPU (mode change or param change). Cleared
// inside uploadBuffersIndexed after the first upload. Per-frame
// calls then skip the SliceToTypedArray + bufferData work and go
// straight to drawElements with the still-bound buffers.
var staticGeomDirty = true

// ExtraNavHTML lets the host page inject a small HTML snippet into
// the controls panel (typically a link to a fullscreen-only variant
// of the page). Set BEFORE calling Run(). Empty string = no slot
// rendered. The snippet is inserted as innerHTML into a span that
// sits next to the model dropdown — keep it short.
var ExtraNavHTML string

// PanelStartHidden makes the controls panel start with display:none so
// only the small ▤ toggle button (bottom-left) is visible; clicking it
// reveals the panel. Set BEFORE calling Run(). Intended for host pages
// that want the visualizer to render unobstructed by default (e.g.
// magnetosphere.net's front page where a logo sits over the canvas).
//
// The ?panel= URL query parameter overrides this variable at runtime:
//
//	?panel=hidden           → start hidden (even if var is false)
//	?panel=shown | visible  → start shown  (even if var is true)
//
// so a link like /?panel=shown can invite users to open the controls
// without the host having to switch modes.
var PanelStartHidden bool

// ForceStandalonePanel makes Run() build the controls as a fixed
// standalone overlay with the full dock/resize/float chrome even when
// the host page has a <footer>. Without this, a detected <footer>
// causes the panel to append inline (subtle-styled, sits inside the
// footer) with dock/resize/float wiring skipped — a design meant for
// early days when the panel was just a strip of controls, now a strict
// downgrade.
//
// Set true for host pages that want the full standalone UX while
// keeping their footer for other content (e.g. magnetosphere.net keeps
// its cart + shipping in the footer but wants a proper draggable panel
// too). Set BEFORE calling Run().
var ForceStandalonePanel bool

var (
	paused          bool    = false
	stopped         bool    = false
	pausedCount     int     = 0
	autoRotate      bool    = true
	usePoints       bool    = false
	persistTrail    bool    = false
	gradientSource  int     = 2 // gradient parameter source: 0=X,1=Y,2=Z,3=trail
	gradientColors  int     = 2 // palette: 1=mono,2=two-color,3=three-color,4=rainbow
	gradientReverse bool    = false
	gradientFreq    float32 = 1 // rainbow gradient cycles over the range (period control)
	gradientPhase   float32 = 0 // animated rainbow hue offset (flows through the spectrum)
	// trailModFrac is the fraction of the trail drawn this frame (1 = full).
	// Audio modulation shortens it live without touching the vertex buffer:
	// uploadVerticesOnly just draws the most-recent frac·count points.
	trailModFrac float32 = 1
	dragging     bool    = false
	dragLastX    float32
	dragLastY    float32
)

// ── Selection ────────────────────────────────────────────────────────────────

var selectedMode string

// preCustomMode remembers the attractor to return to when the "Edit eqn" switch
// is toggled back off.
var preCustomMode string

// ── WebGL state ──────────────────────────────────────────────────────────────

// These are package-level for convenience (lots of helper functions
// across the package reach for them), but they MUST NOT be initialized
// at package-var time. When attractor is imported as a library (e.g.
// from m2/wasm/stl2), package-var init runs before the host's DOM is
// ready, so getElementById("gocanvas") returns null and the subsequent
// canvasEl.Call("getContext", "webgl") panics with
// "syscall/js: call of Value.Call on null". initWebGL(), called from
// Run() once the canvas exists, populates them.
var (
	doc      js.Value
	body     js.Value
	canvasEl js.Value
	width    int
	height   int
	gl       js.Value

	shaderProgram         js.Value
	attractorVertexBuffer js.Value
	attractorIndexBuffer  js.Value
	attractorVertices     []float32
	attractorIndices      []uint16

	// gradientStride is the number of floats per vertex in
	// attractorVertices, set by whichever upload path last ran:
	// 4 for interleaved attractor data (x,y,z,t) via
	// uploadVerticesOnly, 3 for packed xyz indexed geometry via
	// uploadBuffersIndexed. updateGradientRange reads it so it scans
	// the right stride instead of assuming 4 (which would misread the
	// min/max range for polyhedra and other indexed modes).
	gradientStride = 4

	glTypes GLTypes
)

// sizeCanvasToViewport sizes the canvas BACKING STORE to CSS-viewport ×
// devicePixelRatio (capped at 3) while pinning the element's CSS size to CSS
// pixels — so HiDPI displays render the scope traces at native resolution
// instead of a soft 1× upscale. Reports whether a valid size was applied.
// width/height globals are backing-store pixels (aspect and NDC math are
// ratio-based, so both stay correct).
func sizeCanvasToViewport() bool {
	cssW := doc.Get("body").Get("clientWidth").Int()
	cssH := doc.Get("body").Get("clientHeight").Int()
	if cssW <= 0 || cssH <= 0 {
		return false
	}
	d := js.Global().Get("devicePixelRatio").Float()
	if d < 1 {
		d = 1
	} else if d > 3 {
		d = 3 // beyond 3× the fill cost outruns any visible sharpness gain
	}
	width = int(float64(cssW) * d)
	height = int(float64(cssH) * d)
	canvasEl.Set("width", width)
	canvasEl.Set("height", height)
	st := canvasEl.Get("style")
	st.Set("width", strconv.Itoa(cssW)+"px")
	st.Set("height", strconv.Itoa(cssH)+"px")
	return true
}

func initWebGL() {
	doc = js.Global().Get("document")
	body = doc.Get("body")
	canvasEl = doc.Call("getElementById", "gocanvas")
	if canvasEl.IsUndefined() || canvasEl.IsNull() {
		return
	}
	opts := js.Global().Get("Object").New()
	opts.Set("preserveDrawingBuffer", true)
	gl = canvasEl.Call("getContext", "webgl", opts)
	sizeCanvasToViewport()
	if gl.IsUndefined() {
		gl = canvasEl.Call("getContext", "experimental-webgl", opts)
	}
	if gl.IsUndefined() {
		js.Global().Call("alert", "browser might not support webgl")
		return
	}
	shaderProgram = gl.Call("createProgram")
	attractorVertexBuffer = gl.Call("createBuffer")
	attractorIndexBuffer = gl.Call("createBuffer")
}

// ── DOM element refs ─────────────────────────────────────────────────────────

var (
	rtc                 js.Value
	cameraControl       js.Value
	rotationControlsX   js.Value
	rotationControlsY   js.Value
	rotationControlsZ   js.Value
	sliderZoom          js.Value
	sliderX             js.Value
	sliderY             js.Value
	sliderZ             js.Value
	uBaseColorLoc       js.Value
	uTopColorLoc        js.Value
	uMidColorLoc        js.Value
	uMinZLoc            js.Value
	uMaxZLoc            js.Value
	uMinXLoc            js.Value
	uMaxXLoc            js.Value
	uMinYLoc            js.Value
	uMaxYLoc            js.Value
	uGradientSourceLoc  js.Value
	uGradientColorsLoc  js.Value
	uGradientFreqLoc    js.Value
	uGradientPhaseLoc   js.Value
	uGradientReverseLoc js.Value
	uPointSizeLoc       js.Value
	uMmatrixLoc         js.Value
	uVmatrixLoc         js.Value
	uTrailHeadLoc       js.Value
	positionLoc         js.Value
	aTrailTLoc          js.Value
	aDwellLoc           js.Value
	shadersReady        bool
	renderFrame         js.Func
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
