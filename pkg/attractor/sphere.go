//go:build js && wasm

package attractor

import "math"

var (
	sphereRadius  float32 = 1.0
	sphereStacksF float32 = 30
	sphereSlicesF float32 = 30
	torusR        float32 = 1.5
	torusr        float32 = 0.5
	torusStacksF  float32 = 30
	torusSlicesF  float32 = 30
	globeLatF     float32 = 18
	globeLonF     float32 = 36
)

func sphereVerticesIndices(radius float32, stacks, slices int, baseIdx uint16) ([]float32, []uint16) {
	var vertices []float32
	var indices []uint16
	for i := 0; i <= stacks; i++ {
		phi := float32(i) * float32(math.Pi) / float32(stacks)
		for j := 0; j <= slices; j++ {
			theta := float32(j) * 2.0 * float32(math.Pi) / float32(slices)
			xv := radius * float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := radius * float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := radius * float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := 0; i < stacks; i++ {
		for j := 0; j <= slices; j++ {
			indices = append(indices, baseIdx+uint16(i*(slices+1)+j), baseIdx+uint16((i+1)*(slices+1)+j))
		}
	}
	return vertices, indices
}

func torusVerticesIndices(R, r float32, stacks, slices int, baseIdx uint16) ([]float32, []uint16) {
	var vertices []float32
	var indices []uint16
	for i := 0; i <= stacks; i++ {
		theta := float32(i) * 2.0 * math.Pi / float32(stacks)
		for j := 0; j <= slices; j++ {
			phi := float32(j) * 2.0 * math.Pi / float32(slices)
			xv := (R + r*float32(math.Cos(float64(phi)))) * float32(math.Cos(float64(theta)))
			yv := (R + r*float32(math.Cos(float64(phi)))) * float32(math.Sin(float64(theta)))
			zv := r * float32(math.Sin(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := 0; i < stacks; i++ {
		for j := 0; j < slices; j++ {
			cur := baseIdx + uint16(i*(slices+1)+j)
			next := cur + 1
			below := baseIdx + uint16((i+1)*(slices+1)+j)
			// Horizontal ring edge
			indices = append(indices, cur, next)
			// Vertical edge
			indices = append(indices, cur, below)
		}
	}
	return vertices, indices
}

func generateSphere() {
	stacks := int(sphereStacksF)
	slices := int(sphereSlicesF)
	vertices, indices := sphereVerticesIndices(sphereRadius, stacks, slices, 0)
	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}

func generateTorus() {
	stacks := int(torusStacksF)
	slices := int(torusSlicesF)
	vertices, indices := torusVerticesIndices(torusR, torusr, stacks, slices, 0)
	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}

func generateGlobe() {
	lat := int(globeLatF)
	lon := int(globeLonF)
	var vertices []float32
	var indices []uint16
	pts := 60 // points per circle

	// Latitude lines
	for i := 1; i < lat; i++ {
		phi := float32(i) * float32(math.Pi) / float32(lat)
		base := uint16(len(vertices) / 3)
		for j := 0; j <= pts; j++ {
			theta := float32(j) * 2.0 * float32(math.Pi) / float32(pts)
			xv := float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
			if j > 0 {
				indices = append(indices, base+uint16(j-1), base+uint16(j))
			}
		}
	}

	// Longitude lines
	for j := 0; j < lon; j++ {
		theta := float32(j) * 2.0 * float32(math.Pi) / float32(lon)
		base := uint16(len(vertices) / 3)
		for i := 0; i <= pts; i++ {
			phi := float32(i) * float32(math.Pi) / float32(pts)
			xv := float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
			if i > 0 {
				indices = append(indices, base+uint16(i-1), base+uint16(i))
			}
		}
	}

	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}

func generateMagnetosphere() {
	var allVerts []float32
	var allIdx []uint16

	// Central sphere
	sv, si := sphereVerticesIndices(0.5, 16, 16, 0)
	allVerts = append(allVerts, sv...)
	allIdx = append(allIdx, si...)

	// Magnetic field lines — dipole field: r = R*cos²(θ)
	nLines := 12
	ptsPerLine := 80
	for i := 0; i < nLines; i++ {
		angle := float32(i) * 2.0 * math.Pi / float32(nLines)
		base := uint16(len(allVerts) / 3)
		R := float32(3.0)
		for j := 0; j <= ptsPerLine; j++ {
			theta := float32(-math.Pi/2) + float32(j)*float32(math.Pi)/float32(ptsPerLine)
			ct := float32(math.Cos(float64(theta)))
			r := R * ct * ct
			xv := r * ct * float32(math.Cos(float64(angle)))
			yv := r * ct * float32(math.Sin(float64(angle)))
			zv := r * float32(math.Sin(float64(theta)))
			allVerts = append(allVerts, xv, yv, zv)
			if j > 0 {
				allIdx = append(allIdx, base+uint16(j-1), base+uint16(j))
			}
		}
	}

	uploadBuffersIndexed(allVerts, allIdx, glTypes.Line)
}
