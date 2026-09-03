//go:build js && wasm

package attractor

import "syscall/js"

// Poincaré section (the Trace > Sect switch): the classic dimension-reducing
// view — sample the trajectory only where it pierces a plane (positive-going
// z crossings through the attractor's center), and the flow's sheets collapse
// into the section's fractal scatter. Points accumulate in a ring (newest
// replace oldest) and draw as an overlay on the normal trail, gold, so you
// see both the flow and its section at once. Works for every registered flow
// (classics, Sprott catalog, hyperrossler, custom) via flowFor4.

var (
	sectOn     bool
	sectSeeded string
	sectState  [4]float64
	sectPts    []float32 // accumulated crossing points (x,y,z,t quads, raw coords·scale)
	sectDraw   []float32 // per-frame upload copy (uploadVerticesOnly centers in place)
	sectHead   int
	sectCount  int
)

const sectCap = 8192

func sectInvalidate() { sectSeeded = "" }

// sectTick advances a private integrator, accumulates plane crossings, and
// draws the scatter. Called at the end of every attractor-path frame (scan,
// ring and twin alike) — it's an overlay, not a replacement.
func sectTick(mode string) {
	if !sectOn {
		return
	}
	sys, ok := flowFor4(mode)
	if !ok {
		return
	}
	if sectSeeded != mode {
		ic := initCondFor(mode)
		sectState = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.w()}
		sectHead, sectCount = 0, 0
		sectSeeded = mode
	}
	if sectPts == nil {
		sectPts = make([]float32, sectCap*4)
		sectDraw = make([]float32, sectCap*4)
	}
	// The plane: z at the attractor's center, in RAW coordinates (centerOffset
	// is in display coordinates, i.e. already scaled).
	scale := sys.scale
	plane := float64(centerOffset[2] / scale)
	steps := 4096
	if sys.interpreted {
		steps = 512
	}
	dt := sys.dt() * float64(speedScale)
	if dt <= 0 {
		return
	}
	for i := 0; i < steps; i++ {
		prev := sectState
		twinStep(sys, &sectState, dt)
		if twinDiverged(sectState) {
			ic := initCondFor(mode)
			sectState = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.w()}
			continue
		}
		if prev[2] < plane && sectState[2] >= plane {
			f := (plane - prev[2]) / (sectState[2] - prev[2])
			cx := prev[0] + f*(sectState[0]-prev[0])
			cy := prev[1] + f*(sectState[1]-prev[1])
			j := sectHead * 4
			sectPts[j] = float32(cx) * scale
			sectPts[j+1] = float32(cy) * scale
			sectPts[j+2] = float32(plane) * scale
			sectPts[j+3] = float32(sectHead) / float32(sectCap-1)
			sectHead = (sectHead + 1) % sectCap
			if sectCount < sectCap {
				sectCount++
			}
		}
	}
	if sectCount < 2 {
		return
	}
	// Overlay draw: gold points via the monochrome override. Copy first —
	// uploadVerticesOnly subtracts centerOffset in place, and these points
	// must persist across frames in raw form.
	n := sectCount * 4
	copy(sectDraw[:n], sectPts[:n])
	gl.Call("uniform1i", uGradientColorsLoc, 1)
	gl.Call("uniform3f", uBaseColorLoc, 1.0, 0.8, 0.15)
	uploadVerticesOnly(sectDraw[:n], glTypes.Points, sectCount)
	gl.Call("uniform1i", uGradientColorsLoc, gradientColors)
}

// wireSectSwitch hooks up the Trace > Sect checkbox.
func wireSectSwitch() {
	sw := doc.Call("getElementById", "sect-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		sectOn = sw.Get("checked").Bool()
		sectInvalidate()
		return nil
	}))
}
