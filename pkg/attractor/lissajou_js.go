//go:build js && wasm

package attractor

// liss is the page's Lissajous beam (lissajou.go), whose ratios its knobs
// hold.
var liss = NewLissajousBeam()

func (l *LissajousBeam) generateLissajou() {
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	l.Advance(sim.speedScale, sim.speedSteps)
	for i := range sim.steps {
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2] = l.At(i, sim.steps)
		vertices[j+3] = float32(i) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}
