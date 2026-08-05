package stlview

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	m "math"

	"github.com/go-gl/mathgl/mgl32"
)

// verticesNative / colorsNative / indicesNative are the default cube
// data used as fallbacks before the sphere generator runs. Kept for
// parity with the pre-refactor behavior.
var verticesNative = []float32{
	-1, -1, -1, 1, -1, -1, 1, 1, -1, -1, 1, -1,
	-1, -1, 1, 1, -1, 1, 1, 1, 1, -1, 1, 1,
	-1, -1, -1, -1, 1, -1, -1, 1, 1, -1, -1, 1,
	1, -1, -1, 1, 1, -1, 1, 1, 1, 1, -1, 1,
	-1, -1, -1, -1, -1, 1, 1, -1, 1, 1, -1, -1,
	-1, 1, -1, -1, 1, 1, 1, 1, 1, 1, 1, -1,
}

var colorsNative = []float32{
	5, 3, 7, 5, 3, 7, 5, 3, 7, 5, 3, 7,
	1, 1, 3, 1, 1, 3, 1, 1, 3, 1, 1, 3,
	0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1,
	1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0,
	1, 1, 0, 1, 1, 0, 1, 1, 0, 1, 1, 0,
	0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0,
}

var indicesNative = []uint32{
	0, 1, 2, 0, 2, 3, 4, 5, 6, 4, 6, 7,
	8, 9, 10, 8, 10, 11, 12, 13, 14, 12, 14, 15,
	16, 17, 18, 16, 18, 19, 20, 21, 22, 20, 22, 23,
}

func generateSphereVertices(radius float32, stacks, slices int) ([]float32, []uint32) {
	var vertices []float32
	var indices []uint32

	// Generate random initial rotation
	rotationMatrix := randomRotationMatrix()

	// Generate sphere vertices
	for i := 0; i <= stacks; i++ {
		phi := float32(i) * float32(m.Pi) / float32(stacks)
		for j := 0; j <= slices; j++ {
			theta := float32(j) * 2.0 * float32(m.Pi) / float32(slices)
			x := radius * float32(m.Sin(float64(phi))) * float32(m.Cos(float64(theta)))
			y := radius * float32(m.Sin(float64(phi))) * float32(m.Sin(float64(theta)))
			z := radius * float32(m.Cos(float64(phi)))

			// Apply rotation to the vertex
			vertex := mgl32.Vec3{z, y, x}
			rotatedVertex := rotationMatrix.Mul4x1(vertex.Vec4(1.0))

			// Append rotated vertex
			vertices = append(vertices, rotatedVertex[0], rotatedVertex[1], rotatedVertex[2])
		}
	}

	// Generate sphere indices
	for i := 0; i < stacks; i++ {
		for j := 0; j <= slices; j++ {
			indices = append(indices, uint32(i*(slices+1)+j), uint32((i+1)*(slices+1)+j))
		}
	}

	return vertices, indices
}

func randomRotationMatrix() mgl32.Mat4 {
	// Generate random rotation angles (in radians) using crypto/rand
	rotX := randomFloat32() * 2 * float32(m.Pi)
	rotY := randomFloat32() * 2 * float32(m.Pi)
	rotZ := randomFloat32() * 2 * float32(m.Pi)

	// Create rotation matrices for each axis
	rotMatrixX := mgl32.HomogRotate3DX(rotX)
	rotMatrixY := mgl32.HomogRotate3DY(rotY)
	rotMatrixZ := mgl32.HomogRotate3DZ(rotZ)

	// Combine rotations (Z * Y * X)
	return rotMatrixZ.Mul4(rotMatrixY).Mul4(rotMatrixX)
}

func randomFloat32() float32 {
	var randomValue uint32
	err := binary.Read(rand.Reader, binary.BigEndian, &randomValue)
	if err != nil {
		log.Fatalf("Failed to read random value: %v", err)
	}
	return float32(randomValue) / float32(0xFFFFFFFF)
}
