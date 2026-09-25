//go:build js && wasm

package attractor

import "github.com/go-gl/mathgl/mgl32"

// ── Camera ───────────────────────────────────────────────────────────────────

// camera is where a view is watched from and how the model is turned in it.
type camera struct {
	// initDist and defaultDist are the fitted camera distance: what the view
	// opens at, and what the zoom control returns to.
	initDist, defaultDist float32

	// angleX, angleY and angleZ are the absolute orientation in radians —
	// the source of truth for the model's pose. The "digital-potentiometer"
	// rotation knobs set them directly; the X/Y/Z rate sliders and
	// auto-rotate advance them over time. modelMat is rebuilt from them every
	// frame (see view.rebuildModelMatrix), so a held angle stays put and a
	// spinning one is just the angle marching. Mirrors Glen's 3D projective
	// unit, whose front-panel pots set absolute X/Y angles shown on 7-seg
	// displays.
	angleX, angleY, angleZ float32

	// panX and panY shift the whole scene laterally on screen — the
	// oscilloscope X/Y position controls. Offsetting eye and center together
	// keeps the view direction fixed, so the object slides by (panX, panY)
	// regardless of its rotation.
	panX, panY float32

	modelMat mgl32.Mat4 // ball.orient · the euler pose; see rebuildModelMatrix
	viewMat  mgl32.Mat4 // the look-at; the textured program is fed it too

	ball trackball
	ctl  cameraControls
}

// trackball is the mouse/touch drag orientation. It is composed OUTSIDE the
// euler pose, so a drag stays screen-aligned whatever the knob and spin
// angles are; folding it into the angles coupled badly with tilt.
type trackball struct {
	orient mgl32.Mat4

	// The drag session: the canvas center and radius, whether the grab began
	// near the periphery (z-roll rather than x/y-tilt), and where the pointer
	// was last.
	cx, cy, r    float64
	zMode        bool
	lastTheta    float64
	lastX, lastY float32
}

// cameraControls are the panel's camera controls as last read. The DOM is
// read only on each control's input event (user interaction, or the
// synthetic dispatch from wheel-on-input); renderLoop reads these instead of
// calling parseFloat every frame.
type cameraControls struct {
	zoom                float32
	panX, panY          float32
	spinX, spinY, spinZ float32
	autoRotate          bool
}

// newCamera returns a camera at the distance a view opens at, turning.
func newCamera() camera {
	return camera{
		initDist:    100,
		defaultDist: 100,
		ball:        trackball{orient: mgl32.Ident4()},
		ctl:         cameraControls{autoRotate: true},
	}
}

// ── Color state ──────────────────────────────────────────────────────────────

// traceStyle is how the trace is drawn: its colors and gradient, points or a
// line, and how much of the trail is shown.
type traceStyle struct {
	baseColor       [3]float32
	topColor        [3]float32
	midColor        [3]float32
	bgColor         [3]float32
	usePoints       bool
	persistTrail    bool
	gradientSource  int // gradient parameter source: 0=X,1=Y,2=Z,3=trail
	gradientColors  int // palette: 1=mono,2=two-color,3=three-color,4=rainbow
	gradientReverse bool
	gradientFreq    float32 // rainbow gradient cycles over the range (period control)
	gradientPhase   float32 // animated rainbow hue offset (flows through the spectrum)

	// trailModFrac is the fraction of the trail drawn this frame (1 = full).
	// Audio modulation shortens it live without touching the vertex buffer:
	// uploadVerticesOnly just draws the most-recent frac·count points.
	trailModFrac float32
}

var style = traceStyle{
	baseColor:      [3]float32{1.0, 0.0, 0.0},
	topColor:       [3]float32{0.0, 0.0, 1.0},
	midColor:       [3]float32{0.0, 1.0, 0.0},
	bgColor:        [3]float32{0.0, 0.0, 0.0},
	gradientSource: 2,
	gradientColors: 2,
	gradientFreq:   1,
	trailModFrac:   1,
}

// ── Interaction state ────────────────────────────────────────────────────────

// rebindParamWheel re-wires wheel-on-input listeners to every range/
// number input inside the params div. Set in Run(); called from
// buildParamPanel after it rebuilds the panel's children.
var rebindParamWheel func()

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

// runState is which model is running and whether it is.
type runState struct {
	paused       bool
	stopped      bool
	pausedCount  int
	selectedMode string

	// preCustomMode remembers the attractor to return to when the "Edit eqn" switch
	// is toggled back off.
	preCustomMode string
}

var run runState

var dragging bool

// ── Selection ────────────────────────────────────────────────────────────────

// viewState is ONE view of a model: the camera fitted to it, how far the
// model reaches, and the one-shot override a mode uses to say what the fit
// should be measured against.
//
// These were four package variables in four files — camera_js.go, split_js.go
// and render.go — which is the same state said in a way that permits exactly
// one view. Drawing two embeddings side by side means two of these, so they
// become a struct first and the second one becomes possible after.
//
// What is NOT here yet, and is the reason a second view cannot be DRAWN even
// with this in place: autoFitCamera writes the on-screen zoom control
// (cameraControl and sliderZoom), so the camera is bound to singleton DOM
// elements. A second view needs those to follow whichever view has focus,
// which is a panel decision rather than a rendering one. The state moving
// here is what makes that decision implementable; it does not make it.
type viewState struct {
	camera

	// fitExtent is how far the model reaches from its center, as measured
	// the last time the camera was fitted to it. Zero until then. The depth
	// partition measures against it — see split_js.go.
	fitExtent float32

	// fitOverride is a bound a mode supplies for its own fit, consumed by
	// the next autoFitCamera and cleared there. The audio modes use it to
	// fit to a FIXED worst case rather than to the instantaneous figure,
	// which is what keeps a loud passage from walking off the screen.
	fitOverride float32
}

// newViewState returns a view at the distances the camera opens at.
func newViewState() *viewState {
	return &viewState{camera: newCamera()}
}

// view is the single on-screen view. A second one is another of these.
var view = newViewState()

// Two views have independent cameras, which is the whole reason the state
// moved into a struct. While it was four package variables this could not
// be written, because there was only ever one of each.
