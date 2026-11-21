package chester

import (
	"math"
	"strconv"
)

// The model: FORM picks one of Chester's demonstrations, MORPH runs its
// transformation and DETAIL adds what he finds inside or around it. MORPH
// runs 0..2 with 1 the form as he shows it; DETAIL runs 0..MaxDetail.

// Forms, in the order of the FORM knob. New ones go on the end, so a link
// made earlier still lands on its form.
const (
	FormSolid  = iota // tetrahedron → chestahedron → cube
	FormStar          // a pentagram folded up beside the exact solid
	FormInner         // the band, the hexagrams, the cube, the merkaba, the core
	FormSphere        // the sphere, the seed and flower of life, the cube, the core
	FormBell          // the solid spun: the bell
	FormAir           // the solid turned inside out
	FormDeca          // the decatria: the corners cut, his "dualization"
	FormHeart         // nested cones, the fibers' angle turning through them
	FormSpin          // the tetrahedron turned in the cube: fire in earth
	FormFirst         // his first seven-sided form: a tetrahedron in four pieces
	FormRoll          // the paper disk rolled into the heart's cone
	FormFire          // the double heart: fire, the solid's movement in time
	FormBeat          // the wire solid split, its halves turned both ways
	FormBubble        // the soap bubble in the wire solid, and the ventricles
	NumForms
)

// FormNames label the FORM knob.
var FormNames = []string{"solid", "star", "inner", "sphere", "bell", "air", "deca", "heart", "spin", "first", "roll", "fire", "beat", "bubble"}

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
	return modelOf(p, sphereC, sphereR)
}

func modelOf(p Poly, c V, scale float64) Poly {
	out := Poly{F: p.F}
	for _, v := range p.V {
		out.V = append(out.V, toModel(v, c, scale))
	}
	return out
}

// cubeModel puts the unit cube's center at the origin and its corners on
// the unit sphere: the frame of the forms drawn in the cube.
func cubeModel(p Poly) Poly { return modelOf(p, V{0.5, 0.5, 0.5}, math.Sqrt(3)/2) }

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
	case FormSpin:
		spin(&l, morph, d)
	case FormFirst:
		first(&l, morph, d)
	case FormRoll:
		roll(&l, morph, d)
	case FormFire:
		fire(&l, morph, d)
	case FormBeat:
		beat(&l, morph, d)
	case FormBubble:
		bubble(&l, morph, d)
	default:
		l.poly(solidModel(Family(morph)))
		if d >= 2 { // the cube it is sliced from
			l.poly(solidModel(Family(2)))
		}
		if d >= 3 { // the cube it fits: peak in a corner, base on three edges
			l.poly(solidModel(EarthCube()))
		}
	}
	return l
}

// EarthCube is the cube the chestahedron fits in (the lecture's earth study;
// the book: "it fits exactly only when its axis is along the space
// diagonal"): [0,c]³, the peak in its corner (c,c,c), the base corners on
// the three edges from the opposite corner, a distance 1 along them.
func EarthCube() Poly {
	p := Family(2)
	for i, v := range p.V {
		p.V[i] = v.mul(exactC)
	}
	return p
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

// Hexagrams is the book's construction in the band's plane (pp. 15-16), for
// a band of circumradius R: the six-pointed star on the band's corners, the
// circle inscribed in its middle (radius R/2), which meets the star at six
// points, the star on those points (half the size), and that star's own
// inner hexagon, of circumradius R/(2√3): the outline of the cube standing
// on its point at the solid's center.
func Hexagrams(r float64) (small, cubeOutline float64) { return r / 2, r / (2 * math.Sqrt(3)) }

// inner is the rubber band up the sides; then the book's hexagrams; the
// cube at the center, standing on the book's lower hexagon; the merkaba in
// the cube; the sphere round the cube, Chester's inner core of the Earth.
func inner(l *Lines, m float64, d int) {
	p := solidModel(Chestahedron())
	l.poly(p)
	b := Band(p, bandAt(m))
	l.loop(b[:]...)
	var c V
	for _, v := range b {
		c = c.add(v)
	}
	c = c.mul(1.0 / 6)
	r := 0.0
	for _, v := range b {
		r += math.Hypot(v[0]-c[0], v[2]-c[2]) / 6
	}
	x, y, z := V{1, 0, 0}, V{0, 1, 0}, V{0, 0, 1}
	// Directions of the band's corners from its center, round the axis.
	var dir [6]V
	for i, v := range b {
		dir[i] = V{v[0] - c[0], 0, v[2] - c[2]}.unit()
	}
	at := func(i int, rad float64) V { return c.add(dir[i%6].mul(rad)) }
	if d >= 1 {
		// The star on the band, its inscribed circle, and the half star.
		l.loop(at(0, r), at(2, r), at(4, r))
		l.loop(at(1, r), at(3, r), at(5, r))
		half, _ := Hexagrams(r)
		l.circle(c, x, z, half, 72)
		l.loop(at(0, half), at(2, half), at(4, half))
		l.loop(at(1, half), at(3, half), at(5, half))
	}
	if d < 2 {
		return
	}
	// The cube on its point: its outline is the half star's inner hexagon,
	// whose corners lie between the star's points, 30° round from them.
	_, ro := Hexagrams(r)
	e := ro / math.Sqrt(2.0/3)
	top, bot := c.add(y.mul(e*math.Sqrt(3)/2)), c.sub(y.mul(e*math.Sqrt(3)/2))
	var hi, lo [3]V
	for i := range 3 {
		for k, ring := range [2]*[3]V{&hi, &lo} {
			between := dir[2*i+k].add(dir[(2*i+k+1)%6]).unit()
			h := e * math.Sqrt(3) / 6
			if k == 1 {
				h = -h
			}
			ring[i] = c.add(between.mul(ro)).add(y.mul(h))
		}
	}
	for i := range 3 {
		l.seg(top, hi[i])
		l.seg(bot, lo[i])
		l.seg(hi[i], lo[i])
		l.seg(hi[i], lo[(i+2)%3])
	}
	// The book's lower hexagon, in the plane the cube stands on, and the
	// drop from each corner of the band to it, as the book's photograph
	// draws them.
	low := LowerHexagon(p, bot[1])
	l.loop(low[:]...)
	for _, v := range b {
		near := low[0]
		for _, w := range low {
			if math.Hypot(w[0]-v[0], w[2]-v[2]) < math.Hypot(near[0]-v[0], near[2]-v[2]) {
				near = w
			}
		}
		l.seg(v, near)
	}
	if d >= 3 { // the merkaba: the cube's two tetrahedra
		l.loop(top, lo[0], lo[1])
		l.loop(top, lo[1], lo[2])
		l.loop(top, lo[2], lo[0])
		l.loop(bot, hi[0], hi[1])
		l.loop(bot, hi[1], hi[2])
		l.loop(bot, hi[2], hi[0])
	}
	if d >= 4 { // the sphere round the cube: diameter √3 × its edge
		rs := e * math.Sqrt(3) / 2
		l.circle(c, x, z, rs, 72)
		l.circle(c, x, y, rs, 72)
		l.circle(c, z, y, rs, 72)
	}
}

// InnerCore is the radius of the sphere round the book's central cube, in
// the solid's sphere's radii: Chester's inner core of the Earth, when the
// solid's sphere is the Earth.
func InnerCore() float64 {
	b := Band(solidModel(Chestahedron()), HexagonAt())
	var c V
	for _, v := range b {
		c = c.add(v)
	}
	c = c.mul(1.0 / 6)
	r := math.Hypot(b[0][0]-c[0], b[0][2]-c[2])
	_, ro := Hexagrams(r)
	return ro / math.Sqrt(2.0/3) * math.Sqrt(3) / 2
}

// The cone the book drops into the solid from its peak (26°), and the
// sphere that settles in it at the solid's sphere's center.
const coneDeg = 26.0

// ConeCore is the radius of the sphere held at the center by the 26° cone
// from the peak: sin 13°, the peak being on the unit sphere.
func ConeCore() float64 { return math.Sin(coneDeg / 2 * math.Pi / 180) }

// EarthInnerCore is the Earth's inner core over its radius (1221/6371 km),
// for comparison.
const EarthInnerCore = 1221.0 / 6371.0

// sphere is the solid in its sphere (peak and base corners on it); then the
// seed of life across the equator, the cube whose faces touch the sphere,
// the flower of life in place of the seed, and the 26° cone from the peak
// with the core sphere it holds.
func sphere(l *Lines, d int) {
	ch := solidModel(Chestahedron())
	l.poly(ch)
	x, y, z := V{1, 0, 0}, V{0, 1, 0}, V{0, 0, 1}
	o := V{}
	// The sphere as a globe, so it reads as a sphere from any side: three
	// great circles seen square on are a circle and two lines.
	for i := range 6 { // meridians through the peak
		a := math.Pi * float64(i) / 6
		l.circle(o, V{math.Cos(a), 0, math.Sin(a)}, y, 1, 96)
	}
	for _, lat := range []float64{-60, -30, 0, 30, 60} {
		p := lat * math.Pi / 180
		l.circle(V{0, math.Sin(p), 0}, x, z, math.Cos(p), 96)
	}
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
	if d >= 4 {
		peak := ch.V[vPeak]
		down := peak.mul(-1).unit()
		half := coneDeg / 2 * math.Pi / 180
		length := 2 * peak.len() // through the center and on
		rim := peak.add(down.mul(length))
		rr := length * math.Tan(half)
		var ring []V
		for i := range 48 {
			a := 2 * math.Pi * float64(i) / 48
			v := rim.add(x.mul(rr * math.Cos(a))).add(z.mul(rr * math.Sin(a)))
			ring = append(ring, v)
			if i%6 == 0 {
				l.seg(peak, v)
			}
		}
		l.loop(ring...)
		rc := ConeCore()
		l.circle(o, x, z, rc, 48)
		l.circle(o, x, y, rc, 48)
		l.circle(o, z, y, rc, 48)
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
// threefold symmetry is the whole bell at 1; on from 1 to 2 the bell's three
// cones come in (BellCones), reaching out toward their apexes.
func bell(l *Lines, m float64, d int) {
	p := solidModel(Chestahedron())
	n := BellCopies(d)
	sweep := math.Min(m, 1) * 2 * math.Pi / 3
	for k := range n {
		l.poly(turnPoly(p, sweep*float64(k)/float64(n)))
	}
	if m > 1 {
		bellCones(l, m-1)
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

// deca is Frank's dualization: the corners cut toward the center, the
// decatria at 1 and the dual heptahedron at 2, with the chestahedron round
// it from DETAIL 1.
func deca(l *Lines, m float64, d int) {
	l.poly(solidModel(Dualized(m)))
	if d >= 1 {
		l.poly(solidModel(Chestahedron()))
	}
}

// spin is the tetrahedron turned in the cube through the chestahedron and
// the octahedron to the other tetrahedron; the cube from DETAIL 1, and from
// 2 the solid traced through time, a copy every step of MORPH so far.
func spin(l *Lines, m float64, d int) {
	l.poly(cubeModel(Spin(m)))
	if d >= 1 {
		l.poly(cubeModel(Family(2)))
	}
	if d >= 2 {
		steps := 4 * d
		for k := range steps {
			l.poly(cubeModel(Spin(m * float64(k) / float64(steps))))
		}
	}
}

// first is the four pieces of the tetrahedron, MORPH drawing them apart,
// and from DETAIL 1 the tetrahedron they leave in the middle.
func first(l *Lines, m float64, d int) {
	pieces, inner := FirstPieces(0.5 * m)
	scale := math.Sqrt(3) / 2 * (1 + 0.5*m)
	for _, p := range pieces {
		l.poly(modelOf(p, V{0.5, 0.5, 0.5}, scale))
	}
	if d >= 1 {
		l.poly(modelOf(inner, V{0.5, 0.5, 0.5}, scale))
	}
}

// RollTurns is how many times round the disk is rolled at MORPH m: flat at
// 0, four turns (his cone that fits the solid's center) at 1, eight at 2.
func RollTurns(m float64) float64 {
	if m <= 1 {
		return 1 + 3*m
	}
	return 4 + 4*(m-1)
}

// roll is the paper disk rolled into a cone with lines drawn on it, as the
// heart's fibers: DETAIL 0 the √3 chord (halfway between the center and the
// rim, the line where two circles of a vesica cross), 1 the six of them (a
// hexagram), and from 2 parallel lines across the disk, more with DETAIL.
func roll(l *Lines, m float64, d int) {
	base := len(l.P)
	defer func() { // a disk of radius 1 rolled four times is a narrow cone: draw it larger
		for i := base; i < len(l.P); i++ {
			l.P[i] = l.P[i].mul(1.5)
		}
	}()
	k := RollTurns(m)
	ca := math.Sqrt(1 - 1/(k*k))
	shift := V{0, -0.5 * ca, 0}
	on := func(px, pz float64) V {
		rho := math.Hypot(px, pz)
		phi := math.Mod(math.Atan2(pz, px)+2*math.Pi, 2*math.Pi)
		return Roll(rho, phi, k).add(shift)
	}
	const n = 360
	rim := make([]V, n+1)
	for i := range rim {
		rim[i] = Roll(1, 2*math.Pi*float64(i)/n*(1-1e-9), k).add(shift)
	}
	l.path(rim...)
	l.seg(Roll(0, 0, k).add(shift), Roll(1, 0, k).add(shift)) // the cut
	line := func(x0, z0, x1, z1 float64) {
		pts := make([]V, 121)
		for i := range pts {
			t := float64(i) / 120
			pts[i] = on(x0+(x1-x0)*t, z0+(z1-z0)*t)
		}
		l.path(pts...)
	}
	h := math.Sqrt(3) / 2 // half the √3 chord
	switch {
	case d == 0:
		line(-h, 0.5, h, 0.5)
	case d == 1:
		for i := range 6 {
			a := float64(i) * math.Pi / 3
			s, c := math.Sin(a), math.Cos(a)
			// the chord at distance ½, turned by a
			x0, z0 := -h*c-0.5*s, -h*s+0.5*c
			x1, z1 := h*c-0.5*s, h*s+0.5*c
			line(x0, z0, x1, z1)
		}
	default:
		count := 2*d - 1
		for i := range count {
			zz := -1 + 2*(float64(i)+0.5)/float64(count)
			hw := math.Sqrt(1 - zz*zz)
			line(-hw, zz, hw, zz)
		}
	}
}

// HeartLayers is how many nested cones the heart draws: one, then the
// eight layers of the heart's muscle.
func HeartLayers(d int) int { return min(8, 1+7*d) }

// Faces is the solid a form is built on, in the model frame, for wrapping
// a skin round.
func Faces(form int, morph float64) Poly {
	switch form {
	case FormSolid:
		return solidModel(Family(upTo(morph, 2)))
	case FormDeca:
		return solidModel(Dualized(morph))
	case FormSpin:
		return cubeModel(Spin(morph))
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
	counts := func(p Poly) string {
		return strconv.Itoa(len(p.V)) + "  " + strconv.Itoa(len(p.Edges())) + "  " + strconv.Itoa(len(p.F))
	}
	switch form {
	case FormStar:
		lines[0][1] = "pentagram folded " + f(math.Min(morph, 1)*100, 0) + "%, gap closed " + f(math.Max(0, morph-1)*100, 0) + "%"
		lines[3] = [2]string{"near miss", "a pentagram kite is " + f(StarKiteRatio()*100, 1) + "% of a triangle's area"}
	case FormInner:
		lines[0][1] = "band " + f(bandAt(morph), 4) + " up the sides; regular at " + f(HexagonAt(), 4) + " (1/φ = " + f(1/phi, 4) + ")"
		lines[3] = [2]string{"layers", []string{"band", "+ hexagrams", "+ cube on the lower hexagon (" + f(LowerHexagonRatio()*100, 1) + "% of the upper)", "+ merkaba", "+ core: " + f(InnerCore(), 3) + " (Earth " + f(EarthInnerCore, 3) + ")"}[min(d, 4)]}
	case FormSphere:
		lines[0][1] = "sphere through the peak and base; center " + f(SphereCenterOverBand(), 3) + " of the band's height"
		lines[3] = [2]string{"layers", []string{"sphere", "+ seed of life", "+ cube around", "+ flower of life", "+ 26° cone, core " + f(ConeCore(), 3) + " (Earth " + f(EarthInnerCore, 3) + ")"}[min(d, 4)]}
	case FormBell:
		lines[0][1] = "edges turned " + f(math.Min(morph, 1)*120, 0) + "° about the axis, " + strconv.Itoa(BellCopies(d)) + " copies"
		if morph > 1 {
			c := BellCones()
			deg := func(r float64) string { return f(2*r*180/math.Pi, 2) + "°" }
			lines[3] = [2]string{"cones", "outer = inner " + deg(c.Half) + " (book 26°), wall " + f(c.Wall, 3) + "; tip cone " + deg(c.TipHalf)}
		}
	case FormAir:
		t := morph
		if morph > 1 {
			t = 2 - morph
		}
		lines[0][1] = "inside out " + f(t*100, 0) + "%: face pyramids folded over their faces"
	case FormDeca:
		lines[0][1] = "corners cut toward the center: " + f(morph*50, 0) + "% (decatria at 50%, the dual at 100%)"
		lines[3] = [2]string{"V E F", counts(Dualized(morph))}
	case FormHeart:
		lines[0][1] = strconv.Itoa(HeartLayers(d)) + " cones at 22.5°, fibers ±" + f(math.Min(85, 60*morph), 0) + "°"
	case FormSpin:
		stage := []string{"tetrahedron", "chestahedron", "octahedron", "chestahedron", "tetrahedron"}[int(math.Round(morph*2))]
		lines[0][1] = "the tetrahedron in the cube: " + stage + " (fire in earth)"
		lines[3] = [2]string{"V E F", counts(Spin(morph))}
	case FormFirst:
		lines[0][1] = "his first seven-sided form: four of them and a tetrahedron make one"
		lines[2] = [2]string{"piece", "3 kites (60° 90° 120° 90°) + 4 triangles, 7 corners, 12 edges"}
		lines[3] = [2]string{"apart", f(morph*50, 0) + "%"}
	case FormRoll:
		k := RollTurns(morph)
		lines[0][1] = "disk rolled " + f(k, 2) + " times: cone of " + f(2*math.Asin(1/k)*180/math.Pi, 1) + "°"
		lines[3] = [2]string{"lines", []string{"the √3 chord", "six √3 chords"}[min(d, 1)]}
		if d >= 2 {
			lines[3][1] = strconv.Itoa(2*d-1) + " parallel lines"
		}
	case FormFire:
		lines[0][1] = "one body, a heart at each end pointing in to the waist, the small 1/φ of the big, its lobes turned " + f(upTo(morph, 2)*90, 0) + "°"
		lines[3] = [2]string{"note", "the shape as described: the motion that makes it is not published"}
	case FormBeat:
		lines[0][1] = "the wire solid split in two, the halves turned ±" + f(upTo(morph, 2)*30, 0) + "° (a six-pointed star at ±30°)"
	case FormBubble:
		lines[0][1] = "soap film in the wire solid, rounded: p = " + f(bubblePower(morph), 1) + " (the solid as p grows)"
		if d >= 2 {
			lv, rv, _ := Ventricles(morph)
			lines[3] = [2]string{"sand", "left " + f(lv, 4) + ", right " + f(rv, 4) + ": the same"}
		}
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
