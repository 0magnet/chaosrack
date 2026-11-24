package measure

import "math"

// Reading a figure's circles and lines off its segments.
//
// A renderer holds a figure as segments, and a circle as many short ones. To
// solve exactly where two circles meet, Detect first has to see that those
// segments ARE a circle: it chains them into curves, breaking at corners, and
// keeps a curve as a line when it is straight and as a circle when its
// points are all one distance from a center. An arc is kept as its whole
// circle; Prepare then keeps only the meeting points that lie on what is
// drawn, so an arc's missing part adds nothing.

// detect fills Lines, Circles and the circles' centers into Marks from Segs,
// when the figure did not say them itself.
func (s *Scene) detect() {
	eps := s.scale * 1e-7
	for _, chain := range chains(s.Segs, eps) {
		if len(chain) < 2 {
			continue
		}
		if collinear(chain, s.scale*1e-9) {
			s.Lines = append(s.Lines, [2]P{chain[0], chain[len(chain)-1]})
			s.lineDrawn = append(s.lineDrawn, [][]P{chain})
			continue
		}
		if len(chain) < 5 {
			continue
		}
		c, ok := fitCircle(chain, s.scale*1e-6)
		if !ok {
			continue
		}
		// A circle another curve touches is broken there into arcs, and
		// each arc fits the same circle: one circle, drawn in pieces.
		same := -1
		for i, d := range s.Circles {
			if d.C.near(c.C, s.scale*1e-7) && math.Abs(d.R-c.R) <= s.scale*1e-7 {
				same = i
			}
		}
		if same >= 0 {
			s.circleDrawn[same] = append(s.circleDrawn[same], chain)
			continue
		}
		s.Circles = append(s.Circles, c)
		s.circleDrawn = append(s.circleDrawn, [][]P{chain})
		s.addMark(c.C)
	}
}

// drawnSegs are a curve's drawn pieces as segments, for onDrawing; nil when
// the figure named the curve itself and nothing is known of what is drawn.
func drawnSegs(curves [][][]P, i int) [][2]P {
	if i >= len(curves) {
		return nil
	}
	var out [][2]P
	for _, pts := range curves[i] {
		for k := 1; k < len(pts); k++ {
			out = append(out, [2]P{pts[k-1], pts[k]})
		}
	}
	return out
}

// addMark adds a named point once.
func (s *Scene) addMark(p P) {
	for _, q := range s.Marks {
		if q.near(p, s.scale*1e-7) {
			return
		}
	}
	s.Marks = append(s.Marks, p)
}

// collinear reports whether a chain's points lie on one line.
func collinear(pts []P, eps float64) bool {
	a, b := pts[0], pts[len(pts)-1]
	d := b.sub(a)
	l := d.norm()
	if l == 0 {
		return false
	}
	for _, p := range pts[1 : len(pts)-1] {
		v := p.sub(a)
		t := v.dot(d) / (l * l)
		if v.sub(d.mul(t)).norm() > eps*max(1, l) {
			return false
		}
	}
	return true
}

// fitCircle is the circle through a chain's points, if they lie in a plane
// of constant z and all one distance from its center.
func fitCircle(pts []P, eps float64) (Circle, bool) {
	z := pts[0][2]
	for _, p := range pts {
		if math.Abs(p[2]-z) > eps {
			return Circle{}, false
		}
	}
	a, b, c := pts[0], pts[len(pts)/3], pts[2*len(pts)/3]
	// The circumcenter of a, b, c in the plane.
	d := 2 * (a[0]*(b[1]-c[1]) + b[0]*(c[1]-a[1]) + c[0]*(a[1]-b[1]))
	if math.Abs(d) < 1e-18 {
		return Circle{}, false
	}
	sq := func(p P) float64 { return p[0]*p[0] + p[1]*p[1] }
	cx := (sq(a)*(b[1]-c[1]) + sq(b)*(c[1]-a[1]) + sq(c)*(a[1]-b[1])) / d
	cy := (sq(a)*(c[0]-b[0]) + sq(b)*(a[0]-c[0]) + sq(c)*(b[0]-a[0])) / d
	center := P{cx, cy, z}
	r := Dist(a, center)
	for _, p := range pts {
		if math.Abs(Dist(p, center)-r) > eps*max(1, r) {
			return Circle{}, false
		}
	}
	return Circle{center, r}, true
}

// chains joins segments into curves: runs through joints where exactly two
// pieces meet and go smoothly on, broken at corners, branches and ends.
func chains(segs [][2]P, eps float64) [][]P {
	q := 1 / max(eps, 1e-12)
	key := func(p P) [3]int64 {
		return [3]int64{int64(math.Round(p[0] * q)), int64(math.Round(p[1] * q)), int64(math.Round(p[2] * q))}
	}
	type end struct {
		seg   int
		other P
	}
	adj := map[[3]int64][]end{}
	pos := map[[3]int64]P{}
	for i, g := range segs {
		if g[0].near(g[1], eps) {
			continue
		}
		ka, kb := key(g[0]), key(g[1])
		adj[ka] = append(adj[ka], end{i, g[1]})
		adj[kb] = append(adj[kb], end{i, g[0]})
		pos[ka], pos[kb] = g[0], g[1]
	}
	through := func(k [3]int64) bool {
		e := adj[k]
		if len(e) != 2 {
			return false
		}
		p := pos[k]
		u, v := e[0].other.sub(p), e[1].other.sub(p)
		nu, nv := u.norm(), v.norm()
		if nu == 0 || nv == 0 {
			return false
		}
		// Going on through is 180° between the two pieces; a joint that
		// turns more than 20° is a corner.
		return math.Acos(max(-1, min(1, u.dot(v)/(nu*nv))))*180/math.Pi > 160
	}
	used := make([]bool, len(segs))
	var out [][]P
	walk := func(start [3]int64, first end) []P {
		pts := []P{pos[start]}
		e := first
		for {
			used[e.seg] = true
			pts = append(pts, e.other)
			cur := key(e.other)
			if !through(cur) || key(pts[0]) == cur {
				return pts
			}
			next := end{-1, P{}}
			for _, f := range adj[cur] {
				if !used[f.seg] {
					next = f
				}
			}
			if next.seg < 0 {
				return pts
			}
			e = next
		}
	}
	// Open curves first, from their ends and corners; then the closed loops
	// that are left, from anywhere on them.
	for _, pass := range []bool{false, true} {
		for k, es := range adj {
			if !pass && through(k) {
				continue
			}
			for _, e := range es {
				if !used[e.seg] {
					out = append(out, walk(k, e))
				}
			}
		}
	}
	return out
}

// onDrawing reports whether p lies on one of the segments, within tol.
func onDrawing(p P, segs [][2]P, tol float64) bool {
	for _, g := range segs {
		d := g[1].sub(g[0])
		l := d.dot(d)
		t := 0.0
		if l > 0 {
			t = max(0, min(1, p.sub(g[0]).dot(d)/l))
		}
		if p.sub(g[0].add(d.mul(t))).norm() <= tol {
			return true
		}
	}
	return false
}
