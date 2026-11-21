package chester

import (
	"math"
	"strconv"
)

// The model: FORM picks one of Chester's demonstrations, MORPH runs its
// transformation and DETAIL adds what he finds inside or around it. MORPH
// runs 0..2 with 1 the form as he shows it; DETAIL runs 0..MaxDetail.

// Forms, in the order of the FORM knob.
const (
	FormSolid  = iota // tetrahedron → chestahedron → cube
	FormStar          // a pentagram folded up beside the exact solid
	FormInner         // the rubber band, the hexagram, the cube, the merkaba
	FormSphere        // the sphere, the seed and flower of life, the cube around
	FormBell          // the solid spun: the bell
	FormAir           // the solid turned inside out
	FormDeca          // the dodecahedron sliced: thirteen faces
	FormHeart         // nested 22.5° cones, the fibers' angle turning through them
	NumForms
)

// FormNames label the FORM knob.
var FormNames = []string{"solid", "star", "inner", "sphere", "bell", "air", "deca", "heart"}

// MaxDetail is the DETAIL knob's top.
const MaxDetail = 8

// The model frame: the axis up, the solid's sphere centered at the origin
// with radius 1, a base corner (e3) toward +z.
var (
	ex = V{1, -1, 0}.unit()
	ez = V{-1, -1, 2}.unit()

	sphereC, sphereR = Sphere()
)

// toModel takes a cube-frame point into the model frame about center c,
// divided by scale.
func toModel(p, c V, scale float64) V {
	d := p.sub(c)
	return V{d.dot(ex) / scale, d.dot(axis) / scale, d.dot(ez) / scale}
}

func solidModel(p Poly) Poly {
	out := Poly{F: p.F}
	for _, v := range p.V {
		out.V = append(out.V, toModel(v, sphereC, sphereR))
	}
	return out
}

// upTo holds x to 0..hi.
func upTo(x, hi float64) float64 { return math.Max(0, math.Min(hi, x)) }

// Form is the wireframe of form at morph and detail.
func Form(form int, morph, detail float64) Lines {
	morph = upTo(morph, 2)
	d := int(upTo(math.Round(detail), MaxDetail))
	var l Lines
	switch form {
	case FormStar:
		star(&l, morph, d)
	case FormInner:
		inner(&l, morph, d)
	case FormSphere:
		sphere(&l, d)
	case FormBell:
		bell(&l, morph, d)
	case FormAir:
		air(&l, morph)
	case FormDeca:
		deca(&l, morph, d)
	case FormHeart:
		heart(&l, morph, d)
	default:
		l.poly(solidModel(Family(morph)))
		if d >= 2 { // the cube it is sliced from
			l.poly(solidModel(Family(2)))
		}
	}
	return l
}

// star folds three arms of a pentagram (0..1), then (1..2) closes the
// half-degree gap between it and the exact solid, drawn beside it from
// DETAIL 1.
func star(l *Lines, m float64, d int) {
	exact := solidModel(Chestahedron())
	sc := math.Sqrt2 / phi / sphereR // pentagon side 1 → the solid's triangles
	peakAt := exact.V[vPeak]
	_, corners, points := StarFold(m)
	place := func(v V) V { return peakAt.add(v.mul(sc)) }
	// Where each folded corner goes on the exact solid. In the model frame
	// e1 lies at azimuth -30°, e3 at 90° and e2 at 210°, which are star
	// points 0, 1 and 2; corner j lies between points j-1 and j.
	tipOf := [4]int{vP12, vP13, vP23, vP12}
	ptOf := [3]int{vE1, vE3, vE2}
	if m > 1 {
		g := m - 1
		for j := range corners {
			corners[j] = lerp(place(corners[j]), exact.V[tipOf[j]], g)
		}
		for j := range points {
			points[j] = lerp(place(points[j]), exact.V[ptOf[j]], g)
		}
	} else {
		for j := range corners {
			corners[j] = place(corners[j])
		}
		for j := range points {
			points[j] = place(points[j])
		}
	}
	for j := range points {
		l.loop(peakAt, corners[j], points[j], corners[j+1])
	}
	if m >= 1 { // closed: the side triangles and the base
		l.loop(points[0], corners[1], points[1])
		l.loop(points[1], corners[2], points[2])
		l.loop(points[2], corners[3], points[0])
		l.loop(points[0], points[1], points[2])
	}
	if d >= 1 {
		l.poly(exact)
	}
}

// bandAt maps MORPH onto the band's place up the sides: 0 the base, 1 the
// regular hexagon, 2 the tips.
func bandAt(m float64) float64 {
	h := HexagonAt()
	if m <= 1 {
		return m * h
	}
	return h + (m-1)*(1-h)
}

// inner is the rubber band up the sides; then the hexagram its corners
// make, the cube that hexagram is the outline of seen down its diagonal,
// and the two tetrahedra (the merkaba) in that cube.
func inner(l *Lines, m float64, d int) {
	p := solidModel(Chestahedron())
	l.poly(p)
	b := Band(p, bandAt(m))
	l.loop(b[:]...)
	if d >= 1 {
		l.loop(b[0], b[2], b[4])
		l.loop(b[1], b[3], b[5])
	}
	if d < 2 {
		return
	}
	var c V
	for _, v := range b {
		c = c.add(v)
	}
	c = c.mul(1.0 / 6)
	r := 0.0
	for _, v := range b {
		r += math.Hypot(v[0]-c[0], v[2]-c[2]) / 6
	}
	e := r / math.Sqrt(2.0/3) // the cube's edge, its hexagon outline r
	up := V{0, 1, 0}
	top, bot := c.add(up.mul(e*math.Sqrt(3)/2)), c.sub(up.mul(e*math.Sqrt(3)/2))
	var hi, lo [3]V
	for i := range 3 {
		for k, ring := range [2]*[3]V{&hi, &lo} {
			v := b[2*i+k]
			dir := V{v[0] - c[0], 0, v[2] - c[2]}.unit()
			h := e * math.Sqrt(3) / 6
			if k == 1 {
				h = -h
			}
			ring[i] = c.add(dir.mul(r)).add(up.mul(h))
		}
	}
	for i := range 3 {
		l.seg(top, hi[i])
		l.seg(bot, lo[i])
		l.seg(hi[i], lo[i])
		l.seg(hi[i], lo[(i+2)%3])
	}
	if d >= 3 {
		l.loop(top, lo[0], lo[1])
		l.loop(top, lo[1], lo[2])
		l.loop(top, lo[2], lo[0])
		l.loop(bot, hi[0], hi[1])
		l.loop(bot, hi[1], hi[2])
		l.loop(bot, hi[2], hi[0])
	}
}

// sphere is the solid in its sphere (peak and base corners on it); then the
// seed of life across the equator, the cube whose faces touch the sphere,
// and the flower of life in place of the seed.
func sphere(l *Lines, d int) {
	l.poly(solidModel(Chestahedron()))
	x, y, z := V{1, 0, 0}, V{0, 1, 0}, V{0, 0, 1}
	o := V{}
	l.circle(o, x, z, 1, 96)
	l.circle(o, x, y, 1, 96)
	l.circle(o, z, y, 1, 96)
	if d >= 1 {
		r, rings := 0.5, 1 // the seed: seven circles, the outer one the equator
		if d >= 3 {
			r, rings = 1.0/3, 2 // the flower: nineteen
		}
		for _, c := range hexLattice(rings) {
			l.circle(V{c[0] * r, 0, c[1] * r}, x, z, r, 64)
		}
	}
	if d >= 2 { // the cube round the sphere, a body diagonal upright
		e := 2.0
		cube := Family(2)
		for i, v := range cube.V {
			cube.V[i] = toModel(v, V{0.5, 0.5, 0.5}, 1).mul(e)
		}
		l.poly(cube)
	}
}

// hexLattice is the centers of the circles of the seed (rings 1) or the
// flower (rings 2) of life, in units of the radius.
func hexLattice(rings int) [][2]float64 {
	out := [][2]float64{{0, 0}}
	for k := 1; k <= rings; k++ {
		for side := range 6 {
			a0 := float64(side) * math.Pi / 3
			a1 := a0 + math.Pi/3
			p0 := [2]float64{float64(k) * math.Cos(a0), float64(k) * math.Sin(a0)}
			p1 := [2]float64{float64(k) * math.Cos(a1), float64(k) * math.Sin(a1)}
			for j := range k {
				t := float64(j) / float64(k)
				out = append(out, [2]float64{p0[0] + (p1[0]-p0[0])*t, p0[1] + (p1[1]-p0[1])*t})
			}
		}
	}
	return out
}

// BellCopies is how many turned copies of the edges the bell draws.
func BellCopies(d int) int { return 12 * (d + 2) }

// bell is the solid's edges turned about its axis: each edge that does not
// cross the axis sweeps a hyperboloid, and together they outline a cone
// above a flare. MORPH turns them through m × 120°, which by the solid's
// threefold symmetry is the whole bell at 1.
func bell(l *Lines, m float64, d int) {
	p := solidModel(Chestahedron())
	n := BellCopies(d)
	sweep := m * 2 * math.Pi / 3
	for k := range n {
		a := sweep * float64(k) / float64(n)
		s, c := math.Sin(a), math.Cos(a)
		q := Poly{F: p.F}
		for _, v := range p.V {
			q.V = append(q.V, V{c*v[0] - s*v[2], v[1], s*v[0] + c*v[2]})
		}
		l.poly(q)
	}
}

// air is the solid turned inside out: each face's pyramid to the center
// folded out over its face, 0..1, and back in again 1..2.
func air(l *Lines, m float64) {
	t := m
	if m > 1 {
		t = 2 - m
	}
	p := solidModel(Chestahedron())
	l.poly(p)
	for i, apex := range Everted(p, t) {
		for _, v := range p.F[i] {
			l.seg(apex, p.V[v])
		}
	}
}

// decaScale puts the dodecahedron's circumsphere (radius √3) at 1.
var decaScale = math.Sqrt(3)

// deca slices the dodecahedron's corner off (0..1); from DETAIL 1 the
// chestahedron stands inside it on the slice, and from 2 the cube the
// dodecahedron is built round.
func deca(l *Lines, m float64, d int) {
	p, tri := Decatriahedron(math.Min(m, 1))
	model := func(v V) V { return toModel(v, V{}, decaScale) }
	q := Poly{F: p.F}
	for _, v := range p.V {
		q.V = append(q.V, model(v))
	}
	l.poly(q)
	if d >= 1 {
		ch := ChestOnTriangle(p.V[tri[0]], p.V[tri[1]], p.V[tri[2]])
		for i, v := range ch.V {
			ch.V[i] = model(v)
		}
		l.poly(ch)
	}
	if d >= 2 {
		cube := Family(2)
		for i, v := range cube.V {
			cube.V[i] = model(v.mul(2).sub(V{1, 1, 1}))
		}
		l.poly(cube)
	}
}

// ChestOnTriangle is the chestahedron standing on an equilateral triangle
// whose plane is square to the axis (1,1,1), its peak up the axis.
func ChestOnTriangle(a, b, c V) Poly {
	ch := Chestahedron()
	s := a.sub(b).len() / math.Sqrt2
	// The solid's base centroid is (1,1,1)/3 and the triangle's is m, both
	// on the axis: scale about the one, turn about the axis until e1 points
	// where a does, and move it to the other. One corner suffices, since a
	// turn of the threefold solid that lands e1 on a lands e2, e3 on b, c.
	m := a.add(b).add(c).mul(1.0 / 3)
	base := V{1, 1, 1}.mul(1.0 / 3)
	from, to := V{1, 0, 0}.sub(base), a.sub(m)
	ang := math.Atan2(cross(from, to).dot(axis), from.dot(to))
	for i, v := range ch.V {
		ch.V[i] = m.add(turn(v.sub(base), ang).mul(s))
	}
	return ch
}

// turn rotates v by ang about the axis (Rodrigues).
func turn(v V, ang float64) V {
	s, c := math.Sin(ang), math.Cos(ang)
	return v.mul(c).add(cross(axis, v).mul(s)).add(axis.mul(axis.dot(v) * (1 - c)))
}

// HeartLayers is how many nested cones the heart draws: one, then the
// eight layers of the heart's muscle.
func HeartLayers(d int) int { return min(8, 1+7*d) }

// Faces is the solid a form is built on, in the model frame, for wrapping
// a skin round: the family at morph, the decatriahedron, or the
// chestahedron itself.
func Faces(form int, morph float64) Poly {
	switch form {
	case FormSolid:
		return solidModel(Family(upTo(morph, 2)))
	case FormDeca:
		p, _ := Decatriahedron(upTo(morph, 1))
		for i, v := range p.V {
			p.V[i] = toModel(v, V{}, decaScale)
		}
		return p
	}
	return solidModel(Chestahedron())
}

// heart is nested cones of half-angle 22.5°, apex down, with fibers wound
// on each at a helix angle that turns from +A on the outside through flat to
// -A inside, A = 60° × MORPH (anatomy gives about 60°). A fiber at a fixed
// angle to a cone's circles is a loxodrome: it turns ln(ℓ)/(sin α tan β).
func heart(l *Lines, m float64, d int) {
	const (
		alpha  = 22.5 * math.Pi / 180
		fibers = 12
		steps  = 96
	)
	amp := math.Min(85, 60*m) * math.Pi / 180
	layers := HeartLayers(d)
	for k := range layers {
		apex := V{0, -1 + 0.09*float64(k), 0}
		lmax := (2 - 0.09*float64(k)) / math.Cos(alpha)
		l0 := 0.25 * lmax
		beta := amp
		if layers > 1 {
			beta = amp * (1 - 2*float64(k)/float64(layers-1))
		}
		at := func(ell, th float64) V {
			r := ell * math.Sin(alpha)
			return apex.add(V{r * math.Cos(th), ell * math.Cos(alpha), r * math.Sin(th)})
		}
		var rim []V
		for i := range 72 {
			rim = append(rim, at(lmax, 2*math.Pi*float64(i)/72))
		}
		l.loop(rim...)
		for f := range fibers {
			th0 := 2*math.Pi*float64(f)/fibers + 0.2*float64(k)
			if math.Abs(beta) < 10*math.Pi/180 { // near flat: the circles themselves
				ell := l0 + (lmax-l0)*(float64(f)+0.5)/fibers
				var ring []V
				for i := range 72 {
					ring = append(ring, at(ell, 2*math.Pi*float64(i)/72))
				}
				l.loop(ring...)
				continue
			}
			pts := make([]V, steps+1)
			for i := range pts {
				ell := l0 + (lmax-l0)*float64(i)/steps
				pts[i] = at(ell, th0+math.Log(ell/l0)/(math.Sin(alpha)*math.Tan(beta)))
			}
			l.path(pts...)
		}
	}
}

// Info is the Equation module's four lines for a form, label and text.
func Info(form int, morph, detail float64) [4][2]string {
	f := func(x float64, n int) string { return strconv.FormatFloat(x, 'f', n, 64) }
	kb, kp, kt := KiteAngles()
	a, b, c := Exact()
	d := int(upTo(math.Round(detail), MaxDetail))
	lines := [4][2]string{
		{"form", ""},
		{"exact", "tips (a,a,b) a=" + f(a, 6) + " b=" + f(b, 6) + ", peak c=" + f(c, 6)},
		{"kite", f(kb, 2) + "° " + f(kp, 2) + "° " + f(kt, 2) + "°  (pentagram 36 72 126)"},
		{"V E F", "7  12  7 · 4 triangles + 3 kites, equal areas"},
	}
	switch form {
	case FormStar:
		lines[0][1] = "pentagram folded " + f(math.Min(morph, 1)*100, 0) + "%, gap closed " + f(math.Max(0, morph-1)*100, 0) + "%"
		lines[3] = [2]string{"near miss", "a pentagram kite is " + f(StarKiteRatio()*100, 1) + "% of a triangle's area"}
	case FormInner:
		lines[0][1] = "band " + f(bandAt(morph), 4) + " up the sides; regular at " + f(HexagonAt(), 4) + " (1/φ = " + f(1/phi, 4) + ")"
		lines[3] = [2]string{"layers", []string{"band", "+ hexagram", "+ cube", "+ merkaba"}[min(d, 3)]}
	case FormSphere:
		lines[0][1] = "sphere through the peak and base; center " + f(SphereCenterOverBand(), 3) + " of the band's height"
		lines[3] = [2]string{"layers", []string{"sphere", "+ seed of life", "+ cube around", "+ flower of life"}[min(d, 3)]}
	case FormBell:
		lines[0][1] = "edges turned " + f(morph*120, 0) + "° about the axis, " + strconv.Itoa(BellCopies(d)) + " copies"
	case FormAir:
		t := morph
		if morph > 1 {
			t = 2 - morph
		}
		lines[0][1] = "inside out " + f(t*100, 0) + "%: face pyramids folded over their faces"
	case FormDeca:
		lines[0][1] = "dodecahedron, corner sliced " + f(math.Min(morph, 1)*100, 0) + "%"
		lines[3] = [2]string{"V E F", "19  30  13 · 9 pentagons, 3 quads, 1 triangle"}
	case FormHeart:
		lines[0][1] = strconv.Itoa(HeartLayers(d)) + " cones at 22.5°, fibers ±" + f(math.Min(85, 60*morph), 0) + "°"
	default:
		switch {
		case morph < 1:
			lines[0][1] = "tetrahedron opening: " + f(morph*100, 0) + "% to the chestahedron"
		case morph > 1:
			lines[0][1] = "chestahedron closing to the cube: " + f((morph-1)*100, 0) + "%"
		default:
			lines[0][1] = "the chestahedron"
		}
	}
	return lines
}

// StarKiteRatio is a pentagram kite's area over the triangle it is folded
// beside: the near miss in one number.
func StarKiteRatio() float64 {
	kite := pentR*pentR*math.Sin(2*math.Pi/5)/2 + math.Sqrt(phi*phi-0.25)/2
	tri := math.Sqrt(3) / 4 * phi * phi
	return kite / tri
}

// SphereCenterOverBand is the sphere's center's height over the base, over
// the regular band's: Chester puts the band on the equator; it is near.
func SphereCenterOverBand() float64 {
	p := Chestahedron()
	base := p.V[vE1].dot(axis)
	b := Band(p, HexagonAt())
	return (sphereC.dot(axis) - base) / (b[0].dot(axis) - base)
}
