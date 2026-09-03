//go:build js && wasm

package attractor

import "github.com/go-gl/mathgl/mgl32"

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
