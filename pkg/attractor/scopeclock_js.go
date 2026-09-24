//go:build js && wasm

package attractor

import (
	"math"
	"time"

	"github.com/0magnet/chaosrack/pkg/scope"
)

// generateScopeClock walks the tour into the trace buffer.
func generateScopeClock() {
	path := scope.ClockPolyline(time.Now())
	if len(path) < 2 {
		return
	}
	// Cumulative arc length, so the beam can be placed at even distances along
	// the whole path rather than evenly per stroke.
	cum := make([]float64, len(path))
	for i := 1; i < len(path); i++ {
		dx := path[i].X - path[i-1].X
		dy := path[i].Y - path[i-1].Y
		cum[i] = cum[i-1] + math.Hypot(dx, dy)
	}
	total := cum[len(cum)-1]
	if total <= 0 {
		return
	}

	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	seg := 1
	for i := range sim.steps {
		want := total * float64(i) / float64(sim.steps-1)
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
		vertices[j] = float32(a.X + (b.X-a.X)*t)
		vertices[j+1] = float32(a.Y + (b.Y-a.Y)*t)
		vertices[j+2] = 0 // a scope screen is flat; the pipeline is 3-D anyway
		vertices[j+3] = float32(i) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}

func init() {
	registerGenerate("scopeclock", generateScopeClock)
}
