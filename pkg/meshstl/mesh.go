// Package meshstl builds triangle meshes and writes them as STL.
//
// It exists so the same geometry can be three things at once: a file you can
// print, a model the app can render in its STL mode without loading anything
// from disk, and a drawing of the rack that is dimensioned in the same
// millimeters as pkg/rackspec. Nothing here imports the app, and nothing here
// is js-tagged, so the generators are testable on the host.
//
// Everything is in millimeters, right-handed, +Z out of the panel.
package meshstl

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// V3 is a point or a vector, in millimeters.
type V3 [3]float64

func (v V3) Add(o V3) V3      { return V3{v[0] + o[0], v[1] + o[1], v[2] + o[2]} }
func (v V3) Sub(o V3) V3      { return V3{v[0] - o[0], v[1] - o[1], v[2] - o[2]} }
func (v V3) Mul(s float64) V3 { return V3{v[0] * s, v[1] * s, v[2] * s} }
func (v V3) Dot(o V3) float64 { return v[0]*o[0] + v[1]*o[1] + v[2]*o[2] }
func (v V3) Len() float64     { return math.Sqrt(v.Dot(v)) }

func (v V3) Cross(o V3) V3 {
	return V3{v[1]*o[2] - v[2]*o[1], v[2]*o[0] - v[0]*o[2], v[0]*o[1] - v[1]*o[0]}
}

// Norm returns the unit vector, or the zero vector if there isn't one.
func (v V3) Norm() V3 {
	if l := v.Len(); l > 0 {
		return v.Mul(1 / l)
	}
	return V3{}
}

// Tri is one triangle, wound counter-clockwise seen from outside — which is
// what makes the right-hand normal point out of the solid, and what every STL
// consumer assumes.
type Tri struct{ A, B, C V3 }

// Normal is the outward unit normal implied by the winding.
func (t Tri) Normal() V3 { return t.B.Sub(t.A).Cross(t.C.Sub(t.A)).Norm() }

// Area is the triangle's area — zero for a degenerate one.
func (t Tri) Area() float64 { return t.B.Sub(t.A).Cross(t.C.Sub(t.A)).Len() / 2 }

// Mesh is a triangle soup. Nothing here maintains an index or a topology: STL
// has neither, and the app's loader takes the same soup.
type Mesh struct{ Tris []Tri }

// Add appends triangles, dropping degenerate ones. Sweeping a curve produces
// zero-area triangles wherever consecutive points coincide, and a zero-area
// triangle has no normal — slicers either reject the file or guess.
func (m *Mesh) Add(tris ...Tri) {
	for _, t := range tris {
		if t.Area() > 1e-12 {
			m.Tris = append(m.Tris, t)
		}
	}
}

// AddQuad adds a planar quad as two triangles, wound a-b-c-d.
func (m *Mesh) AddQuad(a, b, c, d V3) { m.Add(Tri{a, b, c}, Tri{a, c, d}) }

// Append merges another mesh in.
func (m *Mesh) Append(o Mesh) { m.Tris = append(m.Tris, o.Tris...) }

// Translate and Scale move a whole mesh; both return a new one.
func (m Mesh) Translate(d V3) Mesh {
	out := Mesh{Tris: make([]Tri, len(m.Tris))}
	for i, t := range m.Tris {
		out.Tris[i] = Tri{t.A.Add(d), t.B.Add(d), t.C.Add(d)}
	}
	return out
}

func (m Mesh) Scale(s float64) Mesh {
	out := Mesh{Tris: make([]Tri, len(m.Tris))}
	for i, t := range m.Tris {
		out.Tris[i] = Tri{t.A.Mul(s), t.B.Mul(s), t.C.Mul(s)}
	}
	return out
}

// Bounds returns the axis-aligned bounding box.
func (m Mesh) Bounds() (min, max V3) {
	if len(m.Tris) == 0 {
		return V3{}, V3{}
	}
	min, max = m.Tris[0].A, m.Tris[0].A
	for _, t := range m.Tris {
		for _, v := range [3]V3{t.A, t.B, t.C} {
			for i := 0; i < 3; i++ {
				if v[i] < min[i] {
					min[i] = v[i]
				}
				if v[i] > max[i] {
					max[i] = v[i]
				}
			}
		}
	}
	return min, max
}

// Size is the bounding box's extent.
func (m Mesh) Size() V3 {
	min, max := m.Bounds()
	return max.Sub(min)
}

// CenterXY recenters the mesh on the origin in X and Y, leaving Z alone —
// the app's viewer frames a model on its origin, and a panel modeled from a
// corner would sit off to one side.
func (m Mesh) CenterXY() Mesh {
	min, max := m.Bounds()
	return m.Translate(V3{-(min[0] + max[0]) / 2, -(min[1] + max[1]) / 2, 0})
}

// FitTo scales the mesh so its largest dimension is size, and centers it.
// This is what the app's viewer wants: a model normalized into view, whatever
// it was modeled in.
func (m Mesh) FitTo(size float64) Mesh {
	s := m.Size()
	biggest := math.Max(s[0], math.Max(s[1], s[2]))
	if biggest <= 0 {
		return m
	}
	min, max := m.Bounds()
	center := V3{(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2}
	return m.Translate(center.Mul(-1)).Scale(size / biggest)
}

// WriteBinarySTL writes the mesh as a binary STL.
//
// Binary rather than ASCII because these get large — an attractor swept as a
// tube is a few hundred thousand triangles, which is 25 times the size as
// text — and because the app's own loader reads both.
func WriteBinarySTL(w io.Writer, m Mesh, header string) error {
	bw := bufio.NewWriter(w)

	// The header is exactly 80 bytes, and must not begin with "solid" or
	// readers that sniff the format will try to parse the file as ASCII.
	var head [80]byte
	copy(head[:], "chaosrack ")
	copy(head[10:], header)
	if _, err := bw.Write(head[:]); err != nil {
		return err
	}
	// Compared in uint64, not against math.MaxUint32 as an int: TinyGo's wasm
	// target has a 32-bit int, where that constant does not fit one and the
	// package would not compile at all. The count is a uint32 on the wire
	// either way.
	if uint64(len(m.Tris)) > 0xFFFFFFFF {
		return fmt.Errorf("mesh has %d triangles, more than an STL can index", len(m.Tris))
	}
	if err := binary.Write(bw, binary.LittleEndian, uint32(len(m.Tris))); err != nil { //nolint:gosec // bounded above
		return err
	}
	buf := make([]float32, 12)
	for _, t := range m.Tris {
		n := t.Normal()
		v := [4]V3{n, t.A, t.B, t.C}
		for i := 0; i < 4; i++ {
			buf[i*3+0] = float32(v[i][0])
			buf[i*3+1] = float32(v[i][1])
			buf[i*3+2] = float32(v[i][2])
		}
		if err := binary.Write(bw, binary.LittleEndian, buf); err != nil {
			return err
		}
		if err := binary.Write(bw, binary.LittleEndian, uint16(0)); err != nil { // attribute byte count
			return err
		}
	}
	return bw.Flush()
}
