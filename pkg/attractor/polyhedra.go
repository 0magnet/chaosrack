//go:build js && wasm

package attractor

import "math"

func tetrahedronVertices() []float32 {
	s := float32(1.0)
	return []float32{
		s, s, s,
		s, -s, -s,
		-s, s, -s,
		-s, -s, s,
	}
}

func generateTetrahedron() {
	vertices := tetrahedronVertices()
	// 6 edges as vertex pairs for GL_LINES
	indices := []uint16{
		0, 1, 0, 2, 0, 3,
		1, 2, 1, 3, 2, 3,
	}
	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}

func octahedronVertices() []float32 {
	return []float32{
		1, 0, 0, // 0: +x
		-1, 0, 0, // 1: -x
		0, 1, 0, // 2: +y
		0, -1, 0, // 3: -y
		0, 0, 1, // 4: +z
		0, 0, -1, // 5: -z
	}
}

func generateOctahedron() {
	vertices := octahedronVertices()
	// 12 edges as vertex pairs for GL_LINES
	indices := []uint16{
		0, 2, 0, 3, 0, 4, 0, 5,
		1, 2, 1, 3, 1, 4, 1, 5,
		2, 4, 2, 5, 3, 4, 3, 5,
	}
	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}

func dodecahedronVertices() []float32 {
	phi := float32((1 + math.Sqrt(5)) / 2) // golden ratio
	invPhi := float32(1) / phi
	return []float32{
		// cube vertices
		1, 1, 1, 1, 1, -1, 1, -1, 1, 1, -1, -1,
		-1, 1, 1, -1, 1, -1, -1, -1, 1, -1, -1, -1,
		// rectangle vertices on xy plane
		0, phi, invPhi, 0, phi, -invPhi, 0, -phi, invPhi, 0, -phi, -invPhi,
		// rectangle vertices on yz plane
		invPhi, 0, phi, invPhi, 0, -phi, -invPhi, 0, phi, -invPhi, 0, -phi,
		// rectangle vertices on xz plane
		phi, invPhi, 0, phi, -invPhi, 0, -phi, invPhi, 0, -phi, -invPhi, 0,
	}
}

func generateDodecahedron() {
	vertices := dodecahedronVertices()
	// Edges of a dodecahedron (30 edges)
	indices := []uint16{
		0, 8, 0, 12, 0, 16,
		1, 9, 1, 13, 1, 16,
		2, 10, 2, 12, 2, 17,
		3, 11, 3, 13, 3, 17,
		4, 8, 4, 14, 4, 18,
		5, 9, 5, 15, 5, 18,
		6, 10, 6, 14, 6, 19,
		7, 11, 7, 15, 7, 19,
		8, 9, 10, 11, 12, 14,
		13, 15, 16, 17, 18, 19,
	}
	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}

func icosahedronVertices() []float32 {
	phi := float32((1 + math.Sqrt(5)) / 2)
	return []float32{
		0, 1, phi, 0, 1, -phi, 0, -1, phi, 0, -1, -phi,
		1, phi, 0, 1, -phi, 0, -1, phi, 0, -1, -phi, 0,
		phi, 0, 1, phi, 0, -1, -phi, 0, 1, -phi, 0, -1,
	}
}

func generateIcosahedron() {
	vertices := icosahedronVertices()
	indices := []uint16{
		0, 2, 0, 4, 0, 6, 0, 8, 0, 10,
		1, 3, 1, 4, 1, 6, 1, 9, 1, 11,
		2, 5, 2, 8, 2, 10,
		3, 5, 3, 9, 3, 11,
		4, 6, 4, 8, 4, 9,
		5, 7, 5, 8, 5, 9,
		6, 10, 6, 11,
		7, 10, 7, 11,
		7, 2, 7, 3,
	}
	uploadBuffersIndexed(vertices, indices, glTypes.Line)
}
