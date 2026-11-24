// Package measure is the Measure module's arithmetic: finding the point a
// click means on a figure of lines, and the distances and angles between
// such points, named when they are numbers worth naming.
//
// A click is aimed at a point that matters — a corner, a place where two
// lines cross, the center a circle was drawn about — and it is never quite
// on it. Snap takes the click to the nearest such point on screen, preferring
// the exact ones a figure can name (its centers, and where its true circles
// and lines meet, solved exactly) to the ones read off its line segments.
//
// It is pure: the page projects points to the screen and hands Snap the
// projection, and a native rack can do the same.
package measure

import "math"

// P is a point in the figure's own space.
type P [3]float64

func (a P) sub(b P) P                { return P{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a P) add(b P) P                { return P{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a P) mul(s float64) P          { return P{a[0] * s, a[1] * s, a[2] * s} }
func (a P) dot(b P) float64          { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a P) norm() float64            { return math.Sqrt(a.dot(a)) }
func (a P) near(b P, e float64) bool { return a.sub(b).norm() <= e }

// Dist is the distance between two points.
func Dist(a, b P) float64 { return a.sub(b).norm() }

// Angle is the angle at b between a and c, in degrees.
func Angle(a, b, c P) float64 {
	u, v := a.sub(b), c.sub(b)
	nu, nv := u.norm(), v.norm()
	if nu == 0 || nv == 0 {
		return 0
	}
	return math.Acos(max(-1, min(1, u.dot(v)/(nu*nv)))) * 180 / math.Pi
}

// Circle is a circle in a plane of constant z: center and radius.
type Circle struct {
	C P
	R float64
}

// Kind is what a snapped point is.
type Kind int

// The kinds, most exact first: a figure's own named point, where its true
// circles and lines meet, a corner of its segments, where two segments cross.
const (
	Mark Kind = iota
	Meet
	Corner
	Crossing
)

// KindNames are the kinds as the module says them.
var KindNames = []string{"center", "meet", "corner", "cross"}

// Scene is a figure as measurable: its drawn segments, and what it can say
// exactly about itself — named points, true circles and true lines.
type Scene struct {
	Segs    [][2]P
	Marks   []P
	Circles []Circle
	Lines   [][2]P

	corners []P
	// circleDrawn and lineDrawn are, for curves detect read off the
	// segments, the pieces of each that are drawn.
	circleDrawn [][][]P
	lineDrawn   [][][]P
	meets       []P
	scale       float64
}

// maxExactLines bounds solving every pair of true lines (and circles): past
// it, a figure's crossings are found near the click from its segments only.
const maxExactLines = 400

// Prepare finds the scene's corners and solves where its true circles and
// lines meet. Call it once per figure, before Snap.
func (s *Scene) Prepare() {
	s.scale = s.extent()
	s.corners = corners(s.Segs, s.scale*1e-7)
	s.meets = nil
	if len(s.Circles) == 0 && len(s.Lines) == 0 {
		s.detect()
	}
	if len(s.Circles)+len(s.Lines) > maxExactLines {
		return
	}
	eps := s.scale * 1e-9
	// A meeting point counts only where something is drawn: an arc is
	// solved as its whole circle, and its missing part must add nothing. The
	// tolerance allows for a circle drawn in pieces, whose chords fall short
	// of it by up to their sagitta.
	drawnTol := s.scale * 2e-3
	circleSegs := make([][][2]P, len(s.Circles))
	for i := range s.Circles {
		circleSegs[i] = drawnSegs(s.circleDrawn, i)
	}
	lineSegs := make([][][2]P, len(s.Lines))
	for i := range s.Lines {
		lineSegs[i] = drawnSegs(s.lineDrawn, i)
	}
	// on reports whether p is on what is drawn of a curve: its own pieces
	// when they are known, else anything drawn.
	on := func(p P, drawn [][2]P) bool {
		if drawn == nil {
			drawn = s.Segs
		}
		return len(drawn) == 0 || onDrawing(p, drawn, drawnTol)
	}
	add := func(p P, a, b [][2]P) {
		if !on(p, a) || !on(p, b) {
			return
		}
		for _, q := range s.meets {
			if q.near(p, s.scale*1e-7) {
				return
			}
		}
		s.meets = append(s.meets, p)
	}
	for i, a := range s.Circles {
		for j := i + 1; j < len(s.Circles); j++ {
			for _, p := range circleCircle(a, s.Circles[j], eps) {
				add(p, circleSegs[i], circleSegs[j])
			}
		}
		for j, l := range s.Lines {
			for _, p := range circleLine(a, l, eps) {
				add(p, circleSegs[i], lineSegs[j])
			}
		}
	}
	for i, a := range s.Lines {
		for j := i + 1; j < len(s.Lines); j++ {
			if p, ok := segmentMeet(a, s.Lines[j], eps); ok {
				add(p, lineSegs[i], lineSegs[j])
			}
		}
	}
}

// extent is the size of the scene, for tolerances.
func (s *Scene) extent() float64 {
	r := 0.0
	for _, g := range s.Segs {
		r = max(r, g[0].norm(), g[1].norm())
	}
	if r == 0 {
		return 1
	}
	return r
}

// Snapped is the point a click meant.
type Snapped struct {
	P    P
	Kind Kind
	// Px is how far from the click it is on screen, in pixels.
	Px float64
}

// Project maps a point to the screen, and false when it is behind the eye.
type Project func(P) (x, y float64, ok bool)

// Snap is the point within radius pixels of the click at (x, y) that the
// click means: the nearest on screen, an exact kind winning over a near one
// within a few pixels of it.
func (s *Scene) Snap(x, y float64, project Project, radius float64) (Snapped, bool) {
	best := Snapped{Px: math.Inf(1)}
	found := false
	consider := func(p P, k Kind) {
		sx, sy, ok := project(p)
		if !ok {
			return
		}
		d := math.Hypot(sx-x, sy-y)
		if d > radius {
			return
		}
		// A more exact kind wins unless it is clearly further away.
		const slack = 4
		if !found || d < best.Px-slack || (d <= best.Px+slack && k < best.Kind) {
			best, found = Snapped{p, k, d}, true
		}
	}
	for _, p := range s.Marks {
		consider(p, Mark)
	}
	for _, p := range s.meets {
		consider(p, Meet)
	}
	for _, p := range s.corners {
		consider(p, Corner)
	}
	for _, p := range s.crossingsNear(x, y, project, radius) {
		consider(p, Crossing)
	}
	return best, found
}

// crossingsNear are the points where two of the scene's segments that pass
// near the click meet in space (two lines crossing on screen and passing
// each other at different depths do not count).
func (s *Scene) crossingsNear(x, y float64, project Project, radius float64) []P {
	var near [][2]P
	for _, g := range s.Segs {
		ax, ay, ok1 := project(g[0])
		bx, by, ok2 := project(g[1])
		if ok1 && ok2 && pointSegment2(x, y, ax, ay, bx, by) <= radius {
			near = append(near, g)
			if len(near) > 300 {
				break
			}
		}
	}
	eps := s.scale * 1e-6
	var out []P
	for i, a := range near {
		for _, b := range near[i+1:] {
			if p, ok := segmentMeet(a, b, eps); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// pointSegment2 is the screen distance from (x, y) to segment a–b.
func pointSegment2(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l := dx*dx + dy*dy
	t := 0.0
	if l > 0 {
		t = max(0, min(1, ((x-ax)*dx+(y-ay)*dy)/l))
	}
	return math.Hypot(x-ax-t*dx, y-ay-t*dy)
}

// segmentMeet is where segments a and b meet, if they do: their closest
// points within eps of each other, each within its segment, and the two not
// sharing an end (a polyline's own joints are corners, not crossings).
func segmentMeet(a, b [2]P, eps float64) (P, bool) {
	d1, d2 := a[1].sub(a[0]), b[1].sub(b[0])
	r := a[0].sub(b[0])
	aa, bb, ab := d1.dot(d1), d2.dot(d2), d1.dot(d2)
	den := aa*bb - ab*ab
	if aa == 0 || bb == 0 || den <= 1e-18*aa*bb {
		return P{}, false // parallel
	}
	c, f := d1.dot(r), d2.dot(r)
	s := (ab*f - bb*c) / den
	t := (aa*f - ab*c) / den
	const in = 1e-9
	if s < -in || s > 1+in || t < -in || t > 1+in {
		return P{}, false
	}
	p, q := a[0].add(d1.mul(s)), b[0].add(d2.mul(t))
	if !p.near(q, eps) {
		return P{}, false
	}
	m := p.add(q).mul(0.5)
	for _, e := range []P{a[0], a[1]} {
		if (e.near(b[0], eps) || e.near(b[1], eps)) && e.near(m, eps) {
			return P{}, false
		}
	}
	return m, true
}

// circleCircle is where two coplanar circles meet.
func circleCircle(a, b Circle, eps float64) []P {
	if math.Abs(a.C[2]-b.C[2]) > eps {
		return nil
	}
	dx, dy := b.C[0]-a.C[0], b.C[1]-a.C[1]
	d := math.Hypot(dx, dy)
	if d < eps || d > a.R+b.R+eps || d < math.Abs(a.R-b.R)-eps {
		return nil
	}
	l := (a.R*a.R - b.R*b.R + d*d) / (2 * d)
	h := math.Sqrt(max(0, a.R*a.R-l*l))
	mx, my := a.C[0]+l*dx/d, a.C[1]+l*dy/d
	p := P{mx - h*dy/d, my + h*dx/d, a.C[2]}
	if h < eps {
		return []P{p}
	}
	return []P{p, {mx + h*dy/d, my - h*dx/d, a.C[2]}}
}

// circleLine is where a circle meets a segment lying in its plane.
func circleLine(c Circle, l [2]P, eps float64) []P {
	if math.Abs(l[0][2]-c.C[2]) > eps || math.Abs(l[1][2]-c.C[2]) > eps {
		return nil
	}
	d := l[1].sub(l[0])
	f := l[0].sub(c.C)
	a, b, cc := d[0]*d[0]+d[1]*d[1], 2*(f[0]*d[0]+f[1]*d[1]), f[0]*f[0]+f[1]*f[1]-c.R*c.R
	disc := b*b - 4*a*cc
	if a == 0 || disc < -eps {
		return nil
	}
	sq := math.Sqrt(max(0, disc))
	var out []P
	for _, t := range []float64{(-b - sq) / (2 * a), (-b + sq) / (2 * a)} {
		if t >= -1e-9 && t <= 1+1e-9 {
			out = append(out, l[0].add(d.mul(t)))
		}
	}
	return out
}

// corners are the points where segments end, branch, or turn: not the
// joints of a smooth curve drawn in pieces.
func corners(segs [][2]P, eps float64) []P {
	type node struct {
		p    P
		dirs []P
	}
	key := func(p P) [3]int64 {
		q := 1 / max(eps, 1e-12)
		return [3]int64{int64(math.Round(p[0] * q)), int64(math.Round(p[1] * q)), int64(math.Round(p[2] * q))}
	}
	nodes := map[[3]int64]*node{}
	var order [][3]int64
	at := func(p P) *node {
		k := key(p)
		n, ok := nodes[k]
		if !ok {
			n = &node{p: p}
			nodes[k] = n
			order = append(order, k)
		}
		return n
	}
	for _, g := range segs {
		d := g[1].sub(g[0])
		if d.norm() == 0 {
			continue
		}
		d = d.mul(1 / d.norm())
		at(g[0]).dirs = append(at(g[0]).dirs, d)
		at(g[1]).dirs = append(at(g[1]).dirs, d.mul(-1))
	}
	const smooth = 20 // degrees a curve may turn at a joint and still be one curve
	var out []P
	for _, k := range order {
		n := nodes[k]
		switch len(n.dirs) {
		case 2:
			// Two pieces going on through: straight on is 180° between them.
			turn := 180 - math.Acos(max(-1, min(1, n.dirs[0].dot(n.dirs[1]))))*180/math.Pi
			if turn > smooth {
				out = append(out, n.p)
			}
		default:
			out = append(out, n.p)
		}
	}
	return out
}
