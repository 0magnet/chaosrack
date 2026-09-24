//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"math"
	"strconv"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// rebuildModelMatrix reconstructs view.modelMat as the camera-relative drag
// orientation composed with the absolute X/Y/Z euler pose (X→Y→Z order,
// matching randomizeOrientation). Called every frame and after any direct
// change (knob, drag, permalink restore).
func (c *camera) rebuildModelMatrix() {
	euler := mgl32.HomogRotate3DX(c.angleX)
	euler = euler.Mul4(mgl32.HomogRotate3DY(c.angleY))
	euler = euler.Mul4(mgl32.HomogRotate3DZ(c.angleZ))
	c.modelMat = c.ball.orient.Mul4(euler)
}

// dragQuatString serializes the trackball-drag orientation as "x,y,z,w" for
// the permalink (empty when there's no drag). The euler knob angles are saved
// separately (&rot); this captures the mouse/touch-dragged part so a link or
// refresh restores the exact pose.
func (c *camera) dragQuatString() string {
	if c.ball.orient == mgl32.Ident4() {
		return ""
	}
	q := mgl32.Mat4ToQuat(c.ball.orient)
	return permaFmt(q.V[0]) + "," + permaFmt(q.V[1]) + "," + permaFmt(q.V[2]) + "," + permaFmt(q.W)
}

// setDragQuat restores the trackball orientation from a serialized quaternion.
func (c *camera) setDragQuat(x, y, z, w float32) {
	q := mgl32.Quat{W: w, V: mgl32.Vec3{x, y, z}}
	if q.Len() == 0 {
		return
	}
	c.ball.orient = q.Normalize().Mat4()
	c.rebuildModelMatrix()
}

// beginDrag starts a trackball drag at screen (cx,cy): it records the
// canvas center and decides z-roll (grab near the rim) vs x/y-tilt.
func beginDrag(cx, cy float64) {
	if glctx.Canvas.Truthy() {
		r := glctx.Canvas.Call("getBoundingClientRect")
		w, h := r.Get("width").Float(), r.Get("height").Float()
		view.ball.cx = r.Get("left").Float() + w/2
		view.ball.cy = r.Get("top").Float() + h/2
		view.ball.r = math.Min(w, h) / 2
	}
	dx, dy := cx-view.ball.cx, cy-view.ball.cy
	view.ball.zMode = view.ball.r > 0 && math.Hypot(dx, dy) > 0.65*view.ball.r
	view.ball.lastTheta = math.Atan2(dy, dx)
	view.ball.lastX, view.ball.lastY = float32(cx), float32(cy)
	// A rim twist on a figure that has weight spins the FIGURE, not the camera.
	// The gesture is the one that was already there; with mass in the room there
	// is a better thing to aim it at.
	if view.ball.zMode {
		grab.spinBegin()
	} else {
		grab.tiltBegin()
	}
}

// dragMove applies a trackball step: near the rim it rolls about the
// camera Z axis (angle swept around center); otherwise it tilts about the
// camera Y (horizontal) and X (vertical) axes. Camera-relative, so it's
// pre-multiplied onto view.ball.orient.
func dragMove(cx, cy float64) {
	// Spinning the figure rather than rolling the view: same swept angle, aimed
	// at the body.
	if grab.spinDrag {
		th := math.Atan2(cy-view.ball.cy, cx-view.ball.cx)
		d := th - view.ball.lastTheta
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		view.ball.lastTheta = th
		grab.spinBy(-float32(d))
		return
	}
	// Tilting the figure in three dimensions rather than the camera: same screen
	// axes a trackball uses, aimed at the object.
	if grab.tiltDrag {
		dax := float32((cy - float64(view.ball.lastY)) * 0.01)
		day := float32((cx - float64(view.ball.lastX)) * 0.01)
		grab.tiltMove(dax, day)
		view.ball.lastX, view.ball.lastY = float32(cx), float32(cy)
		return
	}
	// Camera-RELATIVE trackball that also keeps the X/Y/Z knobs in sync: build the
	// incremental rotation about the SCREEN axes, pre-multiply it onto the CURRENT
	// pose (so it rotates about the screen axes regardless of how the model is
	// turned — no inversion when the grabbed point is on the far side), then
	// decompose the result back into the absolute X→Y→Z euler angles the knobs
	// display. (The old code added the drag straight into the euler angles, which
	// rotate about fixed WORLD axes and so inverted once the model was flipped.)
	var inc mgl32.Mat4
	if view.ball.zMode {
		th := math.Atan2(cy-view.ball.cy, cx-view.ball.cx)
		d := th - view.ball.lastTheta
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		view.ball.lastTheta = th
		inc = mgl32.HomogRotate3DZ(-float32(d)) // roll about the screen normal
	} else {
		dax := float32((cy - float64(view.ball.lastY)) * 0.01) // vertical drag → pitch about camera X
		day := float32((cx - float64(view.ball.lastX)) * 0.01) // horizontal drag → yaw about camera Y
		inc = mgl32.HomogRotate3DX(dax).Mul4(mgl32.HomogRotate3DY(day))
	}
	x, y, z := decomposeXYZ(inc.Mul4(view.modelMat)) // apply in the camera frame, re-extract euler
	view.angleX, view.angleY, view.angleZ = wrapTwoPi(x), wrapTwoPi(y), wrapTwoPi(z)
	view.ball.orient = mgl32.Ident4() // pose now fully in the euler angles
	view.ball.lastX, view.ball.lastY = float32(cx), float32(cy)
	view.rebuildModelMatrix()
	rotKnobs.update()
}

// ── Rotation knob DOM (digital-pot style) ────────────────────────────────────

// rotationKnobs is the three digital-pot rotation knobs: their pointers and
// degree readouts.
type rotationKnobs struct {
	ptr     [3]js.Value // pointer element per axis (X,Y,Z)
	led     [3]js.Value // degree readout per axis
	ready   bool
	lastDeg [3]int
}

var rotKnobs = rotationKnobs{
	lastDeg: [3]int{-1, -1, -1},
}

// setSpinAxis / addAngleAxis let the knob pointer handlers in main.go poke
// the rotation state without exporting the vars.
func setSpinAxis(axis int, v float32) {
	switch axis {
	case 0:
		view.ctl.spinX = v
	case 1:
		view.ctl.spinY = v
	case 2:
		view.ctl.spinZ = v
	}
}

func addAngleAxis(axis int, d float32) {
	switch axis {
	case 0:
		view.angleX = wrapTwoPi(view.angleX + d)
	case 1:
		view.angleY = wrapTwoPi(view.angleY + d)
	case 2:
		view.angleZ = wrapTwoPi(view.angleZ + d)
	}
	view.rebuildModelMatrix()
	view.updateModelMatrix()
	rotKnobs.update()
}

// update rotates each knob's pointer and refreshes its LED readout
// to match the live angle. Cheap: it only touches the DOM for an axis
// whose integer degree changed since the last frame, so a held pose costs
// nothing and a spin is ~2 writes/frame per moving axis.
func (r *rotationKnobs) update() {
	if !r.ready {
		return
	}
	angs := [3]float32{view.angleX, view.angleY, view.angleZ}
	for i := range 3 {
		deg := int(angs[i]*57.2957795+0.5) % 360
		if deg == r.lastDeg[i] {
			continue
		}
		r.lastDeg[i] = deg
		if r.ptr[i].Truthy() {
			r.ptr[i].Get("style").Set("transform", "translate(-50%,-100%) rotate("+strconv.Itoa(deg)+"deg)")
		}
		if r.led[i].Truthy() {
			s := strconv.Itoa(deg)
			for len(s) < 3 {
				s = "0" + s
			}
			r.led[i].Set("textContent", s+"°")
		}
	}
}

var fragShaderCode = `
	precision mediump float;
	// The colormap, as a 256x1 texture. See palette_js.go for why a texture and
	// not an array of stops.
	uniform sampler2D uPalette;
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
	uniform float uDashDuty;
	uniform float uDashCount;
	uniform float uGradientFreq;
	uniform float uGradientPhase;
	// Where the colormap window starts. One more float against a fragment
	// uniform budget palette_js.go already admits is over the guaranteed
	// minimum — which is exactly why it is one float and not a second copy of
	// uGradientFreq: the period this pairs with is the rainbow's, reused.
	uniform float uPaletteShift;
	uniform float uTrailHead;
	// The depth partition. uSplitSide is 0 when there is none, which is the
	// only state the rest of the app has ever produced, so nothing is discarded
	// unless something asks for it: +1 keeps what is NEARER than the plane, -1
	// keeps what is FARTHER.
	uniform float uSplitZ;
	uniform int uSplitSide;
	varying vec3 vPosition;
	varying float vAudioT;
	varying float vTrailT;
	varying float vDwell;
	varying float vViewZ;

	vec3 hsv2rgb(vec3 c) {
		vec4 K = vec4(1.0, 2.0/3.0, 1.0/3.0, 3.0);
		vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
		return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
	}

	void main(void) {
		// The camera looks down -Z, so a point nearer the eye has the LARGER
		// (less negative) view z. Discarding here rather than clipping keeps
		// the two passes pixel-exact against each other: every fragment is
		// evaluated by exactly one of them, so the seam is where the plane is
		// and not where a triangle happened to be cut.
		if (uSplitSide > 0 && vViewZ < uSplitZ) { discard; }
		if (uSplitSide < 0 && vViewZ >= uSplitZ) { discard; }
		// Points and a continuous line as two ends of one knob, rather than a
		// switch between two draw modes. The trail parameter is already a ramp
		// along the curve, so cutting it into uDashCount cycles and keeping
		// uDashDuty of each gives a dashed line: duty 1 is the solid line
		// unchanged, and as duty falls the dashes shorten toward the beads a
		// POINTS draw would put at the same places. Everything in between —
		// long dashes, short dashes, a dotted line — is a position on the way,
		// which is the thing a mode switch cannot express.
		//
		// It is a discard rather than a second draw call so the dashes inherit
		// the gradient, the dwell exposure and the split plane exactly as the
		// solid line does; drawing points separately would mean a second set of
		// answers to all three.
		if (uDashDuty < 0.999) {
			if (fract(vTrailT * uDashCount) > uDashDuty) { discard; }
		}
		// Coloring is a source × scheme product. uGradientSource picks what the
		// gradient parameter t follows: 0=X, 1=Y, 2=Z (spatial), 3=trail age,
		// 4=audio (the table above, filled per frame — a short-time spectrum
		// along the trail where the trail is a time axis, one flat value
		// everywhere else).
		// uGradientColors picks the palette: 1=monochrome, 2=two-color,
		// 3=three-color, 4=rainbow (HSV, animated via uGradientPhase).
		float t;
		if (uGradientSource == 0) {
			t = clamp((vPosition.x - uMinX) / max(uMaxX - uMinX, 0.001), 0.0, 1.0);
		} else if (uGradientSource == 1) {
			t = clamp((vPosition.y - uMinY) / max(uMaxY - uMinY, 0.001), 0.0, 1.0);
		} else if (uGradientSource == 2) {
			t = clamp((vPosition.z - uMinZ) / max(uMaxZ - uMinZ, 0.001), 0.0, 1.0);
		} else if (uGradientSource >= 4 && uGradientSource != 5) {
			// Both audio sources arrive through the same trail table; which
			// quantity filled it was decided on the CPU side.
			t = vAudioT;
		} else {
			// Ring-trail mode: age relative to the beam head (uTrailHead=0 in
			// scan mode makes this exactly t = vTrailT). Left as the else so an
			// unexpected source still lands on a sensible one.
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
		} else if (uGradientColors >= 5) {
			// A spectrogram colormap, sampled at the same t every other palette
			// uses — so switching between them changes the color language and
			// not what the color is saying — but through a WINDOW onto the map.
			// uGradientFreq (the period knob, shared with the rainbow) is how
			// many times the map is crossed across the figure; uPaletteShift is
			// where that crossing starts. At their defaults, 1 and 0, pt is t
			// and the fold below is the identity on it, so an untouched view is
			// the picture it was.
			float pt = t * uGradientFreq + uPaletteShift;
			// The fold, and it is a REFLECTION rather than a wrap. Hue is a
			// circle so fract() joins its ends invisibly; a colormap is a line
			// whose ends are different colors, and joining them draws a hard
			// seam across the figure — one that MOVES once the shift is
			// modulated, which is the most conspicuous thing on the screen and
			// hides the sweep it is supposed to be showing. Clamping has no
			// seam and no sweep either: the audio features are non-negative, so
			// a routed shift spends most of its travel pinned at an end with
			// every fragment the same color. Turning back at each end keeps the
			// color field continuous and the sweep alive at any depth. See
			// pkg/colormap; Fold there is this same expression,
			// and its test is what holds the two together.
			pt = abs(pt - 2.0 * floor(pt * 0.5 + 0.5));
			color = texture2D(uPalette, vec2(pt, 0.5)).rgb;
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
	// The audio gradient table, read HERE rather than in the fragment shader.
	// GLSL ES 1.0 only requires dynamic indexing of a uniform array in the
	// vertex stage — a fragment shader may reject an index that is not a
	// constant expression, and some drivers do. It is also cheaper: one
	// lookup per vertex instead of one per fragment, and the varying
	// interpolates between the stops for free.
	uniform float uAudioLUT[32];
	varying vec3 vPosition;
	varying float vTrailT;
	varying float vDwell;
	varying float vViewZ;
	varying float vAudioT;
	void main(void) {
		// The view-space position is computed on its own so the fragment stage
		// can be told how far from the camera it is. Projected depth would not
		// do: it is non-linear and clip-space, and the partition has to be a
		// plane at a stated distance in front of the eye.
		vec4 viewPos = Vmatrix * Mmatrix * vec4(position, 1.0);
		vViewZ = viewPos.z;
		gl_Position = Pmatrix * viewPos;
		gl_PointSize = uPointSize;
		vPosition = position;
		vTrailT = aTrailT;
		vDwell = aDwell;
		vAudioT = uAudioLUT[int(clamp(aTrailT, 0.0, 1.0) * 31.0 + 0.5)];
	}
`

func (r *renderer) setupShaders() {
	vertShader := glctx.GL.Call("createShader", glctx.Types.VertexShader)
	glctx.GL.Call("shaderSource", vertShader, vertShaderCode)
	glctx.GL.Call("compileShader", vertShader)

	fragShader := glctx.GL.Call("createShader", glctx.Types.FragmentShader)
	glctx.GL.Call("shaderSource", fragShader, fragShaderCode)
	glctx.GL.Call("compileShader", fragShader)

	glctx.GL.Call("attachShader", r.program, vertShader)
	glctx.GL.Call("attachShader", r.program, fragShader)
	glctx.GL.Call("linkProgram", r.program)

	r.aPosition = glctx.GL.Call("getAttribLocation", r.program, "position")
	r.aTrailT = glctx.GL.Call("getAttribLocation", r.program, "aTrailT")
	r.aDwell = glctx.GL.Call("getAttribLocation", r.program, "aDwell")
	// Geometry / indexed paths leave the dwell attribute array disabled and
	// use this constant instead (uniform brightness).
	glctx.GL.Call("vertexAttrib1f", r.aDwell, 1.0)
	glctx.GL.Call("useProgram", r.program)

	// Set stride-4 attribute pointers (16 bytes per vertex: x,y,z,t)
	glctx.GL.Call("vertexAttribPointer", r.aPosition, 3, glctx.Types.Float, false, 16, 0)
	glctx.GL.Call("enableVertexAttribArray", r.aPosition)
	glctx.GL.Call("vertexAttribPointer", r.aTrailT, 1, glctx.Types.Float, false, 16, 12)
	glctx.GL.Call("enableVertexAttribArray", r.aTrailT)

	r.u.baseColor = glctx.GL.Call("getUniformLocation", r.program, "uBaseColor")
	r.u.topColor = glctx.GL.Call("getUniformLocation", r.program, "uTopColor")
	r.u.midColor = glctx.GL.Call("getUniformLocation", r.program, "uMidColor")
	r.u.minZ = glctx.GL.Call("getUniformLocation", r.program, "uMinZ")
	r.u.maxZ = glctx.GL.Call("getUniformLocation", r.program, "uMaxZ")
	r.u.minX = glctx.GL.Call("getUniformLocation", r.program, "uMinX")
	r.u.maxX = glctx.GL.Call("getUniformLocation", r.program, "uMaxX")
	r.u.minY = glctx.GL.Call("getUniformLocation", r.program, "uMinY")
	r.u.maxY = glctx.GL.Call("getUniformLocation", r.program, "uMaxY")
	r.u.gradientSource = glctx.GL.Call("getUniformLocation", r.program, "uGradientSource")
	r.u.audioLUT = glctx.GL.Call("getUniformLocation", r.program, "uAudioLUT")
	r.u.palette = glctx.GL.Call("getUniformLocation", r.program, "uPalette")
	r.u.dashDuty = glctx.GL.Call("getUniformLocation", r.program, "uDashDuty")
	r.u.dashCount = glctx.GL.Call("getUniformLocation", r.program, "uDashCount")
	// The colormap lives on its own texture unit; tell the sampler which.
	glctx.GL.Call("uniform1i", r.u.palette, paletteUnit)
	r.u.gradientColors = glctx.GL.Call("getUniformLocation", r.program, "uGradientColors")
	r.u.gradientFreq = glctx.GL.Call("getUniformLocation", r.program, "uGradientFreq")
	r.u.gradientPhase = glctx.GL.Call("getUniformLocation", r.program, "uGradientPhase")
	r.u.paletteShift = glctx.GL.Call("getUniformLocation", r.program, "uPaletteShift")
	r.u.gradientReverse = glctx.GL.Call("getUniformLocation", r.program, "uGradientReverse")
	r.u.pointSize = glctx.GL.Call("getUniformLocation", r.program, "uPointSize")
	r.u.model = glctx.GL.Call("getUniformLocation", r.program, "Mmatrix")
	r.u.view = glctx.GL.Call("getUniformLocation", r.program, "Vmatrix")
	r.u.trailHead = glctx.GL.Call("getUniformLocation", r.program, "uTrailHead")
	glctx.GL.Call("uniform1f", r.u.trailHead, 0)
	r.u.splitZ = glctx.GL.Call("getUniformLocation", r.program, "uSplitZ")
	r.u.splitSide = glctx.GL.Call("getUniformLocation", r.program, "uSplitSide")
	setSplitPlane(splitNone, 0)
	glctx.GL.Call("uniform1f", r.u.pointSize, 2.0)
	glctx.GL.Call("uniform3f", r.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
	glctx.GL.Call("uniform3f", r.u.topColor, style.topColor[0], style.topColor[1], style.topColor[2])
	glctx.GL.Call("uniform3f", r.u.midColor, style.midColor[0], style.midColor[1], style.midColor[2])
	glctx.GL.Call("uniform1f", r.u.minZ, float64(-1))
	glctx.GL.Call("uniform1f", r.u.maxZ, float64(1))
	glctx.GL.Call("uniform1f", r.u.minX, float64(-1))
	glctx.GL.Call("uniform1f", r.u.maxX, float64(1))
	glctx.GL.Call("uniform1f", r.u.minY, float64(-1))
	glctx.GL.Call("uniform1f", r.u.maxY, float64(1))
	glctx.GL.Call("uniform1i", r.u.gradientSource, 2) // Z
	glctx.GL.Call("uniform1i", r.u.gradientColors, 2) // two-color
	glctx.GL.Call("uniform1i", r.u.gradientReverse, 0)
	r.ready = true

	glctx.GL.Call("clearColor", 0, 0, 0, 0)
	glctx.GL.Call("clearDepth", 1.0)
	glctx.GL.Call("viewport", 0, 0, r.width, r.height)
	glctx.GL.Call("depthFunc", glctx.Types.LEqual)
}

func setupMatrices() {
	// gpu.proj is a pkg var (textured_js.go) so texp.program can reuse it.
	// Far plane must clear the auto-fit camera distance (maxExtent·3, capped at
	// 300) PLUS the model's own extent (~maxExtent) PLUS the zoom-out range —
	// otherwise the back of a large attractor pokes past the far plane and gets
	// clipped ("cutting through the black background"). 1500 covers the worst
	// case with margin.
	gpu.proj = mgl32.Perspective(mgl32.DegToRad(45.0), float32(gpu.width)/float32(gpu.height), 1, 1500.0)
	glctx.GL.Call("useProgram", gpu.program)
	glctx.GL.Call("uniformMatrix4fv", glctx.GL.Call("getUniformLocation", gpu.program, "Pmatrix"), false, texp.mat4ToTyped(&gpu.proj))

	view.modelMat = mgl32.Ident4()
	view.updateViewMatrix()
	view.updateModelMatrix()
}

// updateViewMatrix recomputes the camera and uploads it to the attractor
// program. texp.program receives it separately (useTexProgram reads the
// camera's viewMat), so we force the attractor program active here to
// keep the upload correct even if a textured draw left texp.program bound.
func (c *camera) updateViewMatrix() {
	cameraPosition := mgl32.Vec3{-c.panX, -c.panY, c.defaultDist}
	center := mgl32.Vec3{-c.panX, -c.panY, 0.0}
	c.viewMat = mgl32.LookAtV(cameraPosition, center, mgl32.Vec3{0.0, 1.0, 0.0})
	glctx.GL.Call("useProgram", gpu.program)
	glctx.GL.Call("uniformMatrix4fv", gpu.u.view, false, texp.mat4ToTyped(&c.viewMat))
}

func (c *camera) updateModelMatrix() {
	glctx.GL.Call("useProgram", gpu.program)
	glctx.GL.Call("uniformMatrix4fv", gpu.u.model, false, texp.mat4ToTyped(&c.modelMat))
}

// autoFitCamera fits the camera to what was last uploaded.
//
// view.fitOverride, when set, provides the TRUE extent of the attractor
// (measured during a warmup) instead of the current trail's — for systems
// whose visible window is only a small arc of a much larger structure
// (hyper-Rössler), fitting the instantaneous arc left the camera blind for
// most of the orbit. It is consumed and cleared here.
func (vs *viewState) autoFitCamera() {
	if len(gpu.verts) < 3 {
		return
	}
	maxAbs := float32(0)
	for i := 0; i < len(gpu.verts); i++ {
		v := gpu.verts[i]
		if v < 0 {
			v = -v
		}
		if v > maxAbs {
			maxAbs = v
		}
	}
	if vs.fitOverride > 0 {
		maxAbs = vs.fitOverride
		vs.fitOverride = 0
	}
	// Kept because the depth partition needs to know how deep the model is:
	// the plane sweeps from just beyond its far side to just in front of its
	// near one, and that span is this number. See split_js.go.
	vs.fitExtent = maxAbs
	dist := fitDistFor(maxAbs)
	vs.initDist = dist
	vs.defaultDist = dist
	camPanel.cameraControl.Set("value", "0")
	camPanel.sliderZoom.Set("textContent", "0")
	vs.updateViewMatrix()
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
	if w, h := glctx.Canvas.Get("clientWidth").Float(), glctx.Canvas.Get("clientHeight").Float(); w > 0 && h > 0 && w < h {
		dist *= float32(h / w)
	}
	// A grid cell is a fraction of the canvas, and the viewport maps the
	// same NDC cube into it, so a fit made for the whole canvas draws the
	// figure at 1/cols by 1/rows inside the cell and leaves the rest of it
	// black. Coming in by min(cols, rows) magnifies by exactly the amount
	// the tighter axis lost: at 3x3 the figure fills the cell, and at 2x1 —
	// full-height cells that only lost width — the factor is 1 and nothing
	// moves, which is what the A/B view has always done.
	dist /= float32(gridFitFactor(grid.n()))
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
	// (texp.program); update its texture and draw it, then bail out of the
	// attractor path.
	if isSpectroSurface(mode) {
		spect.renderSpectrogramMode(frameNowMs)
		return
	}
	// The recurrence plot is a texture on a plane too — its own square one
	// rather than the spectrogram's landscape one — so it takes the same early
	// exit. Its generator is in the registry like every other mode's; it is
	// dispatched here instead of at the bottom of this function because the
	// vertex pipeline in between would bind its shader over the plot's.
	// The scope face goes up BEFORE the trace, so the trace is drawn over
	// its own graticule rather than under it. Behind the early exits above
	// because those modes are textured planes, and a ruled face behind a
	// spectrogram would be a ruler behind a photograph.
	if scopeFaceOn() {
		drawScopeGraticule(view.fitExtent)
	}
	if mode == "recurrence" {
		if fn := modeGenerate[mode]; fn != nil {
			fn()
		}
		return
	}
	// Spectrogram skin: paint the live texture onto a surface model
	// instead of its wireframe, drawn through the same textured pipeline.
	if skin.spectroSkin() && isSkinnable(mode) {
		skin.renderSkinnedMode(mode, frameNowMs)
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
	if aud.modeActive {
		aud.deactivateAudioMode()
	}
	// Ensure the attractor program is bound — a prior spectrogram frame
	// leaves texp.program active, and the uniform/draw calls below apply to
	// whatever program is current.
	if !gpu.program.IsUndefined() {
		glctx.GL.Call("useProgram", gpu.program)
	}
	if gpu.ready {
		glctx.GL.Call("uniform1i", gpu.u.gradientSource, style.gradientSource)
		// Only when it is being used: the fill runs a short FFT per table slot,
		// which is not work to do for a figure colored by Z.
		if gradientSourceIsAudio(style.gradientSource) {
			acolor.updateAudioColorLUT(run.selectedMode)
			glctx.GL.Call("uniform1fv", gpu.u.audioLUT, acolor.lutToTyped())
		}
		glctx.GL.Call("uniform1i", gpu.u.gradientColors, gradientColorsUniform())
		// Uploaded before the draw that reads it, and only when a colormap is
		// actually selected — the upload is skipped on the palettes that do not
		// sample it, and a failed build falls back to the two-color mix rather
		// than sampling a texture that is not there.
		if !pal.ensurePaletteTexture(style.gradientColors) && gradientColorsUniform() >= colormap.First {
			glctx.GL.Call("uniform1i", gpu.u.gradientColors, 2)
		}
		updateDashFromPointCount(gpu.lastDrawn)
		glctx.GL.Call("uniform1f", gpu.u.dashDuty, dashDuty)
		glctx.GL.Call("uniform1f", gpu.u.dashCount, dashCount)
		glctx.GL.Call("uniform1f", gpu.u.gradientFreq, style.gradientFreq)
		// The colormap window's other half. Uploaded beside the period it pairs
		// with rather than under a "is this a colormap" test: the branch that
		// reads it is in the shader already, and a second copy of that
		// condition here is a second thing to keep in step with colormap.First.
		// Read AFTER applyViewModulation (which runs before generateForMode
		// gets here), so a shift routed from audio lands on this frame rather
		// than the next one — the same ordering the rainbow period depends on.
		glctx.GL.Call("uniform1f", gpu.u.paletteShift, gradientShift)
		// Animate the rainbow: advance the hue offset each frame so the
		// spectrum flows. At a low period only a slice is visible at once,
		// and it cycles gradually through all colors over time rather than
		// staying stuck on part of the spectrum.
		//
		// Not when the trail parameter is a turtle path's tint, though. That is
		// a set of six colors standing for six things — which pass laid a step
		// down, how many times it has been walked — rather than a spectrum to
		// flow along, and rotating the hue makes every segment already on screen
		// change color for no reason anything in the figure did.
		// Nor when the color is following the SOUND. The same argument as the
		// turtle case below, and it bites harder: the whole claim of the audio
		// source is that a stretch of trail is this color BECAUSE of what was
		// playing when it was drawn. A hue offset marching under it at a fixed
		// rate makes every segment already on screen change color for a reason
		// nothing in the sound did — and since the phase advances every frame
		// while the spectrum only sometimes moves, the drift is what the eye
		// picks up. It reads as "the rainbow is cycling", which is precisely
		// the reading that hides the feature.
		if !(run.selectedMode == "turtle" && style.gradientSource == 3) && !gradientSourceIsAudio(style.gradientSource) {
			style.gradientPhase += 0.003
			if style.gradientPhase >= 1 {
				style.gradientPhase -= 1
			}
		}
		glctx.GL.Call("uniform1f", gpu.u.gradientPhase, style.gradientPhase)
		if style.gradientReverse {
			glctx.GL.Call("uniform1i", gpu.u.gradientReverse, 1)
		} else {
			glctx.GL.Call("uniform1i", gpu.u.gradientReverse, 0)
		}
		// Scope phosphor: override the gradient with the phosphor's mono color.
		if phos.active() {
			phos.applyPhosphorColor()
		}
	}
	// Audio-reactive: modulate ODE params (dt, primary chaos param) for
	// this integration step, and the colors / point size for this frame.
	// No-op unless audio-reactive is on and the mode is an attractor.
	saved := applyAudioModulation(mode)
	// Track the most recent real flow mode — the bifurcation explorer sweeps
	// it and the Poincaré section sections it. Both are excluded by name: they
	// are not flows, but dynamics.FlowFor4 also answers from integrate3D's per-frame
	// capture, so a mode that ever reached that loop could name ITSELF as its
	// own source and section its own scatter.
	if _, isFlow := dynamics.FlowFor4(mode); isFlow && mode != "bifurcation" && mode != "poincare" {
		bif.lastFlowMode = mode
	}
	// Twin-trajectory divergence (Trace > Twin): draws both copies itself.
	if twin.tick(mode) {
		restoreAudioModulation(saved)
		sect.tick(mode)
		return
	}
	// Ring-trail beam step (Trace > Ring): draws the frame itself when active
	// and primed; otherwise the scan generator below runs (and primes it).
	if ring.tick(mode) {
		restoreAudioModulation(saved)
		sect.tick(mode)
		return
	}
	if fn := modeGenerate[mode]; fn != nil {
		fn()
	}
	restoreAudioModulation(saved)
	ring.primeAfterScan(mode)
	sect.tick(mode) // Poincaré overlay draws above the finished trail
}

// tmark is the previous frame's timestamp, which renderLoop measures the
// frame from.
var tmark float32

func renderLoop(this js.Value, args []js.Value) any {
	// The rack scope is its own instrument on its own canvas: it draws
	// every frame regardless of what the model is doing, and before the
	// early exits below, because a scope that goes dark when the MODEL
	// knob moves to a polyhedron is not an instrument in the rack.
	scopeMark := timingStart()
	rscope.drawRackScope()
	tpanel.budget.Scope += scopeMark.ms()
	// Stop button: clear once, do not reschedule. Loop dies here.
	if run.stopped {
		glctx.GL.Call("clearColor", 0, 0, 0, 0)
		glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
		glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
		return nil
	}

	// Audio modes (spectrogram, xy) render with their own shader
	// program on the shared #gocanvas. Route them here so we skip the
	// entire 3D attractor pipeline for the frame. Transition helpers
	// swap the useProgram binding when moving between attractor and
	// audio modes so neither pipeline sees the other's state.
	if len(args) > 0 {
		frameNowMs = args[0].Float() // rAF timestamp (ms), used by spectrogram scroll
		tpanel.frame(frameNowMs)     // the rack's own frame meter; see timing.go
		// Latched here rather than at the end of the frame because renderLoop
		// has four exits and a meter that misses the paused one would go blank
		// exactly when someone stopped to read it. The window is thirty frames
		// long, so latching before this frame's spans land costs nothing.
		tpanel.tick(frameNowMs)
		jamTick(frameNowMs)
	}

	// Everything from the tap to the last sequencer clock is the METERS span
	// of the frame budget: the work the rack does on its own audio rather than
	// on the model. It is the share the analyzers' own knobs move, so it is
	// the one worth showing beside the model's.
	metersMark := timingStart()

	// Fan the audio stream out for this frame BEFORE anything reads it. Every
	// consumer below (the counter here, the backdrop and the model later) takes
	// its own copy from the tap; draining the source twice would split it.
	// This sits ahead of the audio-mode return below, which is a live frame too.
	tap.pump()

	// Refresh audio features (no-op unless audio-reactive is on); Phase 2
	// mappings read these to modulate the attractors.
	af.updateAudioFeatures()
	counter.tick() // frequency-counter gate (no-op unless the module is on)
	// The three window analyzers, here or elsewhere. mc.workerTick hands
	// the audio to the worker and reports that it owns them; when there is no
	// worker it reports false and they run on this thread exactly as before.
	// See metersclient_js.go.
	if !mc.workerTick() {
		thd.tick(frameNowMs)  // distortion analysis (no-op unless the module is on screen)
		lufs.tick(frameNowMs) // loudness (no-op unless the module is on screen)
		wow.tick(frameNowMs)  // wow & flutter (no-op unless the module is on screen)
	}
	genEnvTick() // Envelope module shaper (no-op unless the gen audio runs)
	tm.tick()    // Tonematrix sequencer clock (no-op unless the module runs)
	rhy.tick()   // Rhythm section clock (no-op unless the module runs)
	tpanel.budget.Meters += metersMark.ms()

	if isAudioMode(run.selectedMode) {
		if !aud.modeActive {
			aud.activateAudioMode()
		}
		renderAudioFrame(run.selectedMode)
		js.Global().Call("requestAnimationFrame", renderFrame)
		return nil
	}
	if aud.modeActive {
		aud.deactivateAudioMode()
	}

	now := float32(args[0].Float())
	tdiff := now - tmark
	tmark = now

	// Debug frame timing
	if debugEnabled && fstats.lastStart > 0 {
		frameMs := now - fstats.lastStart
		fstats.count++
		fstats.totalMs += frameMs
		if frameMs < fstats.minMs {
			fstats.minMs = frameMs
		}
		if frameMs > fstats.maxMs {
			fstats.maxMs = frameMs
		}
	}
	fstats.lastStart = now

	glctx.GL.Call("enable", glctx.Types.DepthTest)
	bgOn := bgVisualActive()
	if phos.active() {
		// Phosphor persistence: don't clear the color buffer — fade it toward
		// black by the phosphor's decay so old trace lingers as an afterglow.
		// Clear depth only so the new trace still draws on top.
		glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
		phos.drawPhosphorFade()
		glctx.GL.Call("enable", glctx.Types.DepthTest)
	} else if !style.persistTrail || bgOn {
		// A background visualizer repaints the whole backdrop each frame, so
		// the color buffer must be cleared first even if Persist is on (a
		// persisted trail can't coexist with a live scrolling backdrop).
		glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
		glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
	} else {
		// Only clear depth so new draws appear on top, but keep color buffer (old trails)
		glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
	}

	// Background audio visualizer: paint the reactive backdrop (spectrogram or
	// xy scope) behind the model, then hand the depth-tested attractor pipeline
	// a clean depth buffer to draw over it.
	if bgOn {
		renderBackgroundVisual(frameNowMs)
		glctx.GL.Call("enable", glctx.Types.DepthTest)
		glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
		// The backdrop bound its own program / buffers; force geometry models
		// to re-upload their static vertex+index buffers next draw.
		if !isAttractorMode(run.selectedMode) {
			gpu.staticDirty = true
		}
	}

	if run.paused {
		// Redraw current geometry without advancing trail / auto-rotate.
		// Attractors use drawArrays with the line-strip buffer
		// (pausedCount = last frame's step count). Polyhedra +
		// geometry primitives use drawElements via generateForMode,
		// so for those we need to re-emit the geometry — drawArrays
		// alone would not consult the index buffer and the canvas
		// goes blank. Both paths skip the integrator step so the
		// visual snapshot is preserved.
		if isAttractorMode(run.selectedMode) {
			glctx.GL.Call("drawArrays", mapDrawMode(run.selectedMode), 0, run.pausedCount)
		} else {
			generateForMode(run.selectedMode)
		}
		// Still allow camera interaction while paused (zoom read
		// from the Go-side cache instead of parseFloat per frame).
		zoomVal := view.ctl.zoom
		newDist := view.initDist - zoomVal
		if newDist != view.defaultDist {
			view.defaultDist = newDist
			view.updateViewMatrix()
		}
		// Still allow drag while paused (view.ball.orient is updated by the
		// pointer handlers; rebuild+upload so it shows).
		view.rebuildModelMatrix()
		view.updateModelMatrix()
		js.Global().Call("requestAnimationFrame", renderFrame)
		return nil
	}

	// Audio-modulate the camera/motion controls (zoom, pan, spin rates, line)
	// AND the rainbow period in place before they're consumed; restored at
	// frame end so the base values (and their knobs) are unchanged. This must
	// run BEFORE generateForMode, which reads gradientFreq into the shader and
	// draws — otherwise the rainbow-period mod lands a frame too late (i.e.
	// never takes visible effect).
	viewSaved := pmod.applyViewModulation()

	// CRT (phosphor) mode draws a real scope trace: instead of redrawing the
	// whole curve every frame, draw only a short advancing beam and let the
	// phosphor persistence (the non-cleared, fading framebuffer) leave the trail
	// behind it. The trail length then comes from the phosphor's afterglow, not
	// the geometric point count — so the Trail control is dimmed in this mode.
	// Implemented by shrinking `steps` (which the generators use for both the
	// integrate-count and the draw-count) just around generateForMode.
	realSteps := sim.steps
	if crtBeam() && crtBeamLen < sim.steps {
		sim.steps = crtBeamLen
	}
	// With the Fore knob at either end this is one pass, exactly as it always
	// was. In between, the model is drawn twice — near half onto the canvas
	// above the panel, far half onto the one below it — which is the only way
	// to have DOM sitting between two parts of one scene.
	// The MODEL span of the frame budget: the passes that put the instrument
	// on the canvas, which is what the rack is for.
	modelMark := timingStart()
	switch {
	case splitDrawing():
		// The Fore knob owns the passes when it is in play: near and far
		// of one scene are what it exists to draw, and stacking a
		// side-by-side split inside that is two splits arguing.
		drawSplitPasses(run.selectedMode)
	default:
		// One view or two, side by side. See views_js.go.
		drawViewPasses(run.selectedMode)
	}
	tpanel.budget.Model += modelMark.ms()
	// The gradient extents, if the mode change could not take them: an audio
	// mode has no geometry on its first frame, and this is the first frame that
	// does. Costs one comparison per frame once it has been paid.
	if gradientRangeDue() {
		refreshGradient()
	}
	// The lens goes last: it re-reads the finished frame through the fluid
	// surface, so everything drawn above — model, backdrop, overlays — is what
	// it refracts. Before the model it would only distort an empty buffer.
	if waterActive() {
		water.drawWaterLens()
	}

	beamDrawn := gpu.lastDrawn // what was drawn, which is not always all of `steps`
	sim.steps = realSteps

	// Slider values come from view.ctl.zoom/RotX/Y/Z, kept in sync by
	// input listeners in Run(). Eliminates 4 parseFloat round-trips
	// + 4 textContent writes per frame.
	zoomVal := view.ctl.zoom
	rotationX, rotationY, rotationZ := view.ctl.spinX, view.ctl.spinY, view.ctl.spinZ
	// Auto-rotate is added HERE rather than being written into the Y rate
	// slider. Baking it into the slider made the switch and the rate two
	// representations of one thing that every code path had to keep in step,
	// and they came apart constantly: the permalink serializes ar and ry
	// independently, so restoring a link subtracted a contribution that had
	// never been added (the model span with the switch reading off, and
	// switching it on canceled the rate to zero — the control read backwards)
	// or added one that was already there (the rate crept +0.1 per reload).
	// As a separate term there is nothing to keep in sync: the slider holds
	// only what the user asked for, and the switch means what it says.
	if view.ctl.autoRotate {
		rotationY += autoRotYDelta
	}

	// Zoom slider directly controls camera distance (absolute position);
	// the X/Y position knobs pan the scene. Both go through the view matrix.
	newDist := view.initDist - zoomVal
	// Map the ±8 X/Y position sliders to ≈±1 screen of travel, so the model can
	// be pushed just off-screen (like an oscilloscope's position controls) yet
	// not miles away. The screen half-extent at the object plane is
	// dist·tan(fov/2); scale X by the aspect so ±8 spans a full screen width and
	// ±8 spans a full screen height — and it tracks zoom so a given setting keeps
	// the model at the same screen fraction.
	halfH := newDist * 0.41421 // tan(22.5°), half the 45° vertical FOV
	aspect := float32(1.6)
	if gpu.height > 0 {
		aspect = float32(gpu.width) / float32(gpu.height)
	}
	psY := halfH * 2 / 8
	psX := psY * aspect
	npx, npy := view.ctl.panX*psX, view.ctl.panY*psY
	if newDist != view.defaultDist || npx != view.panX || npy != view.panY {
		view.defaultDist = newDist
		view.panX, view.panY = npx, npy
		view.updateViewMatrix()
	}
	// Advance the absolute angles by the per-axis spin rate (the X/Y/Z
	// rate sliders), scaled by how long the frame actually took so the
	// rate is per SECOND and not per frame. See frameScale: a fixed step
	// per frame makes every dropped frame a visible hesitation and every
	// refresh rate a different speed.
	fs := frameScale(tdiff)
	if rotationX != 0 {
		view.angleX += rotationX / 20 * fs
	}
	if rotationY != 0 {
		view.angleY += rotationY / 20 * fs
	}
	if rotationZ != 0 {
		view.angleZ += rotationZ / 20 * fs
	}

	view.angleX = wrapTwoPi(view.angleX)
	view.angleY = wrapTwoPi(view.angleY)
	view.angleZ = wrapTwoPi(view.angleZ)
	view.rebuildModelMatrix()
	rotKnobs.update()

	view.updateModelMatrix()
	restoreAudioModulation(viewSaved) // put zoom/pan/spin base values back
	run.pausedCount = beamDrawn       // paused CRT keeps showing just the beam

	js.Global().Call("requestAnimationFrame", renderFrame)
	return nil
}
