//go:build js && wasm

package attractor

import (
	"math"
	"syscall/js"
)

// Blanked-beam rendering for the scope demo modes (Pong, Fourier Text).
//
// The attractor pipeline draws ONE connected line strip, so a multi-shape
// drawing (court + paddles + ball, or a row of glyphs) would show every
// jump between shapes as a retrace line. A real scope blanks the beam
// (z-axis off) during those jumps. beamLines is that blanking: it
// resamples a multi-stroke drawing into independent gl.LINES segment
// pairs — uniform arc-length density WITHIN each stroke, and no geometry
// at all between strokes. The beam simply does not exist where it
// shouldn't draw.

// beamLines fills vertBuf with gl.LINES pairs for the given strokes (each
// a polyline of x,y waypoint pairs, z = 0) and returns the vertex count to
// draw. phase (0..1) rotates the gradient parameter along the total drawn
// length, so the trail gradient sweeps the figure like a live beam.
func beamLines(strokes [][]float64, phase float64) int {
	maxV := steps // vertex budget: vertBuf holds steps×4 floats
	if maxV < 16 {
		return 0
	}
	// Every chord emits at least one pair, so a dense drawing (a banner of
	// 1024-sample glyph circuits) must first be DECIMATED — chording through
	// skipped polyline points, which keeps curves continuous — until the
	// chord count fits the budget; leftover budget then subdivides long
	// chords for even gradient density.
	nSeg := 0
	for _, s := range strokes {
		if n := len(s)/2 - 1; n > 0 {
			nSeg += n
		}
	}
	if nSeg == 0 {
		return 0
	}
	stride := (nSeg*2 + maxV - 1) / maxV
	if stride < 1 {
		stride = 1
	}
	total := 0.0
	nChords := 0
	eachChord := func(fn func(s []float64, j, k int)) {
		for _, s := range strokes {
			np := len(s) / 2
			for j := 0; j+1 < np; j += stride {
				k := j + stride
				if k >= np {
					k = np - 1
				}
				fn(s, j, k)
			}
		}
	}
	eachChord(func(s []float64, j, k int) {
		total += math.Hypot(s[k*2]-s[j*2], s[k*2+1]-s[j*2+1])
		nChords++
	})
	if total <= 0 {
		return 0
	}
	sub := total // no subdivision unless there's room
	if room := maxV/2 - nChords; room > 0 {
		sub = total / float64(room)
	}
	v := 0
	arc := 0.0
	eachChord(func(s []float64, j, k int) {
		x1, y1, x2, y2 := s[j*2], s[j*2+1], s[k*2], s[k*2+1]
		segLen := math.Hypot(x2-x1, y2-y1)
		if segLen == 0 {
			return
		}
		n := int(segLen/sub) + 1
		for q := 0; q < n && v+2 <= maxV; q++ {
			f0 := float64(q) / float64(n)
			f1 := float64(q+1) / float64(n)
			t0 := math.Mod((arc+segLen*f0)/total+phase, 1)
			t1 := math.Mod((arc+segLen*f1)/total+phase, 1)
			o := v * 4
			vertBuf[o] = float32(x1 + (x2-x1)*f0)
			vertBuf[o+1] = float32(y1 + (y2-y1)*f0)
			vertBuf[o+2] = 0
			vertBuf[o+3] = float32(t0)
			vertBuf[o+4] = float32(x1 + (x2-x1)*f1)
			vertBuf[o+5] = float32(y1 + (y2-y1)*f1)
			vertBuf[o+6] = 0
			vertBuf[o+7] = float32(t1)
			v += 2
		}
		arc += segLen
	})
	return v
}

// beamDrawMode: blanked-beam modes draw segment pairs — or points when the
// Points switch asks for dots.
func beamDrawMode() js.Value {
	if usePoints {
		return glTypes.Points
	}
	return glTypes.Lines
}
