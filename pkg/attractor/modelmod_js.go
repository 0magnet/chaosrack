//go:build js && wasm

package attractor

import "math"

// The tap: where the model's current output is read from, and the smoothed
// value each source holds between frames.
//
// Read from vertBuf rather than from any model's own state, because there is
// no "any model's own state" — every generator writes its trail into the
// shared vertex buffer and nothing else is common to all of them. The head of
// that trail is where the system is NOW, which is what a source has to mean.

// modelModState is each source's smoothed value, 0..1.
var modelModState = map[string]float32{}

// modelModHead is the newest point of the trail: the model's current output.
//
// The LAST vertex, because the generators append forward and the trail is
// drawn oldest-first — the same order sonify walks it in. Returns ok=false
// before there is a trail, which is every frame of a mode change and the
// first frames after a reset.
func modelModHead() (x, y, z float32, ok bool) {
	n := steps
	if n < 1 || len(vertBuf) < n*4 {
		return 0, 0, 0, false
	}
	i := (n - 1) * 4
	return vertBuf[i], vertBuf[i+1], vertBuf[i+2], true
}

// modelModValue is one model source's current value, 0..1, smoothed.
//
// Called once per routed parameter per frame, so several parameters routed
// from the same axis share one smoothed value — which is what they should:
// they are all listening to the same signal, and giving each its own filter
// would let two knobs following "model x" disagree about where x is.
func modelModValue(src string) float32 {
	x, y, z, ok := modelModHead()
	if !ok {
		return modelModState[src] // hold the last value through a gap
	}
	ext := view.fitExtent
	var target float32
	switch src {
	case modSrcModelX:
		target = modelModNorm(x, ext)
	case modSrcModelY:
		target = modelModNorm(y, ext)
	case modSrcModelZ:
		target = modelModNorm(z, ext)
	case modSrcModelR:
		// Distance from the origin, normalized against the same extent. Not
		// centered like the axes are — a radius is already non-negative, and
		// mapping it through modelModNorm would put a model sitting at the
		// origin at half scale rather than at zero.
		r := float32(math.Sqrt(float64(x*x + y*y + z*z)))
		if ext > 0 {
			target = r / ext
		}
		if target > 1 {
			target = 1
		}
	default:
		return 0
	}
	v := modelModSmooth(modelModState[src], target)
	modelModState[src] = v
	return v
}

// resetModelMod clears the smoothed state. Called when the model changes:
// the loop's memory is of a system that is no longer running, and carrying
// it across would drive the new model from the old one's last position.
func resetModelMod() {
	for k := range modelModState {
		delete(modelModState, k)
	}
}
