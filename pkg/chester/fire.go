package chester

import "math"

// The rest of what the lecture and the book show: the book's lower hexagon,
// the three cones of the bell, the fire form (the double heart), the beat
// (the wire solid split and turned both ways) and the soap bubble with the
// ventricles.

// meet is the point on three planes; they must not share a line.
func meet(a, b, c plane) V {
	det := a.n.dot(cross(b.n, c.n))
	return cross(b.n, c.n).mul(a.d).add(cross(c.n, a.n).mul(b.d)).add(cross(a.n, b.n).mul(c.d)).mul(1 / det)
}

// rotY turns v about the axis (the model frame's y) by a.
func rotY(v V, a float64) V {
	s, c := math.Sin(a), math.Cos(a)
	return V{c*v[0] - s*v[2], v[1], s*v[0] + c*v[2]}
}

func turnPoly(p Poly, a float64) Poly {
	q := Poly{F: p.F, V: make([]V, len(p.V))}
	for i, v := range p.V {
		q.V[i] = rotY(v, a)
	}
	return q
}

// The lower hexagon.

// LowerHexagon is the book's second hexagon (p. 16), at height y in the
// model frame: parallel to the base, its corners on the planes of the three
// side triangles and three of its edges inside the solid. Those planes cut
// the level in a triangle; the regular hexagon with two corners on each side
// cuts the triangle's corners off at a third.
func LowerHexagon(p Poly, y float64) [6]V {
	ps := facePlanes(p)
	side := ps[3:6] // the three side triangles, in F's order
	flat := plane{V{0, 1, 0}, y}
	var corner [3]V
	for i := range 3 {
		corner[i] = meet(side[i], side[(i+1)%3], flat)
	}
	var out [6]V
	for i := range 3 {
		a, b := corner[i], corner[(i+1)%3]
		out[2*i] = lerp(a, b, 1.0/3)
		out[2*i+1] = lerp(a, b, 2.0/3)
	}
	return out
}

// LowerHexagonY is the lower hexagon's height. The book gives none, but its
// photograph has the cube at the center (the one round which the inner core
// is drawn) standing on its point in the hexagon's plane, so that is where
// it is taken: the regular band's height less the cube's half diagonal.
func LowerHexagonY() float64 {
	return Band(solidModel(Chestahedron()), HexagonAt())[0][1] - InnerCore()
}

// LowerHexagonRatio is the lower hexagon's size over the upper's: the book's
// "almost exactly the same size (the lower is slightly smaller)".
func LowerHexagonRatio() float64 {
	p := solidModel(Chestahedron())
	b := Band(p, HexagonAt())
	lo := LowerHexagon(p, LowerHexagonY())
	return math.Hypot(lo[0][0], lo[0][2]) / math.Hypot(b[0][0], b[0][2])
}

// The bell's cones.

// Cones are the three cones the spun solid makes (book p. 13: "one cone
// fills the interior space, one hugs the outside, and a last cone is made
// from the projection of the tip of the bell"). Between the base and the
// tips the six side edges all sweep one waisted surface. The outer cone
// runs through its two rims; the inner is parallel to it and touches the
// surface; the book gives both as 26° and the wall between them as the left
// ventricle's. The edges from the tips to the peak sweep an exact cone.
type Cones struct {
	Half    float64 // the outer and inner cones' half-angle, radians
	Wall    float64 // how far apart they are, square to their sides
	TipHalf float64 // the peak cone's half-angle

	base, tip, touch [2]float64 // (y, r): the two rims, and where the inner cone touches
	peak             float64    // the peak's height
}

// BellCones measures the cones, in the model frame.
func BellCones() Cones {
	p := solidModel(Chestahedron())
	e, t := p.V[vE1], p.V[vP12]
	rad := func(v V) float64 { return math.Hypot(v[0], v[2]) }
	c := Cones{base: [2]float64{e[1], rad(e)}, tip: [2]float64{t[1], rad(t)}, peak: p.V[vPeak][1]}
	slope := c.slope()
	c.Half = math.Atan(-slope)
	// Along the edge the swept radius is a hyperbola in y; the inner cone
	// touches it where its slope is the chord's.
	d := t.sub(e)
	f := func(s float64) float64 {
		q := e.add(d.mul(s))
		return (q[0]*d[0]+q[2]*d[2])/(rad(q)*d[1]) - slope
	}
	lo, hi := 0.0, 1.0
	for range 100 {
		m := (lo + hi) / 2
		if (f(m) < 0) == (f(lo) < 0) {
			lo = m
		} else {
			hi = m
		}
	}
	q := e.add(d.mul((lo + hi) / 2))
	c.touch = [2]float64{q[1], rad(q)}
	c.Wall = (c.outer(q[1]) - c.touch[1]) * math.Cos(c.Half)
	c.TipHalf = math.Atan2(c.tip[1], c.peak-c.tip[0])
	return c
}

// slope is the outer cone's dr/dy: negative, the rims narrowing upward.
func (c Cones) slope() float64 { return (c.tip[1] - c.base[1]) / (c.tip[0] - c.base[0]) }

func (c Cones) outer(y float64) float64 { return c.base[1] + c.slope()*(y-c.base[0]) }
func (c Cones) inner(y float64) float64 { return c.touch[1] + c.slope()*(y-c.touch[0]) }
func (c Cones) tipCone(y float64) float64 {
	return (c.peak - y) * math.Tan(c.TipHalf)
}

// cone draws a cone of radius r(y) about the axis from y0 to y1: its two
// rings and twelve of its lines.
func (l *Lines) cone(r func(float64) float64, y0, y1 float64) {
	x, z := V{1, 0, 0}, V{0, 0, 1}
	for _, y := range []float64{y0, y1} {
		if rr := r(y); rr > 1e-9 {
			l.circle(V{0, y, 0}, x, z, rr, 72)
		}
	}
	for i := range 12 {
		a := 2 * math.Pi * float64(i) / 12
		on := func(y float64) V {
			rr := math.Max(0, r(y))
			return V{rr * math.Cos(a), y, rr * math.Sin(a)}
		}
		l.seg(on(y0), on(y1))
	}
}

// bellCones draws the three cones, g (0..1) of the way out to their apexes:
// the outer and inner over the flank and on up toward the outer's apex, the
// peak's cone on down past the tips toward the base.
func bellCones(l *Lines, g float64) {
	c := BellCones()
	apex := c.base[0] - c.base[1]/c.slope()
	top := c.tip[0] + g*(apex-c.tip[0])
	l.cone(c.outer, c.base[0], top)
	innerTop := math.Min(top, c.touch[0]-c.touch[1]/c.slope())
	l.cone(c.inner, c.base[0], innerTop)
	l.cone(c.tipCone, c.tip[0]-g*(c.tip[0]-c.base[0]), c.peak)
}

// The fire form.

// The fire form (lecture 32:55): the solid "put into movement so that the
// corners, vertices, turn in space and time", which Frank calls a double
// heart, a valentine, "a little one at the top and at the bottom one is a big
// one". The motion that makes it is not published. What he holds up is one
// carved body with a heart at each end, both pointing in to a thick waist,
// the small one 1/φ of the big (his "two curves with the golden mean") and
// its lobes turned square to the big one's, so that end on the two make a
// cross of unequal arms. Across the axis the body is a circle at the waist
// that parts into two lobes toward either end, each lobe closing round.
const (
	lobeTilt = 22 * math.Pi / 180 // how fast the lobes part
	lobeR    = 0.37               // the big heart's lobes at their widest
	waistR   = 0.20               // the body at the waist
	fireLen  = 1.2                // the big heart's length, the small one's 1/φ of it
)

// waistY puts the waist where the body is centered on the origin.
var waistY = fireLen * (1 - 1/phi) / 2

// lobeWidest is u√(1-u⁴) at its widest, u = 3^(-1/4).
var lobeWidest = math.Pow(3, -0.25) * math.Sqrt(2.0/3)

// fireEnd is one heart of the body, from the waist (u = 0) out to its end
// (u = 1): o the waist's center, a the unit axis outward, w the direction
// its lobes part in and v square to both.
type fireEnd struct {
	o, a, w, v V
	len, r     float64
}

// section is the heart's section u of the way out: two circles of radius
// rho with centers c either side of the axis along w. At the waist they are
// one circle; at the end each closes round to a point.
func (e fireEnd) section(u float64) (c, rho float64) {
	rho = waistR*(1-u)*(1-u) + e.r/lobeWidest*u*math.Sqrt(math.Max(0, 1-u*u*u*u))
	return u * e.len * math.Tan(lobeTilt), rho
}

// at is the point at angle t round lobe k (+1 or -1) of section u.
func (e fireEnd) at(u, k, t float64) V {
	c, rho := e.section(u)
	center := e.o.add(e.a.mul(u * e.len)).add(e.w.mul(k * c))
	return center.add(e.w.mul(rho * math.Cos(t))).add(e.v.mul(rho * math.Sin(t)))
}

// hidden tells whether that point lies inside the other lobe.
func (e fireEnd) hidden(u, k, t float64) bool {
	c, rho := e.section(u)
	return math.Hypot(2*k*c+rho*math.Cos(t), rho*math.Sin(t)) < rho*0.999
}

// fireEnds are the body's two hearts at MORPH m: the big one below, the
// small one above, its lobes turned m × 90° about the axis.
func fireEnds(m float64) (big, small fireEnd) {
	o := V{0, waistY, 0}
	big = fireEnd{o, V{0, -1, 0}, V{1, 0, 0}, V{0, 0, 1}, fireLen, lobeR}
	turn := upTo(m, 2) * math.Pi / 2
	small = fireEnd{o, V{0, 1, 0}, rotY(V{1, 0, 0}, turn), rotY(V{0, 0, 1}, turn), fireLen / phi, lobeR / phi}
	return big, small
}

// clipped draws the polyline vs, leaving out the points hidden marks.
func (l *Lines) clipped(vs []V, hidden []bool) {
	start := -1
	for i := 0; i <= len(vs); i++ {
		if i < len(vs) && !hidden[i] {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= 2 {
			l.path(vs[start:i]...)
		}
		start = -1
	}
}

// fire draws the body: sections across it and lines along each lobe, more of
// both with DETAIL, each lobe left out where it runs inside the other.
func fire(l *Lines, m float64, d int) {
	big, small := fireEnds(m)
	rings, lines := 5+2*d, 6+2*d
	const steps = 48
	for _, e := range []fireEnd{big, small} {
		for _, k := range []float64{1, -1} {
			draw := func(n int, pt func(j int) (u, t float64)) {
				pts, hid := make([]V, n+1), make([]bool, n+1)
				for j := range pts {
					u, t := pt(j)
					pts[j], hid[j] = e.at(u, k, t), e.hidden(u, k, t)
				}
				l.clipped(pts, hid)
			}
			for i := 1; i <= rings; i++ {
				u := float64(i) / float64(rings+1)
				draw(steps, func(j int) (float64, float64) { return u, 2 * math.Pi * float64(j) / steps })
			}
			for i := range lines {
				t := 2 * math.Pi * float64(i) / float64(lines)
				draw(steps, func(j int) (float64, float64) { return float64(j) / steps, t })
			}
		}
	}
	// The waist, where the two hearts meet.
	l.circle(V{0, waistY, 0}, V{1, 0, 0}, V{0, 0, 1}, waistR, 48)
}

// The beat.

// beat is the wire solid split in two (lecture 58:20, "I split all the
// lines in half … the other two are spinning in opposite directions"): the
// halves turned opposite ways, ±30° × MORPH, so that at 1 their bases make a
// six-pointed star and at 2 they are one solid again. DETAIL 1 adds the
// band's star, each of its triangles carried round with a half, and from 2
// the paths the halves have turned through: two bells wound opposite ways.
func beat(l *Lines, m float64, d int) {
	p := solidModel(Chestahedron())
	b := Band(p, HexagonAt())
	for k, sg := range []float64{1, -1} {
		a := sg * upTo(m, 2) * math.Pi / 6
		l.poly(turnPoly(p, a))
		if d >= 1 {
			l.loop(rotY(b[k%6], a), rotY(b[(k+2)%6], a), rotY(b[(k+4)%6], a))
		}
		if d >= 2 {
			steps := 4 * d
			for i := 1; i < steps; i++ {
				l.poly(turnPoly(p, a*float64(i)/float64(steps)))
			}
		}
	}
}

// The bubble and the ventricles.

// The bubble (lecture 60:22): the wire solid dipped in soap holds a
// seven-sided bubble, "and once that soap is broken these edges become
// round". The bubble is the solid's faces softened into one surface: its
// radius toward direction u is 1/‖(n·u/h)₊‖ₚ over the faces (n the face's
// normal, h its distance), which is the solid as p grows and rounder as it
// shrinks. MORPH runs p from the solid (0) through the bubble (1, p = 6) to
// rounder (2).
func bubblePower(m float64) float64 { return 6 / math.Max(upTo(m, 2), 0.08) }

func bubbleR(ps []plane, u V, pw float64) float64 {
	s := 0.0
	for _, q := range ps {
		if x := q.n.dot(u) / q.d; x > 0 {
			s += math.Pow(x, pw)
		}
	}
	return math.Pow(s, -1/pw)
}

func bubbleDir(th, ph float64) V {
	return V{math.Sin(th) * math.Cos(ph), math.Cos(th), math.Sin(th) * math.Sin(ph)}
}

// The right ventricle (lecture 62:22, book p. 13): "the other vortex",
// exactly the same size as the left but upside down, cupping it in a
// crescent; fill each with sand and "it's the same amount of sand". It is
// drawn as a crescent round one side of the bubble, rvHalf either side of
// azimuth rvAt, broad where the bubble comes to its point and running out
// where the bubble is broad (upside down to it), its thickness scaled until
// it holds the bubble's volume.
const (
	rvAt    = math.Pi / 2
	rvHalf  = 100 * math.Pi / 180
	heart36 = 36 * math.Pi / 180 // how the heart sits in the body
)

// rvShape is the crescent's thickness over the bubble's radius, at scale 1.
// The bubble's point is its top (the peak, θ = 0); the crescent is fullest
// toward the top and runs out to nothing at the bottom and at its two edges.
func rvShape(th, ph float64) float64 {
	dp := math.Remainder(ph-rvAt, 2*math.Pi)
	if math.Abs(dp) >= rvHalf {
		return 0
	}
	c := math.Cos(math.Pi / 2 * dp / rvHalf)
	return c * c * math.Sin(th) * (1 + math.Cos(th)) / 2
}

// Ventricles are the bubble's volume (the left ventricle), the crescent's
// (the right) and the crescent's scale that makes them equal, at MORPH m.
func Ventricles(m float64) (left, right, scale float64) {
	ps := facePlanes(solidModel(Chestahedron()))
	pw := bubblePower(m)
	const nt, np = 60, 120
	dth, dph := math.Pi/nt, 2*math.Pi/np
	type cell struct{ w, s float64 }
	var cells []cell
	for i := range nt {
		th := (float64(i) + 0.5) * dth
		for j := range np {
			ph := (float64(j) + 0.5) * dph
			r := bubbleR(ps, bubbleDir(th, ph), pw)
			w := math.Sin(th) * dth * dph / 3 * r * r * r
			left += w
			if s := rvShape(th, ph); s > 0 {
				cells = append(cells, cell{w, s})
			}
		}
	}
	vol := func(t float64) float64 {
		v := 0.0
		for _, c := range cells {
			k := 1 + t*c.s
			v += c.w * (k*k*k - 1)
		}
		return v
	}
	lo, hi := 0.0, 16.0
	for range 80 {
		mid := (lo + hi) / 2
		if vol(mid) < left {
			lo = mid
		} else {
			hi = mid
		}
	}
	scale = (lo + hi) / 2
	return left, vol(scale), scale
}

// bubble draws the bubble; DETAIL 1 adds the wire frame it hangs in, 2 the
// right ventricle cupping it, and 3 tilts the pair 36°, as the heart sits.
func bubble(l *Lines, m float64, d int) {
	base := len(l.P)
	p := solidModel(Chestahedron())
	ps := facePlanes(p)
	pw := bubblePower(m)
	at := func(th, ph, k float64) V {
		u := bubbleDir(th, ph)
		return u.mul(bubbleR(ps, u, pw) * k)
	}
	const rings, lines, steps = 11, 18, 72
	for i := 1; i <= rings; i++ {
		th := math.Pi * float64(i) / (rings + 1)
		pts := make([]V, steps)
		for j := range pts {
			pts[j] = at(th, 2*math.Pi*float64(j)/steps, 1)
		}
		l.loop(pts...)
	}
	for j := range lines {
		ph := 2 * math.Pi * float64(j) / lines
		pts := make([]V, steps+1)
		for i := range pts {
			pts[i] = at(math.Pi*float64(i)/steps, ph, 1)
		}
		l.path(pts...)
	}
	if d >= 1 {
		l.poly(p)
	}
	if d >= 2 {
		_, _, sc := Ventricles(m)
		k := func(th, ph float64) float64 { return 1 + sc*rvShape(th, ph) }
		const across = 48
		for i := 1; i <= rings; i++ {
			th := math.Pi * float64(i) / (rings + 1)
			pts := make([]V, across+1)
			for j := range pts {
				ph := rvAt - rvHalf + 2*rvHalf*float64(j)/across
				pts[j] = at(th, ph, k(th, ph))
			}
			l.path(pts...)
		}
		for j := 1; j < 10; j++ {
			ph := rvAt - rvHalf + 2*rvHalf*float64(j)/10
			pts := make([]V, steps+1)
			for i := range pts {
				th := math.Pi * float64(i) / steps
				pts[i] = at(th, ph, k(th, ph))
			}
			l.path(pts...)
		}
	}
	if d >= 3 {
		s, c := math.Sin(heart36), math.Cos(heart36)
		for i := base; i < len(l.P); i++ {
			v := l.P[i]
			l.P[i] = V{v[0], c*v[1] - s*v[2], s*v[1] + c*v[2]}
		}
	}
}
