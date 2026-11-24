package sacred

import (
	"math"
	"math/cmplx"
	"sort"

	"github.com/0magnet/chaosrack/pkg/conway"
)

// The Penrose tiling and, one dimension up, the rhombic triacontahedron.
//
// Penrose's rhombs (P3) cover the plane with five-fold symmetry and never
// repeat. They are grown here by deflation: start from a sun of ten Robinson
// triangles, each a half rhomb, and cut every triangle into smaller ones by
// the golden ratio, generation after generation. Drawing each triangle's two
// legs and not its base joins the halves into the rhombs.
//
// The rhombs are the shadows of a five-dimensional cube's faces; the
// rhombic triacontahedron is a six-dimensional cube's shadow, thirty golden
// rhombi round a ball, which is the Penrose tiling's counterpart in space.

// robinson is a Robinson triangle: thin (0) or thick (1), apex a.
type robinson struct {
	thick   bool
	a, b, c complex128
}

// PenroseGenerations is how many times the sun is cut at detail d: three to
// begin with, one more a step, to seven, past which the 16-bit index buffer
// would not hold the edges.
func PenroseGenerations(d int) int { return min(3+d, 7) }

// penroseTriangles are the sun of ten deflated n times.
func penroseTriangles(n int) []robinson {
	var ts []robinson
	for i := range 10 {
		b := cmplx.Rect(1, float64(2*i-1)*math.Pi/10)
		c := cmplx.Rect(1, float64(2*i+1)*math.Pi/10)
		if i%2 == 0 {
			b, c = c, b
		}
		ts = append(ts, robinson{false, 0, b, c})
	}
	phi := complex(Phi, 0)
	for range n {
		next := make([]robinson, 0, len(ts)*3)
		for _, t := range ts {
			if !t.thick {
				p := t.a + (t.b-t.a)/phi
				next = append(next, robinson{false, t.c, p, t.b}, robinson{true, p, t.c, t.a})
				continue
			}
			q := t.b + (t.a-t.b)/phi
			r := t.b + (t.c-t.b)/phi
			next = append(next, robinson{true, r, t.c, t.a}, robinson{true, q, r, t.b}, robinson{false, r, q, t.a})
		}
		ts = next
	}
	return ts
}

// penrose is the tiling n generations deep: every rhomb edge once, nearest
// the center first, so the construction grows outward.
func penrose(n int) []stroke {
	key := func(z complex128) [2]int64 {
		return [2]int64{int64(math.Round(real(z) * 1e7)), int64(math.Round(imag(z) * 1e7))}
	}
	seen := map[[4]int64]bool{}
	type edge struct {
		a, b complex128
		r    float64
	}
	var edges []edge
	add := func(a, b complex128) {
		ka, kb := key(a), key(b)
		if ka[0] > kb[0] || (ka[0] == kb[0] && ka[1] > kb[1]) {
			ka, kb = kb, ka
		}
		k := [4]int64{ka[0], ka[1], kb[0], kb[1]}
		if seen[k] {
			return
		}
		seen[k] = true
		edges = append(edges, edge{a, b, cmplx.Abs((a + b) / 2)})
	}
	for _, t := range penroseTriangles(n) {
		add(t.c, t.a)
		add(t.a, t.b)
	}
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].r < edges[j].r })
	out := make([]stroke, len(edges))
	for i, e := range edges {
		out[i] = line(V{real(e.a), imag(e.a), 0}, V{real(e.b), imag(e.b), 0})
	}
	return out
}

// PenroseCounts is how many thick and thin rhombs n generations make: each
// rhomb is two Robinson triangles.
func PenroseCounts(n int) (thick, thin int) {
	for _, t := range penroseTriangles(n) {
		if t.thick {
			thick++
		} else {
			thin++
		}
	}
	return thick / 2, thin / 2
}

// triacontahedron is the rhombic triacontahedron: the icosahedron's twelve
// corners and, a little further in, its twenty faces' centers (the
// dodecahedron's corners), each corner joined to the five face centers round
// it — sixty edges, thirty golden rhombi. The face centers sit where each
// rhomb comes out flat.
func triacontahedron() []stroke {
	ico := conway.Regular(3, 5)
	unit := func(v V) V { return scale(v, 1/math.Sqrt(v[0]*v[0]+v[1]*v[1]+v[2]*v[2])) }
	var corners []V
	for _, v := range ico.Verts {
		corners = append(corners, unit(V{v.X, v.Y, v.Z}))
	}
	var centers []V
	for _, f := range ico.Faces {
		var c V
		for _, i := range f {
			c = add(c, corners[i])
		}
		centers = append(centers, unit(c))
	}
	dot := func(a, b V) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	// A rhomb is two corners joined by an icosahedron edge and the centers
	// of the two faces on that edge. Its diagonals cross only if the edge's
	// midpoint and the two centers' midpoint are one point: b, the centers'
	// radius, is what makes them so.
	e := ico.Edges()[0]
	f0, f1 := -1, -1
	for i, f := range ico.Faces {
		has := 0
		for _, v := range f {
			if v == e[0] || v == e[1] {
				has++
			}
		}
		if has == 2 {
			if f0 < 0 {
				f0 = i
			} else {
				f1 = i
			}
		}
	}
	theta := math.Acos(dot(corners[e[0]], corners[e[1]]))
	psi := math.Acos(dot(centers[f0], centers[f1]))
	b := math.Cos(theta/2) / math.Cos(psi/2)
	var out []stroke
	for fi, f := range ico.Faces {
		for _, v := range f {
			out = append(out, line(corners[v], scale(centers[fi], b)))
		}
	}
	return out
}
