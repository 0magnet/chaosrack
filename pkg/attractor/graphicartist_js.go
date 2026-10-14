//go:build js && wasm

package attractor

// The Graphic Artist on the page: GraphicArtist (graphicartist.go) traced
// into the vertex buffer.

var ga = *NewGraphicArtist()

// generateGraphicArtist traces one closed pass of the Lissajous wireframe into
// the vertex buffer, drifting a frame's worth. Called every frame from
// generateForMode, so it continuously renders like the attractors.
func generateGraphicArtist() {
	ga.Drift(1, sim.speedScale)
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	ga.Trace(sim.steps, func(i int, x, y, z float64) {
		j := i * 4
		vertices[j], vertices[j+1], vertices[j+2], vertices[j+3] = float32(x), float32(y), float32(z), float32(i)*invN
	})
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}
