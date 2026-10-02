package conway

import (
	"math"
	"math/cmplx"
)

// Regular polyhedra and tilings, by Schläfli symbol.
//
// {p,q} is a face with p sides, q of them meeting at every corner. Two
// numbers name all five Platonic solids — {3,3} the tetrahedron, {4,3} the
// cube, {3,4} the octahedron, {5,3} the dodecahedron, {3,5} the icosahedron —
// and turning either one is a step from one solid to another: add a side to
// the tetrahedron's triangles and it is the cube, add a face at its corners
// and it is the octahedron.
//
// Most pairs are not solids. Where (p-2)(q-2) is 4 the corners' angles add
// to exactly a full turn and the faces lie flat — {4,4} is squares on a
// plane, {3,6} triangles, {6,3} hexagons. Past that they add to MORE than a
// turn, and there is no room for them on a plane either: they tile the
// hyperbolic plane, drawn here in Poincaré's disk, where the same polygons
// shrink toward the rim. A knob on p or q is not dead anywhere; it moves
// between the three geometries a regular tiling can have.

// Curvature is which of the three geometries {p,q} lives in.
type Curvature int

// The three geometries.
const (
	Spherical  Curvature = iota // a solid
	Flat                        // a tiling of the plane
	Hyperbolic                  // a tiling of the hyperbolic plane
)

// CurvatureOf is the geometry of {p,q}.
func CurvatureOf(p, q int) Curvature {
	switch k := (p - 2) * (q - 2); {
	case k < 4:
		return Spherical
	case k == 4:
		return Flat
	default:
		return Hyperbolic
	}
}

// Regular builds {p,q}: the Platonic solid, or a patch of the flat or
// hyperbolic tiling, lying in the z=0 plane and inside the unit disk. p and
// q below 3 are taken as 3.
func Regular(p, q int) Solid {
	p, q = max(p, 3), max(q, 3)
	switch CurvatureOf(p, q) {
	case Spherical:
		return platonic(p, q).normalize()
	case Flat:
		return tiling(p, q, false)
	default:
		return tiling(p, q, true)
	}
}

// platonic is the seed for a spherical {p,q}. There are exactly five.
func platonic(p, q int) Solid {
	switch {
	case p == 3 && q == 3:
		return seedTetrahedron()
	case p == 4 && q == 3:
		return seedCube()
	case p == 3 && q == 4:
		return seedOctahedron()
	case p == 5 && q == 3:
		return seedDodecahedron()
	default: // {3,5}
		return seedIcosahedron()
	}
}

// Morph walks a solid to its dual through its truncations: 0 is the solid,
// 1 the rectified solid (ambo — the cuboctahedron between the cube and the
// octahedron), 2 the dual, and between them the corners cut ever deeper.
//
// The two halves meet exactly. A regular solid and its dual cross each
// other's edges at their midpoints, so cutting the solid's corners back to
// the midpoints and cutting the dual's back to the same points arrive at one
// shape from both sides, and the knob has no jump at 1.
func Morph(s Solid, m float64) Solid {
	s = s.normalize()
	switch {
	case m <= 1e-3:
		return s
	case m >= 2-1e-3:
		return s.dual().normalize()
	case math.Abs(m-1) < 1e-3:
		return s.ambo().normalize()
	case m < 1:
		return s.truncate(m / 2).normalize()
	default:
		return s.dual().normalize().truncate((2 - m) / 2).normalize()
	}
}

// MorphStops are the morph positions that are named solids: the solid, its
// truncation, the rectified solid, the dual's truncation, the dual.
var MorphStops = []float64{0, 2.0 / 3, 1, 4.0 / 3, 2}

// Kis raises a pyramid of height h on every face (below zero, pushes one
// in). Not normalized after, or the height would be projected away.
func (p Solid) Kis(h float64) Solid { return p.kis(h) }

// Apply is operator o, projected back to the unit sphere.
func (p Solid) Apply(o int) Solid {
	if o <= 0 || o >= len(Ops) {
		return p
	}
	return Ops[o].Apply(p).normalize()
}

// ── the tilings ───────────────────────────────────────────────────────────

// tilingMaxFaces bounds a patch: enough to read as a tiling, few enough that
// a wireframe of it stays well inside a 16-bit index buffer.
const tilingMaxFaces = 700

// arcSteps is how many straight pieces a hyperbolic edge is drawn in: its
// geodesic is a circular arc in the disk.
const arcSteps = 8

// tiling is a patch of {p,q} grown outward from one face at the center by
// reflecting faces across their edges, which is how the regular tiling is
// generated: the reflection of a face in any edge is its neighbor.
func tiling(p, q int, hyperbolic bool) Solid {
	// The central face's corners. Flat, the circumradius is 1; hyperbolic, it
	// is where the corner angle is 2π/q: cosh R = cot(π/p) cot(π/q), at
	// Euclidean distance tanh(R/2) from the disk's center.
	r := 1.0
	if hyperbolic {
		coshR := 1 / (math.Tan(math.Pi/float64(p)) * math.Tan(math.Pi/float64(q)))
		r = math.Tanh(math.Acosh(coshR) / 2)
	}
	first := make([]complex128, p)
	for k := range first {
		first[k] = cmplx.Rect(r, math.Pi/2+2*math.Pi*float64(k)/float64(p))
	}
	reflect := reflectFlat
	if hyperbolic {
		reflect = reflectHyperbolic
	}
	// Grown breadth first, so a capped patch is round rather than a spur.
	faces := [][]complex128{first}
	seen := map[[2]int64]bool{faceKey(first): true}
	for i := 0; i < len(faces) && len(faces) < tilingMaxFaces; i++ {
		f := faces[i]
		for k := range f {
			nf := make([]complex128, p)
			for j, z := range f {
				nf[j] = reflect(z, f[k], f[(k+1)%p])
			}
			key := faceKey(nf)
			if seen[key] || !inPatch(nf, hyperbolic) {
				continue
			}
			seen[key] = true
			faces = append(faces, nf)
		}
	}

	// Into a Solid: shared corners and shared arc points are one vertex.
	var out Solid
	ids := map[[2]int64]int{}
	vertex := func(z complex128) int {
		k := pointKey(z)
		if i, ok := ids[k]; ok {
			return i
		}
		out.Verts = append(out.Verts, Vert{real(z), imag(z), 0})
		ids[k] = len(out.Verts) - 1
		return ids[k]
	}
	for _, f := range faces {
		var loop []int
		for k := range f {
			a, b := f[k], f[(k+1)%p]
			loop = append(loop, vertex(a))
			if hyperbolic {
				for s := 1; s < arcSteps; s++ {
					loop = append(loop, vertex(geodesicPoint(a, b, float64(s)/arcSteps)))
				}
			}
		}
		out.Faces = append(out.Faces, loop)
	}
	if hyperbolic {
		// The disk's rim: the boundary at infinity the tiling never reaches,
		// drawn the way every picture of the disk draws it.
		const n = 96
		var rim []int
		for k := range n {
			rim = append(rim, vertex(cmplx.Rect(1, 2*math.Pi*float64(k)/n)))
		}
		out.Faces = append(out.Faces, rim)
		return out
	}
	// Flat: scaled into the unit disk like the other two.
	far := 0.0
	for _, v := range out.Verts {
		far = max(far, math.Hypot(v.X, v.Y))
	}
	if far > 0 {
		for i := range out.Verts {
			out.Verts[i].X /= far
			out.Verts[i].Y /= far
		}
	}
	return out
}

// inPatch is whether a face is close enough in to draw: flat, within a few
// face widths of the center; hyperbolic, before the faces shrink to nothing
// at the rim.
func inPatch(f []complex128, hyperbolic bool) bool {
	lim := 6.0
	if hyperbolic {
		lim = 0.995
	}
	for _, z := range f {
		if cmplx.Abs(z) > lim {
			return false
		}
	}
	return true
}

// reflectFlat is z reflected in the line through a and b.
func reflectFlat(z, a, b complex128) complex128 {
	d := b - a
	return a + d/cmplx.Conj(d)*cmplx.Conj(z-a)
}

// toOrigin is the disk's isometry taking a to the center, and fromOrigin its
// inverse. Moved there, a geodesic through a is a straight line.
func toOrigin(z, a complex128) complex128   { return (z - a) / (1 - cmplx.Conj(a)*z) }
func fromOrigin(w, a complex128) complex128 { return (w + a) / (1 + cmplx.Conj(a)*w) }

// reflectHyperbolic is z reflected in the geodesic through a and b.
func reflectHyperbolic(z, a, b complex128) complex128 {
	bb := toOrigin(b, a)
	w := toOrigin(z, a)
	return fromOrigin(bb/cmplx.Conj(bb)*cmplx.Conj(w), a)
}

// geodesicPoint is the point s of the way from a to b along their geodesic,
// by hyperbolic distance — so the points an edge is drawn through are the
// same ones whichever end it is walked from, and two faces sharing the edge
// share them.
func geodesicPoint(a, b complex128, s float64) complex128 {
	bb := toOrigin(b, a)
	d := cmplx.Abs(bb)
	if d == 0 {
		return a
	}
	w := complex(math.Tanh(s*math.Atanh(d))/d, 0) * bb
	return fromOrigin(w, a)
}

// pointKey and faceKey identify points and faces across the floating-point
// noise of repeated reflection.
func pointKey(z complex128) [2]int64 {
	return [2]int64{int64(math.Round(real(z) * 1e6)), int64(math.Round(imag(z) * 1e6))}
}

func faceKey(f []complex128) [2]int64 {
	var c complex128
	for _, z := range f {
		c += z
	}
	return pointKey(c / complex(float64(len(f)), 0))
}
