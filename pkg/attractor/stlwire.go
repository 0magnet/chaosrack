package attractor

import (
	"bytes"
	"errors"

	"gitlab.com/russoj88/stl/stl"

	"github.com/0magnet/chaosrack/pkg/geom"
	"github.com/0magnet/chaosrack/pkg/meshstl"
	"github.com/0magnet/chaosrack/pkg/stlmodels"
)

// A stereolithograph as the STL File model draws it, shared by the page
// (stlfile_js.go) and a native rack: its triangles' edges, centered and
// normalized to the extent the built-in geometry lives at, decimated to the
// 16-bit index budget.

// STLWire is a mesh as wireframe: packed xyz per triangle corner, the
// triangles' edge pairs, and how many triangles are drawn.
type STLWire struct {
	Verts []float32
	Idx   []uint16
	Tris  int
}

// STLFileWire parses STL bytes, binary or ASCII, into their wireframe.
func STLFileWire(data []byte) (STLWire, error) {
	solid, err := stl.From(bytes.NewReader(data))
	if err != nil {
		return STLWire{}, err
	}
	return stlWire(len(solid.Triangles), func(i int) [3]meshstl.V3 {
		v := solid.Triangles[i].Vertices
		return [3]meshstl.V3{
			{float64(v[0].X), float64(v[0].Y), float64(v[0].Z)},
			{float64(v[1].X), float64(v[1].Y), float64(v[1].Z)},
			{float64(v[2].X), float64(v[2].Y), float64(v[2].Z)},
		}
	})
}

// STLMeshWire takes a mesh straight from a generator, with no STL in
// between. The built-in models used to be encoded to bytes and parsed back by
// the same program that had just built them — a megabyte of round trip per
// selection to arrive at the triangles it already had.
func STLMeshWire(m meshstl.Mesh) (STLWire, error) {
	return stlWire(len(m.Tris), func(i int) [3]meshstl.V3 {
		t := m.Tris[i]
		return [3]meshstl.V3{t.A, t.B, t.C}
	})
}

// stlWire is the shared half: center on the bounding box, normalize to
// the extent the built-in geometry lives at, decimate to the 16-bit index
// budget, and emit triangle edges. at(i) returns one triangle.
func stlWire(n int, at func(int) [3]meshstl.V3) (STLWire, error) {
	if n == 0 {
		return STLWire{}, errors.New("no triangles in file")
	}
	stride := (n + stlmodels.MaxTris - 1) / stlmodels.MaxTris

	first := true
	var minX, minY, minZ, maxX, maxY, maxZ float32
	for i := 0; i < n; i += stride {
		for _, v := range at(i) {
			x, y, z := float32(v[0]), float32(v[1]), float32(v[2])
			if first {
				minX, maxX, minY, maxY, minZ, maxZ = x, x, y, y, z, z
				first = false
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
		}
	}
	cx, cy, cz := (minX+maxX)/2, (minY+maxY)/2, (minZ+maxZ)/2
	half := (maxX - minX) / 2
	if h := (maxY - minY) / 2; h > half {
		half = h
	}
	if h := (maxZ - minZ) / 2; h > half {
		half = h
	}
	scale := float32(1)
	if half > 0 {
		scale = 1.5 / half
	}

	verts := make([]float32, 0, (n/stride+1)*9)
	idx := make([]uint16, 0, (n/stride+1)*6)
	var vi uint16
	for i := 0; i < n; i += stride {
		for _, v := range at(i) {
			verts = append(verts,
				(float32(v[0])-cx)*scale,
				(float32(v[1])-cy)*scale,
				(float32(v[2])-cz)*scale)
		}
		idx = append(idx, vi, vi+1, vi+1, vi+2, vi+2, vi)
		vi += 3
	}
	return STLWire{Verts: verts, Idx: idx, Tris: len(idx) / 6}, nil
}

// STLWireFigure is a wireframe as a figure.
func STLWireFigure(w STLWire) Figure {
	return linesFigure(geom.Lines{Vertices: w.Verts, Indices: w.Idx})
}

// STLStandIn is what the STL File model shows before anything is loaded: the
// plain cube, so the screen is not blank while Load waits.
func STLStandIn() Figure {
	return linesFigure(geom.Lines{Vertices: verticesCube[:72], Indices: indicesCube[:36]})
}
