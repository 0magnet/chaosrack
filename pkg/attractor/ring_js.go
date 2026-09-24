//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"math"
	"runtime"
	"strconv"
	"syscall/js"
)

// Ring-trail mode (the Trace > Ring switch): instead of re-integrating the
// ENTIRE trail every frame (scan mode — whole-curve response to knob/audio
// changes, but up to steps×speedSteps ODE steps per frame), the trail lives
// in a persistent ring buffer and only the advancing BEAM writes new points —
// like a real scope: the head sweeps, the tail is history. Parameter changes
// bend the path from the head forward while the old trail scrolls out under
// the old dynamics. Cost per frame drops from `steps` integrations to
// ringPointsPerFrame, independent of trail length.
//
// Implementation notes:
//   - The per-vertex trail parameter t=i/(steps-1) stays STATIC; the shader
//     gets uTrailHead and computes the age as fract(t−head), so the gradient
//     follows the beam without rewriting the buffer.
//   - Only the newly written segment is uploaded (bufferSubData), wrap-aware.
//   - The strip is drawn as two ranges split at the head so no line connects
//     the newest point back to the oldest. (The old→new seam at the buffer
//     wrap is one undrawn segment among `steps` — invisible in practice.)
//   - Centering is FROZEN at prime time (scan mode re-centers every frame):
//     the GPU already holds old points centered by the primed offset, so a
//     drifting offset would shear the trail.
//   - Priming = let the mode's normal scan generator run one frame (fills
//     the buffer, warms centerOffset, and — for integrate3D modes — captures
//     the vector field into the flow registry); the beam takes over next
//     frame via dynamics.FlowFor4 (3D flows lifted with w≡0, so 4D equation modes —
//     hyperrossler, custom — ring like everything else). Modes without a
//     registered field (parametric, geometry) simply stay in scan mode.

const ringPointsPerFrame = 120 // beam advance per frame at speed 1

// ringTrail is the ring trail: the circular vertex buffer the Trace > Ring
// switch draws from.
type ringTrail struct {
	on        bool
	head      int    // next slot to overwrite (the OLDEST point)
	sig       string // mode|steps the ring was primed for ("" = not primed)
	center    [3]float32
	x         float64 // beam integrator state (float64, like integrate3D)
	dwellMean float32
	y         float64
	z         float64
	w         float64 // hidden 4th state for 4D flows

	// Persistent scratch for segment uploads (sized to the largest segment seen).
	segUint8 js.Value
	segCap   int
}

var ring = ringTrail{
	dwellMean: 1,
}

// ringInvalidate forces a re-prime (mode/trail-length/reset changes).
func (r *ringTrail) invalidate() { r.sig = "" }

// ringTick advances and draws the ring trail. Returns false when the caller's
// normal scan generator should run instead (ring off, mode has no registered
// flow, or the ring isn't primed for this mode/steps yet — the scan frame it
// falls back to IS the priming pass).
func (r *ringTrail) tick(mode string) bool {
	if !r.on {
		return false
	}
	sys, ok := dynamics.FlowFor4(mode)
	if !ok {
		return false
	}
	sig := mode + "|" + strconv.Itoa(steps)
	if r.sig != sig {
		return false // let the scan generator prime this frame; see below
	}

	// Advance the beam: fixed points/frame, each integrated with the same
	// dt·speedScale and speedSteps sub-steps as scan mode, so the trajectory
	// (and the Speed knob's meaning) is unchanged — only the redraw model is.
	// The sub-steps get the same anti-freeze budget as the scan generators
	// (interpreted equation systems are ~10× the per-step cost).
	n := ringPointsPerFrame
	if n > steps {
		n = steps
	}
	budget := frameBudgetCompiled
	if sys.Interpreted {
		budget = frameBudgetInterpreted
	}
	sub := effSubSteps(speedSteps, n, budget)
	dt := sys.Dt() * float64(speedScale)
	const lim = 1e4
	start := r.head
	invN := float32(1) / float32(steps-1)
	scale := sys.Scale
	for i := 0; i < n; i++ {
		for s := 0; s < sub; s++ {
			dx, dy, dz, dw := sys.F(r.x, r.y, r.z, r.w)
			r.x += dt * dx
			r.y += dt * dy
			r.z += dt * dz
			r.w += dt * dw
			if !(r.x > -lim && r.x < lim && r.y > -lim && r.y < lim && r.z > -lim && r.z < lim && r.w > -lim && r.w < lim) {
				ic := dynamics.InitCond[mode]
				r.x, r.y, r.z = float64(ic[0]), float64(ic[1]), float64(ic[2])
				r.w = 0
			}
		}
		j := r.head * 4
		vertBuf[j] = float32(r.x)*scale - r.center[0]
		vertBuf[j+1] = float32(r.y)*scale - r.center[1]
		vertBuf[j+2] = float32(r.z)*scale - r.center[2]
		vertBuf[j+3] = float32(r.head) * invN
		r.head++
		if r.head == steps {
			r.head = 0
		}
	}
	// Keep the render-state globals in sync so drag/permalink/sonify (SCAN)
	// and a later switch back to scan mode continue from the beam.
	x, y, z = float32(r.x), float32(r.y), float32(r.z)
	x64, y64, z64 = r.x, r.y, r.z
	sys.SetW(r.w)

	r.uploadAndDraw(start, n)
	return true
}

// ringPrimeAfterScan records the ring baseline right after a scan frame has
// filled the buffer (called at the end of generateForMode). The scan frame
// already uploaded + drew; the beam takes over on the next frame.
func (r *ringTrail) primeAfterScan(mode string) {
	if !r.on {
		return
	}
	sys, ok := dynamics.FlowFor4(mode)
	if !ok {
		return
	}
	sig := mode + "|" + strconv.Itoa(steps)
	if r.sig == sig {
		return
	}
	r.sig = sig
	r.head = 0
	r.center = centerOffset
	r.x, r.y, r.z = x64, y64, z64
	if r.x == 0 && r.y == 0 && r.z == 0 {
		r.x, r.y, r.z = float64(x), float64(y), float64(z)
	}
	r.w = sys.W() // continue the hidden state, not restart it
}

// ringUploadAndDraw pushes the newly written slots to the GPU (wrap-aware)
// and draws the trail as two strips split at the head.
func (r *ringTrail) uploadAndDraw(start, n int) {
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, gpu.vbuf)
	glctx.GL.Call("vertexAttribPointer", gpu.aPosition, 3, glctx.Types.Float, false, 16, 0)
	glctx.GL.Call("enableVertexAttribArray", gpu.aPosition)
	glctx.GL.Call("vertexAttribPointer", gpu.aTrailT, 1, glctx.Types.Float, false, 16, 12)
	glctx.GL.Call("enableVertexAttribArray", gpu.aTrailT)

	upload := func(from, count int) {
		if count <= 0 {
			return
		}
		seg := vertBuf[from*4 : (from+count)*4]
		js.CopyBytesToJS(r.jsSegUint8(len(seg)*4), sliceToByteSlice(seg))
		glctx.GL.Call("bufferSubData", glctx.Types.ArrayBuffer, from*16, r.jsSegView(len(seg)))
	}
	if start+n <= steps {
		upload(start, n)
	} else {
		upload(start, steps-start)
		upload(0, (start+n)-steps)
	}
	runtime.KeepAlive(vertBuf)

	// Dwell for the freshly written slots (mean from the last full-scan pass
	// is close enough between primes; exact per-segment mean would flicker).
	r.updateDwell(start, n)

	glctx.GL.Call("uniform1f", gpu.u.trailHead, float64(r.head)/float64(steps-1))
	// Older stretch: head..end, newer stretch: 0..head. The split prevents a
	// newest→oldest flyback line across the model.
	if steps-r.head >= 2 {
		glctx.GL.Call("drawArrays", gpu.drawMode, r.head, steps-r.head)
	}
	if r.head >= 2 {
		glctx.GL.Call("drawArrays", gpu.drawMode, 0, r.head)
	}
}

func (r *ringTrail) jsSegUint8(nBytes int) js.Value {
	if nBytes > r.segCap {
		r.segUint8 = js.Global().Get("Uint8Array").New(nBytes)
		r.segCap = nBytes
	}
	return js.Global().Get("Uint8Array").New(r.segUint8.Get("buffer"), 0, nBytes)
}

func (r *ringTrail) jsSegView(nFloats int) js.Value {
	return js.Global().Get("Float32Array").New(r.segUint8.Get("buffer"), 0, nFloats)
}

// ringUpdateDwell refreshes the beam-dwell attribute for the slots the beam
// just rewrote, using the mean already established by the priming scan.
func (r *ringTrail) updateDwell(start, n int) {
	if len(gpu.dwell.buf) != steps || gpu.dwell.gl.IsUndefined() {
		return
	}
	var total float32
	cnt := 0
	upd := func(i int) {
		if i <= 0 || i >= steps {
			return
		}
		a, b := (i-1)*4, i*4
		dx := vertBuf[b] - vertBuf[a]
		dy := vertBuf[b+1] - vertBuf[a+1]
		dz := vertBuf[b+2] - vertBuf[a+2]
		d := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
		total += d
		cnt++
		w := r.dwellMean / (d + r.dwellMean*0.15)
		if w > 1.8 {
			w = 1.8
		} else if w < 0.25 {
			w = 0.25
		}
		gpu.dwell.buf[i] = w
	}
	for k := 0; k < n; k++ {
		upd((start + k) % steps)
	}
	if cnt > 0 { // slow-track the mean so long ring sessions stay calibrated
		r.dwellMean += (total/float32(cnt) - r.dwellMean) * 0.02
		if r.dwellMean <= 0 {
			r.dwellMean = 1e-6
		}
	}
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, gpu.dwell.gl)
	js.CopyBytesToJS(gpu.dwell.u8, sliceToByteSlice(gpu.dwell.buf))
	glctx.GL.Call("bufferData", glctx.Types.ArrayBuffer, gpu.dwell.f32, glctx.Types.DynamicDraw)
	glctx.GL.Call("vertexAttribPointer", gpu.aDwell, 1, glctx.Types.Float, false, 0, 0)
	glctx.GL.Call("enableVertexAttribArray", gpu.aDwell)
	glctx.GL.Call("bindBuffer", glctx.Types.ArrayBuffer, gpu.vbuf)
}
