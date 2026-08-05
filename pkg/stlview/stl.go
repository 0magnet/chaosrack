// STL parser + model type. Converts stereolithograph data into
// renderer-ready vertex/color/index buffers with random rotation +
// random per-triangle gradient.
package stlview

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"gitlab.com/russoj88/stl/stl"
)

// STL is a stereolithograph
type STL struct {
	v []float32
	c []float32
	i []uint32
}

// Vertex is a point in model space.
type Vertex struct {
	X, Y, Z float32
}

// GetModel returns the vertex, color, and index buffers.
func (s STL) GetModel() ([]float32, []float32, []uint32) {
	return s.v, s.c, s.i
}

// NewSTL parses STL data (binary or ASCII) into renderer-ready buffers.
func NewSTL(buffer []byte) (o STL, err error) {
	bufferReader := bytes.NewReader(buffer)
	solid, err := stl.From(bufferReader)
	if err != nil {
		return
	}

	// Generate random rotation matrix
	rotationMatrix := randomRotationMatrix()

	// Generate colors
	numColors, _ := cryptoRandIntn(5)
	numColors += 2 // Random number between 2 and 6
	colors := GenerateGradient(numColors, int(solid.TriangleCount))

	var index uint32
	for i, triangle := range solid.Triangles {
		colorR := colors[i].Red
		colorG := colors[i].Green
		colorB := colors[i].Blue

		// Convert each triangle's vertices to custom Vertex type and apply rotation
		v0 := Vertex{X: float32(triangle.Vertices[0].X), Y: float32(triangle.Vertices[0].Y), Z: float32(triangle.Vertices[0].Z)}
		v1 := Vertex{X: float32(triangle.Vertices[1].X), Y: float32(triangle.Vertices[1].Y), Z: float32(triangle.Vertices[1].Z)}
		v2 := Vertex{X: float32(triangle.Vertices[2].X), Y: float32(triangle.Vertices[2].Y), Z: float32(triangle.Vertices[2].Z)}

		// Rotate and add vertices
		o.addRotatedVertex(&index, v0, rotationMatrix, colorR, colorG, colorB)
		o.addRotatedVertex(&index, v1, rotationMatrix, colorR, colorG, colorB)
		o.addRotatedVertex(&index, v2, rotationMatrix, colorR, colorG, colorB)
	}

	return o, err
}

// Add a rotated vertex to the STL structure
func (s *STL) addRotatedVertex(index *uint32, vertex Vertex, rotation mgl32.Mat4, r, g, b float32) {
	// Apply rotation
	rotatedVertex := rotation.Mul4x1(mgl32.Vec3{vertex.X, vertex.Y, vertex.Z}.Vec4(1.0))

	// Add rotated vertex to the STL struct
	s.v = append(s.v, rotatedVertex[0], rotatedVertex[1], rotatedVertex[2])
	s.i = append(s.i, *index)
	s.c = append(s.c, r, g, b)
	(*index)++
}

// ParseBase64 extracts and decodes the base64 payload from a data-URI
// style string (anything after the first "base64," marker).
func ParseBase64(input string) ([]byte, error) {
	const marker = "base64,"
	index := strings.Index(input, marker)
	if index < 0 {
		return nil, errors.New("no base64 payload found")
	}
	return base64.StdEncoding.DecodeString(input[index+len(marker):])
}
