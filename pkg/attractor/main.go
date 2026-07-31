//go:build js && wasm

package attractor

import (
	_ "embed"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"syscall/js"
	"time"

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

// ── Parameter definitions with slider ranges ─────────────────────────────────

type paramDef struct {
	ID    string
	Label string
	Value *float32
	Def   float32
	Min   float32
	Max   float32
	Step  float32
}

var attractorParams = map[string][]paramDef{
	"lorenz": {
		{"lorenz-dt", "dt", &lorenzDT, 0.005, 0.001, 0.05, 0.001},
		{"lorenz-s", "σ", &lorenzS, 10.0, 1, 30, 0.1},
		{"lorenz-r", "ρ", &lorenzR, 28.0, 1, 60, 0.1},
		{"lorenz-b", "β", &lorenzB, 2.7, 0.1, 10, 0.1},
	},
	"rossler": {
		{"rossler-dt", "dt", &rosslerDT, 0.005, 0.001, 0.05, 0.001},
		{"rossler-a", "a", &rosslerA, 0.2, 0.01, 1, 0.01},
		{"rossler-b", "b", &rosslerB, 0.2, 0.01, 1, 0.01},
		{"rossler-c", "c", &rosslerC, 5.7, 1, 20, 0.1},
	},
	"chua": {
		{"chua-dt", "dt", &chuaDT, 0.005, 0.001, 0.05, 0.001},
		{"chua-alpha", "α", &chuaAlpha, 15.6, 5, 30, 0.1},
		{"chua-beta", "β", &chuaBeta, 28.0, 10, 50, 0.1},
		{"chua-m0", "m0", &chuaM0, -1.143, -2, 0, 0.001},
		{"chua-m1", "m1", &chuaM1, -0.714, -2, 0, 0.001},
	},
	"aizawa": {
		{"aizawa-dt", "dt", &aizawaDT, 0.0052, 0.001, 0.02, 0.0001},
		{"aizawa-a", "a", &aizawaA, 0.95, 0.1, 2, 0.01},
		{"aizawa-b", "b", &aizawaB, 0.7, 0.1, 2, 0.01},
		{"aizawa-c", "c", &aizawaC, 0.6, 0.1, 2, 0.01},
		{"aizawa-d", "d", &aizawaD, 3.5, 0.1, 8, 0.01},
		{"aizawa-e", "e", &aizawaE, 0.25, 0.01, 1, 0.01},
		{"aizawa-f", "f", &aizawaF, 0.1, 0.01, 1, 0.01},
	},
	"sprott": {
		{"sprott-dt", "dt", &sprottDT, 0.005, 0.001, 0.05, 0.001},
		{"sprott-a", "a", &sprottA, 1.6, 0.1, 5, 0.01},
		{"sprott-b", "b", &sprottB, 1.85, 0.1, 5, 0.01},
	},
	"lissajou": {
		{"lissajou-a", "a", &lissajouA, 3, 1, 20, 1},
		{"lissajou-b", "b", &lissajouB, 2, 1, 20, 1},
		{"lissajou-c", "c", &lissajouC, 5, 1, 20, 1},
	},
	// Graphic Artist: LEVEL A/B/D + HARMONIC B/C/D (integer). Waveform A/B/C/D
	// tri/square are separate switches (buildGraphicArtistControls).
	// Short labels (Lv/Hm + oscillator letter) so they fit to the left of the
	// centered LED without overlapping it.
	"graphicartist": {
		{"ga-la", "LvA", &gaLevelA, 0.55, 0, 1, 0.05},
		{"ga-lb", "LvB", &gaLevelB, 0.45, 0, 1, 0.05},
		{"ga-ld", "LvD", &gaLevelD, 0.55, 0, 1, 0.05},
		{"ga-hb", "HmB", &gaHarmB, 2, 1, 16, 1},
		{"ga-hc", "HmC", &gaHarmC, 12, 1, 32, 1},
		{"ga-hd", "HmD", &gaHarmD, 1, 1, 16, 1},
	},
	"thomas": {
		{"thomas-dt", "dt", &thomasDT, 0.05, 0.001, 0.1, 0.001},
		{"thomas-b", "b", &thomasB, 0.19, 0.01, 1.0, 0.001},
	},
	"halvorsen": {
		{"halvorsen-dt", "dt", &halvorsenDT, 0.003, 0.001, 0.05, 0.001},
		{"halvorsen-a", "a", &halvorsenA, 1.4, 0.1, 5, 0.01},
	},
	"chen": {
		{"chen-dt", "dt", &chenDT, 0.0005, 0.0001, 0.005, 0.0001},
		{"chen-a", "a", &chenA, 35.0, 10, 50, 0.1},
		{"chen-b", "b", &chenB, 3.0, 0.1, 10, 0.1},
		{"chen-c", "c", &chenC, 28.0, 10, 40, 0.1},
	},
	"dadras": {
		{"dadras-dt", "dt", &dadrasDT, 0.005, 0.001, 0.05, 0.001},
		{"dadras-p", "p", &dadrasP, 3.0, 0.1, 10, 0.1},
		{"dadras-q", "q", &dadrasQ, 2.7, 0.1, 10, 0.1},
		{"dadras-r", "r", &dadrasR, 1.7, 0.1, 10, 0.1},
		{"dadras-s", "s", &dadrasS, 2.0, 0.1, 10, 0.1},
		{"dadras-e", "e", &dadrasE, 9.0, 0.1, 20, 0.1},
	},
	"rabinovich": {
		{"rab-dt", "dt", &rabDT, 0.001, 0.0001, 0.01, 0.0001},
		{"rab-alpha", "α", &rabAlpha, 1.1, 0.01, 2, 0.01},
		{"rab-gamma", "γ", &rabGamma, 0.87, 0.01, 1, 0.01},
	},
	"burkeshaw": {
		{"burke-dt", "dt", &burkeDT, 0.005, 0.001, 0.05, 0.001},
		{"burke-s", "S", &burkeS, 10.0, 1, 20, 0.1},
		{"burke-v", "V", &burkeV, 4.272, 1, 10, 0.001},
	},
	"globe": {
		{"globe-lat", "lat", &globeLatF, 18, 4, 90, 1},
		{"globe-lon", "lon", &globeLonF, 36, 4, 180, 1},
	},
	"sphere": {
		{"sphere-r", "radius", &sphereRadius, 1.0, 0.1, 5, 0.1},
		{"sphere-stacks", "lat", &sphereStacksF, 30, 4, 100, 1},
		{"sphere-slices", "lon", &sphereSlicesF, 30, 4, 100, 1},
	},
	"torus": {
		{"torus-R", "R", &torusR, 1.5, 0.1, 5, 0.1},
		{"torus-r", "r", &torusr, 0.5, 0.1, 3, 0.1},
		{"torus-stacks", "stacks", &torusStacksF, 30, 4, 100, 1},
		{"torus-slices", "slices", &torusSlicesF, 30, 4, 100, 1},
	},
}

// ── Controls panel HTML ──────────────────────────────────────────────────────

//go:embed panel.css
var panelCSS string

// controlsBody is the panel markup; the stylesheet lives in panel.css
// (embedded above) and is prepended as a <style> block at inject time.
const controlsBody = `
<div class="modules">
<div class="sect console"><div class="sect-hdr">Console</div>
<div class="row toprow">
  <span class="grp modelsel-col" data-no-drag id="model-sel-grp" title="Model selector — outer knob picks the category, inner knob the model">
  <span class="u-lbl mlbl">Model</span>
  <span id="modelknob-holder"></span>
  <select id="mode-select" class="selwin" style="display:none"></select>
  <select id="cat-select" class="selwin csel-cat" data-no-drag title="Model category"></select>
  <select id="model-select" class="selwin csel-model" data-no-drag title="Model within the selected category"></select>
  </span>
  <span class="grp" id="dock-controls" title="Dock the controls to an edge of the window, or detach as a floating panel"><span class="dock-lbl">DOCK</span><button class="ctrl-btn dockb" id="dock-top" title="Dock to top">↑</button><button class="ctrl-btn dockb" id="dock-bottom" title="Dock to bottom">↓</button><button class="ctrl-btn dockb" id="dock-left" title="Dock to left (sidebar)">←</button><button class="ctrl-btn dockb" id="dock-right" title="Dock to right (sidebar)">→</button><button class="ctrl-btn dockb" id="dock-float" title="Detach as a floating panel">⧉</button></span>
</div>
<div class="row console-btns">
  <span class="grp btn-row"><button class="pushbtn" id="reset-all-btn" title="Reset parameters, colors, view pose, trail, gradient, effect switches (Front/Fill/audio/backdrops), display style, and the signal generator to their defaults (keeps your dock layout + interface size)"></button><span class="btn-lbl">Reset All</span></span>
  <span class="grp btn-row"><button class="pushbtn" id="normalize-btn" title="Re-center the model to the default (identity) orientation and stop any spin"></button><span class="btn-lbl">Normalize</span></span>
  <span class="grp btn-row"><button class="pushbtn" id="screenshot-btn" title="Save a screenshot"></button><span class="btn-lbl">Screenshot</span></span>
</div>
</div>
<div class="sect"><div class="sect-hdr">Switches</div>
<div class="row toprow swrow swsecs">
  <div class="swsec"><div class="swsec-hdr">Setup</div>
    <label class="grp" style="cursor:pointer;" title="Load the current attractor's equations into the editable Custom mode; toggle off to return to it"><input type="checkbox" class="sw" id="edit-eq-sw"> Edit eqn</label>
    <span id="extra-nav"></span>
  </div>
  <div class="swsec"><div class="swsec-hdr">Motion</div>
    <label class="grp" style="cursor:pointer;" title="Continuously spin the model about the vertical (Y) axis — folds into the Y spin-rate knob"><input type="checkbox" class="sw" id="auto-rotate" checked> Auto-rotate</label>
    <label class="grp" style="cursor:pointer;" title="Pause / freeze the animation"><input type="checkbox" class="sw" id="pause-sw"> Pause</label>
  </div>
  <div class="swsec"><div class="swsec-hdr">Trace</div>
    <label class="grp" style="cursor:pointer;" title="Draw the trajectory as discrete points instead of a connected line"><input type="checkbox" class="sw" id="use-points"> Points</label>
    <label class="grp" style="cursor:pointer;" title="Draw the model in front of the controls (controls stay clickable through it)"><input type="checkbox" class="sw" id="model-front"> Front</label>
    <label class="grp" style="cursor:pointer;" title="Fill the screen with the spectrogram / FVF display (face-on) instead of the rotatable plane"><input type="checkbox" class="sw" id="spect-fill"> Fill</label>
    <label class="grp" style="cursor:pointer;" title="Keep the entire trail on screen (never clear old points) — accumulates the full attractor"><input type="checkbox" class="sw" id="persist-trail"> Persist</label>
    <label class="grp" style="cursor:pointer;" title="Ring trail — scope-style beam: only the advancing head integrates each frame (the trail is its history), so knob/audio changes bend the path from the head forward instead of reshaping the whole curve, and long trails cost almost nothing. Off = classic scan (whole curve recomputed and reshaped every frame)."><input type="checkbox" class="sw" id="ring-sw"> Ring</label>
    <label class="grp" style="cursor:pointer;" title="Reverse the color-gradient direction (swap start and end)"><input type="checkbox" class="sw" id="gradient-reverse"> Invert</label>
  </div>
  <div class="swsec"><div class="swsec-hdr">Audio</div>
    <label class="grp" style="cursor:pointer;" title="Enable audio-reactive modulation — reveals the MOD + EQ modules for routing audio features to each control"><input type="checkbox" class="sw" id="audio-mod"> Audio mod</label>
    <label class="grp" style="cursor:pointer;" title="Play a sweeping test tone (captured back via the server) to exercise the audio modulation"><input type="checkbox" class="sw" id="test-tone"> Test tone</label>
    <label class="grp" style="cursor:pointer;" title="Show / hide the top-left audio feature meters (amp / bass / mid / treble / cntr / beat) while Audio mod is on"><input type="checkbox" class="sw" id="show-meters" checked> Meters</label>
    <label class="grp" style="cursor:pointer;" title="Signal generator — a built-in client-side audio source (three X/Y/Z oscillators). Drives audio modulation, the spectrogram and the xy scope with no server or microphone, and can play over the speakers. Controls are in the Generator module."><input type="checkbox" class="sw" id="fg-on"> Signal gen</label>
    <label class="grp" style="cursor:pointer;" title="Paint the live audio spectrogram onto the current surface model (sphere / globe / torus)"><input type="checkbox" class="sw" id="spectro-skin"> Spectro-skin</label>
    <label class="grp" style="cursor:pointer;" title="Backdrop: scrolling spectrogram behind the model"><input type="checkbox" class="sw" id="bg-spectro"> Spectro bg</label>
    <label class="grp" style="cursor:pointer;" title="Backdrop: XY oscilloscope behind the model"><input type="checkbox" class="sw" id="bg-xy"> XY bg</label>
    <select id="bg-visual" style="display:none"><option value="">off</option><option value="spectrogram">spectrogram</option><option value="xy">xy scope</option></select>
  </div>
  <div class="swsec"><div class="swsec-hdr">Window</div>
    <label class="grp" style="cursor:pointer;" title="Overlay a short description of the current attractor / model on the view"><input type="checkbox" class="sw" id="show-info"> Info</label>
    <label class="grp" style="cursor:pointer;" title="Fullscreen"><input type="checkbox" class="sw" id="fullscreen-sw"> Fullscreen</label>
    <label class="grp" style="cursor:pointer;" title="Show the Template module — a labeled legend of every named slot a module can have (header, label, readout, knob, ring, inner, fine, dial, reset). Hover a slot for the Go struct field it maps to."><input type="checkbox" class="sw" id="tpl-on"> Template</label>
  </div>
</div>
</div>
<div class="sect"><div class="sect-hdr">params</div><div id="params" class="row"></div></div>
<div class="sect"><div class="sect-hdr">Colors</div>
<div class="row vmrow">
  <span class="pcell axcol" id="src-cell" title="Outer ring = gradient source (which axis the color follows); inner ring = number of colors. Overridden while a Phosphor is selected."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">src</span><span id="gradient-stack"></span></span></span><select id="gradient-source" title="Gradient source — which axis (X / Y / Z / trail) the trace color follows" style="display:none">
    <option value="0">X</option>
    <option value="1">Y</option>
    <option value="2" selected>Z</option>
    <option value="3">trail</option>
  </select><select id="gradient-colors" title="Palette — number of gradient colors (mono / 2-color / 3-color / rainbow)" style="display:none">
    <option value="1">mono</option>
    <option value="2" selected>2-color</option>
    <option value="3">3-color</option>
    <option value="4">rainbow</option>
  </select></span>
  <span class="pcell axcol vmcell" id="grp-rainbow" title="Rainbow period — how much of the spectrum spans the trail at once (low = a narrow flowing slice)"><span class="punit-top"><span class="plabel">period</span><input type="number" id="slider-value-rfreq" class="numin" min="0.05" max="20" step="0.05" value="1"></span><input type="range" id="rainbow-freq" min="0.05" max="20" value="1" step="0.05" title="Rainbow period — spectrum span across the trail"><button class="rst" id="rst-rfreq" title="Reset rainbow period">↺</button></span>
  <span class="pcell axcol vmcell" id="trail-controls" title="Trail length — how many recent points stay lit"><span class="punit-top"><span class="plabel">Trail</span><input type="number" id="slider-value-trail" class="numin" min="1000" max="500000" step="1000" value="20000"></span><input type="range" id="trail-slider" min="1000" max="500000" value="20000" step="1000" title="Trail length — how many recent points stay lit"><button class="rst" id="rst-trail" title="Reset trail length">↺</button></span>
</div></div>
<div class="sect"><div class="sect-hdr">Palette</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell pal-cell" id="grp-cstart" title="Gradient START color (low end of the source axis) — pick with the swatch or dial it with the Hue (outer) / Level (inner) knob"><span class="punit-top"><span class="plabel" id="lbl-cstart">start</span><span class="cswatch"><input type="color" id="color-base" value="#ff0000" title="Gradient start color"><button class="rst" id="rst-color-base" title="Reset start color">↺</button></span></span><span class="ckholder" id="ck-color-base"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cmid" title="Gradient MIDDLE color (3-color palette only) — swatch or Hue / Level knob"><span class="punit-top"><span class="plabel">mid</span><span class="cswatch"><input type="color" id="color-mid" value="#00ff00" title="Gradient middle color"><button class="rst" id="rst-color-mid" title="Reset middle color">↺</button></span></span><span class="ckholder" id="ck-color-mid"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cend" title="Gradient END color (high end of the source axis) — swatch or Hue / Level knob"><span class="punit-top"><span class="plabel">end</span><span class="cswatch"><input type="color" id="color-top" value="#0000ff" title="Gradient end color"><button class="rst" id="rst-color-top" title="Reset end color">↺</button></span></span><span class="ckholder" id="ck-color-top"></span></span>
  <span class="pcell axcol vmcell pal-cell" id="grp-cbg" title="Background color behind the model — swatch or Hue / Level knob"><span class="punit-top"><span class="plabel">bg</span><span class="cswatch"><input type="color" id="color-bg" value="#000000" title="Background color"><button class="rst" id="rst-color-bg" title="Reset background color">↺</button></span></span><span class="ckholder" id="ck-color-bg"></span></span>
</div></div>
<div class="sect"><div class="sect-hdr">View</div>
<div class="row vmrow vmaxrow">
  <span class="pcell axcol axrot" data-no-drag title="X axis — angle knob, spin rate, horizontal position">
    <span class="plabel">X</span>
    <span class="grp"><div class="knob" id="knob-x" title="X rotation angle — drag or turn to tilt about X"><i class="knob-ptr" id="knobptr-x"></i></div><span class="led" id="led-x" title="X rotation angle in degrees — drag the model or turn the outer ring to change">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-x" min="-1" max="1" value="0" step="0.1" title="X spin rate — continuous rotation about X"><input type="number" id="slider-value-x" class="numin" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-rx" title="Reset X angle + spin rate">↺</button></span>
  </span>
  <span class="pcell axcol axrot" data-no-drag title="Y axis — angle knob, spin rate, vertical position">
    <span class="plabel">Y</span>
    <span class="grp"><div class="knob" id="knob-y" title="Y rotation angle — drag or turn to tilt about Y"><i class="knob-ptr" id="knobptr-y"></i></div><span class="led" id="led-y" title="Y rotation angle in degrees — drag the model or turn the outer ring to change">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-y" min="-1" max="1" value="0" step="0.1" title="Y spin rate — continuous rotation about Y"><input type="number" id="slider-value-y" class="numin" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-ry" title="Reset Y angle + spin rate">↺</button></span>
  </span>
  <span class="pcell axcol axrot" data-no-drag title="Z axis — angle knob, spin rate, zoom (depth)">
    <span class="plabel">Z</span>
    <span class="grp"><div class="knob" id="knob-z" title="Z rotation angle — drag near the rim to roll about Z"><i class="knob-ptr" id="knobptr-z"></i></div><span class="led" id="led-z" title="Z rotation angle in degrees — drag near the rim or turn the outer ring to change">000°</span></span>
    <span class="grp axsub"><span class="axlbl">rate</span><input type="range" id="rotation-controls-z" min="-1" max="1" value="0" step="0.1" title="Z spin rate — continuous rotation about Z"><input type="number" id="slider-value-z" class="numin" min="-1" max="1" step="0.1" value="0"><button class="rst" id="rst-rz" title="Reset Z angle + spin rate">↺</button></span>
  </span>
</div>
</div>
<div class="sect"><div class="sect-hdr">Position</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">X</span><input type="number" id="slider-value-panx" class="numin" min="-8" max="8" step="1" value="0"></span><input type="range" id="pan-x" min="-8" max="8" value="0" step="1" title="X position — slide the model horizontally"><button class="rst" id="rst-panx" title="Reset X position">↺</button></span>
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Y</span><input type="number" id="slider-value-pany" class="numin" min="-8" max="8" step="1" value="0"></span><input type="range" id="pan-y" min="-8" max="8" value="0" step="1" title="Y position — slide the model vertically"><button class="rst" id="rst-pany" title="Reset Y position">↺</button></span>
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Zoom</span><input type="number" id="slider-value-zoom" class="numin" min="-95" max="95" step="1" value="0"></span><input type="range" id="camera-zoom" min="-95" max="95" value="0" step="1" title="Zoom — camera distance to the model"><button class="rst" id="rst-zoom" title="Reset zoom">↺</button></span>
</div>
</div>
<div class="sect"><div class="sect-hdr">Display</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Speed</span><input type="number" id="slider-value-speed" class="numin" min="0.01" max="100" step="0.01" value="1"></span><input type="range" id="speed-slider" min="-2" max="2" value="0" step="0.1" title="Speed — animation rate (sub-steps / dt scale)"><button class="rst" id="rst-speed" title="Reset speed">↺</button></span>
  <span class="pcell axcol vmcell"><span class="punit-top"><span class="plabel">Line</span><input type="number" id="slider-value-line" class="numin" min="1" max="10" step="1" value="1"></span><input type="range" id="line-width" min="1" max="10" value="1" step="1" title="Line width — trace thickness"><button class="rst" id="rst-line" title="Reset line width">↺</button></span>
  <span class="pcell axcol vmcell sf-cell" id="stepfine-grp" title="Outer ring = Step× (coarse step), inner ring = Fine× (fraction of a step)"><span class="sf-hdr"><span class="sf-row"><span class="plabel">step</span><span class="led sf-led" id="step-led" title="current Step×">1</span></span></span><span class="grp vmbay"><span id="stepfine-stack"></span></span><span class="sf-ftr"><span class="sf-row"><span class="plabel">fine</span><span class="led sf-led" id="fine-led" title="current Fine×">.1</span></span></span><select id="step-ratio" title="Step ×" style="display:none"><option value="0.25">0.25</option><option value="0.5">0.5</option><option value="1" selected>1</option><option value="2">2</option><option value="5">5</option></select><select id="fine-ratio" title="Fine ×" style="display:none"><option value="1">1</option><option value="0.1" selected>0.1</option><option value="0.01">0.01</option><option value="0.001">0.001</option></select></span>
</div></div>
<div class="sect"><div class="sect-hdr">Style</div>
<div class="row vmrow">
  <span class="pcell axcol" title="Size of every knob (S / M / L / XL) — scales the whole control interface."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Size</span><span id="size-stack"></span></span></span></span>
  <span class="pcell axcol" title="Outer ring = knob appearance (std / flat / vint / chrome / gold / carbon); inner ring = LED readout color (Red / Green / Blue / Amber)."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Knob</span><span id="knobstyle-stack"></span></span></span></span>
  <span class="pcell axcol" id="phosphor-cell" title="CRT phosphor for scope traces (Lissajous / Graphic Artist) — sets trace color + afterglow. P31 crisp green … P7 blue→green … P33 long amber."><span class="grp vmbay"><span class="grp knoblbl"><span class="plabel">Phosphor</span><span id="phosphor-stack"></span></span></span></span>
</div>
<select id="knob-size" title="Size (all knobs)" style="display:none"><option value="0.85">S</option><option value="1" selected>M</option><option value="1.3">L</option><option value="1.7">XL</option></select>
<select id="knob-style" title="Knob style" style="display:none"><option value="std" selected>std</option><option value="flat">flat</option><option value="vint">vint</option><option value="chrome">chrome</option><option value="gold">gold</option><option value="carbon">carbon</option></select>
<select id="phosphor" title="CRT phosphor" style="display:none"></select>
<select id="led-color" title="LED readout color" style="display:none"></select>
</div>
<div class="sect gen-osc" id="gen-x-module"><div class="sect-hdr" title="Oscillator X — the xy scope's horizontal axis. freq knob (log, equal turn per octave), level knob, and a dual knob: outer ring = speaker channel, inner = waveform.">Gen X</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin gen-led" id="gen-x-led" min="27.5" max="28160" step="1" value="200"></span><input type="range" id="gen-x-freq" min="0" max="120" step="1" value="34" style="display:none"><span class="grp vmbay"><span id="gen-x-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="gen-x-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-x-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="gen-x-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-x-ostack"></span></span><select id="gen-x-out" title="X → speaker channel (outer ring)" style="display:none"><option value="off" selected>off</option><option value="l">L</option><option value="r">R</option><option value="both">L+R</option></select><select id="gen-x-wave" title="X waveform (inner knob)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option></select></span>
</div></div>
<div class="sect gen-osc" id="gen-y-module"><div class="sect-hdr" title="Oscillator Y — the xy scope's vertical axis. freq knob (log), level knob, and a dual knob: outer ring = speaker channel, inner = waveform.">Gen Y</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin gen-led" id="gen-y-led" min="27.5" max="28160" step="1" value="300"></span><input type="range" id="gen-y-freq" min="0" max="120" step="1" value="41" style="display:none"><span class="grp vmbay"><span id="gen-y-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="gen-y-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-y-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="gen-y-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-y-ostack"></span></span><select id="gen-y-out" title="Y → speaker channel (outer ring)" style="display:none"><option value="off" selected>off</option><option value="l">L</option><option value="r">R</option><option value="both">L+R</option></select><select id="gen-y-wave" title="Y waveform (inner knob)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option></select></span>
</div></div>
<div class="sect gen-osc" id="gen-z-module"><div class="sect-hdr" title="Oscillator Z — a third generator (audio / modulation). freq knob (log), level knob, and a dual knob: outer ring = speaker channel, inner = waveform.">Gen Z</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">freq</span><input type="number" class="numin gen-led" id="gen-z-led" min="27.5" max="28160" step="1" value="150"></span><input type="range" id="gen-z-freq" min="0" max="120" step="1" value="29" style="display:none"><span class="grp vmbay"><span id="gen-z-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="gen-z-lvl-led" min="0" max="100" step="1" value="80"></span><input type="range" id="gen-z-lvl" min="0" max="100" step="1" value="80" style="display:none"><span class="grp vmbay"><span id="gen-z-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">out</span></span><span class="grp vmbay"><span id="gen-z-ostack"></span></span><select id="gen-z-out" title="Z → speaker channel (outer ring)" style="display:none"><option value="off" selected>off</option><option value="l">L</option><option value="r">R</option><option value="both">L+R</option></select><select id="gen-z-wave" title="Z waveform (inner knob)" style="display:none"><option value="0">sine</option><option value="1">tri</option><option value="2">sqr</option><option value="3">saw</option></select></span>
</div></div>
<div class="sect gen-osc" id="sonify-module"><div class="sect-hdr" title="Model Out — HEAR the attractor. Inner knob picks how: FLOW integrates the attractor's own equations at audio rate, so the pitch is the system's natural orbital frequency (chaos chirps, periodic windows lock to tones, parameter changes are audible) and RATE transposes it (A4 = ×1); SCAN traces the drawn trail as one waveform period at exactly RATE Hz (a stable, playable tone). MAP picks which two coordinates drive L/R (CAM = the screen's x/y, so rotating the model changes the sound; off = silent). LVL is output level.">Model Out</div>
<div class="row vmrow">
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">rate</span><input type="number" class="numin gen-led" id="sonify-led" min="27.5" max="28160" step="1" value="110"></span><input type="range" id="sonify-freq" min="0" max="120" step="1" value="24" style="display:none"><span class="grp vmbay"><span id="sonify-fstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell"><span class="punit-top"><span class="plabel">lvl</span><input type="number" class="numin gen-led" id="sonify-lvl-led" min="0" max="100" step="1" value="60"></span><input type="range" id="sonify-lvl" min="0" max="100" step="1" value="60" style="display:none"><span class="grp vmbay"><span id="sonify-lstack"></span></span></span>
  <span class="pcell axcol vmcell gen-cell gen-out-cell"><span class="punit-top"><span class="plabel">map</span></span><span class="grp vmbay"><span id="sonify-mstack"></span></span><select id="sonify-map" title="Model Out mapping (outer ring) — which two model coordinates drive L/R (off = silent)" style="display:none"><option value="off" selected>off</option><option value="cam">CAM</option><option value="xy">XY</option><option value="xz">XZ</option><option value="yz">YZ</option></select><select id="sonify-mode" title="Model Out mode (inner knob) — FLOW: audify the dynamics (pitch emergent, RATE transposes) · SCAN: trail as wavetable (RATE = exact Hz)" style="display:none"><option value="flow" selected>FLOW</option><option value="scan">SCAN</option></select></span>
</div></div>
</div>
<div id="runtime" style="color:#555;font-size:11px;padding-left:40px;"></div>
`

// ── main ─────────────────────────────────────────────────────────────────────

// ledColorDefs is the ORDERED single source for the LED-readout color options.
// It drives the LED-color knob's detents (in sweep order — amber sits between
// red and green), the hidden <select>, the CSS-var color map, and the dial
// dots. Add a color here and it appears everywhere.
var ledColorDefs = []struct{ name, col, glow, bg, bd string }{
	{"red", "#ff3b30", "#ff2a20", "#170000", "#3a0000"},
	{"amber", "#ffb000", "#d08000", "#171000", "#3a2a00"},
	{"green", "#35e06a", "#1c9a44", "#001709", "#0a3a1a"},
	{"blue", "#3bb0ff", "#1c7ad0", "#001017", "#0a2a3a"},
	{"cyan", "#2ce0e0", "#159a9a", "#001515", "#0a3838"},   // the old audio-mod readout hue
	{"violet", "#b06cff", "#8040d0", "#12001f", "#2a0a3a"}, // new
}

// styleKnobRot staggers the knob-style label ring by this many degrees so its
// labels sit between the LED-color dots on the inner ring; the style knob's
// pointer is offset by the same amount so it still points at its labels.
const styleKnobRot = 30.0

func Run() {
	// Lazy WebGL init — see initWebGL doc. Must run after the host
	// DOM is ready (caller's responsibility); otherwise gocanvas
	// won't exist yet and canvasEl.Call("getContext", ...) panics.
	initWebGL()
	if body.IsUndefined() || body.IsNull() {
		js.Global().Call("alert", "cannot get html body, exiting")
		return
	}
	if canvasEl.IsUndefined() || canvasEl.IsNull() {
		js.Global().Call("alert", "cannot find #gocanvas, exiting")
		return
	}
	installErrorNet()

	// Build controls panel. If the host page already has a <footer>
	// (e.g. m2/magnetosphere.net puts cart + shipping nav there), append
	// the controls inline into it so the existing footer nav stays
	// accessible. Otherwise fall back to a fixed-bottom overlay for
	// standalone use.
	panel := doc.Call("createElement", "div")
	panel.Set("id", "controls-panel")
	panel.Set("innerHTML", "<style>"+panelCSS+"</style>"+controlsBody)
	buildModeSelect(panel) // the mode <select> derives from the mode registry
	footers := doc.Call("getElementsByTagName", "footer")
	var existingFooter js.Value
	if footers.Get("length").Int() > 0 {
		existingFooter = footers.Index(0)
	}
	if existingFooter.Truthy() {
		panel.Set("style", "color:#aaa;font-family:'B612 Mono',monospace;font-size:12px;padding:8px 12px;background:rgba(0,0,0,0.85);border-top:1px solid #333;")
		existingFooter.Call("appendChild", panel)
	} else {
		standalonePanel = true
		body.Call("appendChild", panel)
		wireDockButtons()
		initDockResize()
		initFloatWindow()
		applyDock(readDockPref())
	}

	// Refresh DOM
	doc = js.Global().Get("document")
	body = doc.Get("body")
	injectFonts() // embedded @font-face rules for the panel / LED / header fonts

	// Get control element references
	rtc = doc.Call("getElementById", "runtime")
	cameraControl = doc.Call("getElementById", "camera-zoom")
	rotationControlsX = doc.Call("getElementById", "rotation-controls-x")
	rotationControlsY = doc.Call("getElementById", "rotation-controls-y")
	rotationControlsZ = doc.Call("getElementById", "rotation-controls-z")
	sliderZoom = doc.Call("getElementById", "slider-value-zoom")
	sliderX = doc.Call("getElementById", "slider-value-x")
	sliderY = doc.Call("getElementById", "slider-value-y")
	sliderZ = doc.Call("getElementById", "slider-value-z")

	// ── Rotation knobs (digital-pot style, one per axis) ──────────────
	// Turning a knob sets that axis's absolute angle (position) and zeroes
	// its spin rate — "the speed obeys the knob". Moving the rate slider
	// spins the axis and the knob pointer/LED track it live — "the knob
	// obeys the speed". Faithful to Glen's 3D projective unit, whose panel
	// pots set absolute X/Y angles on 7-seg displays.
	knobPtr[0] = doc.Call("getElementById", "knobptr-x")
	knobPtr[1] = doc.Call("getElementById", "knobptr-y")
	knobPtr[2] = doc.Call("getElementById", "knobptr-z")
	knobLED[0] = doc.Call("getElementById", "led-x")
	knobLED[1] = doc.Call("getElementById", "led-y")
	knobLED[2] = doc.Call("getElementById", "led-z")
	knobsReady = true
	// Shared drag state (only one knob turns at a time). Document-level
	// move/up listeners (below) let the drag continue when the cursor
	// leaves the small knob, without setPointerCapture (a JS throw there
	// would panic the Go callback).
	knobAxis := -1 // which axis is being turned (-1 = none)
	var knobCX, knobCY, knobPrevAng float64
	knobAngleAt := func(e js.Value) float64 {
		return math.Atan2(e.Get("clientY").Float()-knobCY, e.Get("clientX").Float()-knobCX)
	}
	attachKnob := func(knobID string, axis int, spin, spinNum js.Value) {
		kn := doc.Call("getElementById", knobID)
		if !kn.Truthy() {
			return
		}
		kn.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			r := kn.Call("getBoundingClientRect")
			knobCX = r.Get("left").Float() + r.Get("width").Float()/2
			knobCY = r.Get("top").Float() + r.Get("height").Float()/2
			knobPrevAng = knobAngleAt(e)
			knobAxis = axis
			// Grabbing the knob holds the pose: stop this axis's spin
			// ("the speed obeys the knob").
			spin.Set("value", "0")
			if spinNum.Truthy() {
				spinNum.Set("value", "0")
			}
			setSpinAxis(axis, 0)
			if axis == 1 { // Y spin just zeroed → reflect auto-rotate off
				clearAutoRotateFlag()
			}
			return nil
		}))
		// Scroll over the angle ring nudges the pose by 5° per notch.
		kn.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			step := float32(math.Pi / 36) // 5°
			if e.Get("deltaY").Float() > 0 {
				step = -step
			}
			addAngleAxis(axis, step)
			return nil
		}))
	}
	attachKnob("knob-x", 0, rotationControlsX, sliderX)
	attachKnob("knob-y", 1, rotationControlsY, sliderY)
	attachKnob("knob-z", 2, rotationControlsZ, sliderZ)
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if knobAxis < 0 {
			return nil
		}
		cur := knobAngleAt(args[0])
		d := cur - knobPrevAng
		for d > math.Pi { // shortest-arc delta so it turns endlessly
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		knobPrevAng = cur
		addAngleAxis(knobAxis, float32(d))
		return nil
	}))
	knobRelease := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		knobAxis = -1
		return nil
	})
	doc.Call("addEventListener", "pointerup", knobRelease)
	doc.Call("addEventListener", "pointercancel", knobRelease)
	updateRotKnobs()
	initKnobDrag() // document listeners for the bounded param/camera knobs

	// Knobify the fixed sliders too (zoom, speed, line, trail, spin rates):
	// hide each range input and insert a bounded knob that drives it. Reuses
	// each slider's existing 'input' handler, so behavior is unchanged.
	knobifyFixed := func(sliderID, numID string, dial bool) js.Value {
		sl := doc.Call("getElementById", sliderID)
		if !sl.Truthy() {
			return js.Undefined()
		}
		num := doc.Call("getElementById", numID)
		sl.Get("style").Set("display", "none")
		// The knob is inserted bare; the cell's reset button stays a cell child and
		// is pinned to the header's top-right by CSS (standard cell template).
		k := makeKnob(sl, num, true, true, dial)
		parent := sl.Get("parentNode")
		// Insert the knob right after the (hidden) slider. Only insertBefore the
		// numeric when it's actually a sibling — with the label-column layout the
		// numeric lives in a separate span, so we just append to the column.
		if num.Truthy() && num.Get("parentNode").Equal(parent) {
			parent.Call("insertBefore", k, num)
		} else {
			parent.Call("appendChild", k)
		}
		return k
	}
	// Value knobs get a numeric scale dial; the rotation-rate knobs don't (they
	// nest as the inner disc of the View angle knobs, which already carry a
	// degree dial — a second scale would collide).
	knobifyFixed("camera-zoom", "slider-value-zoom", true)
	knobifyFixed("pan-x", "slider-value-panx", true)
	knobifyFixed("pan-y", "slider-value-pany", true)
	knobifyFixed("speed-slider", "slider-value-speed", true)
	knobifyFixed("line-width", "slider-value-line", true)
	knobifyFixed("trail-slider", "slider-value-trail", true)
	knobifyFixed("rainbow-freq", "slider-value-rfreq", true)
	rkx := knobifyFixed("rotation-controls-x", "slider-value-x", false)
	rky := knobifyFixed("rotation-controls-y", "slider-value-y", false)
	rkz := knobifyFixed("rotation-controls-z", "slider-value-z", false)

	// Stack each axis's angle knob (outer ring) around its spin-rate knob
	// (inner), oscilloscope-style, with an analog degree dial around the ring.
	// Beside it, a digital readout column: the degrees LED, then the spin-rate
	// numeric directly below it, then reset. The cell label becomes "<axis> / Rate".
	// Each axis is a NARROW vertical strip: label ("X / Rate") · degrees LED ·
	// angle knob (in a square, with the reset tucked in its lower-right corner)
	// · spin-rate numeric. This sets the minimum module slot width.
	stackAxis := func(axisLbl, angleID, ledID, rateNumID, rateRstID string, rateKnob js.Value) {
		ak := doc.Call("getElementById", angleID)
		if !ak.Truthy() || !rateKnob.Truthy() {
			return
		}
		led := doc.Call("getElementById", ledID)
		rateNum := doc.Call("getElementById", rateNumID)
		rst := doc.Call("getElementById", rateRstID)
		cell := ak.Call("closest", ".pcell")
		angleGrp := ak.Get("parentNode")
		var rateGrp js.Value
		if rateNum.Truthy() {
			rateGrp = rateNum.Get("parentNode")
		}

		stack := stackKnobs(ak, rateKnob)
		addAngleDial(stack)

		// square wrapper so the reset can sit in the knob's corner
		knobBox := doc.Call("createElement", "span")
		knobBox.Set("className", "axknob-box")
		knobBox.Call("appendChild", stack)
		if rst.Truthy() {
			rst.Get("classList").Call("add", "axknob-rst")
			knobBox.Call("appendChild", rst)
		}

		col := doc.Call("createElement", "span")
		col.Set("className", "grp axstack")
		// Top line: axis label ("X") to the LEFT of the degrees LED readout.
		topRow := doc.Call("createElement", "span")
		topRow.Set("className", "grp axrow toprow")
		if cell.Truthy() {
			if lbl := cell.Call("querySelector", ".plabel"); lbl.Truthy() {
				lbl.Set("textContent", axisLbl)
				topRow.Call("appendChild", lbl)
			}
		}
		if led.Truthy() {
			topRow.Call("appendChild", led)
		}
		col.Call("appendChild", topRow)
		col.Call("appendChild", knobBox)
		// Bottom line: "Rate" label to the LEFT of the spin-rate numeric.
		botRow := doc.Call("createElement", "span")
		botRow.Set("className", "grp axrow botrow")
		rlbl := doc.Call("createElement", "span")
		rlbl.Set("className", "plabel sym") // ω = angular rate; .sym keeps it lowercase (not Ω)
		rlbl.Set("textContent", "ω")
		botRow.Call("appendChild", rlbl)
		if rateNum.Truthy() {
			botRow.Call("appendChild", rateNum)
		}
		col.Call("appendChild", botRow)

		// swap the column in for the old [knob][led] grp, hide the rate row
		if angleGrp.Truthy() {
			p := angleGrp.Get("parentNode")
			p.Call("insertBefore", col, angleGrp)
			p.Call("removeChild", angleGrp)
		}
		if rateGrp.Truthy() {
			rateGrp.Get("style").Set("display", "none")
		}
	}
	stackAxis("X", "knob-x", "led-x", "slider-value-x", "rst-rx", rkx)
	stackAxis("Y", "knob-y", "led-y", "slider-value-y", "rst-ry", rky)
	stackAxis("Z", "knob-z", "led-z", "slider-value-z", "rst-rz", rkz)

	// "Front" switch: draw the model in front of the controls. Raising the
	// canvas above the panel and making it pointer-transparent lets the model
	// overlap the controls while clicks/drags still reach them (drag-rotate
	// is bound to the document, so it keeps working through the canvas).
	if mf := doc.Call("getElementById", "model-front"); mf.Truthy() {
		mf.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			cont := doc.Call("getElementById", "gocanvas-container")
			if !cont.Truthy() {
				return nil
			}
			st := cont.Get("style")
			if mf.Get("checked").Bool() {
				st.Set("zIndex", "var(--z-canvas-front)")
				st.Set("pointerEvents", "none")
			} else {
				st.Set("zIndex", "var(--z-canvas)")
				st.Set("pointerEvents", "auto")
				// Front off ⇒ normal layering; drop the recovery-raise so stacking
				// is predictable next time. The panel falls back to the CSS base
				// var(--z-panel)=10 (> canvas) — never to auto(0).
				body.Get("classList").Call("remove", "panel-raised")
			}
			return nil
		}))
	}

	if sf := doc.Call("getElementById", "spect-fill"); sf.Truthy() {
		sf.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			spectFill = sf.Get("checked").Bool()
			return nil
		}))
	}

	// Floating show/hide button for the whole control panel (it can block the
	// view). Lives outside the panel so it can bring it back.
	panelToggle := doc.Call("createElement", "button")
	panelToggle.Set("textContent", "▤")
	panelToggle.Set("title", "Show / hide controls (brings them back if the model's 'Front' overlay is hiding them)")
	panelToggle.Set("style", "position:fixed;bottom:6px;left:6px;z-index:var(--z-toggle);background:#222;color:#ccc;border:1px solid #555;border-radius:3px;font-family:'B612 Mono',monospace;font-size:14px;cursor:pointer;padding:2px 8px;opacity:0.55;")
	panelToggle.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		p := doc.Call("getElementById", "controls-panel")
		if !p.Truthy() {
			return nil
		}
		st := p.Get("style")
		hidden := st.Get("display").String() == "none"
		// The recovery-raise is ONE bit: body.panel-raised. CSS lifts the panel
		// AND its chrome (resize strip, float grip, dock buttons) together, so
		// the whole recovery unit surfaces above the Front canvas as a piece.
		cl := body.Get("classList")
		raised := cl.Call("contains", "panel-raised").Bool()
		frontOn := false
		if mf := doc.Call("getElementById", "model-front"); mf.Truthy() {
			frontOn = mf.Get("checked").Bool()
		}
		// Recover whenever the panel is hidden OR the "Front" canvas is drawn over
		// it — otherwise an opaque backdrop with Front on buries the panel and
		// toggling display alone can never bring it back. Raise it above the Front
		// canvas so one press always restores it; otherwise hide it so the model
		// can be viewed unobstructed.
		if hidden || (frontOn && !raised) {
			st.Set("display", "")
			cl.Call("add", "panel-raised")
		} else {
			st.Set("display", "none")
			cl.Call("remove", "panel-raised")
		}
		return nil
	}))
	body.Call("appendChild", panelToggle)

	// "Size / Step × / Fine ×" — one 3-layer concentric knob: the outermost
	// ring is Size (scales every knob via --kscale), the middle ring is Step×
	// (scales every param knob's coarse step), the inner knob is Fine× (the
	// inner disc's fraction of a coarse step). Step×/Fine× are read live by the
	// param knobs.
	sr := doc.Call("getElementById", "step-ratio")
	fr := doc.Call("getElementById", "fine-ratio")
	ks := doc.Call("getElementById", "knob-size")
	if sr.Truthy() && fr.Truthy() && ks.Truthy() {
		if holder := doc.Call("getElementById", "stepfine-stack"); holder.Truthy() {
			// Two concentric clickable label rings: Step× (outer) · Fine× (inner).
			// Size moved to its own Style module so this stays compact.
			sr.Set("title", "Step × — coarse step-size multiplier for every parameter knob")
			fr.Set("title", "Fine × — fine-trim step as a fraction of one coarse step")
			stepFine := stackKnobs(makeSelectorKnob(sr), makeSelectorKnob(fr))
			addSelectorLabels(stepFine, []string{".25", ".5", "1", "2", "5"}, sr, 43)
			addSelectorLabels(stepFine, []string{"1", ".1", ".01", ".001"}, fr, 31)
			holder.Call("appendChild", stepFine)
			sr.Get("style").Set("display", "none")
			fr.Get("style").Set("display", "none")
			// Separate LED readouts for Step× and Fine× (mirror the label rings).
			// Fixed decimals so the readouts are constant width (zero-padded) and
			// never resize as the ratios change.
			sled := doc.Call("getElementById", "step-led")
			fled := doc.Call("getElementById", "fine-led")
			fmtF := func(s string, dec int) string {
				v, _ := strconv.ParseFloat(s, 64)
				return strconv.FormatFloat(v, 'f', dec, 64)
			}
			upd := func() {
				if sled.Truthy() {
					sled.Set("textContent", fmtF(sr.Get("value").String(), 2)) // 0.25 … 5.00
				}
				if fled.Truthy() {
					fled.Set("textContent", fmtF(fr.Get("value").String(), 3)) // 1.000 … 0.001
				}
			}
			sr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
			fr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
			upd()
		}
		// Size knob lives in its own Style module (a lone labeled selector ring).
		if sh := doc.Call("getElementById", "size-stack"); sh.Truthy() {
			ks.Set("title", "Size — scales the whole control interface (S / M / L / XL)")
			sh.Call("appendChild", singleSelectorKnob(ks, []string{"S", "M", "L", "XL"}, 46))
			ks.Get("style").Set("display", "none")
		}
		// Knob-style selector (outer) with the LED-color selector stacked as its
		// inner ring — appearance + readout color on one shaft, saving a cell.
		if st := doc.Call("getElementById", "knob-style"); st.Truthy() {
			lc := doc.Call("getElementById", "led-color")
			st.Set("title", "Knob style — knob face appearance (std / flat / vint / chrome / gold / carbon)")
			if kh := doc.Call("getElementById", "knobstyle-stack"); kh.Truthy() {
				if lc.Truthy() {
					lc.Set("title", "LED color — readout color for every numeric LED readout")
					// Populate the LED-color options from the single ordered source.
					var dotCols []string
					for _, d := range ledColorDefs {
						o := doc.Call("createElement", "option")
						o.Set("value", d.name)
						o.Set("textContent", d.name)
						lc.Call("appendChild", o)
						dotCols = append(dotCols, d.col)
					}
					// Outer = knob style (labeled ring). Inner = LED color: a ring of
					// colored dots (one per option in its own LED color), the selected
					// one highlighted.
					stStack := stackKnobs(makeSelectorKnob(st, styleKnobRot), makeSelectorKnob(lc))
					addSelectorLabels(stStack, []string{"std", "flat", "vint", "chrm", "gold", "carb"}, st, 46, styleKnobRot) // labels staggered off the LED dots; pointer offset to match
					addSelectorDotLabels(stStack, dotCols, lc, 36)                                                            // ring just outside the style knob, inside its text labels
					kh.Call("appendChild", stStack)
				} else {
					kh.Call("appendChild", singleSelectorKnob(st, []string{"std", "flat", "vint", "chrm", "gold", "carb"}, 44))
				}
			}
			applyStyle := func() {
				cl := doc.Call("getElementById", "controls-panel").Get("classList")
				for _, s := range []string{"ks-std", "ks-flat", "ks-vint", "ks-chrome", "ks-gold", "ks-carbon"} {
					cl.Call("remove", s)
				}
				cl.Call("add", "ks-"+st.Get("value").String())
			}
			st.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				applyStyle()
				return nil
			}))
			applyStyle()
			st.Get("style").Set("display", "none")
			// LED readout color presets → CSS vars on the panel.
			if lc.Truthy() {
				ledCols := map[string][4]string{}
				for _, d := range ledColorDefs {
					ledCols[d.name] = [4]string{d.col, d.glow, d.bg, d.bd}
				}
				applyLED := func() {
					c := ledCols[lc.Get("value").String()]
					s := doc.Call("getElementById", "controls-panel").Get("style")
					s.Call("setProperty", "--led-col", c[0])
					s.Call("setProperty", "--led-glow", c[1])
					s.Call("setProperty", "--led-bg", c[2])
					s.Call("setProperty", "--led-bd", c[3])
				}
				// Readout below the knob: an "LED" label to the left of a color-name
				// display (shown in the current LED color), like a labeled readout.
				var ledRO js.Value
				if kh := doc.Call("getElementById", "knobstyle-stack"); kh.Truthy() {
					row := doc.Call("createElement", "span")
					row.Set("className", "ledcolor-row")
					lab := doc.Call("createElement", "span")
					lab.Set("className", "plabel ledcolor-lbl")
					lab.Set("textContent", "LED")
					ledRO = doc.Call("createElement", "span")
					ledRO.Set("className", "selk-readout ledcolor-ro")
					row.Call("appendChild", lab)
					row.Call("appendChild", ledRO)
					kh.Get("parentNode").Call("appendChild", row)
				}
				updLEDRO := func() {
					if ledRO.Truthy() {
						idx := lc.Get("selectedIndex").Int()
						if idx < 0 {
							idx = 0
						}
						ledRO.Set("textContent", lc.Get("options").Index(idx).Get("text").String())
					}
				}
				lc.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} { applyLED(); updLEDRO(); return nil }))
				applyLED()
				updLEDRO()
				lc.Get("style").Set("display", "none")
			}
		}
		sr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if v, err := strconv.ParseFloat(sr.Get("value").String(), 64); err == nil && v > 0 {
				coarseRatio = v
			}
			return nil
		}))
		fr.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if v, err := strconv.ParseFloat(fr.Get("value").String(), 64); err == nil && v > 0 {
				fineRatio = v
			}
			return nil
		}))
		applyKS := func() {
			if v, err := strconv.ParseFloat(ks.Get("value").String(), 64); err == nil {
				setKScale(v)
			}
		}
		ks.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			applyKS()
			return nil
		}))
		// Restore a saved (continuous) interface scale from a prior resize-drag;
		// otherwise honor the Size knob's default.
		if ls := js.Global().Get("localStorage"); ls.Truthy() {
			if s := ls.Call("getItem", "wasmstuff-kscale"); s.Truthy() {
				if v, err := strconv.ParseFloat(s.String(), 64); err == nil && v > 0 {
					setKScale(v)
				} else {
					applyKS()
				}
			} else {
				applyKS()
			}
		} else {
			applyKS()
		}
	}

	// Event: mode change
	doc.Call("getElementById", "mode-select").Call("addEventListener", "change", trackedFuncOf(onModeChange))

	// Event: color pickers
	colorCallback := trackedFuncOf(onColorChange)
	doc.Call("getElementById", "color-base").Call("addEventListener", "input", colorCallback)
	doc.Call("getElementById", "color-mid").Call("addEventListener", "input", colorCallback)
	doc.Call("getElementById", "color-top").Call("addEventListener", "input", colorCallback)
	attachColorKnobs() // Hue/Sat/Val knob under each color swatch

	// Event: per-control reset buttons for colors
	doc.Call("getElementById", "rst-color-base").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		baseColor = [3]float32{1.0, 0.0, 0.0}
		doc.Call("getElementById", "color-base").Set("value", "#ff0000")
		gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
		return nil
	}))
	doc.Call("getElementById", "rst-color-mid").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		midColor = [3]float32{0.0, 1.0, 0.0}
		doc.Call("getElementById", "color-mid").Set("value", "#00ff00")
		gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
		return nil
	}))
	doc.Call("getElementById", "rst-color-top").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		topColor = [3]float32{0.0, 0.0, 1.0}
		doc.Call("getElementById", "color-top").Set("value", "#0000ff")
		gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
		return nil
	}))

	// Event: reset all button
	doc.Call("getElementById", "reset-all-btn").Call("addEventListener", "click", trackedFuncOf(onResetAll))

	// Any reset button (↺) — re-sync every knob pointer to its freshly-reset
	// value on the next frame (after the button's own handler has set values),
	// so knobs whose handler sets the slider without dispatching 'input' still
	// snap their pointer back.
	resetSync := trackedFuncOf(func(this js.Value, args []js.Value) interface{} { syncKnobs(); return nil })
	doc.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if t := a[0].Get("target"); t.Truthy() && t.Call("closest", ".rst").Truthy() {
			js.Global().Call("requestAnimationFrame", resetSync)
		}
		return nil
	}))

	// Event: normalize — reorient the current model to the default
	// (identity) pose and stop any slider-driven spin.
	doc.Call("getElementById", "normalize-btn").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		normalizeOrientation()
		return nil
	}))

	// Event: Edit eqn — load the current attractor's equations into the
	// editable Custom mode (if we have parseable forms for it) and switch.
	doc.Call("getElementById", "edit-eq-sw").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		sw := doc.Call("getElementById", "edit-eq-sw")
		s := doc.Call("getElementById", "mode-select")
		if sw.Get("checked").Bool() {
			if selectedMode != "custom" {
				preCustomMode = selectedMode // remember where to return
			}
			seedCustomFromMode(preCustomMode)
			s.Set("value", "custom")
		} else {
			back := preCustomMode
			if back == "" || back == "custom" {
				back = "lorenz"
			}
			s.Set("value", back)
		}
		s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))

	// Event: speed slider
	// (Speed's input/reset wiring is owned by its ControlDesc registration.)

	// Event: auto-rotate switch — fold the auto-spin into the Y rate knob.
	doc.Call("getElementById", "auto-rotate").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setAutoRotate(doc.Call("getElementById", "auto-rotate").Get("checked").Bool())
		return nil
	}))

	// Event: spectrogram-skin checkbox — paint the live spectrogram onto
	// the current surface model (sphere/globe/torus).
	doc.Call("getElementById", "spectro-skin").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		spectroSkin = doc.Call("getElementById", "spectro-skin").Get("checked").Bool()
		skinDirty = true
		generateForMode(selectedMode)
		return nil
	}))

	// Event: Backdrop — two mutually-exclusive 2-position switches (Spectro bg /
	// XY bg) drive the hidden #bg-visual select (turning one on turns the other
	// off; turning the active one off clears the backdrop).
	if bv := doc.Call("getElementById", "bg-visual"); bv.Truthy() {
		spectro := doc.Call("getElementById", "bg-spectro")
		xy := doc.Call("getElementById", "bg-xy")
		syncBackdrop := func() {
			v := bv.Get("value").String()
			if spectro.Truthy() {
				spectro.Set("checked", v == "spectrogram")
			}
			if xy.Truthy() {
				xy.Set("checked", v == "xy")
			}
		}
		bv.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			setBackgroundVisual(bv.Get("value").String())
			syncBackdrop()
			return nil
		}))
		wireBg := func(sw js.Value, mode string) {
			if !sw.Truthy() {
				return
			}
			sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				if sw.Get("checked").Bool() {
					bv.Set("value", mode)
				} else if bv.Get("value").String() == mode {
					bv.Set("value", "")
				}
				bv.Call("dispatchEvent", js.Global().Get("Event").New("change"))
				return nil
			}))
		}
		wireBg(spectro, "spectrogram")
		wireBg(xy, "xy")
		syncBackdrop()
	}

	// Phosphor selector — populate from the phosphor table + set phosphorIdx.
	// Lives in the Style module as a rotary knob with a name readout (too many
	// options / too-long names for a label ring).
	if ph := doc.Call("getElementById", "phosphor"); ph.Truthy() {
		for i, p := range phosphors {
			opt := doc.Call("createElement", "option")
			opt.Set("value", strconv.Itoa(i))
			opt.Set("textContent", p.name)
			ph.Call("appendChild", opt)
		}
		ph.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if v, err := strconv.Atoi(ph.Get("value").String()); err == nil {
				phosphorIdx = v
			}
			// Selecting a phosphor IS how you enter CRT mode now: the trace is
			// drawn monochrome on that phosphor, so the gradient source + palette
			// colors are overridden (and dimmed).
			crtMode = phosphorIdx > 0
			updateCRTOverlay()
			updateCRTDim()
			return nil
		}))
		ph.Set("title", "Phosphor — CRT trace color + afterglow for scope modes (P31 crisp green … P7 blue→green … P33 long amber)")
		if holder := doc.Call("getElementById", "phosphor-stack"); holder.Truthy() {
			pk := selectorKnobReadout(ph)
			holder.Call("appendChild", pk)
			addPhosphorTraces(pk.Call("querySelector", ".knobstack"), ph)
		}
	}

	// Event: audio-mod checkbox — enable per-parameter audio modulation.
	// The per-parameter routing controls appear under each attractor
	// parameter (built by buildParamPanel) while this is checked.
	doc.Call("getElementById", "audio-mod").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setAudioMod(doc.Call("getElementById", "audio-mod").Get("checked").Bool())
		return nil
	}))

	// Event: Meters switch — show/hide the top-left audio feature meters.
	doc.Call("getElementById", "show-meters").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		metersEnabled = doc.Call("getElementById", "show-meters").Get("checked").Bool()
		updateMetersVisibility()
		return nil
	}))

	// Event: function/signal generator — a client-side audio source that works
	// with no server or mic (so audio modulation / spectrogram / xy work on the
	// static site). Toggle + waveform + sweep-rate.
	doc.Call("getElementById", "fg-on").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setFuncGen(doc.Call("getElementById", "fg-on").Get("checked").Bool())
		return nil
	}))
	buildGeneratorModule()
	buildSonifyModule()

	// Template legend module + its Window-group toggle.
	buildTemplateModule()
	doc.Call("getElementById", "tpl-on").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setTemplate(doc.Call("getElementById", "tpl-on").Get("checked").Bool())
		return nil
	}))

	// Event: built-in test-tone generator (loops through the server's capture).
	doc.Call("getElementById", "test-tone").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		setTestTone(doc.Call("getElementById", "test-tone").Get("checked").Bool())
		return nil
	}))

	// Event: points/line toggle
	doc.Call("getElementById", "use-points").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		usePoints = doc.Call("getElementById", "use-points").Get("checked").Bool()
		if usePoints {
			attractorDrawMode = glTypes.Points
		} else {
			attractorDrawMode = glTypes.LineStrip
		}
		return nil
	}))

	// Event: trail length slider
	// (Trail's input/reset wiring is owned by its ControlDesc registration.)

	// Event: ring-trail switch — beam model on/off (re-primes on enable).
	if rs := doc.Call("getElementById", "ring-sw"); rs.Truthy() {
		rs.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			ringOn = rs.Get("checked").Bool()
			ringInvalidate()
			return nil
		}))
	}

	// Event: persist trail checkbox
	doc.Call("getElementById", "persist-trail").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		persistTrail = doc.Call("getElementById", "persist-trail").Get("checked").Bool()
		return nil
	}))

	// Create info overlay div
	infoOverlay := doc.Call("createElement", "div")
	infoOverlay.Set("id", "info-overlay")
	infoOverlay.Set("style", "display:none;position:fixed;top:140px;left:20px;right:20px;z-index:var(--z-hud);"+
		"color:rgba(255,255,255,0.85);font-family:'B612 Mono',monospace;font-size:14px;line-height:1.6;"+
		"white-space:pre-wrap;pointer-events:none;text-shadow:0 0 10px #000,0 0 20px #000;"+
		"max-width:600px;")
	body.Call("appendChild", infoOverlay)

	// Event: show info checkbox
	doc.Call("getElementById", "show-info").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		checked := doc.Call("getElementById", "show-info").Get("checked").Bool()
		overlay := doc.Call("getElementById", "info-overlay")
		if checked {
			if desc, ok := attractorDescriptions[selectedMode]; ok {
				overlay.Set("textContent", desc)
			} else {
				overlay.Set("textContent", selectedMode)
			}
			overlay.Get("style").Set("display", "block")
			positionInfoOverlay()
		} else {
			overlay.Get("style").Set("display", "none")
		}
		return nil
	}))

	// Event: background color picker. Alpha=0 keeps the canvas
	// transparent so the host page's background (e.g. m2's SVG logo)
	// shows through — picking a non-black bg here only tints what's
	// drawn, it doesn't paint over the host.
	doc.Call("getElementById", "color-bg").Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		hex := doc.Call("getElementById", "color-bg").Get("value").String()
		bgColor[0], bgColor[1], bgColor[2] = hexToRGB(hex)
		gl.Call("clearColor", bgColor[0], bgColor[1], bgColor[2], 0)
		return nil
	}))
	doc.Call("getElementById", "rst-color-bg").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		bgColor = [3]float32{0, 0, 0}
		doc.Call("getElementById", "color-bg").Set("value", "#000000")
		gl.Call("clearColor", 0, 0, 0, 0)
		return nil
	}))

	// Event: gradient source + colors selectors (each driven by a rotary knob)
	doc.Call("getElementById", "gradient-source").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.Atoi(doc.Call("getElementById", "gradient-source").Get("value").String()); err == nil {
			gradientSource = v
		}
		return nil
	}))
	doc.Call("getElementById", "gradient-colors").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if v, err := strconv.Atoi(doc.Call("getElementById", "gradient-colors").Get("value").String()); err == nil {
			gradientColors = v
		}
		updateGradientUI()
		return nil
	}))
	doc.Call("getElementById", "gradient-reverse").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		gradientReverse = doc.Call("getElementById", "gradient-reverse").Get("checked").Bool()
		return nil
	}))

	// Event: pause button
	doc.Call("getElementById", "pause-sw").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		paused = doc.Call("getElementById", "pause-sw").Get("checked").Bool()
		return nil
	}))

	// Power switch (default on): OFF stops the render loop and clears the model
	// (canvas) to save GPU, but KEEPS the control panel; ON resumes rendering.
	// Power is now the model selector's "OFF" position (the outer category knob's
	// 6th detent) — see setPowerState, driven from buildNestedModelSelector.

	// Fullscreen switch — on requests fullscreen, off exits. Kept in sync with
	// the actual fullscreen state (fullscreenchange fires on Esc etc.).
	// requestFullscreen/exitFullscreen return a promise that REJECTS when the
	// browser refuses (no user gesture, a permissions-policy block, or some
	// mobile/embedded contexts → "Permissions check failed"). Swallow that with
	// a .catch and re-sync the switch to reality, so it never surfaces as an
	// unhandled promise rejection.
	fsReject := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if sw := doc.Call("getElementById", "fullscreen-sw"); sw.Truthy() {
			sw.Set("checked", doc.Get("fullscreenElement").Truthy() || doc.Get("webkitFullscreenElement").Truthy())
		}
		return nil
	})
	catchFs := func(pr js.Value) {
		if pr.Truthy() && !pr.Get("then").IsUndefined() {
			pr.Call("catch", fsReject)
		}
	}
	doc.Call("getElementById", "fullscreen-sw").Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		want := doc.Call("getElementById", "fullscreen-sw").Get("checked").Bool()
		if want {
			docEl := doc.Get("documentElement")
			if !docEl.Get("requestFullscreen").IsUndefined() {
				catchFs(docEl.Call("requestFullscreen"))
			} else if !docEl.Get("webkitRequestFullscreen").IsUndefined() {
				catchFs(docEl.Call("webkitRequestFullscreen"))
			}
		} else {
			if !doc.Get("exitFullscreen").IsUndefined() {
				catchFs(doc.Call("exitFullscreen"))
			} else if !doc.Get("webkitExitFullscreen").IsUndefined() {
				catchFs(doc.Call("webkitExitFullscreen"))
			}
		}
		return nil
	}))
	syncFsSwitch := trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if sw := doc.Call("getElementById", "fullscreen-sw"); sw.Truthy() {
			sw.Set("checked", doc.Get("fullscreenElement").Truthy() || doc.Get("webkitFullscreenElement").Truthy())
		}
		return nil
	})
	doc.Call("addEventListener", "fullscreenchange", syncFsSwitch)
	doc.Call("addEventListener", "webkitfullscreenchange", syncFsSwitch)

	// Event: screenshot button
	doc.Call("getElementById", "screenshot-btn").Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dataURL := canvasEl.Call("toDataURL", "image/png")
		link := doc.Call("createElement", "a")
		link.Set("download", "attractor.png")
		link.Set("href", dataURL)
		link.Call("click")
		return nil
	}))

	// isInteractiveDragTarget returns true when the mousedown/touchstart
	// target is something the user is clicking deliberately (button,
	// link, form input, the controls panel itself) — in those cases we
	// must NOT hijack the click to start a drag.
	isInteractiveDragTarget := func(target js.Value) bool {
		if !target.Truthy() {
			return false
		}
		if closest := target.Get("closest"); closest.Type() == js.TypeFunction {
			match := target.Call("closest", "a, button, input, label, select, textarea, #controls-panel, [data-no-drag]")
			if match.Truthy() {
				return true
			}
		}
		return false
	}

	// Event: mouse drag rotation. Bound to document (not canvasEl) so
	// rotation still works when the host page paints other elements
	// (e.g. magnetosphere.net's SVG logo) above the canvas. The target
	// filter above lets clicks on links/buttons/inputs through.
	doc.Call("addEventListener", "mousedown", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if isInteractiveDragTarget(e.Get("target")) {
			return nil
		}
		dragging = true
		beginDrag(e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	js.Global().Call("addEventListener", "mousemove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if !dragging {
			return nil
		}
		e := args[0]
		dragMove(e.Get("clientX").Float(), e.Get("clientY").Float())
		return nil
	}))
	js.Global().Call("addEventListener", "mouseup", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dragging = false
		return nil
	}))

	// Event: touch drag rotation. Same doc-binding rationale as mouse.
	// Do NOT preventDefault here unconditionally — that breaks tap+drag
	// on host-page links/buttons. Only preventDefault when we're
	// actually starting a drag (target isn't interactive).
	doc.Call("addEventListener", "touchstart", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		if isInteractiveDragTarget(e.Get("target")) {
			return nil
		}
		e.Call("preventDefault")
		t := e.Get("touches").Index(0)
		dragging = true
		beginDrag(t.Get("clientX").Float(), t.Get("clientY").Float())
		return nil
	}))
	doc.Call("addEventListener", "touchmove", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if !dragging {
			return nil
		}
		e := args[0]
		e.Call("preventDefault")
		t := e.Get("touches").Index(0)
		dragMove(t.Get("clientX").Float(), t.Get("clientY").Float())
		return nil
	}))
	doc.Call("addEventListener", "touchend", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		dragging = false
		return nil
	}))

	// Event: scroll wheel zoom
	canvasEl.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		// deltaY is ~100–130 per mouse notch; keep the per-notch zoom step
		// small (~2–3) while still scaling gently on fine trackpads.
		delta := float32(e.Get("deltaY").Float()) * 0.02
		zoomVal := float32(js.Global().Get("parseFloat").Invoke(cameraControl.Get("value")).Float())
		zoomVal -= delta
		if zoomVal < -95 {
			zoomVal = -95
		}
		if zoomVal > 95 {
			zoomVal = 95
		}
		cameraControl.Set("value", strconv.FormatFloat(float64(zoomVal), 'f', 0, 64))
		cachedZoom = zoomVal // render loop reads the cache, not the DOM
		// Fire 'input' so the zoom knob pointer + numeric box track the wheel.
		cameraControl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return nil
	}))

	// Initial mode — read from URL hash if present. The hash may carry
	// permalink state after the mode ("#aizawa&p.a=1.19&..."); take only
	// the leading mode token here (the rest is applied post-setup).
	selectedMode = "globe"
	hash := js.Global().Get("location").Get("hash").String()
	if len(hash) > 1 {
		hashMode := hashModeToken()
		// Validate against the mode registry. (The old params-map + hardcoded
		// list pair silently rejected several real modes, e.g. sphere and fvf.)
		if knownMode(hashMode) {
			selectedMode = hashMode
		}
	}
	// Select the matching dropdown option
	sel := doc.Call("getElementById", "mode-select")
	if !sel.IsNull() && !sel.IsUndefined() {
		sel.Set("value", selectedMode)
	}
	buildParamPanel(selectedMode)
	updateTrailVisibility()

	// Model selector: two concentric detented rotary encoders — the outer ring
	// picks the category (attractors / polyhedra / geometry / audio / custom),
	// the inner knob the model within it. Two dropdowns below mirror them. The
	// hidden #mode-select stays the single source of truth for all the rest of
	// the code (permalink, param panel, render loop).
	selWindow = doc.Call("getElementById", "sel-window")
	updateSelWindow()
	initSelKnobDrag()
	buildNestedModelSelector()
	// Gradient source + palette: one concentric knob (source ring outside,
	// number-of-colors inside), with both dropdowns in a column beside it.
	gsrc := doc.Call("getElementById", "gradient-source")
	gcol := doc.Call("getElementById", "gradient-colors")
	if gsrc.Truthy() && gcol.Truthy() {
		if holder := doc.Call("getElementById", "gradient-stack"); holder.Truthy() {
			gstack := stackKnobs(makeSelectorKnob(gsrc), makeSelectorKnob(gcol))
			// Concentric clickable labels replace the dropdowns: source (what the
			// color follows) on the OUTER ring, palette (color count) on the INNER.
			addSelectorLabels(gstack, []string{"X", "Y", "Z", "trl"}, gsrc, 43)
			addSelectorLabels(gstack, []string{"1", "2", "3", "∞"}, gcol, 31)
			holder.Call("appendChild", gstack)
			gsrc.Get("style").Set("display", "none")
			gcol.Get("style").Set("display", "none")
		}
	}
	updateGradientUI()

	// Initialize persistent JS typed arrays for zero-alloc frame uploads
	jsVertUint8 = js.Global().Get("Uint8Array").New(steps * 4 * 4)
	buf := jsVertUint8.Get("buffer")
	jsVertFloat = js.Global().Get("Float32Array").New(buf, 0, steps*4)

	// Initialize WebGL
	glTypes.New(gl)
	attractorDrawMode = glTypes.LineStrip
	// Bind buffers before setting up attrib pointers in setupShaders
	gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
	gl.Call("bindBuffer", glTypes.ElementArrayBuffer, attractorIndexBuffer)
	setupShaders()
	setupTexShaders()
	setupMatrices()
	generateForMode(selectedMode)
	if isSpectroSurface(selectedMode) {
		setSpectrogramCamera()
	} else {
		autoFitCamera()
	}
	refreshGradient()

	// Check if debug mode is enabled via JS global
	debugVal := js.Global().Get("__WASM_DEBUG__")
	if !debugVal.IsUndefined() && debugVal.Bool() {
		debugEnabled = true
	}

	// Registry-owned fixed controls (registry refactor). One ControlDesc per
	// control owns the LED format, slider→state plumbing, typed entry,
	// wheel-step, reset button, AND Reset All (via the builtControls loop in
	// onResetAll) — previously each of those was a separate wiring site and
	// Reset All silently missed pan-x/pan-y/period. Registered BEFORE the
	// permalink restore below so a restored hash value drives Apply like any
	// other input.
	adoptDescControl(ControlDesc{ID: "camera-zoom", Label: "Zoom", Min: -95, Max: 95, Step: 1, Def: 0,
		Signed: true, PermaKey: "z", LEDID: "slider-value-zoom", ResetID: "rst-zoom",
		Apply: func(v float64) { cachedZoom = float32(v) },
		ResetExtra: func() {
			defaultCameraDist = initCameraDist
			updateViewMatrix()
			syncKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "pan-x", Label: "X", Min: -8, Max: 8, Step: 1, Def: 0,
		Signed: true, PermaKey: "px", LEDID: "slider-value-panx", ResetID: "rst-panx",
		Apply: func(v float64) { cachedPanX = float32(v) }})
	adoptDescControl(ControlDesc{ID: "pan-y", Label: "Y", Min: -8, Max: 8, Step: 1, Def: 0,
		Signed: true, PermaKey: "py", LEDID: "slider-value-pany", ResetID: "rst-pany",
		Apply: func(v float64) { cachedPanY = float32(v) }})
	adoptDescControl(ControlDesc{ID: "rainbow-freq", Label: "period", Min: 0.05, Max: 20, Step: 0.05, Def: 1,
		PermaKey: "rf", LEDID: "slider-value-rfreq", ResetID: "rst-rfreq",
		Apply: func(v float64) { gradientFreq = float32(v) }})
	// Speed: the slider runs log10 (-2..2) while the LED shows the effective
	// multiplier (0.01..100), whole sub-step counts at ≥1 — the one mapping
	// pair below is the SSOT both directions.
	adoptDescControl(ControlDesc{ID: "speed-slider", Label: "Speed", Min: -2, Max: 2, Step: 0.1, Def: 0,
		PermaKey: "sp", LEDID: "slider-value-speed", ResetID: "rst-speed",
		Apply:       applySpeedLog,
		SliderToVal: speedDisplayVal,
		ValToSlider: func(v float64) float64 {
			if v <= 0 {
				return -2
			}
			lg := math.Log10(v)
			if lg < -2 {
				lg = -2
			}
			if lg > 2 {
				lg = 2
			}
			return lg
		},
		LEDMin: 0.01, LEDMax: 100, LEDStep: 0.1,
		ResetExtra: syncKnobs})
	// Line width: WebGL's gl.lineWidth() is capped at 1.0 on most modern
	// browsers/drivers (Chrome enforces it; many ANGLE / Mesa stacks too) —
	// the call still runs, but the visual effect is implementation-dependent.
	adoptDescControl(ControlDesc{ID: "line-width", Label: "Line", Min: 1, Max: 10, Step: 1, Def: 1,
		PermaKey: "lw", LEDID: "slider-value-line", ResetID: "rst-line",
		Apply: func(v float64) {
			if v < 1 {
				v = 1
			}
			gl.Call("lineWidth", v)
		}})
	adoptDescControl(ControlDesc{ID: "trail-slider", Label: "Trail", Min: 1000, Max: 500000, Step: 1000, Def: 20000,
		PermaKey: "tr", LEDID: "slider-value-trail", ResetID: "rst-trail",
		Apply: func(v float64) {
			newSteps := int(v)
			if newSteps != steps {
				steps = newSteps
				vertBuf = make([]float32, steps*4)
				jsVertUint8 = js.Global().Get("Uint8Array").New(steps * 4 * 4)
				buf := jsVertUint8.Get("buffer")
				jsVertFloat = js.Global().Get("Float32Array").New(buf, 0, steps*4)
				resetAttractorState()
				refreshGradient()
			}
		},
		ResetExtra: func() {
			// Resetting the trail also drops persist mode (matches the old
			// bespoke reset: a persisted trail makes the new length invisible).
			persistTrail = false
			doc.Call("getElementById", "persist-trail").Set("checked", false)
		}})
	// Model Out (sonification): trace-rate knob on the generators' concert-
	// pitch semitone scale (the LED shows Hz both directions via the mapping
	// pair), plus output level. The MAP ring is wired in buildSonifyModule.
	adoptDescControl(ControlDesc{ID: "sonify-freq", Label: "trace", Min: 0, Max: float64(genSemitones), Step: 1, Def: 24,
		PermaKey: "sf", LEDID: "sonify-led", ResetID: "",
		Apply:       func(v float64) { sonifyHz = freqFromKnob(v) },
		SliderToVal: sonifyFreqFromSlider,
		ValToSlider: sonifySliderFromFreq,
		LEDMin:      genFreqLo, LEDMax: genFreqHi, LEDStep: 1})
	adoptDescControl(ControlDesc{ID: "sonify-lvl", Label: "lvl", Min: 0, Max: 100, Step: 1, Def: 60,
		PermaKey: "sv", LEDID: "sonify-lvl-led", ResetID: "",
		Apply: func(v float64) { sonifyLevel = v / 100 }})

	// View spin rates: the last controls on the legacy wiring path. Reset
	// also zeroes the axis ANGLE state and rebuilds the matrices (parity
	// with the old bespoke rst-rx/ry/rz handlers).
	adoptDescControl(ControlDesc{ID: "rotation-controls-x", Label: "X rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "rx", LEDID: "slider-value-x", ResetID: "rst-rx",
		Apply: func(v float64) { cachedRotX = float32(v) },
		ResetExtra: func() {
			rotationX, rotationX1, angleX = 0, 0, 0
			rebuildModelMatrix()
			updateModelMatrix()
			updateRotKnobs()
			syncKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "rotation-controls-y", Label: "Y rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "ry", LEDID: "slider-value-y", ResetID: "rst-ry",
		Apply: func(v float64) { cachedRotY = float32(v) },
		ResetExtra: func() {
			rotationY, rotationY1, angleY = 0, 0, 0
			clearAutoRotateFlag() // Y spin (incl. auto) just zeroed
			rebuildModelMatrix()
			updateModelMatrix()
			updateRotKnobs()
		}})
	adoptDescControl(ControlDesc{ID: "rotation-controls-z", Label: "Z rate", Min: -1, Max: 1, Step: 0.1, Def: 0,
		Signed: true, PermaKey: "rz", LEDID: "slider-value-z", ResetID: "rst-rz",
		Apply: func(v float64) { cachedRotZ = float32(v) },
		ResetExtra: func() {
			rotationZ, rotationZ1, angleZ = 0, 0, 0
			rebuildModelMatrix()
			updateModelMatrix()
			updateRotKnobs()
		}})

	// Prime every registry control once: run its Apply from the DOM's current
	// value so engine state matches the panel by construction (the old code
	// did this ad hoc — applyLineWidth() at wiring, readSliderCache, …).
	for _, c := range builtControls {
		c.slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}

	// Permalink: capture pristine control defaults, restore any state
	// encoded in the URL hash, then keep the hash in sync with the live
	// state so the current view is always shareable.
	capturePermaDefaults()
	applyStateFromHash()
	startPermalinkSync()

	// Final tooltip pass now that every selector (gradient / model / style) is
	// built — some are created after the first buildParamPanel's annotate.
	annotateControlTooltips()

	// Start animation loop
	done := make(chan struct{})
	renderFrame = trackedFuncOf(renderLoop)
	js.Global().Call("requestAnimationFrame", renderFrame)

	// Set initial trail-controls visibility for the starting mode.
	updateTrailVisibility()

	// Wire input listeners for the fixed sliders so the cached vars
	// + visible text output stay in sync with user interaction. The
	// renderLoop reads cachedZoom/RotX/Y/Z instead of polling
	// parseFloat per frame.

	// (Speed / Line / Trail LED treatment is owned by their ControlDesc
	// registrations below.)

	// Numeric input → slider: typing a value (committed on Enter/blur)
	// drives the paired slider, reusing its input handler for all the
	// downstream work. Uses "change" (not "input") so the slider handler
	// writing back the formatted value doesn't fight per-keystroke typing.
	// toSlider maps the typed number to the slider's raw value (identity
	// for most; log10 for the speed slider, whose display is the effective
	// multiplier).
	// (line / trail / speed typed entry is owned by their ControlDesc
	// registrations — speed's log10 mapping lives in its ValToSlider.)

	// Line-width slider. WebGL's gl.lineWidth() is capped at 1.0
	// on most modern browsers/drivers (Chrome enforces it; many
	// platforms' ANGLE / Mesa drivers do too) — the call still
	// runs but visual effect is implementation-dependent. If the
	// stack honors it, the slider gives thicker line strokes; if
	// not, this is a harmless no-op for values >1. Wheel-binding
	// for "line-width" is registered in the bindWheelToInput loop
	// further down.
	// (Line width's input/reset wiring is owned by its ControlDesc registration.)

	// Optional host-injected nav snippet (e.g. m2 links to /attractors).
	if ExtraNavHTML != "" {
		nav := doc.Call("getElementById", "extra-nav")
		if nav.Truthy() {
			nav.Set("innerHTML", ExtraNavHTML)
		}
	}

	// Inject a host-page CSS rule killing vertical scroll caused by
	// the controls panel growing the body height. Targets the actual
	// magnetosphere.net symptom (~1cm of overflow) without breaking
	// pages that legitimately scroll — Run() is only invoked on the
	// animation page.
	noScrollStyle := doc.Call("createElement", "style")
	noScrollStyle.Set("textContent", "html,body{overflow:hidden!important;margin:0;padding:0;}")
	doc.Get("head").Call("appendChild", noScrollStyle)

	// Random initial orientation + low-rate rotation so the model doesn't start
	// in the same pose every load — UNLESS the permalink pinned an explicit pose
	// (&rot / &drag), in which case that must win so a shared still-view link is
	// restored faithfully. Must run AFTER the rotation-controls-x/y/z elements
	// are created and queried.
	if !hashPinnedPose {
		randomizeOrientation()
		// randomizeOrientation zeroed the rate sliders — put back any spin
		// rates the permalink explicitly pinned (&rx/&ry/&rz).
		for ax, v := range hashPinnedSpin {
			if sl := doc.Call("getElementById", "rotation-controls-"+ax); sl.Truthy() {
				sl.Set("value", v)
				sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
		if len(hashPinnedSpin) > 0 {
			syncKnobs()
		}
	} else {
		// A pinned pose (&rot) is by construction a STILL view — rot is only
		// serialized when auto-rotate is off and all spin rates are zero — so
		// clear any residual spin rate (e.g. the auto-rotate Y contribution the
		// restored ar=0 subtracted from an unzeroed base) so the restored view
		// doesn't drift away from the shared pose.
		for _, ax := range []string{"x", "y", "z"} {
			if sl := doc.Call("getElementById", "rotation-controls-"+ax); sl.Truthy() && sl.Get("value").String() != "0" {
				sl.Set("value", "0")
				sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
	}
	// The spectrogram wants a static, face-on default instead — undo the
	// randomized pose/spin for an initial #spectrogram load (mode switches
	// go through onModeChange, which already handles this).
	if isSpectroSurface(selectedMode) {
		setSpectrogramCamera()
	}

	// randomizeOrientation zeroed the spin rates; if auto-rotate is on
	// (default, or per the permalink), re-apply its Y-rate contribution so
	// the model actually spins and the Y rate knob reflects it.
	if selectedMode != "spectrogram" && selectedMode != "fvf" && autoRotate {
		autoRotate = false
		setAutoRotate(true)
	}

	// Wheel-on-input: scroll over a range/number input adjusts its
	// value by `step` instead of scrolling the host page. Fires the
	// input event so the existing listener pipelines react.
	bindWheelEl := func(el js.Value) {
		if !el.Truthy() {
			return
		}
		el.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			deltaY := e.Get("deltaY").Float()
			step, _ := strconv.ParseFloat(el.Get("step").String(), 64)
			if step == 0 {
				step = 1
			}
			cur, _ := strconv.ParseFloat(el.Get("value").String(), 64)
			if deltaY < 0 {
				cur += step
			} else {
				cur -= step
			}
			if minS := el.Get("min").String(); minS != "" {
				if mn, err := strconv.ParseFloat(minS, 64); err == nil && cur < mn {
					cur = mn
				}
			}
			if maxS := el.Get("max").String(); maxS != "" {
				if mx, err := strconv.ParseFloat(maxS, 64); err == nil && cur > mx {
					cur = mx
				}
			}
			el.Set("value", strconv.FormatFloat(cur, 'f', -1, 64))
			evtInit := js.Global().Get("Object").New()
			evtInit.Set("bubbles", true)
			evt := js.Global().Get("Event").New("input", evtInit)
			el.Call("dispatchEvent", evt)
			return nil
		}))
	}
	bindWheelToInput := func(id string) {
		bindWheelEl(doc.Call("getElementById", id))
	}
	for _, id := range []string{
		"camera-zoom", "rotation-controls-x", "rotation-controls-y",
		"rotation-controls-z", "speed-slider", "trail-slider", "line-width",
	} {
		bindWheelToInput(id)
	}

	// Wheel-on-select: hover-scroll over a <select> cycles its
	// selectedIndex (option groups skipped automatically — they
	// aren't part of .options). Fires the change event so existing
	// listeners (mode-select onModeChange, gradient-type handler)
	// react as if the user clicked.
	bindWheelToSelect := func(id string) {
		el := doc.Call("getElementById", id)
		if !el.Truthy() {
			return
		}
		el.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			e := args[0]
			e.Call("preventDefault")
			idx := el.Get("selectedIndex").Int()
			n := el.Get("options").Get("length").Int()
			if e.Get("deltaY").Float() > 0 {
				idx++
			} else {
				idx--
			}
			if idx < 0 {
				idx = 0
			}
			if idx >= n {
				idx = n - 1
			}
			el.Set("selectedIndex", idx)
			evtInit := js.Global().Get("Object").New()
			evtInit.Set("bubbles", true)
			evt := js.Global().Get("Event").New("change", evtInit)
			el.Call("dispatchEvent", evt)
			return nil
		}))
	}
	bindWheelToSelect("cat-select")
	bindWheelToSelect("model-select")
	bindWheelToSelect("gradient-source")
	bindWheelToSelect("gradient-colors")
	// Also wheel-bind every numeric input the param panel builds.
	// Re-invoke after each buildParamPanel so newly-added inputs get
	// wired too — wrap the existing helper into a package-level
	// rebinder we can call from buildParamPanel.
	rebindParamWheel = func() {
		params := doc.Call("getElementById", "params")
		if !params.Truthy() {
			return
		}
		inputs := params.Call("querySelectorAll", "input[type=range], input[type=number]")
		for i := 0; i < inputs.Length(); i++ {
			bindWheelEl(inputs.Index(i)) // by element — the number boxes have no id
		}
	}
	rebindParamWheel()

	// Window resize: keep canvas pixel dimensions in sync with the
	// viewport so the model doesn't get stretched when devtools opens
	// or closes (or on phone orientation change).
	js.Global().Call("addEventListener", "resize", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if sizeCanvasToViewport() {
			gl.Call("viewport", 0, 0, width, height)
			setupMatrices()
		}
		return nil
	}))

	// Clock goroutine
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if !rtc.IsUndefined() {
				rtc.Set("innerHTML", time.Now().Format("2006-01-02 15:04:05"))
			}
		}
	}()

	// Debug stats reporter goroutine
	if debugEnabled {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				postDebugStats()
			}
		}()
	}

	<-done
}

func postDebugStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	avgMs := float32(0)
	fps := float32(0)
	if frameCount > 0 {
		avgMs = frameTotalMs / float32(frameCount)
		fps = 1000.0 / avgMs
	}

	payload := fmt.Sprintf(
		`{"mode":"%s","paused":%t,"fps":%.1f,"frame_avg_ms":%.2f,"frame_min_ms":%.2f,"frame_max_ms":%.2f,"frame_count":%d,"speed_steps":%d,"speed_scale":%.4f,"trail_steps":%d,"heap_alloc_mb":%.2f,"heap_sys_mb":%.2f,"heap_objects":%d,"gc_runs":%d,"goroutines":%d}`,
		selectedMode, paused, fps, avgMs, frameMinMs, frameMaxMs, frameCount,
		speedSteps, speedScale, steps,
		float64(ms.HeapAlloc)/1048576, float64(ms.HeapSys)/1048576,
		ms.HeapObjects, gcRunsCount(&ms), runtime.NumGoroutine(),
	)

	// Reset frame stats for next interval
	frameCount = 0
	frameTotalMs = 0
	frameMinMs = 999
	frameMaxMs = 0

	// Post via fetch
	headers := js.Global().Get("Headers").New()
	headers.Call("set", "Content-Type", "application/json")
	opts := js.Global().Get("Object").New()
	opts.Set("method", "POST")
	opts.Set("headers", headers)
	opts.Set("body", payload)
	js.Global().Call("fetch", "/debug/stats", opts)
}

// Per-attractor initial conditions — defaults to (0.1, 0.5, -0.6) for most.
var attractorDescriptions = map[string]string{
	"lorenz": "Lorenz Attractor — Discovered by Edward Lorenz in 1963 while modeling atmospheric convection. " +
		"The butterfly-shaped trajectory arises from a simplified system of three coupled differential equations. " +
		"It was one of the first systems shown to exhibit deterministic chaos, where tiny differences in initial conditions lead to vastly different outcomes.\n\n" +
		"dx/dt = σ(y − x)\ndy/dt = x(ρ − z) − y\ndz/dt = xy − βz",
	"rossler": "Rössler Attractor — Proposed by Otto Rössler in 1976 as a simpler system that produces chaotic behavior. " +
		"Unlike the Lorenz system's two-lobed shape, the Rössler attractor has a single folded-band structure with an outward spiral that occasionally makes a large excursion in the z-direction.\n\n" +
		"dx/dt = −(y + z)\ndy/dt = x + ay\ndz/dt = b + z(x − c)",
	"chua": "Chua's Circuit (Double Scroll Attractor) — Invented by Leon Chua in 1983, this is the first electronic circuit proven to exhibit chaos. " +
		"The system features a piecewise-linear nonlinearity (the Chua diode) that creates the characteristic double-scroll pattern. " +
		"It is also the basis for multi-scroll attractor generalizations.\n\n" +
		"dx/dt = α(y − x − h(x))\ndy/dt = x − y + z\ndz/dt = −βy\nh(x) = m₁x + ½(m₀ − m₁)(|x+1| − |x−1|)",
	"aizawa": "Aizawa Attractor — A chaotic system that produces a toroidal structure with a tendril extending from the center. " +
		"The attractor has a visually striking shape that resembles a sphere with a tail, exhibiting both rotational symmetry and chaotic wandering.\n\n" +
		"dx/dt = (z − b)x − dy\ndy/dt = dx + (z − b)y\ndz/dt = c + az − z³/3 − (x² + y²)(1 + ez) + fzx³",
	"sprott": "Sprott Attractor — One of many simple chaotic systems cataloged by Julien Clinton Sprott. " +
		"These systems were discovered through systematic computer searches for chaotic flows with minimal terms, demonstrating that chaos can arise from remarkably simple equations.\n\n" +
		"dx/dt = y + Axy + xz\ndy/dt = 1 − Bx² + yz\ndz/dt = x − x² − y²",
	"lissajou": "Lissajous Curve — Named after Jules Antoine Lissajous (1822–1880), these are parametric curves formed by combining sinusoidal motions along each axis. " +
		"Not a chaotic system — the curves are periodic and their shape depends on the frequency ratios and phase relationships between the three oscillations.\n\n" +
		"x(t) = sin(at)\ny(t) = sin(bt)\nz(t) = sin(ct)",
	"thomas": "Thomas' Cyclically Symmetric Attractor — Introduced by René Thomas, this system has the elegant property of cyclic symmetry: each variable is damped and driven by the sine of the next variable in the cycle. " +
		"The parameter b controls dissipation; as b decreases the system transitions from stable points through limit cycles to chaos.\n\n" +
		"dx/dt = −bx + sin(y)\ndy/dt = −by + sin(z)\ndz/dt = −bz + sin(x)",
	"halvorsen": "Halvorsen Attractor — A chaotic system with three-fold rotational symmetry, producing a distinctive pinwheel-like shape. " +
		"The attractor consists of three intertwined lobes that spiral around each other, creating a visually complex but structurally symmetric trajectory.\n\n" +
		"dx/dt = −ax − 4y − 4z − y²\ndy/dt = −ay − 4z − 4x − z²\ndz/dt = −az − 4x − 4y − x²",
	"chen": "Chen Attractor — Discovered by Guanrong Chen in 1999, this system was found as a dual of the Lorenz system in a specific mathematical sense. " +
		"It exhibits chaotic behavior with a distinctive two-scroll structure that differs from both the Lorenz and Rössler attractors.\n\n" +
		"dx/dt = a(y − x)\ndy/dt = (c − a)x − xz + cy\ndz/dt = xy − bz",
	"dadras": "Dadras Attractor — A three-dimensional autonomous chaotic system with five parameters, introduced by Sara Dadras and Hamid Reza Momeni. " +
		"The system exhibits rich dynamical behavior including period-doubling routes to chaos.\n\n" +
		"dx/dt = y − px + qyz\ndy/dt = ry − xz + z\ndz/dt = sxy − ez",
	"rabinovich": "Rabinovich-Fabrikant Attractor — Derived by Mikhail Rabinovich and Anatoly Fabrikant from physical equations modeling the stochasticity of three interacting waves. " +
		"The system is known for its complex topology and extreme sensitivity to parameters, producing intricate folded structures.\n\n" +
		"dx/dt = y(z − 1 + x²) + γx\ndy/dt = x(3z + 1 − x²) + γy\ndz/dt = −2z(α + xy)",
	"burkeshaw": "Burke-Shaw Attractor — Introduced by Bill Burke and Robert Shaw, this system exhibits chaotic behavior with a distinctive two-winged structure. " +
		"It arises from the study of nonlinear dynamics and produces complex trajectories confined to a compact region of phase space.\n\n" +
		"dx/dt = −S(x + y)\ndy/dt = −y − Sxz\ndz/dt = Sxy + V",
	"tetrahedron":   "Tetrahedron — The simplest Platonic solid, with 4 triangular faces, 6 edges, and 4 vertices. It is its own dual.",
	"cube":          "Cube (Hexahedron) — A Platonic solid with 6 square faces, 12 edges, and 8 vertices. Its dual is the octahedron.",
	"octahedron":    "Octahedron — A Platonic solid with 8 triangular faces, 12 edges, and 6 vertices. Its dual is the cube.",
	"dodecahedron":  "Dodecahedron — A Platonic solid with 12 pentagonal faces, 30 edges, and 20 vertices. Its dual is the icosahedron.",
	"icosahedron":   "Icosahedron — A Platonic solid with 20 triangular faces, 30 edges, and 12 vertices. Its dual is the dodecahedron.",
	"nestedcube":    "Nested Cube — A cube within a cube, connected at the vertices, illustrating the relationship between inner and outer geometric structures.",
	"globe":         "Globe — A wireframe sphere showing lines of latitude and longitude, similar to the graticule on a geographic globe. Latitude lines are horizontal circles parallel to the equator, longitude lines are great circles passing through the poles.",
	"sphere":        "Sphere — A perfectly round three-dimensional surface where every point is equidistant from the center. Generated as a UV sphere with configurable latitude and longitude subdivisions.",
	"torus":         "Torus — A doughnut-shaped surface of revolution generated by revolving a circle (radius r) around an axis at distance R from the center of the circle.",
	"magnetosphere": "Magnetosphere — A visualization of magnetic field lines surrounding a dipole, similar to Earth's magnetosphere that shields the planet from solar wind.",
}

func resetAttractorState() {
	reseedAttractorState()
	// Hyper-Rössler gets a warmup onto its attractor for EXPLICIT resets
	// (mode entry, reset buttons) — but NOT from checkDiverged: with
	// divergent parameters the warmup itself diverges, and re-running its
	// 60k steps on every diverging sub-step froze the page for minutes
	// (found by the demo recorder, bisected to hyperrossler param edits).
	if selectedMode == "hyperrossler" {
		hyperRosslerWarmup()
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
	// Hyper-Rössler's hidden 4th state; start it on-attractor for that mode,
	// zero otherwise (harmless — only that mode reads it).
	if selectedMode == "hyperrossler" {
		hyperW = 10
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

// ── UI helpers ───────────────────────────────────────────────────────────────

// sizeLEDField fixes a numeric input's width to the widest value it can show
// (sign + max integer digits + dot + dec) and right-aligns it, so it never
// resizes and unsigned/positive values reserve the sign column as a blank.
func sizeLEDField(el js.Value, min, max float64, dec int, signed bool) {
	chars := intDigits(max)
	if d := intDigits(min); d > chars {
		chars = d
	}
	if dec > 0 {
		chars += 1 + dec // decimal point + fraction
	}
	if signed {
		chars++ // sign column
	}
	// DSEG7 is fixed-width, so size in ch (glyph widths) plus the field's own
	// box-model overhead. The inputs are border-box with ~3px padding + ~1px
	// border per side (~8px total), so the added slack must cover that or the
	// widest value clips (e.g. a 7-digit "20480.0"); 9px clears it with a hair to
	// spare.
	st := el.Get("style")
	st.Set("width", "calc("+strconv.Itoa(chars)+"ch + 9px)")
	st.Set("textAlign", "right")
}

// wheelNudge makes scrolling over an LED readout step the paired (usually
// hidden) slider by `step`, clamped to [mn,mx]; the slider's own input handler
// then reformats the readout, so the LED stays formatted.
func wheelNudge(readout, slider js.Value, step, mn, mx float64) {
	if step == 0 {
		step = 1
	}
	readout.Call("addEventListener", "wheel", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		e.Call("preventDefault")
		e.Call("stopPropagation")
		v, _ := strconv.ParseFloat(slider.Get("value").String(), 64)
		if e.Get("deltaY").Float() < 0 {
			v += step
		} else {
			v -= step
		}
		if v < mn {
			v = mn
		}
		if v > mx {
			v = mx
		}
		slider.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		slider.Call("dispatchEvent", js.Global().Get("Event").New("input"))
		return nil
	}))
}

// buildParamUnit builds one parameter "unit": the control (label · knob ·
// value · step · reset) with its MOD/LVL half always beneath it (dimmed when
// Audio mod is off — never reflows). Returns the unit element.
func buildParamUnit(p paramDef) js.Value {
	dec := ledDecimals(float64(p.Step))
	signed := p.Min < 0
	intDig := ledIntDigits(float64(p.Min), float64(p.Max))
	stepStr := strconv.FormatFloat(float64(p.Step), 'g', -1, 32)
	minStr := strconv.FormatFloat(float64(p.Min), 'g', -1, 32)
	maxStr := strconv.FormatFloat(float64(p.Max), 'g', -1, 32)

	unit := doc.Call("createElement", "div")
	unit.Set("className", "punit")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", labelIsSym(p.Label))) // symbols keep case; words uppercase
	lbl.Set("textContent", p.Label)

	slider := doc.Call("createElement", "input")
	slider.Set("type", "range")
	slider.Set("id", p.ID)
	slider.Set("min", minStr)
	slider.Set("max", maxStr)
	slider.Set("step", stepStr) // set before value so the thumb isn't snapped
	slider.Set("value", strconv.FormatFloat(float64(*p.Value), 'g', -1, 32))
	slider.Set("title", p.Label+" — attractor parameter (range "+minStr+" … "+maxStr+")")
	slider.Set("style", "display:none;")

	// The cell is owned by a Control that holds the value source (slider), its
	// default, and its LED format — so reset and readout formatting are the
	// Control's methods instead of inline closures / scattered handlers.
	ctl := &Control{
		module: "Params", kind: kindGeneric, cell: unit, slider: slider, def: p.Def,
		ledInt: intDig, ledDec: dec, ledSign: signed, permaKey: "p." + paramKey(p.ID),
	}
	paramControls = append(paramControls, ctl)

	numInput := doc.Call("createElement", "input")
	numInput.Set("type", "text") // LED display: keeps +/- and trailing zeros
	numInput.Set("inputmode", "decimal")
	numInput.Set("min", minStr)
	numInput.Set("max", maxStr)
	numInput.Set("step", stepStr)
	numInput.Set("value", ctl.formatValue(float64(*p.Value)))
	numInput.Set("className", "numin u-val")
	sizeLEDField(numInput, float64(p.Min), float64(p.Max), dec, signed)

	slider.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(slider.Get("value").String(), 64); err == nil {
			*p.Value = float32(val)
			numInput.Set("value", ctl.formatValue(val))
			staticGeomDirty = true
			resetAttractorState()
			refreshGradient()
		}
		return nil
	}))
	numInput.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(numInput.Get("value").String(), 64); err == nil {
			*p.Value = float32(val)
			slider.Set("value", strconv.FormatFloat(val, 'g', -1, 64))
			staticGeomDirty = true
			resetAttractorState()
			refreshGradient()
		}
		return nil
	}))
	// Scroll over the LED readout steps the value (drives the slider, which
	// reformats the readout).
	wheelNudge(numInput, slider, float64(p.Step), float64(p.Min), float64(p.Max))

	rst := doc.Call("createElement", "button")
	rst.Set("className", "rst")
	rst.Set("title", "Reset "+p.Label)
	rst.Set("textContent", "↺")
	rst.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		ctl.resetToDefault()
		return nil
	}))

	stepInput := doc.Call("createElement", "input")
	stepInput.Set("type", "number")
	stepInput.Set("min", "0.0000001")
	stepInput.Set("step", "any")
	stepInput.Set("value", stepStr)
	stepInput.Set("title", "Step size for "+p.Label+" — how much one knob step changes the value")
	stepInput.Set("className", "numin u-step")
	stepInput.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if val, err := strconv.ParseFloat(stepInput.Get("value").String(), 64); err == nil && val > 0 {
			newStep := strconv.FormatFloat(val, 'g', -1, 64)
			slider.Set("step", newStep)
			numInput.Set("step", newStep)
		}
		return nil
	}))

	// Standard cell header: label pinned left, numeric LED centered over the knob,
	// reset pinned right — all on one line above the knob (see .rst CSS).
	top := doc.Call("createElement", "span")
	top.Set("className", "punit-top")
	top.Call("appendChild", lbl)
	top.Call("appendChild", numInput)
	unit.Call("appendChild", top)
	unit.Call("appendChild", slider) // hidden, drives the value
	unit.Call("appendChild", makeKnob(slider, numInput, true, false, true))
	unit.Call("appendChild", rst) // pinned top-right by CSS
	unit.Call("appendChild", stepInput)
	return unit
}

// buildModCard builds one card for the Modulation module: the target's name
// above its MOD/LVL control (channel + level). All modulation controls live in
// the dedicated Modulation module, never mixed into other modules.
func buildModCard(id, label string, sym bool) js.Value {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", sym))
	lbl.Set("textContent", label)
	card.Call("appendChild", lbl)
	card.Call("appendChild", buildModUnit(id, label))
	return card
}

func buildParamPanel(mode string) {
	// Free the previous build's listener closures, then collect this build's
	// (see funcarena_js.go) — the wipe below kills their DOM in the same
	// synchronous pass.
	releasePanelFuncs()
	panelCollect = true
	defer func() { panelCollect = false }()

	paramsDiv := doc.Call("getElementById", "params")
	paramsDiv.Set("innerHTML", "")
	paramsDiv.Set("className", "row")
	paramControls = paramControls[:0] // rebuilt below by buildParamUnit
	// The panel's am-on/am-off class shows/hides the adjacent Modulation module.
	if panel := doc.Call("getElementById", "controls-panel"); panel.Truthy() {
		cl := panel.Get("classList")
		if audioMod {
			cl.Call("add", "am-on")
			cl.Call("remove", "am-off")
		} else {
			cl.Call("add", "am-off")
			cl.Call("remove", "am-on")
		}
	}

	// Mode-scoped scope extras (run before any early return so they clean up on
	// every mode change): GA waveform switches + the CRT overlay.
	syncGAWaveSwitches(mode)
	updateCRTOverlay()

	// The Equation module only exists in Custom mode (buildCustomPanel makes it).
	if mode != "custom" {
		if em := doc.Call("getElementById", "eqn-module"); em.Truthy() {
			em.Get("parentNode").Call("removeChild", em)
		}
	}

	if mode == "custom" {
		buildCustomPanel(paramsDiv)
		if rebindParamWheel != nil {
			rebindParamWheel()
		}
		quantizeModuleWidths() // Equation + Parameters modules
		return
	}

	params, ok := attractorParams[mode]
	if !ok || len(params) == 0 {
		return
	}

	// Lay the params out as units in a grid: vertical groups of 3 (3 rows),
	// flowing into as many columns as there are params. The enclosing section
	// is the equipment module (its header names it).
	grid := doc.Call("createElement", "div")
	gridCls := "punit-grid"
	if len(params) > 3 { // wraps into 2 columns → draw a divider between them
		gridCls += " two-col"
	}
	grid.Set("className", gridCls)
	for _, p := range params {
		grid.Call("appendChild", buildParamUnit(p))
	}
	paramsDiv.Call("appendChild", grid)

	if mode == "fvf" {
		// Into the GRID, not #params: the grid is the height-bounded
		// column-wrap container, so extra cells flow into a new column and the
		// width quantizer widens the module. Appended to #params they stacked
		// BELOW the grid and were clipped by the module's fixed height.
		appendFVFSelectors(grid) // wave + modulator selector knobs + FX/Listen
	}

	// Audio-modulation controls live in per-group MOD + EQ modules, each pair
	// inserted right after the primary module it modulates and shown only while
	// Audio mod is on (CSS: .am-off hides .modmodule/.eqmodule).
	buildModEQModules(params)
	if rebindParamWheel != nil {
		rebindParamWheel()
	}
	quantizeModuleWidths()    // param count changed → re-snap module widths
	annotateControlTooltips() // role-aware tooltips (labels/readouts/swatches)
}

// modTarget is one modulatable control (its paramMods key + display label).
type modTarget struct {
	id, label string
	sym       bool // label is a math parameter symbol (kept lowercase), not a word
}

// buildModEQModules (re)builds, for each primary module that has modulatable
// controls, an adjacent MOD module (channel/level knobs) and EQ module (graphic-
// EQ band painters), aligned row-for-row with the primary. Params come from the
// current mode; the view/camera/color targets are fixed. Only shown when Audio
// mod is on.
func buildModEQModules(params []paramDef) {
	old := doc.Call("querySelectorAll", ".modmodule, .eqmodule")
	for i := old.Get("length").Int() - 1; i >= 0; i-- {
		n := old.Index(i)
		n.Get("parentNode").Call("removeChild", n)
	}
	var pTargets []modTarget
	for _, p := range params {
		if decimalsForStep(p.Step) > 0 {
			pTargets = append(pTargets, modTarget{p.ID, p.Label, labelIsSym(p.Label)})
		}
	}
	groups := []struct {
		hdr     string
		targets []modTarget
	}{
		{"params", pTargets},
		{"Colors", []modTarget{{"view-rfreq", "rainbow", false}, {"view-trail", "trail", false}}},
		{"View", []modTarget{{"view-spinx", "spin X", false}, {"view-spiny", "spin Y", false}, {"view-spinz", "spin Z", false}}},
		// Order must match the Position control panel (X, Y, Zoom) so each MOD/EQ
		// row lines up with the control it drives ("Pan" dropped — implied by the
		// Position group).
		{"Position", []modTarget{{"view-panx", "X", false}, {"view-pany", "Y", false}, {"view-zoom", "zoom", false}}},
	}
	findSect := func(hdr string) js.Value {
		s := doc.Call("querySelectorAll", ".modules > .sect")
		for i := 0; i < s.Get("length").Int(); i++ {
			m := s.Index(i)
			if h := m.Call("querySelector", ".sect-hdr"); h.Truthy() && h.Get("textContent").String() == hdr {
				return m
			}
		}
		return js.Undefined()
	}
	makeMod := func(cls, title string, cards []js.Value) js.Value {
		mod := doc.Call("createElement", "div")
		mod.Set("className", "sect "+cls)
		h := doc.Call("createElement", "div")
		h.Set("className", "sect-hdr")
		h.Set("textContent", title)
		mod.Call("appendChild", h)
		g := doc.Call("createElement", "div")
		gc := "punit-grid"
		if len(cards) > 3 {
			gc += " two-col"
		}
		g.Set("className", gc)
		for _, c := range cards {
			g.Call("appendChild", c)
		}
		mod.Call("appendChild", g)
		return mod
	}
	for _, grp := range groups {
		if len(grp.targets) == 0 {
			continue
		}
		primary := findSect(grp.hdr)
		if !primary.Truthy() {
			continue
		}
		var modCards, eqCards []js.Value
		for _, t := range grp.targets {
			modCards = append(modCards, buildModCard(t.id, t.label, t.sym))
			eqCards = append(eqCards, buildEQCard(t.id, t.label, t.sym))
		}
		// Short, single-line headers so the module content starts at the same Y
		// as its primary (a wrapped 2-line header would push the knobs down).
		modMod := makeMod("modmodule", "mod", modCards)
		eqMod := makeMod("eqmodule", "eq", eqCards)
		parent := primary.Get("parentNode")
		parent.Call("insertBefore", modMod, primary.Get("nextSibling"))
		parent.Call("insertBefore", eqMod, modMod.Get("nextSibling"))
	}
}

// buildEQCard is one graphic-EQ band-painter card for the EQ module.
func buildEQCard(id, label string, sym bool) js.Value {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", sym))
	lbl.Set("textContent", label)
	card.Call("appendChild", lbl)
	card.Call("appendChild", makeEQStrip(id))
	return card
}

func hexToRGB(hex string) (float32, float32, float32) {
	if len(hex) < 7 {
		return 1, 1, 1
	}
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	return float32(r) / 255.0, float32(g) / 255.0, float32(b) / 255.0
}

// ── Event handlers ───────────────────────────────────────────────────────────

func refreshGradient() {
	if shadersReady && len(attractorVertices) > 0 {
		updateGradientRange(attractorVertices)
	}
}

// standalonePanel is true when the controls are our own fixed overlay (not
// appended into a host page's <footer>); only then can we dock/move them.
var standalonePanel bool
var dockEdge = "bottom"
var dockSizeH float64 // panel height (px) when docked bottom/top
var dockSizeW = 360.0 // panel width (px) when docked left/right
var resizeHandle js.Value
var resizing bool

// panelScale mirrors the CSS --kscale (interface size). Both the Size knob and
// the bottom/top dock resize-drag drive it, so "resize the panel" scales the
// whole control interface — which works even though every module is a
// fixed-height 3-row grid (a plain height drag could only clip it).
var panelScale = 1.0

// setKScale sets the interface scale (clamped), persists it, and re-snaps the
// module widths + resize bar to the new size.
func setKScale(v float64) {
	if v < 0.6 {
		v = 0.6
	} else if v > 2.2 {
		v = 2.2
	}
	panelScale = v
	doc.Get("documentElement").Get("style").Call("setProperty", "--kscale", strconv.FormatFloat(v, 'f', 3, 64))
	quantizeModuleWidths()
	positionResizeHandle()
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", "wasmstuff-kscale", strconv.FormatFloat(v, 'f', 3, 64))
	}
}

// Float mode: a detached, draggable, resizable portrait window (phone-shaped by
// default) that can sit anywhere over the full-screen model.
var (
	floatX               = 24.0
	floatY               = 48.0
	floatW               = 384.0
	floatH               = 720.0
	floatTitle           js.Value
	floatGrip            js.Value
	floatDragging        bool
	floatResizing        bool
	floatOffX, floatOffY float64
)

func pxStr(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }

// clampFloatPos keeps the float's title bar reachable within the current
// viewport. Lives inside positionFloat so EVERY path that moves the float —
// dragging, a persisted position restored into a smaller window, a window
// resize — goes through the one clamp; a stale localStorage position can
// never strand the panel off-screen.
func clampFloatPos() {
	if floatX < 8-floatW+120 {
		floatX = 8 - floatW + 120
	}
	if floatX > winW()-40 {
		floatX = winW() - 40
	}
	if floatY < 0 {
		floatY = 0
	}
	if floatY > winH()-30 {
		floatY = winH() - 30
	}
}

// positionFloat writes the floating panel's geometry (and its resize grip)
// without rebuilding the whole style or touching localStorage — cheap enough
// to call on every pointermove.
func positionFloat() {
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	clampFloatPos()
	st := p.Get("style")
	st.Set("left", pxStr(floatX))
	st.Set("top", pxStr(floatY))
	st.Set("width", pxStr(floatW))
	st.Set("height", pxStr(floatH))
	if floatGrip.Truthy() {
		gs := floatGrip.Get("style")
		gs.Set("left", pxStr(floatX+floatW-16))
		gs.Set("top", pxStr(floatY+floatH-16))
	}
}

func saveFloatGeom() {
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", "wasmstuff-floatX", pxStr(floatX))
		ls.Call("setItem", "wasmstuff-floatY", pxStr(floatY))
		ls.Call("setItem", "wasmstuff-floatW", pxStr(floatW))
		ls.Call("setItem", "wasmstuff-floatH", pxStr(floatH))
	}
}

func winH() float64 {
	if v := js.Global().Get("innerHeight").Float(); v > 0 {
		return v
	}
	return 800
}
func winW() float64 {
	if v := js.Global().Get("innerWidth").Float(); v > 0 {
		return v
	}
	return 1200
}

// applyDock positions the standalone controls panel against a window edge:
// bottom/top become a horizontal strip (height dockSizeH), left/right a
// vertical sidebar (width dockSizeW; the flex rows wrap into a narrow
// scrollable column). The edge + sizes persist in localStorage. No-op when
// the panel lives inside a host footer.
func applyDock(edge string) {
	if !standalonePanel {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	if dockSizeH <= 0 {
		// Default tall enough to show a full module row (~545px at scale 1)
		// without clipping, capped so it never eats the whole viewport.
		dockSizeH = math.Min(560, winH()*0.9)
	}
	// NB: no z-index here — the panel's base z lives in CSS (#controls-panel →
	// var(--z-panel)) so that clearing an inline override falls back to a value
	// above the canvas, never auto(0). Inline z is only ever set as a temporary
	// recovery-raise (see panelToggle / the Front handler).
	const base = "background:rgba(0,0,0,0.85);padding:8px 12px;font-family:'B612 Mono',monospace;" +
		"font-size:12px;color:#aaa;pointer-events:auto;position:fixed;box-sizing:border-box;" +
		"-webkit-overflow-scrolling:touch;overscroll-behavior:contain;touch-action:pan-x pan-y;"
	hpx := strconv.FormatFloat(dockSizeH, 'f', 0, 64) + "px"
	wpx := strconv.FormatFloat(dockSizeW, 'f', 0, 64) + "px"
	var edgeCSS string
	float := false
	switch edge {
	case "top":
		edgeCSS = "left:0;right:0;top:0;max-height:" + hpx + ";overflow-y:auto;border-bottom:1px solid #333;"
	case "left":
		edgeCSS = "top:0;bottom:0;left:0;width:" + wpx + ";overflow-y:auto;border-right:1px solid #333;"
	case "right":
		edgeCSS = "top:0;bottom:0;right:0;width:" + wpx + ";overflow-y:auto;border-left:1px solid #333;"
	case "float":
		float = true
		// portrait window; geometry (left/top/width/height) set by positionFloat.
		edgeCSS = "overflow-y:auto;border:1px solid #2a3a4a;border-radius:8px;box-shadow:0 10px 34px rgba(0,0,0,0.6);"
	default:
		edge = "bottom"
		edgeCSS = "left:0;right:0;bottom:0;max-height:" + hpx + ";overflow-y:auto;border-top:1px solid #333;"
	}
	p.Get("style").Set("cssText", base+edgeCSS)
	dockEdge = edge
	// (The legacy "rack" horizontal-strip layout is superseded by the module
	// system, which lays out the same in every dock mode.)
	p.Get("classList").Call("remove", "rack")
	// Tag the panel with its edge so CSS can adapt (e.g. sidebars stack modules
	// in a single column rather than flowing them side-by-side).
	for _, e := range []string{"top", "bottom", "left", "right", "float"} {
		p.Get("classList").Call("remove", "dk-"+e)
	}
	p.Get("classList").Call("add", "dk-"+edge)
	// The title bar (drag handle + label) shows ONLY when floating; docked modes
	// have no top chrome — the dock controls clip onto the resize bar instead.
	if floatTitle.Truthy() {
		if float {
			floatTitle.Get("classList").Call("add", "floating")
			floatTitle.Get("style").Set("display", "")
		} else {
			floatTitle.Get("classList").Call("remove", "floating")
			floatTitle.Get("style").Set("display", "none")
		}
	}
	if floatGrip.Truthy() {
		fdisp := "none"
		if float {
			fdisp = ""
		}
		floatGrip.Get("style").Set("display", fdisp)
	}
	if float {
		positionFloat()
	}
	positionResizeHandle()
	for _, e := range []string{"top", "bottom", "left", "right", "float"} {
		if b := doc.Call("getElementById", "dock-"+e); b.Truthy() {
			if e == edge {
				b.Get("classList").Call("add", "active")
			} else {
				b.Get("classList").Call("remove", "active")
			}
		}
	}
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", "wasmstuff-dock", edge)
		ls.Call("setItem", "wasmstuff-dockH", strconv.FormatFloat(dockSizeH, 'f', 0, 64))
		ls.Call("setItem", "wasmstuff-dockW", strconv.FormatFloat(dockSizeW, 'f', 0, 64))
	}
	quantizeModuleWidths()
}

// moduleSlot is the base card-slot width. Like rack cards plugged into a
// uniformly-spaced backplane, every module occupies an INTEGER number of slots
// (its content width rounded up), so modules always butt up cleanly.
const moduleSlot = 132.0

// moduleGap matches the .modules flex gap (px). Multi-slot modules add (N-1)
// of these so their right edge lines up with N separate 1-slot modules.
const moduleGap = 4.0

// quantizeModuleWidths snaps every module's width to a whole number of slots.
func quantizeModuleWidths() {
	if !standalonePanel {
		return
	}
	mods := doc.Call("querySelectorAll", ".modules > .sect")
	for i := 0; i < mods.Get("length").Int(); i++ {
		m := mods.Index(i)
		st := m.Get("style")
		// Widen the module so its column-wrap content forms its natural,
		// height-driven columns with nothing clipped, then measure the real
		// content span from the item positions (flex column-wrap misreports both
		// max-content and scrollWidth).
		st.Set("width", "3000px")
		// The actual controls grid — NOT #params (a display:contents .row whose
		// only child is the full-width grid, which would misreport 3000px).
		content := m.Call("querySelector", ".punit-grid, .toprow, .row:not(#params)")
		w := 0.0
		if content.Truthy() {
			items := content.Get("children")
			minL, maxR, any := 1e9, -1e9, false
			for j := 0; j < items.Get("length").Int(); j++ {
				it := items.Index(j)
				// Skip out-of-flow items (e.g. the dock-controls cluster, which is
				// position:fixed) so they don't inflate the module's measured width.
				if js.Global().Get("getComputedStyle").Invoke(it).Get("position").String() == "fixed" {
					continue
				}
				r := it.Call("getBoundingClientRect")
				l, rr := r.Get("left").Float(), r.Get("right").Float()
				if rr-l <= 0 {
					continue // skip hidden/zero-size items
				}
				any = true
				if l < minL {
					minL = l
				}
				if rr > maxR {
					maxR = rr
				}
			}
			if any {
				w = maxR - minL
			}
		}
		slot := moduleSlot * panelScale // slot grows with the interface scale
		slots := int(math.Ceil((w + 13) / slot))
		if slots < 1 {
			slots = 1
		}
		// An N-slot module also spans the (N-1) inter-module gaps it covers, so its
		// right edge lines up with N separate 1-slot modules — corners align across
		// wrapped rows. (moduleGap matches the .modules flex gap.)
		width := float64(slots)*slot + float64(slots-1)*moduleGap
		st.Set("width", strconv.FormatFloat(width, 'f', 0, 64)+"px")
	}
}

// initFloatWindow builds the floating panel's drag title-bar and corner resize
// grip (once), with document-level move/up handlers so a drag continues off the
// small targets. Hidden until float mode is active (applyDock toggles them).
func initFloatWindow() {
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	// Float mode shows a title bar (drag handle + label). The dock controls no
	// longer live on it — they're a body-level fixed cluster that clips onto the
	// end of the resize bar in dock modes (positionDockControls), so no chrome
	// bar steals panel space when docked.
	floatTitle = doc.Call("createElement", "div")
	floatTitle.Set("id", "float-title")
	floatTitle.Set("className", "float-title")
	lbl := doc.Call("createElement", "span")
	lbl.Set("className", "float-title-lbl")
	lbl.Set("textContent", "◈ WASM-STUFF")
	floatTitle.Call("appendChild", lbl)
	if dc := doc.Call("getElementById", "dock-controls"); dc.Truthy() {
		body.Call("appendChild", dc) // detach: positioned onto the resize bar's end
	}
	// (power switch stays inside the Console module — not moved to the top bar)
	p.Call("insertBefore", floatTitle, p.Get("firstChild"))

	floatGrip = doc.Call("createElement", "div")
	floatGrip.Set("id", "float-grip")
	floatGrip.Set("title", "Drag to resize")
	floatGrip.Set("style", "display:none;position:fixed;z-index:var(--z-grip);width:22px;height:22px;cursor:nwse-resize;touch-action:none;"+
		"background:linear-gradient(135deg,transparent 40%,rgba(140,170,200,0.7) 40%,rgba(140,170,200,0.7) 55%,transparent 55%,transparent 68%,rgba(140,170,200,0.7) 68%,rgba(140,170,200,0.7) 83%,transparent 83%);")
	body.Call("appendChild", floatGrip)

	floatTitle.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		e := a[0]
		// Only drag when floating, and not when pressing a dock button.
		if dockEdge != "float" {
			return nil
		}
		if t := e.Get("target"); t.Truthy() && t.Call("closest", "button,select,input").Truthy() {
			return nil
		}
		e.Call("preventDefault")
		floatDragging = true
		floatOffX = e.Get("clientX").Float() - floatX
		floatOffY = e.Get("clientY").Float() - floatY
		return nil
	}))
	floatGrip.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		floatResizing = true
		return nil
	}))
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if !floatDragging && !floatResizing {
			return nil
		}
		e := a[0]
		if floatDragging {
			floatX = e.Get("clientX").Float() - floatOffX
			floatY = e.Get("clientY").Float() - floatOffY
			// (positionFloat clamps — the one clamp for every move path)
		}
		if floatResizing {
			floatW = e.Get("clientX").Float() - floatX
			floatH = e.Get("clientY").Float() - floatY
			if floatW < 240 {
				floatW = 240
			}
			if floatH < 200 {
				floatH = 200
			}
		}
		positionFloat()
		return nil
	}))
	up := trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if floatDragging || floatResizing {
			saveFloatGeom()
		}
		floatDragging, floatResizing = false, false
		return nil
	})
	doc.Call("addEventListener", "pointerup", up)
	doc.Call("addEventListener", "pointercancel", up)
}

// positionResizeHandle places the drag bar along the panel's inner edge (the
// one facing the page center) so dragging it resizes the panel.
func positionResizeHandle() {
	positionDockControls()
	if !resizeHandle.Truthy() {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() {
		return
	}
	// Hidden panel (panel-toggle) or floating (its own corner grip): hide the
	// edge bar and let the meter return to the corner.
	if p.Get("style").Get("display").String() == "none" || dockEdge == "float" {
		resizeHandle.Get("style").Set("display", "none")
		positionAudioMeters()
		positionInfoOverlay()
		return
	}
	resizeHandle.Get("style").Set("display", "")
	// Track the panel's actual rendered inner edge (the side facing the page
	// center) so the bar stays on the edge regardless of content height.
	r := p.Call("getBoundingClientRect")
	px := func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }
	// Thicker bar (12px) with touch-action:none so touch-drags resize instead of
	// scrolling; a lighter center stripe reads as a grip.
	const common = "position:fixed;z-index:var(--z-dock);touch-action:none;background:" +
		"linear-gradient(rgba(120,150,180,0.12),rgba(150,180,210,0.55),rgba(120,150,180,0.12));"
	const commonV = "position:fixed;z-index:var(--z-dock);touch-action:none;background:" +
		"linear-gradient(to right,rgba(120,150,180,0.12),rgba(150,180,210,0.55),rgba(120,150,180,0.12));"
	var s string
	switch dockEdge {
	case "top":
		s = common + "left:0;right:0;top:" + px(r.Get("bottom").Float()-6) + ";height:12px;cursor:ns-resize;"
	case "left":
		s = commonV + "top:0;bottom:0;left:" + px(r.Get("right").Float()-6) + ";width:12px;cursor:ew-resize;"
	case "right":
		s = commonV + "top:0;bottom:0;left:" + px(r.Get("left").Float()-6) + ";width:12px;cursor:ew-resize;"
	default:
		s = common + "left:0;right:0;top:" + px(r.Get("top").Float()-6) + ";height:12px;cursor:ns-resize;"
	}
	resizeHandle.Get("style").Set("cssText", s)
	positionAudioMeters()
	positionInfoOverlay()
}

// positionDockControls clips the dock/float button cluster onto the END of the
// resize bar, protruding into the canvas — so no top chrome bar is needed to
// hold them when docked. Horizontal row for top/bottom docks, vertical column
// for left/right sidebars, and over the title bar's right end when floating.
func positionDockControls() {
	dc := doc.Call("getElementById", "dock-controls")
	if !dc.Truthy() {
		return
	}
	p := doc.Call("getElementById", "controls-panel")
	if !p.Truthy() || p.Get("style").Get("display").String() == "none" {
		dc.Get("style").Set("display", "none")
		return
	}
	r := p.Call("getBoundingClientRect")
	px := func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "px" }
	const chrome = "display:flex;align-items:center;position:fixed;z-index:var(--z-grip);gap:4px;" +
		"background:linear-gradient(#18242f,#0c1218);border:1px solid #2a3a4a;" +
		"padding:2px 5px;border-radius:5px;box-shadow:0 2px 8px rgba(0,0,0,0.5);"
	col := "flex-direction:row;"
	dc.Get("classList").Call("remove", "dc-vert")
	var pos string
	switch dockEdge {
	case "float":
		// Over the title bar, at its right end (label stays visible on the left).
		pos = "top:" + px(r.Get("top").Float()+4) + ";right:" + px(winW()-r.Get("right").Float()+8) + ";"
	case "top":
		// Bar runs along the panel's bottom edge; hang the cluster below it.
		pos = "top:" + px(r.Get("bottom").Float()+2) + ";right:" + px(winW()-r.Get("right").Float()+16) + ";"
	case "left":
		// Bar runs down the panel's right edge; stack the cluster to its right.
		col = "flex-direction:column;"
		dc.Get("classList").Call("add", "dc-vert")
		pos = "left:" + px(r.Get("right").Float()+2) + ";top:" + px(r.Get("top").Float()+16) + ";"
	case "right":
		// Bar runs down the panel's left edge; stack the cluster to its left.
		col = "flex-direction:column;"
		dc.Get("classList").Call("add", "dc-vert")
		pos = "right:" + px(winW()-r.Get("left").Float()+2) + ";top:" + px(r.Get("top").Float()+16) + ";"
	default: // bottom
		// Bar runs along the panel's top edge; perch the cluster above it.
		pos = "bottom:" + px(winH()-r.Get("top").Float()+2) + ";right:" + px(winW()-r.Get("right").Float()+16) + ";"
	}
	dc.Get("style").Set("cssText", chrome+col+pos)
}

// positionAudioMeters keeps the top-left audio-feature meter overlay clear of
// the control panel: it shifts right of a left sidebar or below a top strip,
// and returns to the corner for bottom/right docks or when the panel is hidden.
func positionAudioMeters() {
	if !afOverlay.Truthy() {
		return
	}
	top, left := 8.0, 8.0
	if standalonePanel {
		if p := doc.Call("getElementById", "controls-panel"); p.Truthy() && p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch dockEdge {
			case "left":
				left = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "top":
				top = r.Get("bottom").Float() + 8
			}
		}
	}
	st := afOverlay.Get("style")
	st.Set("top", strconv.FormatFloat(top, 'f', 0, 64)+"px")
	st.Set("left", strconv.FormatFloat(left, 'f', 0, 64)+"px")
}

// positionInfoOverlay keeps the Info text clear of the control panel (offset
// past a left/top sidebar, or below a bottom dock's top edge).
func positionInfoOverlay() {
	ov := doc.Call("getElementById", "info-overlay")
	if !ov.Truthy() || ov.Get("style").Get("display").String() == "none" {
		return
	}
	st := ov.Get("style")
	top, left, right := 70.0, 20.0, 20.0
	if standalonePanel {
		if p := doc.Call("getElementById", "controls-panel"); p.Truthy() && p.Get("style").Get("display").String() != "none" {
			r := p.Call("getBoundingClientRect")
			switch dockEdge {
			case "left":
				left = r.Get("right").Float() + 48 // clear the vertical dock-controls tab
			case "right":
				right = winW() - r.Get("left").Float() + 48 // clear the vertical dock-controls tab
			case "top":
				top = r.Get("bottom").Float() + 20
			case "bottom":
				// panel at the bottom; keep the overlay in the upper area
			}
		}
	}
	st.Set("top", strconv.FormatFloat(top, 'f', 0, 64)+"px")
	st.Set("left", strconv.FormatFloat(left, 'f', 0, 64)+"px")
	st.Set("right", strconv.FormatFloat(right, 'f', 0, 64)+"px")
}

// initDockResize creates the resize bar and its drag handlers (document-level
// so the drag continues past the thin bar).
func initDockResize() {
	resizeHandle = doc.Call("createElement", "div")
	resizeHandle.Set("id", "dock-resize")
	resizeHandle.Set("title", "Drag to resize the control panel")
	body.Call("appendChild", resizeHandle)
	resizeHandle.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		a[0].Call("preventDefault")
		resizing = true
		return nil
	}))
	// The "DOCK" label doubles as a resize grip (a bigger, obvious touch target
	// than the thin bar). Dragging it resizes exactly like the bar.
	if dl := doc.Call("querySelector", "#dock-controls .dock-lbl"); dl.Truthy() {
		dl.Get("style").Set("cursor", "grab")
		dl.Get("style").Set("touchAction", "none")
		dl.Set("title", "Drag to resize the control panel")
		dl.Call("addEventListener", "pointerdown", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			a[0].Call("preventDefault")
			a[0].Call("stopPropagation")
			resizing = true
			return nil
		}))
	}
	doc.Call("addEventListener", "pointermove", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if !resizing {
			return nil
		}
		e := a[0]
		switch dockEdge {
		case "bottom":
			dockSizeH = winH() - e.Get("clientY").Float()
		case "top":
			dockSizeH = e.Get("clientY").Float()
		case "left":
			dockSizeW = e.Get("clientX").Float()
		case "right":
			dockSizeW = winW() - e.Get("clientX").Float()
		}
		if dockSizeH < 120 {
			dockSizeH = 120
		} else if dockSizeH > winH()*0.96 {
			dockSizeH = winH() * 0.96
		}
		if dockSizeW < 150 {
			dockSizeW = 150
		} else if dockSizeW > winW()*0.95 {
			dockSizeW = winW() * 0.95
		}
		applyDock(dockEdge)
		return nil
	}))
	doc.Call("addEventListener", "pointerup", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		resizing = false
		return nil
	}))
	// Keep the bar on the panel's edge as its content height changes
	// (audio-mod rows, section collapse, mode switches, window resize).
	if ro := js.Global().Get("ResizeObserver"); ro.Truthy() {
		obs := ro.New(trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
			positionResizeHandle()
			return nil
		}))
		if p := doc.Call("getElementById", "controls-panel"); p.Truthy() {
			obs.Call("observe", p)
		}
	}
	js.Global().Call("addEventListener", "resize", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if dockEdge == "float" {
			positionFloat() // re-clamp into the new viewport
		}
		positionResizeHandle()
		quantizeModuleWidths()
		return nil
	}))
}

func wireDockButtons() {
	for _, e := range []string{"top", "bottom", "left", "right", "float"} {
		edge := e
		if b := doc.Call("getElementById", "dock-"+e); b.Truthy() {
			b.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
				applyDock(edge)
				return nil
			}))
		}
	}
}

func readDockPref() string {
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		if v := ls.Call("getItem", "wasmstuff-dockH"); v.Truthy() {
			if n, err := strconv.ParseFloat(v.String(), 64); err == nil && n > 0 {
				dockSizeH = n
			}
		}
		if v := ls.Call("getItem", "wasmstuff-dockW"); v.Truthy() {
			if n, err := strconv.ParseFloat(v.String(), 64); err == nil && n > 0 {
				dockSizeW = n
			}
		}
		for key, dst := range map[string]*float64{
			"wasmstuff-floatX": &floatX, "wasmstuff-floatY": &floatY,
			"wasmstuff-floatW": &floatW, "wasmstuff-floatH": &floatH,
		} {
			if v := ls.Call("getItem", key); v.Truthy() {
				if n, err := strconv.ParseFloat(strings.TrimSuffix(v.String(), "px"), 64); err == nil && n > 0 {
					*dst = n
				}
			}
		}
		if v := ls.Call("getItem", "wasmstuff-dock"); v.Truthy() {
			return v.String()
		}
	}
	return "bottom"
}

// updateGradientUI shows only the color controls relevant to the current
// palette: monochrome → one color; two-color → start+end; three-color →
// start+mid+end; rainbow → no fixed colors, show the period knob instead.
func updateGradientUI() {
	// All palette knobs stay visible; the ones that don't apply to the current
	// color count are just dimmed (no populate/depopulate reflow when the count
	// changes). bg always applies.
	dim := func(id string, inactive bool) {
		if el := doc.Call("getElementById", id); el.Truthy() {
			el.Get("style").Set("display", "")
			el.Get("classList").Call("toggle", "pal-dim", inactive)
		}
	}
	dim("grp-cstart", gradientColors == 4)                         // no fixed colors in rainbow
	dim("grp-cmid", gradientColors != 3)                           // mid only in 3-color
	dim("grp-cend", !(gradientColors == 2 || gradientColors == 3)) // end in 2- / 3-color
	dim("grp-rainbow", gradientColors != 4)                        // rainbow period only in rainbow
	if lbl := doc.Call("getElementById", "lbl-cstart"); lbl.Truthy() {
		if gradientColors == 1 {
			lbl.Set("textContent", "color")
		} else {
			lbl.Set("textContent", "start")
		}
	}
	updateViewModRows()
	quantizeModuleWidths()
}

// autoRotYDelta is the Y spin-rate the Auto-rotate switch contributes, so
// the auto-spin shows up on the Y rate knob rather than being a hidden term.
const autoRotYDelta = 0.1

// setAutoRotate turns the gentle auto-spin on/off by adding/removing a fixed
// contribution to the Y spin rate (reflected on the Y rate knob). Idempotent;
// turning it off removes only the auto contribution, leaving a user-set Y
// rate intact. Use this for the switch and Reset All.
func setAutoRotate(on bool) {
	if on == autoRotate {
		return
	}
	autoRotate = on
	sl := doc.Call("getElementById", "rotation-controls-y")
	if sl.Truthy() {
		v, _ := strconv.ParseFloat(sl.Get("value").String(), 64)
		if on {
			v += autoRotYDelta
		} else {
			v -= autoRotYDelta
		}
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		sl.Set("value", strconv.FormatFloat(v, 'g', -1, 64))
		sl.Call("dispatchEvent", js.Global().Get("Event").New("input"))
	}
	if el := doc.Call("getElementById", "auto-rotate"); el.Truthy() {
		el.Set("checked", on)
	}
}

// clearAutoRotateFlag marks auto-rotate off WITHOUT changing the Y rate —
// for callers that have already zeroed the Y spin themselves (grabbing the
// Y knob, resetting Y, spectrogram), so the switch just reflects reality.
func clearAutoRotateFlag() {
	autoRotate = false
	if el := doc.Call("getElementById", "auto-rotate"); el.Truthy() {
		el.Set("checked", false)
	}
}

// selWindow is the little label window on the attractor selector knob.
var selWindow js.Value

// updateSelWindow shows the current dropdown selection's label in the
// selector-knob window. Called on every mode change.
func updateSelWindow() {
	if !selWindow.Truthy() {
		return
	}
	sel := doc.Call("getElementById", "mode-select")
	if !sel.Truthy() {
		return
	}
	opts := sel.Get("selectedOptions")
	if opts.Truthy() && opts.Get("length").Int() > 0 {
		selWindow.Set("textContent", opts.Index(0).Get("textContent").String())
	}
}

// ---- Nested model selector (category ring + model knob) ----

// modelOpt is one selectable model: its #mode-select value and display label.
type modelOpt struct{ value, label string }

var (
	nestedCatOrder  []string              // category display order
	nestedCatModels map[string][]modelOpt // category -> its models
	nestedModeCat   map[string]string     // model value -> category
	nestedSyncing   bool                  // guards sync from re-propagating
)

// catShortLabel is the short tag printed around the outer selector ring.
func catShortLabel(cat string) string {
	switch cat {
	case "Attractors":
		return "ATTR"
	case "Polyhedra":
		return "POLY"
	case "Geometry":
		return "GEO"
	case "Audio":
		return "AUD"
	case "Scope":
		return "SCOPE"
	case "Custom":
		return "CUST"
	case "OFF":
		return "OFF"
	}
	return cat
}

// nestedOffCat is the synthetic 6th outer-knob category: turning to it powers
// the model off (replacing the old PWR switch). It holds no models.
const nestedOffCat = "OFF"

// setPowerState stops or resumes the render loop. Off blanks the canvas but
// keeps the panel; on restarts the loop only if it was actually stopped (so
// switching categories while already running doesn't spawn a second RAF loop).
func setPowerState(on bool) {
	if on {
		if stopped {
			stopped = false
			js.Global().Call("requestAnimationFrame", renderFrame)
		}
		return
	}
	stopped = true
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", glTypes.ColorBufferBit)
	gl.Call("clear", glTypes.DepthBufferBit)
}

// optgroupCategory folds the finer #mode-select optgroups into the top-level
// categories the outer knob switches between (Sprott systems join Attractors).
func optgroupCategory(label string) string {
	switch label {
	case "Polyhedra", "Geometry", "Audio", "Scope":
		return label
	}
	// Attractors + Sprott systems + (Custom, now folded in) → "Attractors".
	return "Attractors"
}

// buildNestedModelSelector reads the hidden #mode-select's optgroups into the
// category/model tables, fills the two visible dropdowns, and wires the
// concentric selector knobs (outer ring = category, inner = model). The hidden
// #mode-select stays the single source of truth: the dropdowns write to it and
// dispatch its change; syncNestedFromMode reads it back.
// attachSelMarquee caps a Console <select> to one unit and overlays a marquee
// readout of the current option: the native text is hidden (see CSS), a
// click-through overlay shows the name, and long names that overflow scroll
// back and forth so they stay readable.
func attachSelMarquee(sel js.Value, colorHex string) {
	parent := sel.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	wrap := doc.Call("createElement", "span")
	wrap.Set("className", "selwrap")
	parent.Call("insertBefore", wrap, sel)
	wrap.Call("appendChild", sel)
	marq := doc.Call("createElement", "span")
	marq.Set("className", "selmarq")
	if colorHex != "" {
		marq.Get("style").Set("color", colorHex)
	}
	inner := doc.Call("createElement", "span")
	marq.Call("appendChild", inner)
	wrap.Call("appendChild", marq)
	upd := func() {
		idx := sel.Get("selectedIndex").Int()
		txt := ""
		if idx >= 0 {
			txt = sel.Get("options").Index(idx).Get("text").String()
		}
		inner.Set("textContent", txt)
		over := inner.Get("offsetWidth").Int() - marq.Get("clientWidth").Int()
		if over > 4 {
			marq.Get("style").Call("setProperty", "--marq-shift", "-"+strconv.Itoa(over+6)+"px")
			marq.Get("classList").Call("add", "scroll")
		} else {
			marq.Get("classList").Call("remove", "scroll")
		}
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
	upd()
}

func buildNestedModelSelector() {
	mode := doc.Call("getElementById", "mode-select")
	catSel := doc.Call("getElementById", "cat-select")
	modelSel := doc.Call("getElementById", "model-select")
	holder := doc.Call("getElementById", "modelknob-holder")
	if !mode.Truthy() || !catSel.Truthy() || !modelSel.Truthy() || !holder.Truthy() {
		return
	}

	nestedCatOrder = nil
	nestedCatModels = map[string][]modelOpt{}
	nestedModeCat = map[string]string{}
	groups := mode.Call("querySelectorAll", "optgroup")
	for i := 0; i < groups.Get("length").Int(); i++ {
		g := groups.Index(i)
		cat := optgroupCategory(g.Get("label").String())
		if _, seen := nestedCatModels[cat]; !seen {
			nestedCatOrder = append(nestedCatOrder, cat)
		}
		opts := g.Call("querySelectorAll", "option")
		for j := 0; j < opts.Get("length").Int(); j++ {
			o := opts.Index(j)
			mo := modelOpt{o.Get("value").String(), o.Get("textContent").String()}
			nestedCatModels[cat] = append(nestedCatModels[cat], mo)
			nestedModeCat[mo.value] = cat
		}
	}
	// Synthetic FIRST category = OFF (power). Placing it before Attractors means
	// one detent counter-clockwise from Attractors reaches OFF (no need to spin
	// all the way around past Custom). No models; selecting it stops.
	nestedCatOrder = append([]string{nestedOffCat}, nestedCatOrder...)

	catSel.Set("innerHTML", "")
	for _, cat := range nestedCatOrder {
		o := doc.Call("createElement", "option")
		o.Set("value", cat)
		o.Set("textContent", cat)
		catSel.Call("appendChild", o)
	}

	// Concentric knobs: outer ring = category (labeled), inner = model.
	catSel.Set("title", "Model category — OFF / Attractors / Scope / Polyhedra / Geometry / Audio / Custom")
	modelSel.Set("title", "Model — the specific system within the selected category")
	stack := stackKnobs(makeSelectorKnob(catSel), makeSelectorKnob(modelSel))
	short := make([]string, len(nestedCatOrder))
	for i, c := range nestedCatOrder {
		short[i] = catShortLabel(c)
	}
	addSelectorLabels(stack, short, catSel, 40)
	// Unique, concise tooltip per category label (turn/click to that category).
	setLabelTooltips(stack, map[string]string{
		"OFF":   "Power off — stop rendering and clear the display",
		"ATTR":  "Attractors — chaotic systems (Lorenz, Rössler…) plus Custom",
		"SCOPE": "Scope — Lissajous, Graphic Artist and XY oscilloscope figures",
		"POLY":  "Polyhedra — wireframe Platonic solids",
		"GEO":   "Geometry — sphere, torus, globe and other surfaces",
		"AUD":   "Audio — spectrogram and FVF wobbulator displays",
	})
	holder.Call("appendChild", stack)
	attachSelMarquee(catSel, "#8fd0ff")   // category (blue)
	attachSelMarquee(modelSel, "#7fe0a0") // model (green) — scrolls long names

	// Category change -> repopulate models, pick the first, drive #mode-select.
	// The OFF category is the power switch: it stops the model and empties the
	// model dropdown without touching #mode-select (so the last model returns
	// when the knob leaves OFF).
	catSel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if nestedSyncing {
			return nil
		}
		if catSel.Get("value").String() == nestedOffCat {
			setPowerState(false)
			populateModelSelect(nestedOffCat, "")
			modelSel.Call("dispatchEvent", js.Global().Get("Event").New("change")) // snap inner knob
			return nil
		}
		setPowerState(true) // leaving OFF (or a normal switch) powers back on
		populateModelSelect(catSel.Get("value").String(), "")
		mode.Set("value", modelSel.Get("value").String())
		mode.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))
	// Model change -> drive #mode-select.
	modelSel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if nestedSyncing {
			return nil
		}
		mode.Set("value", modelSel.Get("value").String())
		mode.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		return nil
	}))

	syncNestedFromMode()
}

// populateModelSelect fills #model-select with the models of cat. If keep names
// a model in that category it stays selected, otherwise the first is chosen.
// Does not dispatch — callers propagate to #mode-select as needed.
func populateModelSelect(cat, keep string) {
	modelSel := doc.Call("getElementById", "model-select")
	if !modelSel.Truthy() {
		return
	}
	modelSel.Set("innerHTML", "")
	for _, mo := range nestedCatModels[cat] {
		o := doc.Call("createElement", "option")
		o.Set("value", mo.value)
		o.Set("textContent", mo.label)
		modelSel.Call("appendChild", o)
	}
	if keep != "" {
		modelSel.Set("value", keep)
		if modelSel.Get("value").String() != keep {
			modelSel.Set("selectedIndex", 0)
		}
	} else {
		modelSel.Set("selectedIndex", 0)
	}
}

// syncNestedFromMode sets the category + model dropdowns (and their knob
// pointers) from the current #mode-select value, without propagating back.
func syncNestedFromMode() {
	mode := doc.Call("getElementById", "mode-select")
	catSel := doc.Call("getElementById", "cat-select")
	modelSel := doc.Call("getElementById", "model-select")
	if !mode.Truthy() || !catSel.Truthy() || !modelSel.Truthy() {
		return
	}
	cat, ok := nestedModeCat[mode.Get("value").String()]
	if !ok {
		return
	}
	nestedSyncing = true
	catSel.Set("value", cat)
	populateModelSelect(cat, mode.Get("value").String())
	// Fire change on both so the selector-knob pointers snap; the guarded
	// handlers skip re-propagation while nestedSyncing is set.
	catSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	modelSel.Call("dispatchEvent", js.Global().Get("Event").New("change"))
	nestedSyncing = false
}

func updateInfoOverlay() {
	overlay := doc.Call("getElementById", "info-overlay")
	if overlay.IsNull() || overlay.IsUndefined() {
		return
	}
	showInfo := doc.Call("getElementById", "show-info")
	if showInfo.IsNull() || showInfo.IsUndefined() || !showInfo.Get("checked").Bool() {
		return
	}
	if desc, ok := attractorDescriptions[selectedMode]; ok {
		overlay.Set("textContent", desc)
	} else {
		overlay.Set("textContent", selectedMode)
	}
}

// updateTrailVisibility shows/hides the Trail slider + Persist
// checkbox depending on whether the current mode renders a trail.
func updateTrailVisibility() {
	el := doc.Call("getElementById", "trail-controls")
	if !el.Truthy() {
		return
	}
	if isAttractorMode(selectedMode) {
		el.Get("style").Set("display", "")
	} else {
		el.Get("style").Set("display", "none")
	}
}

// normalizeOrientation resets the current model to the default identity
// pose and zeroes the per-axis spin rates, so it faces the camera head-on.
// Auto-rotate (if on) still applies afterward.
func normalizeOrientation() {
	angleX, angleY, angleZ = 0, 0, 0
	dragMatrix = mgl32.Ident4()
	zeroRotationSliders()
	clearAutoRotateFlag()
	rebuildModelMatrix()
	updateRotKnobs()
	updateModelMatrix()
}

// randomizeOrientation gives the model a fresh random starting pose
// and a small random rotation rate on each of X/Y/Z. Called on Run()
// startup and from Reset All so the user gets a varied view each
// time instead of always starting at the identity-matrix pose. Only the
// pose is randomized — NOT the X/Y/Z spin-rate sliders. Setting random
// rates made the model spin perpetually and, because the sliders show one
// decimal, it was easy to leave a hidden nonzero rate that kept it turning
// even with auto-rotate off. Spin now comes only from auto-rotate or an
// explicit slider, so unchecking auto-rotate (with the sliders at 0)
// fully stops it.
func randomizeOrientation() {
	mathJS := js.Global().Get("Math")
	randSym := func() float32 { return float32(mathJS.Call("random").Float()*2 - 1) }

	angleX = wrapTwoPi(randSym() * float32(math.Pi))
	angleY = wrapTwoPi(randSym() * float32(math.Pi))
	angleZ = wrapTwoPi(randSym() * float32(math.Pi))
	rebuildModelMatrix()

	// Ensure the spin-rate sliders (and their cache) are zeroed.
	zeroRotationSliders()
	updateRotKnobs()
}

func onModeChange(this js.Value, args []js.Value) interface{} {
	sel := doc.Call("getElementById", "mode-select")
	if sel.Truthy() {
		selectedMode = sel.Get("value").String()
	}
	// Keep the "Edit eqn" switch in sync with whether we're in Custom mode.
	if sw := doc.Call("getElementById", "edit-eq-sw"); sw.Truthy() {
		sw.Set("checked", selectedMode == "custom")
	}
	// New mode means fresh geometry — force an upload on the next
	// uploadBuffersIndexed for static modes, and a skin-mesh rebuild.
	staticGeomDirty = true
	skinDirty = true
	resetAttractorState()
	buildParamPanel(selectedMode)
	updateInfoOverlay()
	updateSelWindow()
	syncNestedFromMode()
	updateTrailVisibility()
	// Run one frame to populate vertices, then update gradient and fit camera
	generateForMode(selectedMode)
	if isSpectroSurface(selectedMode) {
		setSpectrogramCamera()
	} else {
		restoreAutoRotateAfterSpectrogram()
		autoFitCamera()
	}
	// FVF audio-out follows the mode: resume if re-entering FVF with Listen
	// on; stop when leaving so no stray audio plays under other models.
	if selectedMode == "fvf" {
		if fvfListen {
			startFVFAudio()
		}
	} else {
		stopFVFAudio()
	}
	// Model Out likewise: suspend in modes it can't sonify (geometry,
	// spectrogram…) instead of streaming zeros ~23×/s, resume in trail modes.
	sonifyModeSync()
	refreshGradient()
	syncPermalinkNow() // reflect the new mode in the URL immediately
	return nil
}

func onColorChange(this js.Value, args []js.Value) interface{} {
	baseHex := doc.Call("getElementById", "color-base").Get("value").String()
	midHex := doc.Call("getElementById", "color-mid").Get("value").String()
	topHex := doc.Call("getElementById", "color-top").Get("value").String()
	baseColor[0], baseColor[1], baseColor[2] = hexToRGB(baseHex)
	midColor[0], midColor[1], midColor[2] = hexToRGB(midHex)
	topColor[0], topColor[1], topColor[2] = hexToRGB(topHex)
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
	return nil
}

// installErrorNet is the global backstop for the whole CLASS of uncaught async
// errors — promise rejections from browser APIs (fullscreen, permissions,
// media, AudioContext) that reject when the browser refuses, on mobile or in
// embedded/occluded contexts. Rather than chasing a missing .catch at every
// call site one at a time, this contains the class in one place: it logs the
// rejection and prevents the default so a stray one never degrades the session
// or shows the browser's uncaught-error UI. Call-site handling that also updates
// UI (e.g. re-syncing the Fullscreen switch) still lives where it matters; this
// is the safety net beneath it.
func installErrorNet() {
	js.Global().Call("addEventListener", "unhandledrejection", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		if len(a) > 0 {
			js.Global().Get("console").Call("warn", "[async rejection contained]", a[0].Get("reason"))
			a[0].Call("preventDefault")
		}
		return nil
	}))
}

func onResetAll(this js.Value, args []js.Value) interface{} {
	// Reset camera
	defaultCameraDist = initCameraDist
	rotationX1, rotationY1, rotationZ1 = 0, 0, 0

	// Static geometry may need re-upload (params reset to defaults).
	staticGeomDirty = true

	// Reset attractor position
	resetAttractorState()

	// Registry-owned controls (zoom, pan X/Y, rainbow period, …): each resets
	// itself — value, LED format, and any reset hook — so Reset All can never
	// silently miss one again. (Rotation sliders + movMatrix are re-randomized
	// below so the model never lands on the same view twice.)
	for _, c := range builtControls {
		c.resetToDefault()
	}

	// Reset all parameters to defaults
	for _, params := range attractorParams {
		for _, p := range params {
			*p.Value = p.Def
		}
	}
	buildParamPanel(selectedMode)

	// Reset auto-rotate, draw mode. (Speed / line width / trail — values,
	// LEDs, buffer realloc, persist drop — are registry-owned above.)
	paused = false
	if ps := doc.Call("getElementById", "pause-sw"); ps.Truthy() {
		ps.Set("checked", false)
	}
	usePoints = false
	attractorDrawMode = glTypes.LineStrip
	dragMatrix = mgl32.Ident4() // clear trackball drag orientation
	doc.Call("getElementById", "auto-rotate").Set("checked", true)
	doc.Call("getElementById", "use-points").Set("checked", false)
	doc.Call("getElementById", "show-info").Set("checked", false)
	doc.Call("getElementById", "info-overlay").Get("style").Set("display", "none")
	persistTrail = false
	doc.Call("getElementById", "persist-trail").Set("checked", false)
	gradientSource = 2
	gradientColors = 2
	gradientReverse = false
	doc.Call("getElementById", "gradient-source").Set("value", "2")
	doc.Call("getElementById", "gradient-colors").Set("value", "2")
	doc.Call("getElementById", "gradient-reverse").Set("checked", false)
	updateGradientUI()

	// Reset colors
	baseColor = [3]float32{1.0, 0.0, 0.0}
	midColor = [3]float32{0.0, 1.0, 0.0}
	topColor = [3]float32{0.0, 0.0, 1.0}
	bgColor = [3]float32{0, 0, 0}
	doc.Call("getElementById", "color-base").Set("value", "#ff0000")
	doc.Call("getElementById", "color-mid").Set("value", "#00ff00")
	doc.Call("getElementById", "color-top").Set("value", "#0000ff")
	doc.Call("getElementById", "color-bg").Set("value", "#000000")
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
	// Alpha=0: don't paint over the host page's bg (SVG logo etc).
	gl.Call("clearColor", 0, 0, 0, 0)

	// Reset the remaining effect switches to their defaults — dispatch 'change'
	// so each effect's own handler applies it (single source of truth). Layout
	// prefs (dock edge, interface size) and mode toggles (Edit eqn, Fullscreen)
	// are intentionally left alone.
	swDefaults := []struct {
		id  string
		def bool
	}{
		{"model-front", false}, {"spect-fill", false}, {"audio-mod", false},
		{"test-tone", false}, {"fg-on", false}, {"spectro-skin", false},
		{"bg-spectro", false}, {"bg-xy", false}, {"tpl-on", false}, {"show-meters", true},
		{"ring-sw", false},
	}
	for _, s := range swDefaults {
		if sw := doc.Call("getElementById", s.id); sw.Truthy() && sw.Get("checked").Bool() != s.def {
			sw.Set("checked", s.def)
			sw.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	// Display-style + knob-behavior selectors back to defaults.
	resetSel := func(id, val string) {
		if s := doc.Call("getElementById", id); s.Truthy() && s.Get("value").String() != val {
			s.Set("value", val)
			s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	resetSel("knob-style", "std")
	resetSel("step-ratio", "1")
	resetSel("fine-ratio", "0.1")
	resetSel("sonify-map", "off")                          // Model Out ring back to off (silence)
	for _, id := range []string{"phosphor", "led-color"} { // populated at runtime → first option is the default
		if s := doc.Call("getElementById", id); s.Truthy() && s.Get("selectedIndex").Int() != 0 {
			s.Set("selectedIndex", 0)
			s.Call("dispatchEvent", js.Global().Get("Event").New("change"))
		}
	}
	// Signal-generator oscillators back to their default note / level / wave, off.
	for _, g := range []struct {
		id        string
		freq, lvl int
	}{{"gen-x", 34, 80}, {"gen-y", 41, 80}, {"gen-z", 29, 80}} {
		setInput := func(sid, v string) {
			if e := doc.Call("getElementById", sid); e.Truthy() && e.Get("value").String() != v {
				e.Set("value", v)
				e.Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
		setInput(g.id+"-freq", strconv.Itoa(g.freq))
		setInput(g.id+"-lvl", strconv.Itoa(g.lvl))
		resetSel(g.id+"-wave", "0")
		resetSel(g.id+"-out", "off")
	}

	// Randomized starting pose + low-rate rotation. Replaces the old
	// identity-matrix reset so each click of Reset All produces a
	// fresh viewing angle. randomizeOrientation zeroes the spin rates;
	// re-enable the gentle auto-spin afterward (so its Y-rate shows).
	randomizeOrientation()
	autoRotate = false
	setAutoRotate(true)

	// Reset view
	generateForMode(selectedMode)
	updateViewMatrix()
	updateModelMatrix()

	return nil
}
