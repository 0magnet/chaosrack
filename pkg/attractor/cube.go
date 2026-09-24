//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/glctx"

// verticesCube and indicesCube are in cubedata.go, where render can reach them.

func generateNestedCube() {
	gpu.uploadBuffersIndexed(verticesCube, indicesCube, glctx.Types.Line)
}
