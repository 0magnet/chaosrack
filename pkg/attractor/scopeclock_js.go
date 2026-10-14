//go:build js && wasm

package attractor

import "time"

// generateScopeClock walks the tour into the trace buffer (ClockTrace).
func generateScopeClock() {
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	if !ClockTrace(time.Now(), sim.steps, func(i int, x, y float64) {
		j := i * 4
		vertices[j], vertices[j+1] = float32(x), float32(y)
		vertices[j+2] = 0 // a scope screen is flat; the pipeline is 3-D anyway
		vertices[j+3] = float32(i) * invN
	}) {
		return
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}

func init() {
	registerGenerate("scopeclock", generateScopeClock)
}
