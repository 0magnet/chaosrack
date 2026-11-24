// Package sacred is the Sacred Geometry model's figures: the compass and
// straightedge constructions the name usually means — the vesica piscis, the
// Seed, Flower and Fruit of Life, Metatron's Cube, the golden rectangle's
// spiral — and the sunflower's phyllotaxis, which is the golden ratio in a
// living thing.
//
// Each form is built as its construction: an ordered list of strokes, each a
// circle, an arc or a line, in the order it is drawn with compass and
// straightedge. MORPH from 0 to 1 draws that far through the list, so turning
// it (or modulating it) draws the figure as it is made; 1 is the form as it is
// usually shown; past 1 it is twisted into a vortex, every point turned about
// the center by its distance from it. DETAIL is how much there is: rings of
// the flower, squares of the golden spiral, seeds of the sunflower, and the
// construction lines of the rest.
//
// Like pkg/chester it is pure arithmetic with no browser in it, shared by
// the page, a native rack and the README's gallery.
package sacred

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/geom"
)

// The forms, by FORM's position.
const (
	Vesica = iota
	Seed
	Flower
	Fruit
	Metatron
	Golden
	Phyllotaxis
	NumForms
)

// FormNames are FORM's positions, for the knob and its tooltip.
var FormNames = []string{"vesica", "seed", "flower", "fruit", "metatron", "golden", "sunflower"}

// RingLabels are the same, short enough to fit round the dial.
var RingLabels = []string{"ves", "seed", "flwr", "frut", "mtrn", "gold", "sun"}

// MaxDetail is the DETAIL knob's top.
const MaxDetail = 10

// Phi is the golden ratio, (1 + √5)/2.
var Phi = (1 + math.Sqrt(5)) / 2

// GoldenAngle is 360°/φ², in degrees: the turn between a sunflower's seeds.
var GoldenAngle = 360 / (Phi * Phi)

// V is a point in the plane z = 0 or, for the 3-D forms, in space.
type V [3]float64

// Lines is a figure as points and the segments between them. Points no
// segment uses are drawn as nothing; the figure's frame is kept that way, so a
// construction half drawn is framed as the whole will be.
type Lines struct {
	P []V
	E [][2]int
}

// Geom is the figure as a line list for the renderers.
func (l Lines) Geom() geom.Lines {
	out := geom.Lines{Vertices: make([]float32, 0, 3*len(l.P)), Indices: make([]uint16, 0, 2*len(l.E))}
	for _, v := range l.P {
		out.Vertices = append(out.Vertices, float32(v[0]), float32(v[1]), float32(v[2]))
	}
	for _, e := range l.E {
		out.Indices = append(out.Indices, uint16(e[0]), uint16(e[1])) //nolint:gosec // bounded: the largest form is far under 65536 points
	}
	return out
}

// stroke is one move of the construction: a polyline.
type stroke []V

// Form is form at morph and detail, its size about one: drawn from its
// construction as far as morph says, twisted past 1.
func Form(form int, morph, detail float64) Lines {
	form = max(0, min(NumForms-1, form))
	d := int(math.Round(max(0, min(MaxDetail, detail))))
	strokes := build(form, d, morph)
	strokes = normalize(strokes)
	if morph > 1 && form != Phyllotaxis {
		strokes = twist(strokes, morph-1)
	}
	return reveal(strokes, min(max(morph, 0), 1))
}

func build(form, d int, morph float64) []stroke {
	switch form {
	case Vesica:
		return vesica(d)
	case Seed:
		return seed(d)
	case Flower:
		return flower(FlowerRings(d))
	case Fruit:
		return fruit(d)
	case Metatron:
		return metatron(d)
	case Golden:
		return golden(4 + 2*d)
	default:
		angle := GoldenAngle
		if morph > 1 {
			// Past 1, off the golden angle: by 2 it is 2% more, and the
			// spirals have broken into spokes.
			angle *= 1 + 0.02*(min(morph, 2)-1)
		}
		return sunflower(150*(d+1), angle)
	}
}

// circle is the circle about c of radius r, from angle a0 round, in n pieces.
func circle(c V, r, a0 float64, n int) stroke {
	return arc(c, r, a0, a0+2*math.Pi, n)
}

// arc is the arc about c of radius r from angle a0 to a1, in n pieces.
func arc(c V, r, a0, a1 float64, n int) stroke {
	s := make(stroke, n+1)
	for k := range s {
		a := a0 + (a1-a0)*float64(k)/float64(n)
		s[k] = V{c[0] + r*math.Cos(a), c[1] + r*math.Sin(a), c[2]}
	}
	return s
}

// clippedCircle is the circle about c of radius r, kept where it lies within
// R of the origin: the arcs of a flower inside its bounding circle.
func clippedCircle(c V, r, bound float64) []stroke {
	const n = 144
	full := circle(c, r, 0, n)
	in := func(p V) bool { return math.Hypot(p[0], p[1]) <= bound+1e-9 }
	var out []stroke
	var cur stroke
	start := 0
	// Start the walk at a point outside, so a run is never split at the seam.
	for k, p := range full[:n] {
		if !in(p) {
			start = k
			break
		}
	}
	for k := 0; k <= n; k++ {
		p := full[(start+k)%n]
		if in(p) {
			cur = append(cur, p)
			continue
		}
		if len(cur) > 1 {
			out = append(out, cur)
		}
		cur = nil
	}
	if len(cur) > 1 {
		out = append(out, cur)
	}
	if len(out) == 0 && in(full[0]) {
		return []stroke{full} // wholly inside
	}
	return out
}

func line(a, b V) stroke { return stroke{a, b} }

// hexDir is the k-th of the six directions, the first straight up: the
// figures stand on a point, as they are usually drawn.
func hexDir(k int) V {
	a := math.Pi/2 + float64(k)*math.Pi/3
	return V{math.Cos(a), math.Sin(a), 0}
}

func scale(v V, s float64) V { return V{v[0] * s, v[1] * s, v[2] * s} }
func add(a, b V) V           { return V{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

// vesica is two circles, each through the other's center — Euclid's first
// proposition — then the line of centers and the lens's axis, the two
// equilateral triangles they make, and the √3 rectangle round the lens.
func vesica(d int) []stroke {
	a, b := V{-0.5, 0, 0}, V{0.5, 0, 0}
	top, bot := V{0, math.Sqrt(3) / 2, 0}, V{0, -math.Sqrt(3) / 2, 0}
	out := []stroke{circle(a, 1, 0, 96), circle(b, 1, math.Pi, 96)}
	if d >= 1 {
		out = append(out, line(V{-1.5, 0, 0}, V{1.5, 0, 0}), line(top, bot))
	}
	if d >= 2 {
		out = append(out, stroke{a, b, top, a}, stroke{a, b, bot, a})
	}
	if d >= 3 {
		h := math.Sqrt(3) / 2
		out = append(out, stroke{{-0.5, -h, 0}, {0.5, -h, 0}, {0.5, h, 0}, {-0.5, h, 0}, {-0.5, -h, 0}})
	}
	return out
}

// seed is the Seed of Life: a circle, then six more, each centered where the
// last crossed the first, so each new one starts where the compass already
// is. Then the circle round them all and the hexagram through their centers.
func seed(d int) []stroke {
	out := []stroke{circle(V{}, 1, math.Pi/2, 96)}
	for k := range 6 {
		out = append(out, circle(hexDir(k), 1, math.Pi/2+float64(k)*math.Pi/3+math.Pi, 96))
	}
	if d >= 1 {
		out = append(out, circle(V{}, 2, math.Pi/2, 128))
	}
	if d >= 2 {
		out = append(out, stroke{hexDir(0), hexDir(2), hexDir(4), hexDir(0)}, stroke{hexDir(1), hexDir(3), hexDir(5), hexDir(1)})
	}
	return out
}

// latticeRing is the ring of the hexagonal lattice a point is on (0 the
// center, 1 the six round it, …), for a point a·u + b·w.
func latticeRing(a, b int) int {
	abs := func(x int) int { return max(x, -x) }
	return max(abs(a), abs(b), abs(a+b))
}

// latticePoint is a·u + b·w, u and w the lattice's directions 60° apart, u
// straight up.
func latticePoint(a, b int) V {
	return add(scale(hexDir(0), float64(a)), scale(hexDir(1), float64(b)))
}

// flowerCenters are the lattice's points out to ring rings, center first,
// ring by ring, each ring round from the top.
func flowerCenters(rings int) []V {
	out := []V{{}}
	for ring := 1; ring <= rings; ring++ {
		var pts []V
		for a := -ring; a <= ring; a++ {
			for b := -ring; b <= ring; b++ {
				if latticeRing(a, b) == ring {
					pts = append(pts, latticePoint(a, b))
				}
			}
		}
		sortByAngle(pts)
		out = append(out, pts...)
	}
	return out
}

// sortByAngle orders points round from straight up, counterclockwise.
func sortByAngle(pts []V) {
	key := func(p V) float64 {
		a := math.Atan2(p[1], p[0]) - math.Pi/2
		for a < -1e-9 {
			a += 2 * math.Pi
		}
		return a
	}
	for i := 1; i < len(pts); i++ {
		for j := i; j > 0 && key(pts[j]) < key(pts[j-1]); j-- {
			pts[j], pts[j-1] = pts[j-1], pts[j]
		}
	}
}

// flower is the Flower of Life: the lattice of circles each through six
// others' centers, rings deep, inside a circle that cuts the next ring into
// arcs — the 19 circles and their arcs of the usual figure at two rings.
func flower(rings int) []stroke {
	bound := float64(rings + 1)
	var out []stroke
	for _, c := range flowerCenters(rings + 1) {
		out = append(out, clippedCircle(c, 1, bound)...)
	}
	return append(out, circle(V{}, bound, math.Pi/2, 192))
}

// FlowerCounts is how many circles a flower of rings rings draws whole and
// how many arcs its bound cuts from the next ring.
func FlowerCounts(rings int) (whole, arcs int) {
	bound := float64(rings + 1)
	for _, c := range flowerCenters(rings + 1) {
		pieces := clippedCircle(c, 1, bound)
		if len(pieces) == 1 && len(pieces[0]) == 145 {
			whole++
		} else {
			arcs += len(pieces)
		}
	}
	return whole, arcs
}

// FlowerRings is how many rings the flower draws at detail d: d + 1, to six.
// Seven rings and their arcs would pass a 16-bit index buffer's 65536 points,
// and past six the figure reads as texture rather than as circles.
func FlowerRings(d int) int { return max(1, min(d+1, 6)) }

// fruitCenters are the Fruit of Life's thirteen: the center, then six at
// twice the radius and six at four times, along the same directions.
func fruitCenters() []V {
	out := []V{{}}
	for _, r := range []float64{2, 4} {
		for k := range 6 {
			out = append(out, scale(hexDir(k), r))
		}
	}
	return out
}

// fruit is the Fruit of Life: thirteen circles that touch and never cross,
// then the hexagons through their centers.
func fruit(d int) []stroke {
	var out []stroke
	for _, c := range fruitCenters() {
		out = append(out, circle(c, 1, math.Pi/2, 96))
	}
	if d >= 1 {
		for _, r := range []float64{2, 4} {
			h := stroke{}
			for k := range 7 {
				h = append(h, scale(hexDir(k%6), r))
			}
			out = append(out, h)
		}
	}
	return out
}

// metatron is Metatron's Cube: the Fruit's thirteen centers, each joined to
// every other, shortest first; with detail, the circles under them.
func metatron(d int) []stroke {
	cs := fruitCenters()
	var out []stroke
	if d >= 1 {
		for _, c := range cs {
			out = append(out, circle(c, 1, math.Pi/2, 96))
		}
	}
	type seg struct {
		a, b V
		l    float64
	}
	var segs []seg
	for i := range cs {
		for j := i + 1; j < len(cs); j++ {
			segs = append(segs, seg{cs[i], cs[j], math.Hypot(cs[i][0]-cs[j][0], cs[i][1]-cs[j][1])})
		}
	}
	for i := 1; i < len(segs); i++ {
		for j := i; j > 0 && segs[j].l < segs[j-1].l-1e-9; j-- {
			segs[j], segs[j-1] = segs[j-1], segs[j]
		}
	}
	for _, s := range segs {
		out = append(out, line(s.a, s.b))
	}
	return out
}

// golden is the golden rectangle cut into n whirling squares, each cut the
// line across and then the quarter circle in its square, which is the golden
// spiral drawn as Fibonacci drew it.
func golden(n int) []stroke {
	x, y, w, h := 0.0, 0.0, Phi, 1.0
	out := []stroke{{{x, y, 0}, {x + w, y, 0}, {x + w, y + h, 0}, {x, y + h, 0}, {x, y, 0}}}
	for i := range n {
		var cut stroke
		var c V
		var a0, s float64
		switch i % 4 {
		case 0: // the square off the left
			s = h
			cut, c, a0 = line(V{x + s, y, 0}, V{x + s, y + h, 0}), V{x + s, y, 0}, math.Pi
			x, w = x+s, w-s
		case 1: // off the top
			s = w
			cut, c, a0 = line(V{x, y + h - s, 0}, V{x + w, y + h - s, 0}), V{x, y + h - s, 0}, math.Pi/2
			h -= s
		case 2: // off the right
			s = h
			cut, c, a0 = line(V{x + w - s, y, 0}, V{x + w - s, y + h, 0}), V{x + w - s, y + s, 0}, 0
			w -= s
		default: // off the bottom
			s = w
			cut, c, a0 = line(V{x, y + s, 0}, V{x + w, y + s, 0}), V{x + s, y + s, 0}, -math.Pi/2
			y, h = y+s, h-s
		}
		out = append(out, cut, arc(c, s, a0, a0-math.Pi/2, 24))
	}
	return out
}

// sunflower is Vogel's model of a sunflower's head: seed n at radius √n,
// turned n times angle degrees, each seed a small circle.
func sunflower(n int, angle float64) []stroke {
	out := make([]stroke, 0, n)
	a := angle * math.Pi / 180
	for k := range n {
		r := math.Sqrt(float64(k) + 0.5)
		c := V{r * math.Cos(float64(k)*a), r * math.Sin(float64(k)*a), 0}
		out = append(out, circle(c, 0.4, 0, 10))
	}
	return out
}

// normalize centers the figure on its box and scales it to a radius of one.
func normalize(ss []stroke) []stroke {
	lo, hi := V{math.Inf(1), math.Inf(1), math.Inf(1)}, V{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, s := range ss {
		for _, p := range s {
			for a := range 3 {
				lo[a], hi[a] = min(lo[a], p[a]), max(hi[a], p[a])
			}
		}
	}
	if len(ss) == 0 {
		return ss
	}
	mid := scale(add(lo, hi), 0.5)
	r := 0.0
	for _, s := range ss {
		for _, p := range s {
			r = max(r, math.Sqrt((p[0]-mid[0])*(p[0]-mid[0])+(p[1]-mid[1])*(p[1]-mid[1])+(p[2]-mid[2])*(p[2]-mid[2])))
		}
	}
	if r == 0 {
		r = 1
	}
	out := make([]stroke, len(ss))
	for i, s := range ss {
		out[i] = make(stroke, len(s))
		for j, p := range s {
			out[i][j] = scale(V{p[0] - mid[0], p[1] - mid[1], p[2] - mid[2]}, 1/r)
		}
	}
	return out
}

// twist turns each point about the z axis by t half turns at the rim, in
// proportion to its distance from the center: a vortex.
func twist(ss []stroke, t float64) []stroke {
	t = min(t, 1)
	for _, s := range ss {
		for j, p := range s {
			a := t * math.Pi * math.Hypot(p[0], p[1])
			c, sn := math.Cos(a), math.Sin(a)
			s[j] = V{p[0]*c - p[1]*sn, p[0]*sn + p[1]*c, p[2]}
		}
	}
	return ss
}

// reveal draws strokes as far as f (0 to 1) through the construction, the
// last stroke drawn only as far as its share says, after the box's corners,
// which keep the frame of the whole.
func reveal(ss []stroke, f float64) Lines {
	var l Lines
	lo, hi := V{math.Inf(1), math.Inf(1), math.Inf(1)}, V{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, s := range ss {
		for _, p := range s {
			for a := range 3 {
				lo[a], hi[a] = min(lo[a], p[a]), max(hi[a], p[a])
			}
		}
	}
	if len(ss) > 0 {
		l.P = append(l.P, lo, hi, V{lo[0], hi[1], lo[2]}, V{hi[0], lo[1], hi[2]})
	}
	want := f * float64(len(ss))
	for i, s := range ss {
		part := min(want-float64(i), 1)
		if part <= 0 {
			break
		}
		n := len(s) - 1
		upto := int(math.Ceil(part * float64(n)))
		base := len(l.P)
		l.P = append(l.P, s[0])
		for k := 1; k <= upto; k++ {
			p := s[k]
			if float64(k) > part*float64(n) { // the last piece, cut short
				t := part*float64(n) - float64(k-1)
				p = add(s[k-1], scale(V{s[k][0] - s[k-1][0], s[k][1] - s[k-1][1], s[k][2] - s[k-1][2]}, t))
			}
			l.P = append(l.P, p)
			l.E = append(l.E, [2]int{base + k - 1, base + k})
		}
	}
	return l
}
