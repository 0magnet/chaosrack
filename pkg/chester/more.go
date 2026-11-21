package chester

import (
	"math"
)

// The forms from Seth Miller's "A New Sacred Geometry" (2012) and Frank's own
// pages that the lecture only gestures at: the decatria as he defines it,
// the tetrahedron turned in the cube, his first seven-sided form, and the
// rolled paper cone.

// plane is the half-space n·x ≤ d, n of unit length.
type plane struct {
	n V
	d float64
}

// Intersect is the convex solid the half-spaces bound: its corners and its
// faces, wound outward. A plane that only touches it adds no face.
func Intersect(ps []plane) Poly {
	var p Poly
	inside := func(x V) bool {
		for _, q := range ps {
			if q.n.dot(x) > q.d+1e-9 {
				return false
			}
		}
		return true
	}
	for i := range ps {
		for j := i + 1; j < len(ps); j++ {
			for k := j + 1; k < len(ps); k++ {
				a, b, c := ps[i], ps[j], ps[k]
				det := a.n.dot(cross(b.n, c.n))
				if math.Abs(det) < 1e-12 {
					continue
				}
				x := cross(b.n, c.n).mul(a.d).add(cross(c.n, a.n).mul(b.d)).add(cross(a.n, b.n).mul(c.d)).mul(1 / det)
				if !inside(x) {
					continue
				}
				dup := false
				for _, v := range p.V {
					if v.sub(x).len() < 1e-7 {
						dup = true
						break
					}
				}
				if !dup {
					p.V = append(p.V, x)
				}
			}
		}
	}
	for _, q := range ps {
		var face []int
		for i, v := range p.V {
			if math.Abs(q.n.dot(v)-q.d) < 1e-7 {
				face = append(face, i)
			}
		}
		if len(face) >= 3 {
			p.F = append(p.F, windAround(p.V, face, q.n))
		}
	}
	return p
}

// facePlanes are a convex solid's faces as half-spaces.
func facePlanes(p Poly) []plane {
	o := V{}
	for _, v := range p.V {
		o = o.add(v)
	}
	o = o.mul(1 / float64(len(p.V)))
	out := make([]plane, 0, len(p.F))
	for _, f := range p.F {
		a, b, c := p.V[f[0]], p.V[f[1]], p.V[f[2]]
		n := cross(b.sub(a), c.sub(a)).unit()
		if n.dot(o.sub(a)) > 0 {
			n = n.mul(-1)
		}
		out = append(out, plane{n, n.dot(a)})
	}
	return out
}

// Dualized is Frank's dualization of the chestahedron (decatria page): "cut
// each corner at 90 degrees to the form's center and stop cutting one half
// the distance from the corner to the center". The center is the solid's
// sphere's. m runs the cut: 0 the chestahedron, 1 the half cut, which is the
// thirteen-faced decatria (9 pentagons, 3 hexagons, a triangle; the peak's
// own cut leaves no face), 2 the corners cut until the old faces are gone and
// only the seven cuts bound it: the dual heptahedron, the chestahedron turned
// over (its four-cornered base corners become the kites, its peak and tips
// the triangles).
func Dualized(m float64) Poly {
	m = upTo(m, 2)
	ch := Chestahedron()
	ps := facePlanes(ch)
	o := sphereC
	r2 := dualRadius2(ps, o)
	for _, v := range ch.V {
		d := v.sub(o)
		r := d.len()
		// The cut, as a fraction of the corner's distance from the center:
		// from where it first touches the solid (at the corner itself for
		// a sharp corner, further out where a plane square to the center
		// would slice in beside it) to ½ over 0..1, then on to the polar
		// dual's R²/r², at which the cuts alone bound the solid and each of
		// its corners just touches one of the old faces: the dual inside.
		k0 := 0.0
		for _, w := range ch.V {
			k0 = math.Max(k0, d.unit().dot(w.sub(o))/r)
		}
		var k float64
		if m <= 1 {
			k = k0 + (0.5-k0)*m
		} else {
			k = 0.5 + (m-1)*(r2/(r*r)-0.5)
		}
		ps = append(ps, plane{d.unit(), d.unit().dot(o) + k*r})
	}
	return Intersect(ps)
}

// dualRadius2 is R² for the polar dual about o that fits just inside the
// solid: R the distance from o to the nearest face, so the dual's corner over
// that face touches it and the others fall short of theirs.
func dualRadius2(faces []plane, o V) float64 {
	h := math.Inf(1)
	for _, f := range faces {
		h = math.Min(h, f.d-f.n.dot(o))
	}
	return h * h
}

// Spin is the chestahedron born in the cube (the book, pp. 12-14): "within
// the cube the sequence begins with the contraction of the tetrahedron,
// which then produces the Chestahedron before contracting into an
// octahedron … the contraction reverses, expanding … into another
// Chestahedron and then … back into a (now rotated) tetrahedron", about the
// cube's space diagonal. All of it in the unit cube, its peak held in the
// corner (1,1,1):
//
//	0   the tetrahedron on alternate corners
//	½   the chestahedron, as the lecture's earth study places it: the base
//	    corners drawn in along the cube's edges to 1/c, the tips out of the
//	    peak (Family scaled to keep the peak in the corner)
//	1   the octahedron on the cube's face centers: the base corners have
//	    risen to the upper three, the tips sunk to the lower three, and the
//	    peak gone into the upper face: the levels reversed
//	1-2 the same back, turned over through the cube's center, to the other
//	    tetrahedron
//
// The book names the stages; the way from the chestahedron to the
// octahedron is not given, and is drawn here as each corner going straight
// to its place.
func Spin(m float64) Poly {
	m = upTo(m, 2)
	if m > 1 { // the second half is the first, turned over through the center
		p := Spin(2 - m)
		for i, v := range p.V {
			p.V[i] = V{1, 1, 1}.sub(v)
		}
		return p
	}
	if m <= 0.5 {
		p := Family(2 * m)
		s := 1 / p.V[vPeak][0]
		for i, v := range p.V {
			p.V[i] = v.mul(s)
		}
		return p
	}
	t := 2*m - 1
	oct := []V{
		{1, 0.5, 0.5}, {0.5, 1, 0.5}, {0.5, 0.5, 1}, // where the base corners go
		{0.5, 0.5, 0}, {0.5, 0, 0.5}, {0, 0.5, 0.5}, // where the tips go
	}
	if t >= 1 { // the octahedron itself
		return Poly{V: oct, F: [][]int{
			{vE1, vE2, vP12}, {vE1, vE3, vP13}, {vE2, vE3, vP23}, {vE1, vE2, vE3},
			{vP12, vE1, vP13}, {vP23, vE2, vP12}, {vP13, vE3, vP23}, {vP12, vP13, vP23},
		}}
	}
	p := Spin(0.5)
	oct = append(oct, V{2.0 / 3, 2.0 / 3, 2.0 / 3}) // the peak, into the upper face
	for i := range p.V {
		p.V[i] = lerp(p.V[i], oct[i], t)
	}
	return p
}

// FirstPieces is Frank's first seven-sided form, four of them, and the
// tetrahedron they leave (the lecture's opening, and the book: "the
// tetrahedron as composed of four 7-sided forms and a nested tetrahedron").
// Each face of a tetrahedron is three kites, a corner's two edge midpoints
// and the face's center (each kite the two 30-60-90 drafting triangles he
// put together as a boy); a piece is the three kites round one corner, the
// triangle of their three face centers, and three triangles cut to its
// neighbors. Three kites equal, four triangles equal, not equal together.
// Taken out, the four leave the tetrahedron of the face centers, turned
// over: the dual inside. The tetrahedron has edge √2 in the unit cube.
// apart moves each piece out from the center.
func FirstPieces(apart float64) (pieces [4]Poly, inner Poly) {
	t := [4]V{{1, 1, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	o := V{0.5, 0.5, 0.5}
	var center [4]V // center of the face opposite corner i
	for i := range t {
		var s V
		for j := range t {
			if j != i {
				s = s.add(t[j])
			}
		}
		center[i] = s.mul(1.0 / 3)
	}
	for i := range t {
		var others []int
		for j := range t {
			if j != i {
				others = append(others, j)
			}
		}
		// Corners: 0 the corner, 1-3 the midpoints toward the others, 4-6 the
		// centers of the faces at the corner (the face opposite each other).
		p := Poly{V: []V{t[i]}}
		for _, j := range others {
			p.V = append(p.V, lerp(t[i], t[j], 0.5))
		}
		for _, j := range others {
			p.V = append(p.V, center[j])
		}
		// mid(i,j) is on the faces opposite the two others k, l.
		face := func(j int) int { // the piece's corner index of center[j]
			for k, o := range others {
				if o == j {
					return 4 + k
				}
			}
			return -1
		}
		mid := func(j int) int {
			for k, o := range others {
				if o == j {
					return 1 + k
				}
			}
			return -1
		}
		a, b, c := others[0], others[1], others[2]
		p.F = [][]int{
			// the kites: corner, a midpoint, the face center, a midpoint
			{0, mid(a), face(c), mid(b)},
			{0, mid(b), face(a), mid(c)},
			{0, mid(c), face(b), mid(a)},
			// the base: the three face centers
			{face(a), face(b), face(c)},
			// the cuts to the neighbors: a midpoint and the two centers beside it
			{mid(a), face(b), face(c)},
			{mid(b), face(c), face(a)},
			{mid(c), face(a), face(b)},
		}
		shift := p.Centroid().sub(o).mul(apart)
		for k, v := range p.V {
			p.V[k] = v.add(shift)
		}
		pieces[i] = p
	}
	inner = Poly{V: []V{center[0], center[1], center[2], center[3]},
		F: [][]int{{1, 2, 3}, {0, 2, 3}, {0, 1, 3}, {0, 1, 2}}}
	return pieces, inner
}

// Roll is the paper disk of the heart demonstration rolled round on itself
// into a cone: a point at radius rho and angle phi on a unit disk lands on a
// cone of half-angle asin(1/k), k turns round, the apex down at the origin.
// k = 1 is the flat disk. Each turn sits a little outside the last, as paper
// does, so the layers can be told apart.
func Roll(rho, phi, k float64) V {
	k = math.Max(1, k)
	sa := 1 / k
	ca := math.Sqrt(1 - sa*sa)
	th := phi * k
	turn := math.Floor(th / (2 * math.Pi))
	r := rho*sa + 0.012*turn*rho*ca
	return V{r * math.Cos(th), rho * ca, r * math.Sin(th)}
}
