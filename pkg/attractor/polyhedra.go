//go:build js && wasm

package attractor

// The Platonic solids' vertex tables.
//
// These are no longer what DRAWS them — the five seeds are generated from
// their faces, with the Conway operator knob applied; see pkg/conway. They
// remain because buildSkinMesh paints the spectrogram onto these solids and
// reads the vertices directly.

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

func icosahedronVertices() []float32 {
	phi := float32((1 + math.Sqrt(5)) / 2)
	return []float32{
		0, 1, phi, 0, 1, -phi, 0, -1, phi, 0, -1, -phi,
		1, phi, 0, 1, -phi, 0, -1, phi, 0, -1, -phi, 0,
		phi, 0, 1, phi, 0, -1, -phi, 0, 1, -phi, 0, -1,
	}
}
