//go:build js && wasm

package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/meshstl"
)

// setSTLFileTris is the path every model in the STL mode goes through — the
// built-ins and the files off disk both — and it is js-tagged, so nothing
// tested it until this file existed. What it does is arithmetic: center on
// the bounding box, normalize to the extent the built-in geometry lives at,
// decimate to the 16-bit index budget, and emit triangle edges.

func triFn(tris [][3]meshstl.V3) func(int) [3]meshstl.V3 {
	return func(i int) [3]meshstl.V3 { return tris[i] }
}

func TestSetSTLFileTrisCentersAndNormalizes(t *testing.T) {
	// A triangle far from the origin and much bigger than the target extent.
	tris := [][3]meshstl.V3{
		{{1000, 1000, 1000}, {1100, 1000, 1000}, {1000, 1100, 1000}},
	}
	if err := setSTLFileTris(len(tris), triFn(tris)); err != nil {
		t.Fatal(err)
	}
	if stlFileTris != 1 {
		t.Errorf("%d triangles, want 1", stlFileTris)
	}
	if len(stlFileVerts) != 9 {
		t.Fatalf("%d floats, want 9", len(stlFileVerts))
	}
	// Centered: the mean of the bounding box is the origin, so no coordinate
	// may sit a long way from it.
	var maxAbs float32
	for _, v := range stlFileVerts {
		if a := float32(math.Abs(float64(v))); a > maxAbs {
			maxAbs = a
		}
	}
	if maxAbs > 1.51 {
		t.Errorf("largest coordinate %.3f — not normalized to the 1.5 half-extent", maxAbs)
	}
	if maxAbs < 1.4 {
		t.Errorf("largest coordinate %.3f — normalized to something smaller than the target", maxAbs)
	}
	// Edges, not faces: three pairs of indices per triangle.
	if len(stlFileIdx) != 6 {
		t.Errorf("%d indices, want 6 (three edges)", len(stlFileIdx))
	}
}

func TestSetSTLFileTrisRejectsAnEmptyMesh(t *testing.T) {
	if err := setSTLFileTris(0, triFn(nil)); err == nil {
		t.Error("an empty mesh should be an error, not a blank model")
	}
}

// Anything over the budget is decimated by triangle stride. The guarantee
// that matters is the one the 16-bit index pipeline needs: never more than
// 65535 vertices, whatever comes in.
func TestSetSTLFileTrisDecimatesToTheIndexBudget(t *testing.T) {
	const n = stlFileMaxTris * 4
	tris := make([][3]meshstl.V3, n)
	for i := range tris {
		f := float64(i)
		tris[i] = [3]meshstl.V3{{f, 0, 0}, {f + 1, 0, 0}, {f, 1, 0}}
	}
	if err := setSTLFileTris(n, triFn(tris)); err != nil {
		t.Fatal(err)
	}
	if stlFileTris > stlFileMaxTris {
		t.Errorf("kept %d triangles, over the %d budget", stlFileTris, stlFileMaxTris)
	}
	if verts := len(stlFileVerts) / 3; verts > 65535 {
		t.Errorf("%d vertices — more than a uint16 index can reach", verts)
	}
	// Decimation should thin, not empty: a quarter of the budget would mean
	// the stride arithmetic had run away.
	if stlFileTris < stlFileMaxTris/2 {
		t.Errorf("kept only %d of %d triangles", stlFileTris, n)
	}
	for _, i := range stlFileIdx {
		if int(i)*3 >= len(stlFileVerts) {
			t.Fatalf("index %d points past the %d vertices", i, len(stlFileVerts)/3)
		}
	}
}

// A built-in has to survive the trip it actually takes in the app: generated
// as a mesh, fed straight to the viewer's buffers. This is the round trip the
// STL encode-and-parse used to hide.
func TestBuiltInsLoadThroughTheMeshPath(t *testing.T) {
	for _, name := range []string{"module-1slot", "rack-filled", "cube", "lorenz"} {
		m, ok := STLModelByName(name)
		if !ok {
			t.Errorf("no built-in %q", name)
			continue
		}
		if err := setSTLFileMesh(m.Build(STLViewerSeg)); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if stlFileTris == 0 || len(stlFileVerts) == 0 {
			t.Errorf("%s: loaded empty", name)
		}
	}
}
