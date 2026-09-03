//go:build js && wasm

package attractor

import (
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
//     frame via flowFor4 (3D flows lifted with w≡0, so 4D equation modes —
//     hyperrossler, custom — ring like everything else). Modes without a
//     registered field (parametric, geometry) simply stay in scan mode.

const ringPointsPerFrame = 120 // beam advance per frame at speed 1

var (
	ringOn        bool
	ringHead      int    // next slot to overwrite (the OLDEST point)
	ringSig       string // mode|steps the ring was primed for ("" = not primed)
	ringCenter    [3]float32
	ringX         float64 // beam integrator state (float64, like integrate3D)
	ringDwellMean float32 = 1
	ringY         float64
	ringZ         float64
	ringW         float64 // hidden 4th state for 4D flows
)

// ringInvalidate forces a re-prime (mode/trail-length/reset changes).
func ringInvalidate() { ringSig = "" }

// ringTick advances and draws the ring trail. Returns false when the caller's
// normal scan generator should run instead (ring off, mode has no registered
// flow, or the ring isn't primed for this mode/steps yet — the scan frame it
// falls back to IS the priming pass).
func ringTick(mode string) bool {
	if !ringOn {
		return false
	}
	sys, ok := flowFor4(mode)
	if !ok {
		return false
	}
	sig := mode + "|" + strconv.Itoa(steps)
	if ringSig != sig {
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
	if sys.interpreted {
		budget = frameBudgetInterpreted
	}
	sub := effSubSteps(speedSteps, n, budget)
	dt := sys.dt() * float64(speedScale)
	const lim = 1e4
	start := ringHead
	invN := float32(1) / float32(steps-1)
	scale := sys.scale
	for i := 0; i < n; i++ {
		for s := 0; s < sub; s++ {
			dx, dy, dz, dw := sys.f(ringX, ringY, ringZ, ringW)
			ringX += dt * dx
			ringY += dt * dy
			ringZ += dt * dz
			ringW += dt * dw
			if !(ringX > -lim && ringX < lim && ringY > -lim && ringY < lim && ringZ > -lim && ringZ < lim && ringW > -lim && ringW < lim) {
				ic := attractorInitCond[mode]
				ringX, ringY, ringZ = float64(ic[0]), float64(ic[1]), float64(ic[2])
				ringW = 0
			}
		}
		j := ringHead * 4
		vertBuf[j] = float32(ringX)*scale - ringCenter[0]
		vertBuf[j+1] = float32(ringY)*scale - ringCenter[1]
		vertBuf[j+2] = float32(ringZ)*scale - ringCenter[2]
		vertBuf[j+3] = float32(ringHead) * invN
		ringHead++
		if ringHead == steps {
			ringHead = 0
		}
	}
	// Keep the render-state globals in sync so drag/permalink/sonify (SCAN)
	// and a later switch back to scan mode continue from the beam.
	x, y, z = float32(ringX), float32(ringY), float32(ringZ)
	x64, y64, z64 = ringX, ringY, ringZ
	sys.setW(ringW)

	ringUploadAndDraw(start, n)
	return true
}

// ringPrimeAfterScan records the ring baseline right after a scan frame has
// filled the buffer (called at the end of generateForMode). The scan frame
// already uploaded + drew; the beam takes over on the next frame.
func ringPrimeAfterScan(mode string) {
	if !ringOn {
		return
	}
	sys, ok := flowFor4(mode)
	if !ok {
		return
	}
	sig := mode + "|" + strconv.Itoa(steps)
	if ringSig == sig {
		return
	}
	ringSig = sig
	ringHead = 0
	ringCenter = centerOffset
	ringX, ringY, ringZ = x64, y64, z64
	if ringX == 0 && ringY == 0 && ringZ == 0 {
		ringX, ringY, ringZ = float64(x), float64(y), float64(z)
	}
	ringW = sys.w() // continue the hidden state, not restart it
}

// ringUploadAndDraw pushes the newly written slots to the GPU (wrap-aware)
// and draws the trail as two strips split at the head.
func ringUploadAndDraw(start, n int) {
	gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
	gl.Call("vertexAttribPointer", positionLoc, 3, glTypes.Float, false, 16, 0)
	gl.Call("enableVertexAttribArray", positionLoc)
	gl.Call("vertexAttribPointer", aTrailTLoc, 1, glTypes.Float, false, 16, 12)
	gl.Call("enableVertexAttribArray", aTrailTLoc)

	upload := func(from, count int) {
		if count <= 0 {
			return
		}
		seg := vertBuf[from*4 : (from+count)*4]
		js.CopyBytesToJS(jsSegUint8(len(seg)*4), sliceToByteSlice(seg))
		gl.Call("bufferSubData", glTypes.ArrayBuffer, from*16, jsSegView(len(seg)))
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
	ringUpdateDwell(start, n)

	gl.Call("uniform1f", uTrailHeadLoc, float64(ringHead)/float64(steps-1))
	// Older stretch: head..end, newer stretch: 0..head. The split prevents a
	// newest→oldest flyback line across the model.
	if steps-ringHead >= 2 {
		gl.Call("drawArrays", attractorDrawMode, ringHead, steps-ringHead)
	}
	if ringHead >= 2 {
		gl.Call("drawArrays", attractorDrawMode, 0, ringHead)
	}
}

// Persistent scratch for segment uploads (sized to the largest segment seen).
var (
	segUint8 js.Value
	segCap   int
)

func jsSegUint8(nBytes int) js.Value {
	if nBytes > segCap {
		segUint8 = js.Global().Get("Uint8Array").New(nBytes)
		segCap = nBytes
	}
	return js.Global().Get("Uint8Array").New(segUint8.Get("buffer"), 0, nBytes)
}

func jsSegView(nFloats int) js.Value {
	return js.Global().Get("Float32Array").New(segUint8.Get("buffer"), 0, nFloats)
}

// ringUpdateDwell refreshes the beam-dwell attribute for the slots the beam
// just rewrote, using the mean already established by the priming scan.
func ringUpdateDwell(start, n int) {
	if len(dwellBuf) != steps || dwellGL.IsUndefined() {
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
		w := ringDwellMean / (d + ringDwellMean*0.15)
		if w > 1.8 {
			w = 1.8
		} else if w < 0.25 {
			w = 0.25
		}
		dwellBuf[i] = w
	}
	for k := 0; k < n; k++ {
		upd((start + k) % steps)
	}
	if cnt > 0 { // slow-track the mean so long ring sessions stay calibrated
		ringDwellMean += (total/float32(cnt) - ringDwellMean) * 0.02
		if ringDwellMean <= 0 {
			ringDwellMean = 1e-6
		}
	}
	gl.Call("bindBuffer", glTypes.ArrayBuffer, dwellGL)
	js.CopyBytesToJS(jsDwellU8, sliceToByteSlice(dwellBuf))
	gl.Call("bufferData", glTypes.ArrayBuffer, jsDwellF32, glTypes.DynamicDraw)
	gl.Call("vertexAttribPointer", aDwellLoc, 1, glTypes.Float, false, 0, 0)
	gl.Call("enableVertexAttribArray", aDwellLoc)
	gl.Call("bindBuffer", glTypes.ArrayBuffer, attractorVertexBuffer)
}
