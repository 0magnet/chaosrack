//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// dragMatrix is the camera-relative trackball orientation from mouse/touch
// drag, composed OUTSIDE the euler pose so drag stays screen-aligned
// regardless of the knob/spin angles (folding drag into the euler angles
// coupled badly with tilt). movMatrix = dragMatrix · euler.
var dragMatrix = mgl32.Ident4()

// Drag session state (canvas center + radius, and whether the grab began
// near the periphery → z-roll instead of x/y-tilt).
var (
	dragCX, dragCY float64
	dragR          float64
	dragZMode      bool
	dragLastTheta  float64
)

// rebuildModelMatrix reconstructs movMatrix as the camera-relative drag
// orientation composed with the absolute X/Y/Z euler pose (X→Y→Z order,
// matching randomizeOrientation). Called every frame and after any direct
// change (knob, drag, permalink restore).
func rebuildModelMatrix() {
	euler := mgl32.HomogRotate3DX(angleX)
	euler = euler.Mul4(mgl32.HomogRotate3DY(angleY))
	euler = euler.Mul4(mgl32.HomogRotate3DZ(angleZ))
	movMatrix = dragMatrix.Mul4(euler)
}

// dragQuatString serializes the trackball-drag orientation as "x,y,z,w" for
// the permalink (empty when there's no drag). The euler knob angles are saved
// separately (&rot); this captures the mouse/touch-dragged part so a link or
// refresh restores the exact pose.
func dragQuatString() string {
	if dragMatrix == mgl32.Ident4() {
		return ""
	}
	q := mgl32.Mat4ToQuat(dragMatrix)
	return permaFmt(q.V[0]) + "," + permaFmt(q.V[1]) + "," + permaFmt(q.V[2]) + "," + permaFmt(q.W)
}

// setDragQuat restores the trackball orientation from a serialized quaternion.
func setDragQuat(x, y, z, w float32) {
	q := mgl32.Quat{W: w, V: mgl32.Vec3{x, y, z}}
	if q.Len() == 0 {
		return
	}
	dragMatrix = q.Normalize().Mat4()
	rebuildModelMatrix()
}

// beginDrag starts a trackball drag at screen (cx,cy): it records the
// canvas center and decides z-roll (grab near the rim) vs x/y-tilt.
func beginDrag(cx, cy float64) {
	if canvasEl.Truthy() {
		r := canvasEl.Call("getBoundingClientRect")
		w, h := r.Get("width").Float(), r.Get("height").Float()
		dragCX = r.Get("left").Float() + w/2
		dragCY = r.Get("top").Float() + h/2
		dragR = math.Min(w, h) / 2
	}
	dx, dy := cx-dragCX, cy-dragCY
	dragZMode = dragR > 0 && math.Hypot(dx, dy) > 0.65*dragR
	dragLastTheta = math.Atan2(dy, dx)
	dragLastX, dragLastY = float32(cx), float32(cy)
}

// dragMove applies a trackball step: near the rim it rolls about the
// camera Z axis (angle swept around center); otherwise it tilts about the
// camera Y (horizontal) and X (vertical) axes. Camera-relative, so it's
// pre-multiplied onto dragMatrix.
func dragMove(cx, cy float64) {
	// Camera-RELATIVE trackball that also keeps the X/Y/Z knobs in sync: build the
	// incremental rotation about the SCREEN axes, pre-multiply it onto the CURRENT
	// pose (so it rotates about the screen axes regardless of how the model is
	// turned — no inversion when the grabbed point is on the far side), then
	// decompose the result back into the absolute X→Y→Z euler angles the knobs
	// display. (The old code added the drag straight into the euler angles, which
	// rotate about fixed WORLD axes and so inverted once the model was flipped.)
	var inc mgl32.Mat4
	if dragZMode {
		th := math.Atan2(cy-dragCY, cx-dragCX)
		d := th - dragLastTheta
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		dragLastTheta = th
		inc = mgl32.HomogRotate3DZ(-float32(d)) // roll about the screen normal
	} else {
		dax := float32((cy - float64(dragLastY)) * 0.01) // vertical drag → pitch about camera X
		day := float32((cx - float64(dragLastX)) * 0.01) // horizontal drag → yaw about camera Y
		inc = mgl32.HomogRotate3DX(dax).Mul4(mgl32.HomogRotate3DY(day))
	}
	x, y, z := decomposeXYZ(inc.Mul4(movMatrix)) // apply in the camera frame, re-extract euler
	angleX, angleY, angleZ = wrapTwoPi(x), wrapTwoPi(y), wrapTwoPi(z)
	dragMatrix = mgl32.Ident4() // pose now fully in the euler angles
	dragLastX, dragLastY = float32(cx), float32(cy)
	rebuildModelMatrix()
	updateRotKnobs()
}

// ── Rotation knob DOM (digital-pot style) ────────────────────────────────────

var (
	knobPtr     [3]js.Value // pointer element per axis (X,Y,Z)
	knobLED     [3]js.Value // degree readout per axis
	knobsReady  bool
	lastKnobDeg = [3]int{-1, -1, -1}
)

// setSpinAxis / addAngleAxis let the knob pointer handlers in main.go poke
// the rotation state without exporting the vars.
func setSpinAxis(axis int, v float32) {
	switch axis {
	case 0:
		cachedRotX = v
	case 1:
		cachedRotY = v
	case 2:
		cachedRotZ = v
	}
}

func addAngleAxis(axis int, d float32) {
	switch axis {
	case 0:
		angleX = wrapTwoPi(angleX + d)
	case 1:
		angleY = wrapTwoPi(angleY + d)
	case 2:
		angleZ = wrapTwoPi(angleZ + d)
	}
	rebuildModelMatrix()
	updateModelMatrix()
	updateRotKnobs()
}

// updateRotKnobs rotates each knob's pointer and refreshes its LED readout
// to match the live angle. Cheap: it only touches the DOM for an axis
// whose integer degree changed since the last frame, so a held pose costs
// nothing and a spin is ~2 writes/frame per moving axis.
func updateRotKnobs() {
	if !knobsReady {
		return
	}
	angs := [3]float32{angleX, angleY, angleZ}
	for i := 0; i < 3; i++ {
		deg := int(angs[i]*57.2957795+0.5) % 360
		if deg == lastKnobDeg[i] {
			continue
		}
		lastKnobDeg[i] = deg
		if knobPtr[i].Truthy() {
			knobPtr[i].Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.Itoa(deg)+"deg)")
		}
		if knobLED[i].Truthy() {
			s := strconv.Itoa(deg)
			for len(s) < 3 {
				s = "0" + s
			}
			knobLED[i].Set("textContent", s+"°")
		}
	}
}

var fragShaderCode = `
	precision mediump float;
	uniform vec3 uBaseColor;
	uniform vec3 uTopColor;
	uniform vec3 uMidColor;
	uniform float uMinZ;
	uniform float uMaxZ;
	uniform float uMinX;
	uniform float uMaxX;
	uniform float uMinY;
	uniform float uMaxY;
	uniform int uGradientSource;
	uniform int uGradientColors;
	uniform int uGradientReverse;
	uniform float uGradientFreq;
	uniform float uGradientPhase;
	uniform float uTrailHead;
	varying vec3 vPosition;
	varying float vTrailT;
	varying float vDwell;

	vec3 hsv2rgb(vec3 c) {
		vec4 K = vec4(1.0, 2.0/3.0, 1.0/3.0, 3.0);
		vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
		return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
	}

	void main(void) {
		// Coloring is a source × scheme product. uGradientSource picks what the
		// gradient parameter t follows: 0=X, 1=Y, 2=Z (spatial), 3=trail age.
		// uGradientColors picks the palette: 1=monochrome, 2=two-color,
		// 3=three-color, 4=rainbow (HSV, animated via uGradientPhase).
		float t;
		if (uGradientSource == 0) {
			t = clamp((vPosition.x - uMinX) / max(uMaxX - uMinX, 0.001), 0.0, 1.0);
		} else if (uGradientSource == 1) {
			t = clamp((vPosition.y - uMinY) / max(uMaxY - uMinY, 0.001), 0.0, 1.0);
		} else if (uGradientSource == 2) {
			t = clamp((vPosition.z - uMinZ) / max(uMaxZ - uMinZ, 0.001), 0.0, 1.0);
		} else {
			// Ring-trail mode: age relative to the beam head (uTrailHead=0 in
			// scan mode makes this exactly t = vTrailT).
			t = vTrailT - uTrailHead;
			if (t < 0.0) { t += 1.0; }
		}
		if (uGradientReverse == 1) t = 1.0 - t;
		vec3 color;
		if (uGradientColors == 1) {
			color = uBaseColor;
		} else if (uGradientColors == 3) {
			if (t < 0.5) {
				color = mix(uBaseColor, uMidColor, t * 2.0);
			} else {
				color = mix(uMidColor, uTopColor, (t - 0.5) * 2.0);
			}
		} else if (uGradientColors == 4) {
			color = hsv2rgb(vec3(t * uGradientFreq + uGradientPhase, 1.0, 1.0));
		} else {
			color = mix(uBaseColor, uTopColor, t);
		}
		// Beam-dwell exposure: a real CRT trace is bright where the beam
		// lingers and dim where it sweeps fast. vDwell is 1.0 at the trail's
		// mean speed (and for geometry, which disables the attribute).
		gl_FragColor = vec4(color * vDwell, 1.0);
	}
`

var vertShaderCode = `
	attribute vec3 position;
	attribute float aTrailT;
	attribute float aDwell;
	uniform mat4 Pmatrix;
	uniform mat4 Vmatrix;
	uniform mat4 Mmatrix;
	uniform float uPointSize;
	varying vec3 vPosition;
	varying float vTrailT;
	varying float vDwell;
	void main(void) {
		gl_Position = Pmatrix * Vmatrix * Mmatrix * vec4(position, 1.0);
		gl_PointSize = uPointSize;
		vPosition = position;
		vTrailT = aTrailT;
		vDwell = aDwell;
	}
`

func setupShaders() {
	vertShader := gl.Call("createShader", glTypes.VertexShader)
	gl.Call("shaderSource", vertShader, vertShaderCode)
	gl.Call("compileShader", vertShader)

	fragShader := gl.Call("createShader", glTypes.FragmentShader)
	gl.Call("shaderSource", fragShader, fragShaderCode)
	gl.Call("compileShader", fragShader)

	gl.Call("attachShader", shaderProgram, vertShader)
	gl.Call("attachShader", shaderProgram, fragShader)
	gl.Call("linkProgram", shaderProgram)

	positionLoc = gl.Call("getAttribLocation", shaderProgram, "position")
	aTrailTLoc = gl.Call("getAttribLocation", shaderProgram, "aTrailT")
	aDwellLoc = gl.Call("getAttribLocation", shaderProgram, "aDwell")
	// Geometry / indexed paths leave the dwell attribute array disabled and
	// use this constant instead (uniform brightness).
	gl.Call("vertexAttrib1f", aDwellLoc, 1.0)
	gl.Call("useProgram", shaderProgram)

	// Set stride-4 attribute pointers (16 bytes per vertex: x,y,z,t)
	gl.Call("vertexAttribPointer", positionLoc, 3, glTypes.Float, false, 16, 0)
	gl.Call("enableVertexAttribArray", positionLoc)
	gl.Call("vertexAttribPointer", aTrailTLoc, 1, glTypes.Float, false, 16, 12)
	gl.Call("enableVertexAttribArray", aTrailTLoc)

	uBaseColorLoc = gl.Call("getUniformLocation", shaderProgram, "uBaseColor")
	uTopColorLoc = gl.Call("getUniformLocation", shaderProgram, "uTopColor")
	uMidColorLoc = gl.Call("getUniformLocation", shaderProgram, "uMidColor")
	uMinZLoc = gl.Call("getUniformLocation", shaderProgram, "uMinZ")
	uMaxZLoc = gl.Call("getUniformLocation", shaderProgram, "uMaxZ")
	uMinXLoc = gl.Call("getUniformLocation", shaderProgram, "uMinX")
	uMaxXLoc = gl.Call("getUniformLocation", shaderProgram, "uMaxX")
	uMinYLoc = gl.Call("getUniformLocation", shaderProgram, "uMinY")
	uMaxYLoc = gl.Call("getUniformLocation", shaderProgram, "uMaxY")
	uGradientSourceLoc = gl.Call("getUniformLocation", shaderProgram, "uGradientSource")
	uGradientColorsLoc = gl.Call("getUniformLocation", shaderProgram, "uGradientColors")
	uGradientFreqLoc = gl.Call("getUniformLocation", shaderProgram, "uGradientFreq")
	uGradientPhaseLoc = gl.Call("getUniformLocation", shaderProgram, "uGradientPhase")
	uGradientReverseLoc = gl.Call("getUniformLocation", shaderProgram, "uGradientReverse")
	uPointSizeLoc = gl.Call("getUniformLocation", shaderProgram, "uPointSize")
	uMmatrixLoc = gl.Call("getUniformLocation", shaderProgram, "Mmatrix")
	uVmatrixLoc = gl.Call("getUniformLocation", shaderProgram, "Vmatrix")
	uTrailHeadLoc = gl.Call("getUniformLocation", shaderProgram, "uTrailHead")
	gl.Call("uniform1f", uTrailHeadLoc, 0)
	gl.Call("uniform1f", uPointSizeLoc, 2.0)
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
	gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	gl.Call("uniform1f", uMinZLoc, float64(-1))
	gl.Call("uniform1f", uMaxZLoc, float64(1))
	gl.Call("uniform1f", uMinXLoc, float64(-1))
	gl.Call("uniform1f", uMaxXLoc, float64(1))
	gl.Call("uniform1f", uMinYLoc, float64(-1))
	gl.Call("uniform1f", uMaxYLoc, float64(1))
	gl.Call("uniform1i", uGradientSourceLoc, 2) // Z
	gl.Call("uniform1i", uGradientColorsLoc, 2) // two-color
	gl.Call("uniform1i", uGradientReverseLoc, 0)
	shadersReady = true

	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clearDepth", 1.0)
	gl.Call("viewport", 0, 0, width, height)
	gl.Call("depthFunc", glTypes.LEqual)
}

func setupMatrices() {
	// projMatrix is a pkg var (textured_js.go) so texProgram can reuse it.
	// Far plane must clear the auto-fit camera distance (maxExtent·3, capped at
	// 300) PLUS the model's own extent (~maxExtent) PLUS the zoom-out range —
	// otherwise the back of a large attractor pokes past the far plane and gets
	// clipped ("cutting through the black background"). 1500 covers the worst
	// case with margin.
	projMatrix = mgl32.Perspective(mgl32.DegToRad(45.0), float32(width)/float32(height), 1, 1500.0)
	gl.Call("useProgram", shaderProgram)
	gl.Call("uniformMatrix4fv", gl.Call("getUniformLocation", shaderProgram, "Pmatrix"), false, mat4ToTyped(&projMatrix))

	movMatrix = mgl32.Ident4()
	updateViewMatrix()
	updateModelMatrix()
}

// updateViewMatrix recomputes the camera and uploads it to the attractor
// program. texProgram receives it separately (useTexProgram reads the
// pkg-level viewMatrix), so we force the attractor program active here to
// keep the upload correct even if a textured draw left texProgram bound.
// panX/panY shift the whole scene laterally on screen — the oscilloscope
// X/Y position controls. Offsetting eye and center together keeps the view
// direction fixed, so the object slides by (panX,panY) regardless of its
// rotation.
var panX, panY float32

func updateViewMatrix() {
	cameraPosition := mgl32.Vec3{-panX, -panY, defaultCameraDist}
	center := mgl32.Vec3{-panX, -panY, 0.0}
	viewMatrix = mgl32.LookAtV(cameraPosition, center, mgl32.Vec3{0.0, 1.0, 0.0})
	gl.Call("useProgram", shaderProgram)
	gl.Call("uniformMatrix4fv", uVmatrixLoc, false, mat4ToTyped(&viewMatrix))
}

func updateModelMatrix() {
	gl.Call("useProgram", shaderProgram)
	gl.Call("uniformMatrix4fv", uMmatrixLoc, false, mat4ToTyped(&movMatrix))
}

// fitExtentOverride, when set, provides the next autoFitCamera call with the
// TRUE extent of the attractor (measured during a warmup) instead of the
// current trail's — for systems whose visible window is only a small arc of a
// much larger structure (hyper-Rössler), fitting the instantaneous arc left
// the camera blind for most of the orbit.
var fitExtentOverride float32

func autoFitCamera() {
	if len(attractorVertices) < 3 {
		return
	}
	maxAbs := float32(0)
	for i := 0; i < len(attractorVertices); i++ {
		v := attractorVertices[i]
		if v < 0 {
			v = -v
		}
		if v > maxAbs {
			maxAbs = v
		}
	}
	if fitExtentOverride > 0 {
		maxAbs = fitExtentOverride
		fitExtentOverride = 0
	}
	dist := fitDistFor(maxAbs)
	initCameraDist = dist
	defaultCameraDist = dist
	cameraControl.Set("value", "0")
	sliderZoom.Set("textContent", "0")
	updateViewMatrix()
}

// fitDistFor converts a content extent into a camera distance that shows
// the whole thing: ~3× the extent, clamped, and backed off further on
// portrait screens — the projection scales the horizontal FOV by the
// aspect ratio, so without the correction wide content (the Pong court, a
// text banner) runs off both edges on a phone held upright. Every camera
// fit (autoFitCamera and the modes that set distance directly) goes
// through this.
func fitDistFor(ext float32) float32 {
	dist := ext * 3.0
	if w, h := canvasEl.Get("clientWidth").Float(), canvasEl.Get("clientHeight").Float(); w > 0 && h > 0 && w < h {
		dist *= float32(h / w)
	}
	if dist < 5 {
		dist = 5
	}
	if dist > 300 {
		dist = 300
	}
	return dist
}

func generateForMode(mode string) {
	// Spectrogram is a textured plane drawn through the shared 3D pipeline
	// (texProgram); update its texture and draw it, then bail out of the
	// attractor path.
	if isSpectroSurface(mode) {
		renderSpectrogramMode(frameNowMs)
		return
	}
	// Spectrogram skin: paint the live texture onto a surface model
	// instead of its wireframe, drawn through the same textured pipeline.
	if spectroSkin && isSkinnable(mode) {
		renderSkinnedMode(mode, frameNowMs)
		return
	}
	// xy scope draws on its own 2D program via renderAudioFrame; skip the
	// attractor pipeline entirely. This path is still reached via
	// onModeChange / buildParamPanel / paused-frame redraw, so a plain
	// return is the correct response.
	if isAudioMode(mode) {
		return
	}
	// Transitioning back into an attractor mode from audio: restore
	// the attractor useProgram binding NOW rather than waiting for the
	// next render frame, because our caller (onModeChange) is about to
	// issue drawArrays / drawElements and would draw with the wrong
	// shader program bound.
	if audioModeActive {
		deactivateAudioMode()
	}
	// Ensure the attractor program is bound — a prior spectrogram frame
	// leaves texProgram active, and the uniform/draw calls below apply to
	// whatever program is current.
	if !shaderProgram.IsUndefined() {
		gl.Call("useProgram", shaderProgram)
	}
	if shadersReady {
		gl.Call("uniform1i", uGradientSourceLoc, gradientSource)
		gl.Call("uniform1i", uGradientColorsLoc, gradientColors)
		gl.Call("uniform1f", uGradientFreqLoc, gradientFreq)
		// Animate the rainbow: advance the hue offset each frame so the
		// spectrum flows. At a low period only a slice is visible at once,
		// and it cycles gradually through all colors over time rather than
		// staying stuck on part of the spectrum.
		gradientPhase += 0.003
		if gradientPhase >= 1 {
			gradientPhase -= 1
		}
		gl.Call("uniform1f", uGradientPhaseLoc, gradientPhase)
		if gradientReverse {
			gl.Call("uniform1i", uGradientReverseLoc, 1)
		} else {
			gl.Call("uniform1i", uGradientReverseLoc, 0)
		}
		// Scope phosphor: override the gradient with the phosphor's mono color.
		if phosphorActive() {
			applyPhosphorColor()
		}
	}
	// Audio-reactive: modulate ODE params (dt, primary chaos param) for
	// this integration step, and the colors / point size for this frame.
	// No-op unless audio-reactive is on and the mode is an attractor.
	saved := applyAudioModulation(mode)
	// Track the most recent real flow mode — the bifurcation explorer
	// sweeps it.
	if _, isFlow := flowFor4(mode); isFlow && mode != "bifurcation" {
		lastFlowMode = mode
	}
	// Twin-trajectory divergence (Trace > Twin): draws both copies itself.
	if twinTick(mode) {
		restoreAudioModulation(saved)
		sectTick(mode)
		return
	}
	// Ring-trail beam step (Trace > Ring): draws the frame itself when active
	// and primed; otherwise the scan generator below runs (and primes it).
	if ringTick(mode) {
		restoreAudioModulation(saved)
		sectTick(mode)
		return
	}
	if fn := modeGenerate[mode]; fn != nil {
		fn()
	}
	restoreAudioModulation(saved)
	ringPrimeAfterScan(mode)
	sectTick(mode) // Poincaré overlay draws above the finished trail
}

func renderLoop(this js.Value, args []js.Value) interface{} {
	// Stop button: clear once, do not reschedule. Loop dies here.
	if stopped {
		gl.Call("clearColor", 0, 0, 0, 0)
		gl.Call("clear", glTypes.ColorBufferBit)
		gl.Call("clear", glTypes.DepthBufferBit)
		return nil
	}

	// Audio modes (spectrogram, xy) render with their own shader
	// program on the shared #gocanvas. Route them here so we skip the
	// entire 3D attractor pipeline for the frame. Transition helpers
	// swap the useProgram binding when moving between attractor and
	// audio modes so neither pipeline sees the other's state.
	if len(args) > 0 {
		frameNowMs = args[0].Float() // rAF timestamp (ms), used by spectrogram scroll
		jamTick(frameNowMs)
	}

	// Refresh audio features (no-op unless audio-reactive is on); Phase 2
	// mappings read these to modulate the attractors.
	updateAudioFeatures()
	counterTick() // frequency-counter gate (no-op unless the module is on)
	genEnvTick()  // Envelope module shaper (no-op unless the gen audio runs)

	if isAudioMode(selectedMode) {
		if !audioModeActive {
			activateAudioMode()
		}
		renderAudioFrame(selectedMode)
		js.Global().Call("requestAnimationFrame", renderFrame)
		return nil
	}
	if audioModeActive {
		deactivateAudioMode()
	}

	now := float32(args[0].Float())
	tdiff := now - tmark
	tmark = now

	// Debug frame timing
	if debugEnabled && lastFrameStart > 0 {
		frameMs := now - lastFrameStart
		frameCount++
		frameTotalMs += frameMs
		if frameMs < frameMinMs {
			frameMinMs = frameMs
		}
		if frameMs > frameMaxMs {
			frameMaxMs = frameMs
		}
	}
	lastFrameStart = now

	gl.Call("enable", glTypes.DepthTest)
	bgOn := bgVisualActive()
	if phosphorActive() {
		// Phosphor persistence: don't clear the color buffer — fade it toward
		// black by the phosphor's decay so old trace lingers as an afterglow.
		// Clear depth only so the new trace still draws on top.
		gl.Call("clear", glTypes.DepthBufferBit)
		drawPhosphorFade()
		gl.Call("enable", glTypes.DepthTest)
	} else if !persistTrail || bgOn {
		// A background visualizer repaints the whole backdrop each frame, so
		// the color buffer must be cleared first even if Persist is on (a
		// persisted trail can't coexist with a live scrolling backdrop).
		gl.Call("clear", glTypes.ColorBufferBit)
		gl.Call("clear", glTypes.DepthBufferBit)
	} else {
		// Only clear depth so new draws appear on top, but keep color buffer (old trails)
		gl.Call("clear", glTypes.DepthBufferBit)
	}

	// Background audio visualizer: paint the reactive backdrop (spectrogram or
	// xy scope) behind the model, then hand the depth-tested attractor pipeline
	// a clean depth buffer to draw over it.
	if bgOn {
		renderBackgroundVisual(frameNowMs)
		gl.Call("enable", glTypes.DepthTest)
		gl.Call("clear", glTypes.DepthBufferBit)
		// The backdrop bound its own program / buffers; force geometry models
		// to re-upload their static vertex+index buffers next draw.
		if !isAttractorMode(selectedMode) {
			staticGeomDirty = true
		}
	}

	if paused {
		// Redraw current geometry without advancing trail / auto-rotate.
		// Attractors use drawArrays with the line-strip buffer
		// (pausedCount = last frame's step count). Polyhedra +
		// geometry primitives use drawElements via generateForMode,
		// so for those we need to re-emit the geometry — drawArrays
		// alone would not consult the index buffer and the canvas
		// goes blank. Both paths skip the integrator step so the
		// visual snapshot is preserved.
		if isAttractorMode(selectedMode) {
			gl.Call("drawArrays", attractorDrawMode, 0, pausedCount)
		} else {
			generateForMode(selectedMode)
		}
		// Still allow camera interaction while paused (zoom read
		// from the Go-side cache instead of parseFloat per frame).
		zoomVal := cachedZoom
		newDist := initCameraDist - zoomVal
		if newDist != defaultCameraDist {
			defaultCameraDist = newDist
			updateViewMatrix()
		}
		// Still allow drag while paused (dragMatrix is updated by the
		// pointer handlers; rebuild+upload so it shows).
		rebuildModelMatrix()
		updateModelMatrix()
		js.Global().Call("requestAnimationFrame", renderFrame)
		return nil
	}

	// Audio-modulate the camera/motion controls (zoom, pan, spin rates, line)
	// AND the rainbow period in place before they're consumed; restored at
	// frame end so the base values (and their knobs) are unchanged. This must
	// run BEFORE generateForMode, which reads gradientFreq into the shader and
	// draws — otherwise the rainbow-period mod lands a frame too late (i.e.
	// never takes visible effect).
	viewSaved := applyViewModulation()

	// CRT (phosphor) mode draws a real scope trace: instead of redrawing the
	// whole curve every frame, draw only a short advancing beam and let the
	// phosphor persistence (the non-cleared, fading framebuffer) leave the trail
	// behind it. The trail length then comes from the phosphor's afterglow, not
	// the geometric point count — so the Trail control is dimmed in this mode.
	// Implemented by shrinking `steps` (which the generators use for both the
	// integrate-count and the draw-count) just around generateForMode.
	realSteps := steps
	if crtBeam() && crtBeamLen < steps {
		steps = crtBeamLen
	}
	generateForMode(selectedMode)
	beamDrawn := steps
	steps = realSteps

	// Slider values come from cachedZoom/RotX/Y/Z, kept in sync by
	// input listeners in Run(). Eliminates 4 parseFloat round-trips
	// + 4 textContent writes per frame.
	zoomVal := cachedZoom
	rotationX = cachedRotX
	rotationY = cachedRotY
	rotationZ = cachedRotZ

	// Zoom slider directly controls camera distance (absolute position);
	// the X/Y position knobs pan the scene. Both go through the view matrix.
	newDist := initCameraDist - zoomVal
	// Map the ±8 X/Y position sliders to ≈±1 screen of travel, so the model can
	// be pushed just off-screen (like an oscilloscope's position controls) yet
	// not miles away. The screen half-extent at the object plane is
	// dist·tan(fov/2); scale X by the aspect so ±8 spans a full screen width and
	// ±8 spans a full screen height — and it tracks zoom so a given setting keeps
	// the model at the same screen fraction.
	halfH := newDist * 0.41421 // tan(22.5°), half the 45° vertical FOV
	aspect := float32(1.6)
	if height > 0 {
		aspect = float32(width) / float32(height)
	}
	psY := halfH * 2 / 8
	psX := psY * aspect
	npx, npy := cachedPanX*psX, cachedPanY*psY
	if newDist != defaultCameraDist || npx != panX || npy != panY {
		defaultCameraDist = newDist
		panX, panY = npx, npy
		updateViewMatrix()
	}
	// Advance the absolute angles by the per-axis spin rate (the X/Y/Z
	// rate sliders) — same increment as before, now integrated into the
	// pose instead of post-multiplied onto an accumulating matrix.
	if rotationX != 0 {
		rotationX1 = rotationX/20 + tdiff*0.00000001
		angleX += rotationX1
	}
	if rotationY != 0 {
		rotationY1 = rotationY/20 + tdiff*0.00000001
		angleY += rotationY1
	}
	if rotationZ != 0 {
		rotationZ1 = rotationZ/20 + tdiff*0.00000001
		angleZ += rotationZ1
	}

	angleX = wrapTwoPi(angleX)
	angleY = wrapTwoPi(angleY)
	angleZ = wrapTwoPi(angleZ)
	rebuildModelMatrix()
	updateRotKnobs()

	updateModelMatrix()
	restoreAudioModulation(viewSaved) // put zoom/pan/spin base values back
	pausedCount = beamDrawn           // paused CRT keeps showing just the beam

	js.Global().Call("requestAnimationFrame", renderFrame)
	return nil
}
