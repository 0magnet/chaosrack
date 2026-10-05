//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/go-gl/mathgl/mgl32"
)

// The vector display (beampath.go): Model Out's B button. Off, a figure is
// drawn whole every frame, as it always was. On, one beam runs round it at
// SPD's speed, measured on the clock and not in frames, and each frame draws
// only the stretch the beam covered since the last; the phosphor holds the
// rest. Model Out plays the same beam, so the circuit the eye sees and the
// period the ear hears are one thing. C chooses whether the beam is measured
// and heard in the model's own coordinates or as the camera sees it.

// beamState is the beam: its switches, the figure as a path, and where on it
// the display's beam is.
type beamState struct {
	on, cam   bool
	capturing bool // the frame's uploads record the figure instead of drawing it

	path beamPath
	seq  uint64 // the upload the path was built from
	mode string

	w      beamWalker
	lastMs float64
	frame  float64 // the frame the display's beam last advanced in
	buf    []float32

	// The circuit's frequency, on the readout line over the equation.
	hzEl   js.Value
	hzText string
	hzAt   float64 // when it was last measured
}

var beam beamState

// beamBase is the beam's speed at SPD 0, in half-screens a second.
const beamBase = 16000

// beamSpeed is SPD as the beam's speed: every 12 up is twice as fast.
func beamSpeed() float64 { return beamBase * math.Pow(2, float64(modelOut.speed)/12) }

// beamGeometry are the solids: a mesh of lines, and no other sound.
func beamGeometry(mode string) bool {
	switch mode {
	case "polyhedron", "nestedcube", "globe", "torus", "magnetosphere", "stlfile":
		return true
	}
	return false
}

// hasBeam: the model is a figure a beam can trace — the solids, and the
// parametric figures drawn as lines. Not the flows, which run on (their
// sound is their equations), nor the maps, which are dots.
func hasBeam(mode string) bool {
	if beamGeometry(mode) {
		return true
	}
	switch mode {
	case "lissajou", "turtle", "graphicartist", "pong", "scopetext", "scopeclock", "bounceball":
		return true
	}
	return false
}

// hasModelOut: Model Out has a signal for the model — its equations, its
// trail, or its beam.
func hasModelOut(mode string) bool { return isAttractorMode(mode) || beamGeometry(mode) }

// beamDraws: this frame the display is the beam's.
func beamDraws(mode string) bool {
	return beam.on && hasBeam(mode) && !style.usePoints && !ring.on
}

// beamSounds: Model Out plays the beam — always for a solid, which has
// nothing else to play, and for a figure while the beam draws it.
func beamSounds(mode string) bool {
	return len(beam.path.pts) > 1 && beam.mode == mode && (beamGeometry(mode) || beamDraws(mode))
}

// capture rebuilds the path from what the model last uploaded, if that is
// new.
func (b *beamState) capture(mode string) {
	if gpu.uploadSeq == b.seq && mode == b.mode {
		return
	}
	b.seq, b.mode = gpu.uploadSeq, mode
	switch t := gpu.lastTrace; {
	case gpu.stride == 3 && len(gpu.indices) > 1:
		b.path.fromLines(gpu.verts, 3, gpu.indices, len(gpu.indices))
	case gpu.stride == 4 && t.ok && t.mode.Equal(glctx.Types.Lines):
		b.path.fromLines(gpu.verts[min(t.first*4, len(gpu.verts)):], 4, nil, t.n)
	case gpu.stride == 4 && t.ok && t.mode.Equal(glctx.Types.LineStrip):
		b.path.fromStrip(gpu.verts, t.first, t.n)
	default:
		b.path.reset()
	}
}

// beamGenerate runs the model's generator: as ever, or — while the beam
// draws — with its uploads recorded rather than drawn, then the beam's frame.
func beamGenerate(mode string, fn func()) {
	if !beamDraws(mode) {
		fn()
		if hasBeam(mode) {
			beam.capture(mode)
			beam.readout(mode)
		}
		return
	}
	beam.capturing = true
	fn()
	beam.capturing = false
	beam.capture(mode)
	beam.draw()
	beam.readout(mode)
}

// beamHzEveryMs is how often the circuit's frequency is measured for its
// readout: a number to be read, not followed.
const beamHzEveryMs = 250

// appendReadout puts the circuit's frequency on mode's readout line
// (liveReadoutHost), built fresh with each panel.
func (b *beamState) appendReadout(host js.Value) {
	b.hzText, b.hzAt = "", 0
	b.hzEl = liveReadout(host, "hz", dispFullChars, beamHzText(0), doc("ro.hz"))
}

// readout measures the circuit — the beam's speed over the figure's length,
// how many times a second it goes round, and the pitch Model Out plays it
// at — while the beam draws or is heard, and blanks it otherwise.
func (b *beamState) readout(mode string) {
	if !b.hzEl.Truthy() || (b.hzAt != 0 && frameNowMs-b.hzAt < beamHzEveryMs) {
		return
	}
	b.hzAt = frameNowMs
	hz := 0.0
	if beamDraws(mode) || beamSounds(mode) {
		if total := b.path.length(beamFrameNow()); total > 0 {
			hz = beamSpeed() / total
		}
	}
	if s := beamHzText(hz); s != b.hzText {
		b.hzText = s
		setDotText(b.hzEl, s)
	}
}

// draw advances the display's beam by the time since the last frame and
// draws what it crossed. Once a frame however many views draw it.
func (b *beamState) draw() {
	if b.frame != frameNowMs {
		dt := (frameNowMs - b.lastMs) / 1000
		if b.lastMs == 0 || !(dt > 0) || dt > 0.25 {
			dt = 1.0 / 60 // the first frame, or one after a stall
		}
		b.lastMs, b.frame = frameNowMs, frameNowMs
		b.buf = b.buf[:0]
		b.w.walk(&b.path, beamFrameNow(), beamSpeed()*dt, b.emit)
	}
	gpu.drawBeam(b.buf)
}

// emit adds one lit stretch of segment seg to the frame's lines, in the
// model's coordinates (the shader places them) and with each end's trail
// parameter, so the beam colors the figure as the whole drawing would.
func (b *beamState) emit(seg int, f0, f1 float64) {
	n := len(b.path.pts)
	a, c := b.path.pts[seg], b.path.pts[(seg+1)%n]
	ta, tc := float32(0), float32(0)
	if gpu.stride == 4 && seg+1 < n {
		ta, tc = b.trailT(seg), b.trailT(seg+1)
	}
	for _, f := range [2]float32{float32(f0), float32(f1)} {
		b.buf = append(b.buf, a[0]+(c[0]-a[0])*f, a[1]+(c[1]-a[1])*f, a[2]+(c[2]-a[2])*f, ta+(tc-ta)*f)
	}
}

// trailT is the trail parameter of a strip's vertex i, as uploaded.
func (b *beamState) trailT(i int) float32 {
	if t := gpu.lastTrace; t.mode.Equal(glctx.Types.LineStrip) {
		if j := (t.first+i)*4 + 3; j < len(gpu.verts) {
			return gpu.verts[j]
		}
	}
	return 0
}

// beamFrameNow is the frame the beam is measured and heard in, as the view
// stands now.
func beamFrameNow() beamFrame {
	if beam.cam {
		return beamCameraFrame()
	}
	return beamModelFrame()
}

// beamModelFrame is the model's own coordinates, scaled by its extent so a
// figure of any size fills about the same swing.
func beamModelFrame() beamFrame {
	s := 1.0
	if e := float64(view.fitExtent); e > 0 {
		s = 1 / e
	}
	return func(p [3]float32) [3]float64 {
		return [3]float64{float64(p[0]) * s, float64(p[1]) * s, float64(p[2]) * s}
	}
}

// beamCameraFrame is the figure as the camera sees it: x and y where the
// point lands on the screen, in half-heights (x widened by the aspect, so a
// circle stays one), and z its depth from the plane the camera looks at, in
// the same units.
func beamCameraFrame() beamFrame {
	mv := view.viewMat.Mul4(view.modelMat)
	mvp := gpu.proj.Mul4(mv)
	aspect := 1.0
	if gpu.height > 0 {
		aspect = float64(gpu.width) / float64(gpu.height)
	}
	half := float64(view.defaultDist) * math.Tan(22.5*math.Pi/180) // the half-height at the center's depth
	if !(half > 0) {
		half = 1
	}
	return func(p [3]float32) [3]float64 {
		v := mgl32.Vec4{p[0], p[1], p[2], 1}
		c := mvp.Mul4x1(v)
		w := float64(c[3])
		if !(w > 1e-6) {
			w = 1e-6
		}
		e := mv.Mul4x1(v)
		return [3]float64{float64(c[0]) / w * aspect, float64(c[1]) / w, (float64(e[2]) + float64(view.defaultDist)) / half}
	}
}

// drawBeam draws the beam's lines for this frame. The buffer is the one the
// models upload into, so the cached mesh is marked stale (the next static
// frame uploads its own again) and the views replay these lines.
func (r *renderer) drawBeam(v []float32) {
	n := len(v) / 4
	r.staticDirty = true
	r.lastTrace.mode, r.lastTrace.first, r.lastTrace.n, r.lastTrace.ok = glctx.Types.Lines, 0, n, n > 0
	if n < 2 {
		return
	}
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, r.vbuf)
	glctx.GL.Call("vertexAttribPointer", r.aPosition, 3, glctx.Types.Float, false, 16, 0)
	glctx.GL.Call("enableVertexAttribArray", r.aPosition)
	glctx.GL.Call("vertexAttribPointer", r.aTrailT, 1, glctx.Types.Float, false, 16, 12)
	glctx.GL.Call("enableVertexAttribArray", r.aTrailT)
	glctx.GL.Call("disableVertexAttribArray", r.aDwell)
	glctx.GL.Call("vertexAttrib1f", r.aDwell, 1.0)
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, SliceToTypedArray(v), glctx.Types.DynamicDraw)
	glctx.GL.Call("uniform1f", r.u.trailHead, 0)
	r.lastDrawn = n
	glctx.GL.Call("drawArrays", glctx.Types.Lines, 0, n)
}

// wireBeamSwitches hooks up B and C (hidden checkboxes, so the link and
// rackctl see them as every other switch).
func wireBeamSwitches() {
	wireSwitch("mo-beam", func(on bool) {
		beam.on = on
		beam.w, beam.lastMs = beamWalker{}, 0
		gpu.staticDirty = true
		son.sync()
	})
	wireSwitch("mo-cam", func(on bool) { beam.cam = on })
}

// SPD's buttons: B the beam, C the camera's coordinates.
func init() {
	toggle := func(id string) { setSwitch(id, !checkedOn(id)) }
	trioPrograms["mo-spd"] = trioProgram{
		keys: []string{"B", "C", ""},
		help: []string{doc("trio.mo-spd=0"), doc("trio.mo-spd=1")},
		press: func(i int) {
			switch i {
			case 0:
				toggle("mo-beam")
			case 1:
				toggle("mo-cam")
			}
		},
		lits:  func() []bool { return []bool{checkedOn("mo-beam"), checkedOn("mo-cam"), false} },
		drive: []string{"mo-beam", "mo-cam"},
	}
}
