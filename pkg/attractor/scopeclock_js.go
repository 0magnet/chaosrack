//go:build js && wasm

package attractor

import (
	"math"
	"time"
)

// generateScopeClock walks the tour into the trace buffer.
func generateScopeClock() {
	path := clockPolyline(time.Now())
	if len(path) < 2 {
		return
	}
	// Cumulative arc length, so the beam can be placed at even distances along
	// the whole path rather than evenly per stroke.
	cum := make([]float64, len(path))
	for i := 1; i < len(path); i++ {
		dx := path[i].x - path[i-1].x
		dy := path[i].y - path[i-1].y
		cum[i] = cum[i-1] + math.Hypot(dx, dy)
	}
	total := cum[len(cum)-1]
	if total <= 0 {
		return
	}

	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	seg := 1
	for i := 0; i < steps; i++ {
		want := total * float64(i) / float64(steps-1)
		for seg < len(cum)-1 && cum[seg] < want {
			seg++
		}
		a, b := path[seg-1], path[seg]
		span := cum[seg] - cum[seg-1]
		t := 0.0
		if span > 0 {
			t = (want - cum[seg-1]) / span
		}
		j := i * 4
		vertices[j] = float32(a.x + (b.x-a.x)*t)
		vertices[j+1] = float32(a.y + (b.y-a.y)*t)
		vertices[j+2] = 0 // a scope screen is flat; the pipeline is 3-D anyway
		vertices[j+3] = float32(i) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}

func init() {
	registerGenerate("scopeclock", generateScopeClock)
}
