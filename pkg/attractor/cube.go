//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/glctx"

// verticesCube and indicesCube are in cubedata.go, where render can reach them.

// nestedCube is the Nested Cube's one knob, DIM: 3 the cubes, 2 the squares
// that are their faces toward the viewer (SquaresMesh).
var nestedCube = struct{ dimF float32 }{dimF: 3}

func generateNestedCube() {
	if nestedCube.dimF == 2 {
		v, i := SquaresMesh()
		gpu.uploadBuffersIndexed(v, i, glctx.Types.Line)
		return
	}
	gpu.uploadBuffersIndexed(verticesCube, indicesCube, glctx.Types.Line)
}
