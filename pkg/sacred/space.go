package sacred

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/conway"
)

// The forms in three dimensions, DIM 3.
//
// Each is the figure whose shadow is the flat one: turned so that looking
// straight down the screen's axis (z) shows the 2-D form. The Egg of Life's
// eight spheres sit on a cube's corners, and down the cube's diagonal two of
// them line up at the center and six make a hexagon round it — the Seed of
// Life. The star tetrahedron down the same diagonal is the hexagram. The
// sphere packing whose first shell is Fuller's vector equilibrium is the
// Flower of Life's lattice stacked in layers. Look along z and the solid
// gives back the drawing; turn it and the drawing gives way to the solid.

// diagonal is a basis with the cube's diagonal (1,1,1) as its z: v's
// coordinates in it, so a cube's 3-fold axis points at the viewer.
func diagonal(v V) V {
	e3 := V{1 / math.Sqrt(3), 1 / math.Sqrt(3), 1 / math.Sqrt(3)}
	e1 := V{1 / math.Sqrt2, -1 / math.Sqrt2, 0}
	e2 := V{e3[1]*e1[2] - e3[2]*e1[1], e3[2]*e1[0] - e3[0]*e1[2], e3[0]*e1[1] - e3[1]*e1[0]}
	dot := func(a, b V) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	return V{dot(v, e1), dot(v, e2), dot(v, e3)}
}

// sphereWire is a unit sphere about c as rings round its z axis and
// meridians through its poles.
func sphereWire(c V, rings, meridians int) []stroke {
	const r = 1
	const n = 32
	var out []stroke
	for i := 1; i <= rings; i++ {
		phi := math.Pi * float64(i) / float64(rings+1)
		z, rr := r*math.Cos(phi), r*math.Sin(phi)
		out = append(out, circle(V{c[0], c[1], c[2] + z}, rr, 0, n))
	}
	for k := range meridians {
		th := math.Pi * float64(k) / float64(meridians)
		s := make(stroke, n+1)
		for j := range s {
			a := 2 * math.Pi * float64(j) / n
			s[j] = V{c[0] + r*math.Sin(a)*math.Cos(th), c[1] + r*math.Sin(a)*math.Sin(th), c[2] + r*math.Cos(a)}
		}
		out = append(out, s)
	}
	return out
}

// fccPoints are the face-centered cubic lattice's points within radius of
// the origin, nearest neighbors one apart, turned so a 3-fold axis is z:
// its layers are then the Flower of Life's lattice, stacked. Nearest first.
func fccPoints(radius float64) []V {
	k := int(math.Ceil(radius*math.Sqrt2)) + 1
	var out []V
	for i := -k; i <= k; i++ {
		for j := -k; j <= k; j++ {
			for l := -k; l <= k; l++ {
				if (i+j+l)%2 != 0 {
					continue
				}
				p := scale(V{float64(i), float64(j), float64(l)}, 1/math.Sqrt2)
				if math.Sqrt(p[0]*p[0]+p[1]*p[1]+p[2]*p[2]) <= radius+1e-9 {
					out = append(out, diagonal(p))
				}
			}
		}
	}
	norm := func(p V) float64 { return p[0]*p[0] + p[1]*p[1] + p[2]*p[2] }
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && norm(out[j]) < norm(out[j-1])-1e-9; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// vesica3 is two spheres, each through the other's center, and the circle
// they meet in; then the line of centers and the triangles in two planes.
func vesica3(d int) []stroke {
	a, b := V{-0.5, 0, 0}, V{0.5, 0, 0}
	out := append(sphereWire(a, 3, 4), sphereWire(b, 3, 4)...)
	meet := make(stroke, 65)
	for k := range meet {
		t := 2 * math.Pi * float64(k) / 64
		meet[k] = V{0, math.Sqrt(3) / 2 * math.Cos(t), math.Sqrt(3) / 2 * math.Sin(t)}
	}
	out = append(out, meet)
	if d >= 1 {
		out = append(out, line(V{-1.5, 0, 0}, V{1.5, 0, 0}))
	}
	if d >= 2 {
		h := math.Sqrt(3) / 2
		out = append(out, stroke{a, b, {0, h, 0}, a}, stroke{a, b, {0, 0, h}, a})
	}
	return out
}

// egg is the Egg of Life: eight spheres on the corners of a cube, the cube's
// diagonal toward the viewer, sized so that down it they are the Seed of
// Life's seven circles; then the cube.
func egg(d int) []stroke {
	a := math.Sqrt(1.5) / 2 // half the side: the corners round the axis at radius 1
	var corners []V
	for _, s := range [][3]float64{{1, 1, 1}, {-1, -1, -1}, {1, 1, -1}, {1, -1, 1}, {-1, 1, 1}, {-1, -1, 1}, {-1, 1, -1}, {1, -1, -1}} {
		corners = append(corners, diagonal(V{s[0] * a, s[1] * a, s[2] * a}))
	}
	var out []stroke
	for _, c := range corners {
		out = append(out, sphereWire(c, 1, 2)...)
	}
	if d >= 1 {
		out = append(out, cubeEdges(a)...)
	}
	return out
}

// cubeEdges are the twelve edges of the cube of half-side a, turned by
// diagonal.
func cubeEdges(a float64) []stroke {
	var out []stroke
	for i := range 8 {
		p := V{float64(i&1*2-1) * a, float64(i>>1&1*2-1) * a, float64(i>>2&1*2-1) * a}
		for bit := range 3 {
			if i&(1<<bit) == 0 {
				q := p
				q[bit] = a
				out = append(out, line(diagonal(p), diagonal(q)))
			}
		}
	}
	return out
}

// packing is the Flower of Life in space: spheres through their neighbors'
// centers on the face-centered cubic lattice, shells deep — 13 at one shell
// (the vector equilibrium) and 55 at two.
func packing(shells int) []stroke {
	var out []stroke
	for _, c := range fccPoints(float64(shells)) {
		out = append(out, sphereWire(c, 1, 2)...)
	}
	return out
}

// equilibrium is the Fruit of Life in space: Fuller's vector equilibrium,
// thirteen spheres that touch, twelve round one; then the cuboctahedron
// through the twelve centers.
func equilibrium(d int) []stroke {
	cs := fccPoints(1)
	var out []stroke
	for _, c := range cs {
		out = append(out, sphereWire(scale(c, 2), 1, 2)...)
	}
	if d >= 1 {
		for i := 1; i < len(cs); i++ {
			for j := i + 1; j < len(cs); j++ {
				if math.Abs(dist3(cs[i], cs[j])-1) < 1e-9 {
					out = append(out, line(scale(cs[i], 2), scale(cs[j], 2)))
				}
			}
		}
	}
	return out
}

func dist3(a, b V) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// metatron3 is Metatron's Cube in space: the vector equilibrium's thirteen
// centers, each joined to every other, shortest first; with detail, the
// spheres.
func metatron3(d int) []stroke {
	var cs []V
	for _, c := range fccPoints(1) {
		cs = append(cs, scale(c, 2))
	}
	var out []stroke
	if d >= 1 {
		for _, c := range cs {
			out = append(out, sphereWire(c, 1, 2)...)
		}
	}
	return append(out, allPairs(cs)...)
}

// allPairs joins every pair of points, shortest first.
func allPairs(cs []V) []stroke {
	type seg struct {
		a, b V
		l    float64
	}
	var segs []seg
	for i := range cs {
		for j := i + 1; j < len(cs); j++ {
			segs = append(segs, seg{cs[i], cs[j], dist3(cs[i], cs[j])})
		}
	}
	for i := 1; i < len(segs); i++ {
		for j := i; j > 0 && segs[j].l < segs[j-1].l-1e-9; j-- {
			segs[j], segs[j-1] = segs[j-1], segs[j]
		}
	}
	out := make([]stroke, len(segs))
	for i, s := range segs {
		out[i] = line(s.a, s.b)
	}
	return out
}

// goldenRects are three golden rectangles at right angles to each other,
// whose twelve corners are the icosahedron's; then its thirty edges.
func goldenRects(d int) []stroke {
	rect := func(f func(a, b float64) V) stroke {
		return stroke{f(1, Phi), f(-1, Phi), f(-1, -Phi), f(1, -Phi), f(1, Phi)}
	}
	out := []stroke{
		rect(func(a, b float64) V { return V{0, a, b} }),
		rect(func(a, b float64) V { return V{a, b, 0} }),
		rect(func(a, b float64) V { return V{b, 0, a} }),
	}
	if d >= 1 {
		var vs []V
		for _, s := range out {
			vs = append(vs, s[:4]...)
		}
		for i := range vs {
			for j := i + 1; j < len(vs); j++ {
				if math.Abs(dist3(vs[i], vs[j])-2) < 1e-9 {
					out = append(out, line(vs[i], vs[j]))
				}
			}
		}
	}
	return out
}

// fibonacciSphere is the sunflower on a ball: n seeds spaced evenly in
// height, each turned the golden angle from the last, each a small circle
// lying on the sphere.
func fibonacciSphere(n int, angle float64) []stroke {
	out := make([]stroke, 0, n)
	a := angle * math.Pi / 180
	seed := 0.9 * math.Sqrt(4/float64(n)) // a seed's share of the sphere's area
	for k := range n {
		z := 1 - 2*(float64(k)+0.5)/float64(n)
		r := math.Sqrt(1 - z*z)
		th := float64(k) * a
		c := V{r * math.Cos(th), r * math.Sin(th), z}
		// The seed's circle in the plane touching the sphere at c.
		u := V{-math.Sin(th), math.Cos(th), 0}
		w := V{-z * math.Cos(th), -z * math.Sin(th), r}
		s := make(stroke, 9)
		for j := range s {
			t := 2 * math.Pi * float64(j) / 8
			s[j] = add(c, add(scale(u, seed/2*math.Cos(t)), scale(w, seed/2*math.Sin(t))))
		}
		out = append(out, s)
	}
	return out
}

// starTetrahedron is the merkaba: two tetrahedra through each other, their
// corners the cube's, the cube's diagonal toward the viewer, where it is the
// hexagram; then the cube round it (2) and the octahedron the two share (3).
func starTetrahedron(d int) []stroke {
	tet := [][3]float64{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}}
	var out []stroke
	for _, sign := range []float64{1, -1} {
		var vs []V
		for _, t := range tet {
			vs = append(vs, diagonal(V{sign * t[0], sign * t[1], sign * t[2]}))
		}
		for i := range vs {
			for j := i + 1; j < len(vs); j++ {
				out = append(out, line(vs[i], vs[j]))
			}
		}
	}
	// The default detail is 1, and the cube round it hides the star: only
	// from 2.
	if d >= 2 {
		out = append(out, cubeEdges(1)...)
	}
	if d >= 3 {
		axes := []V{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
		for i := range axes {
			for j := i + 1; j < len(axes); j++ {
				if math.Abs(dist3(axes[i], axes[j])-math.Sqrt2) < 1e-9 {
					out = append(out, line(diagonal(axes[i]), diagonal(axes[j])))
				}
			}
		}
	}
	return out
}

// hexagram is the merkaba seen down its axis: two triangles; then the
// hexagon they share and the circle round them.
func hexagram(d int) []stroke {
	tri := func(off int) stroke {
		return stroke{hexDir(off), hexDir(off + 2), hexDir(off + 4), hexDir(off)}
	}
	out := []stroke{tri(0), tri(1)}
	if d >= 1 {
		h := stroke{}
		r := 1 / math.Sqrt(3)
		for k := range 7 {
			a := float64(k)*math.Pi/3 + math.Pi/2 + math.Pi/6
			h = append(h, V{r * math.Cos(a), r * math.Sin(a), 0})
		}
		out = append(out, h)
	}
	if d >= 2 {
		out = append(out, circle(V{}, 1, math.Pi/2, 96))
	}
	return out
}

// keplerSolids are Kepler's five, outermost first, as {p,q}: the cube, the
// tetrahedron, the dodecahedron, the icosahedron, the octahedron.
var keplerSolids = [][2]int{{4, 3}, {3, 3}, {5, 3}, {3, 5}, {3, 4}}

// Planets are the six spheres of Kepler's model, outermost first.
var Planets = []string{"Saturn", "Jupiter", "Mars", "Earth", "Venus", "Mercury"}

// keplerRadii are the six spheres' radii, Saturn's one: each solid's
// sphere inside is its inradius, which is the next solid's circumradius.
func keplerRadii() []float64 {
	r := []float64{1}
	for _, s := range keplerSolids {
		r = append(r, r[len(r)-1]*inradius(s[0], s[1]))
	}
	return r
}

// inradius is {p,q}'s inradius over its circumradius: how far its faces are
// from its center, its corners one away.
func inradius(p, q int) float64 {
	s := conway.Regular(p, q)
	f := s.Faces[0]
	var c V
	for _, i := range f {
		c = add(c, V{s.Verts[i].X, s.Verts[i].Y, s.Verts[i].Z})
	}
	c = scale(c, 1/float64(len(f)))
	return math.Sqrt(c[0]*c[0]+c[1]*c[1]+c[2]*c[2]) / math.Sqrt(s.Verts[0].X*s.Verts[0].X+s.Verts[0].Y*s.Verts[0].Y+s.Verts[0].Z*s.Verts[0].Z)
}

// keplerSpace is the Mysterium Cosmographicum (1596): the five solids nested
// between the planets' six spheres, each sphere touching the faces of the
// solid outside it and the corners of the one inside, Saturn's outermost.
// Each sphere is drawn as three great circles, so the solids show through.
func keplerSpace() []stroke {
	rs := keplerRadii()
	var out []stroke
	greatCircles := func(r float64) {
		out = append(out, circle(V{}, r, 0, 96))
		for _, th := range []float64{0, math.Pi / 2} {
			s := make(stroke, 97)
			for j := range s {
				a := 2 * math.Pi * float64(j) / 96
				s[j] = V{r * math.Sin(a) * math.Cos(th), r * math.Sin(a) * math.Sin(th), r * math.Cos(a)}
			}
			out = append(out, s)
		}
	}
	for i, ps := range keplerSolids {
		greatCircles(rs[i])
		s := conway.Regular(ps[0], ps[1])
		k := rs[i] / math.Sqrt(s.Verts[0].X*s.Verts[0].X+s.Verts[0].Y*s.Verts[0].Y+s.Verts[0].Z*s.Verts[0].Z)
		for _, e := range s.Edges() {
			a, b := s.Verts[e[0]], s.Verts[e[1]]
			out = append(out, line(V{a.X * k, a.Y * k, a.Z * k}, V{b.X * k, b.Y * k, b.Z * k}))
		}
	}
	greatCircles(rs[len(rs)-1])
	return out
}

// keplerPlane is his first attempt, the year before: in the plane, a
// triangle in Saturn's circle with Jupiter's inside it, then a square, a
// pentagon and a hexagon, each in the last's inner circle. The triangle's
// ratio, a half, is near Jupiter's to Saturn's; the rest are not, which is
// why he went to the solids.
func keplerPlane() []stroke {
	r := 1.0
	var out []stroke
	for n := 3; n <= 6; n++ {
		out = append(out, circle(V{}, r, math.Pi/2, 96))
		p := stroke{}
		for k := 0; k <= n; k++ {
			a := math.Pi/2 + 2*math.Pi*float64(k)/float64(n)
			p = append(p, V{r * math.Cos(a), r * math.Sin(a), 0})
		}
		out = append(out, p)
		r *= math.Cos(math.Pi / float64(n))
	}
	return append(out, circle(V{}, r, math.Pi/2, 96))
}
