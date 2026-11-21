// Package chester is Frank Chester's geometry: the chestahedron, the forms
// he finds inside and around it, and the transformations he puts it through
// in "The Wonder of Seven" (2014).
//
// The chestahedron is a heptahedron of four equilateral triangles and three
// kites, all seven of the same area. Open the three upper faces of a
// tetrahedron outward past upright, like petals, and close the gaps with
// three kites meeting at a new peak: there is one opening at which a kite
// has a triangle's area, and that is the solid.
//
// Everything is built in the frame of a cube, which is the solid's own:
// with the triangle edge √2 the base is the three unit points e1, e2, e3,
// the petal tips are the permutations of (a,a,b) and the peak is (c,c,c).
// The tetrahedron is a=b=c=1 (the tips closed onto the peak), the cube is
// a=1, b=0 with its missing corner at the origin. The chestahedron is a cube
// with one corner sliced off through its three neighbors, then stretched
// until its faces agree: "a seven from a six". His thirteen-faced decatria is
// the solid with its corners cut toward the center by half (more.go).
//
// Several of his relations are golden only nearly. The kite's angles are
// 36.50°, 71.75° and 125.88° (the book "A New Sacred Geometry" prints
// 36.49975504° and 125.8750612°, which these match), not a pentagram's 36,
// 72 and 126, and the
// regular hexagon a rubber band finds on the sides sits 0.6149 of the way up
// them, not 1/φ = 0.6180. The pentagram constructions of the solid (MathZone's
// "Phi model", the paper templates) are therefore near misses, and the
// STAR form folds one up beside the exact solid so the gap can be seen.
package chester

import (
	"math"
	"sort"

	"github.com/0magnet/chaosrack/pkg/geom"
)

// V is a point or a direction.
type V [3]float64

func (a V) add(b V) V           { return V{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a V) sub(b V) V           { return V{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a V) mul(s float64) V     { return V{a[0] * s, a[1] * s, a[2] * s} }
func (a V) dot(b V) float64     { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a V) len() float64        { return math.Sqrt(a.dot(a)) }
func (a V) unit() V             { return a.mul(1 / a.len()) }
func lerp(a, b V, t float64) V  { return a.add(b.sub(a).mul(t)) }
func cross(a, b V) V            { return V{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]} }
func triArea(a, b, c V) float64 { return cross(b.sub(a), c.sub(a)).len() / 2 }

// Poly is a polyhedron: its corners and its faces, each a loop of corners.
type Poly struct {
	V []V
	F [][]int
}

// Area is the area of face f (a fan from its first corner; the faces here
// are convex).
func (p Poly) Area(f int) float64 {
	face := p.F[f]
	a := 0.0
	for i := 1; i+1 < len(face); i++ {
		a += triArea(p.V[face[0]], p.V[face[i]], p.V[face[i+1]])
	}
	return a
}

// Centroid is the solid's center of volume.
func (p Poly) Centroid() V {
	var o V
	for _, v := range p.V {
		o = o.add(v)
	}
	o = o.mul(1 / float64(len(p.V)))
	var sum V
	vol := 0.0
	for _, face := range p.F {
		for i := 1; i+1 < len(face); i++ {
			a, b, c := p.V[face[0]], p.V[face[i]], p.V[face[i+1]]
			w := math.Abs(cross(b.sub(o), c.sub(o)).dot(a.sub(o))) / 6
			sum = sum.add(o.add(a).add(b).add(c).mul(w / 4))
			vol += w
		}
	}
	return sum.mul(1 / vol)
}

// Edges are the solid's edges, each once.
func (p Poly) Edges() [][2]int {
	seen := map[[2]int]bool{}
	var out [][2]int
	for _, face := range p.F {
		for i := range face {
			a, b := face[i], face[(i+1)%len(face)]
			if a > b {
				a, b = b, a
			}
			if a != b && !seen[[2]int{a, b}] {
				seen[[2]int{a, b}] = true
				out = append(out, [2]int{a, b})
			}
		}
	}
	return out
}

// The exact solid's three numbers, solved once.
var exactA, exactB, exactC = solve()

// Exact is the chestahedron's a, b and c (see the package comment).
func Exact() (a, b, c float64) { return exactA, exactB, exactC }

// tipA is a for a given b: the petal on e1e2 stays an equilateral triangle
// of edge √2, (a-1)² + a² + b² = 2, opened outward (the larger root).
func tipA(b float64) float64 { return (1 + math.Sqrt(3-2*b*b)) / 2 }

// peakC is c for given tips: the kite (peak, tip, e3, tip) must be flat.
func peakC(a, b float64) float64 {
	e3, p1, p2 := V{0, 0, 1}, V{a, b, a}, V{b, a, a}
	n := cross(p1.sub(e3), p2.sub(e3))
	s := n[0] + n[1] + n[2]
	if math.Abs(s) < 1e-12 { // the tips are closed onto the peak
		return (2*a + b) / 3
	}
	return n.dot(e3) / s
}

// kiteOverTriangle is a kite's area over a triangle's (√3/2 at edge √2).
func kiteOverTriangle(b float64) float64 {
	a := tipA(b)
	c := peakC(a, b)
	e3, p1, p2, A := V{0, 0, 1}, V{a, b, a}, V{b, a, a}, V{c, c, c}
	return (triArea(e3, p1, A) + triArea(e3, A, p2)) / (math.Sqrt(3) / 2)
}

// solve finds the opening at which a kite has a triangle's area. Over
// b in [0.3, 0.9] the ratio falls through 1 once.
func solve() (a, b, c float64) {
	lo, hi := 0.3, 0.9
	for range 200 {
		m := (lo + hi) / 2
		if kiteOverTriangle(m) > 1 {
			lo = m
		} else {
			hi = m
		}
	}
	b = (lo + hi) / 2
	a = tipA(b)
	return a, b, peakC(a, b)
}

// The corners' order in every chestahedron Poly: base e1 e2 e3, then the
// tips over e1e2, e1e3, e2e3, then the peak, then (cube side) the three
// corners of the shrinking base.
const (
	vE1, vE2, vE3 = 0, 1, 2
	vP12, vP13    = 3, 4
	vP23          = 5
	vPeak         = 6
)

// Family is the tetrahedron → chestahedron → cube. m 0..1 opens the petals
// of a tetrahedron, keeping every triangle equilateral, to the chestahedron
// at 1; m 1..2 goes on to the cube, the base triangle shrinking to the
// cube's missing corner (Chester's "the mitral valve becomes a smaller
// triangle … that point becomes six").
func Family(m float64) Poly {
	m = math.Max(0, math.Min(2, m))
	var a, b, t float64
	if m <= 1 {
		b = 1 + (exactB-1)*m
		a = tipA(b)
	} else {
		t = m - 1
		a = exactA + (1-exactA)*t
		b = exactB * (1 - t)
	}
	c := peakC(a, b)
	p := Poly{V: []V{
		{1, 0, 0}, {0, 1, 0}, {0, 0, 1},
		{a, a, b}, {a, b, a}, {b, a, a},
		{c, c, c},
	}}
	kites := [][]int{
		{vPeak, vP12, vE1, vP13},
		{vPeak, vP23, vE2, vP12},
		{vPeak, vP13, vE3, vP23},
	}
	p.F = append(p.F, kites...)
	if t == 0 {
		p.F = append(p.F,
			[]int{vE1, vE2, vP12}, []int{vE1, vE3, vP13}, []int{vE2, vE3, vP23},
			[]int{vE1, vE2, vE3})
		return p
	}
	// The three sides' planes meet on the axis at the cube's corner; the
	// base is cut across the edges that run there, t of the way down.
	s := b / (2*b + 1 - 2*a)
	corner := V{s, s, s}
	q := len(p.V)
	for _, e := range p.V[:3] {
		p.V = append(p.V, lerp(e, corner, t))
	}
	p.F = append(p.F,
		[]int{q, vE1, vP12, vE2, q + 1},
		[]int{q, vE1, vP13, vE3, q + 2},
		[]int{q + 1, vE2, vP23, vE3, q + 2},
		[]int{q, q + 1, q + 2})
	return p
}

// Chestahedron is the solid itself.
func Chestahedron() Poly { return Family(1) }

// Lateral are the six side edges, base corner to tip, in order round the
// solid: a rubber band slid up them stays a closed loop.
var Lateral = [6][2]int{
	{vE1, vP12}, {vE2, vP12}, {vE2, vP23}, {vE3, vP23}, {vE3, vP13}, {vE1, vP13},
}

// Band is where the rubber band crosses the six side edges, f of the way
// from base to tip.
func Band(p Poly, f float64) [6]V {
	var out [6]V
	for i, e := range Lateral {
		out[i] = lerp(p.V[e[0]], p.V[e[1]], f)
	}
	return out
}

// HexagonAt is the fraction up the sides at which the band is a regular
// hexagon. Below it the sides across a petal are the longer, above it the
// ones round a base corner.
func HexagonAt() float64 {
	p := Chestahedron()
	diff := func(f float64) float64 {
		b := Band(p, f)
		return b[1].sub(b[2]).len() - b[0].sub(b[1]).len()
	}
	lo, hi := 0.01, 0.99
	for range 200 {
		m := (lo + hi) / 2
		if (diff(m) < 0) == (diff(lo) < 0) {
			lo = m
		} else {
			hi = m
		}
	}
	return (lo + hi) / 2
}

// KiteAngles are a kite's corners: at its base corner, at the peak and at a
// tip, in degrees.
func KiteAngles() (base, peak, tip float64) {
	p := Chestahedron()
	at := func(o, x, y int) float64 {
		u, v := p.V[x].sub(p.V[o]), p.V[y].sub(p.V[o])
		return math.Acos(u.dot(v)/u.len()/v.len()) * 180 / math.Pi
	}
	return at(vE3, vP13, vP23), at(vPeak, vP13, vP23), at(vP13, vPeak, vE3)
}

// Sphere is the sphere through the peak and the base corners: its center
// (on the axis) and radius, in the cube frame.
func Sphere() (V, float64) {
	p := Chestahedron()
	u := axis
	base := p.V[vE1].dot(u)
	h := p.V[vPeak].dot(u) - base         // peak height over the base
	r0 := p.V[vE1].sub(u.mul(base)).len() // the base corners' radius
	y := (h*h - r0*r0) / (2 * h)          // the center's height over the base
	return u.mul(base + y), h - y
}

var axis = V{1, 1, 1}.unit()

// Inside-out: each face's pyramid to the center, turned over its face.

// Everted is the solid turned inside out by t: each face's pyramid, apex at
// the center of volume, folded out over its face. At 0 the apexes are all
// at the center, at ½ flat on the faces, at 1 outside, and the faces that
// were the outside now lie against the solid. Each pyramid's apex is
// returned with its face.
func Everted(p Poly, t float64) []V {
	o := p.Centroid()
	out := make([]V, len(p.F))
	for i, face := range p.F {
		a, b, c := p.V[face[0]], p.V[face[1]], p.V[face[2]]
		n := cross(b.sub(a), c.sub(a)).unit()
		foot := o.sub(n.mul(o.sub(a).dot(n)))
		out[i] = lerp(o, o.add(foot.sub(o).mul(2)), t)
	}
	return out
}

// Pentagram: the near-miss construction.

// Golden numbers for a pentagram whose pentagon has unit sides.
var (
	phi       = (1 + math.Sqrt(5)) / 2
	pentR     = 1 / (2 * math.Sin(math.Pi/5))                       // the pentagon's circumradius
	starReach = pentR*math.Cos(math.Pi/5) + math.Sqrt(phi*phi-0.25) // center to a star point
)

// StarFold is three arms of a pentagram, each a kite (the pentagon's center
// triangle with the point beyond it), folded about the center by s: flat at
// 0, closed into the chestahedron's top at 1. The fold keeps every kite flat
// and whole: the kites stand as three faces of a pyramid of k = 5 - 2s equal
// sides, five being flat and three closed. Returned: the center (the peak),
// the four spokes' ends (the pentagon corners; the last meets the first when
// closed) and the three star points, for pentagon side 1 with the peak at
// the origin and the solid hanging below it.
func StarFold(s float64) (peak V, corners [4]V, points [3]V) {
	k := 5 - 2*math.Max(0, math.Min(1, s))
	w := 2 * math.Pi / k
	c72, cw := math.Cos(2*math.Pi/5), math.Cos(w)
	cos2 := 0.0
	if 1-cw > 1e-12 {
		cos2 = math.Max(0, (c72-cw)/(1-cw))
	}
	cp, sp := math.Sqrt(cos2), math.Sqrt(1-cos2)
	var spoke [4]V
	for j := range spoke {
		az := (float64(j)-1.5)*w + math.Pi/2
		spoke[j] = V{sp * math.Cos(az), -cp, sp * math.Sin(az)}
		corners[j] = spoke[j].mul(pentR)
	}
	for j := range points {
		points[j] = spoke[j].add(spoke[j+1]).unit().mul(starReach)
	}
	return peak, corners, points
}

// windAround orders a face's corners counterclockwise about its normal.
func windAround(vs []V, face []int, n V) []int {
	var c V
	for _, i := range face {
		c = c.add(vs[i])
	}
	c = c.mul(1 / float64(len(face)))
	u := vs[face[0]].sub(c).unit()
	w := cross(n.unit(), u)
	sort.Slice(face, func(i, j int) bool {
		di, dj := vs[face[i]].sub(c), vs[face[j]].sub(c)
		return math.Atan2(di.dot(w), di.dot(u)) < math.Atan2(dj.dot(w), dj.dot(u))
	})
	return face
}

// Lines: a figure built up as points and the segments between them.

// Lines is a wireframe under construction.
type Lines struct {
	P []V
	E [][2]int
}

func (l *Lines) point(v V) int { l.P = append(l.P, v); return len(l.P) - 1 }

func (l *Lines) seg(a, b V) { l.E = append(l.E, [2]int{l.point(a), l.point(b)}) }

// path joins points in order; loop closes it.
func (l *Lines) path(vs ...V) {
	base := len(l.P)
	l.P = append(l.P, vs...)
	for i := 1; i < len(vs); i++ {
		l.E = append(l.E, [2]int{base + i - 1, base + i})
	}
}

func (l *Lines) loop(vs ...V) {
	base := len(l.P)
	l.path(vs...)
	if len(vs) > 2 {
		l.E = append(l.E, [2]int{base + len(vs) - 1, base})
	}
}

// poly adds a solid's edges.
func (l *Lines) poly(p Poly) {
	base := len(l.P)
	l.P = append(l.P, p.V...)
	for _, e := range p.Edges() {
		l.E = append(l.E, [2]int{base + e[0], base + e[1]})
	}
}

// circle adds a circle of n segments about center c in the plane spanned
// by the unit directions u and w.
func (l *Lines) circle(c, u, w V, r float64, n int) {
	pts := make([]V, n)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = c.add(u.mul(r * math.Cos(a))).add(w.mul(r * math.Sin(a)))
	}
	l.loop(pts...)
}

// Geom is the wireframe as the page uploads it.
func (l Lines) Geom() geom.Lines {
	out := geom.Lines{Vertices: make([]float32, 0, 3*len(l.P)), Indices: make([]uint16, 0, 2*len(l.E))}
	for _, v := range l.P {
		out.Vertices = append(out.Vertices, float32(v[0]), float32(v[1]), float32(v[2]))
	}
	for _, e := range l.E {
		out.Indices = append(out.Indices, uint16(e[0]), uint16(e[1])) //nolint:gosec // the forms stay far under 65536 points (TestFormsFitTheIndexBuffer)
	}
	return out
}
