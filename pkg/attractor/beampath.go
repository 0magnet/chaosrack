package attractor

import "math"

// The beam: a drawing as an XY display traces it.
//
// The rack draws a figure whole, every frame, which is how a raster screen
// shows one. A vector display has one beam, and the figure is wherever that
// beam has been lately: it runs round the path at its own speed, lit where
// the drawing is and blanked across the jumps between strokes, and the
// phosphor holds what it has drawn. So a longer path takes longer to go
// round at the same speed — at constant brightness, because brightness is
// speed — and the time round is the period of the sound the same beam makes
// when its deflections are listened to. The beam's speed is the pitch
// control, and the path's length is the figure's own say in it.

// beamPath is a figure as one closed circuit: segment i runs from pts[i] to
// pts[(i+1)%n] and is drawn where lit[i], blanked otherwise. The last
// segment closes the circuit and is lit only when the figure is a single
// closed stroke.
type beamPath struct {
	pts [][3]float32
	lit []bool
}

// beamBlankSpeed is how many times faster than the beam the retrace crosses
// a blanked jump: fast enough that a jump costs little of the circuit, and
// still a slope rather than a step, so the sound does not click at every one.
const beamBlankSpeed = 8

// fromStrip makes p a line strip (x, y, z and the trail parameter per
// vertex, n of them from first) as a path: one stroke, closed by a blanked
// return unless it ends where it began.
func (p *beamPath) fromStrip(v []float32, first, n int) {
	const stride = 4
	p.reset()
	for i := first; i < first+n && (i+1)*stride <= len(v); i++ {
		p.pts = append(p.pts, [3]float32{v[i*stride], v[i*stride+1], v[i*stride+2]})
		p.lit = append(p.lit, true)
	}
	if len(p.lit) > 0 {
		last := len(p.lit) - 1
		p.lit[last] = p.pts[last] == p.pts[0]
	}
}

// fromLines makes p a set of separate segments — vertex pairs, by index when
// idx is given and in order when it is nil — chained into strokes wherever
// one segment starts at the vertex the last one ended on, and the strokes
// joined end to start by blanked jumps. Meshes list their lines in drawing
// order (a globe's rings, then its meridians), so that order is the beam's.
func (p *beamPath) fromLines(v []float32, stride int, idx []uint16, count int) {
	p.reset()
	vert := func(i int) [3]float32 {
		if idx != nil {
			i = int(idx[i])
		}
		if (i+1)*stride > len(v) {
			return [3]float32{}
		}
		return [3]float32{v[i*stride], v[i*stride+1], v[i*stride+2]}
	}
	if idx != nil {
		count = min(count, len(idx))
	}
	for i := 0; i+1 < count; i += 2 {
		a, b := vert(i), vert(i+1)
		if n := len(p.pts); n > 0 && p.pts[n-1] == a {
			p.lit[n-1] = true
		} else {
			if n > 0 {
				p.lit[n-1] = false // the jump to this stroke
			}
			p.pts = append(p.pts, a)
			p.lit = append(p.lit, true)
		}
		p.pts = append(p.pts, b)
		p.lit = append(p.lit, false) // until a segment continues from it
	}
	if n := len(p.pts); n > 1 && p.pts[n-1] == p.pts[0] {
		p.lit[n-1] = true
	}
}

// reset empties p, keeping its arrays: a figure is rebuilt every frame it
// moves, and TinyGo's collector makes that much garbage visible as stutter.
func (p *beamPath) reset() { p.pts, p.lit = p.pts[:0], p.lit[:0] }

// beamWalker is a beam's place on a path: on segment seg, at frac of the
// way along it.
type beamWalker struct {
	seg  int
	frac float64
}

// beamFrame places a point of the path in the coordinates the beam is
// measured and heard in.
type beamFrame func([3]float32) [3]float64

// segLen is segment i's length in frame f, a blanked one shortened by
// the retrace's speed.
func (p *beamPath) segLen(i int, f beamFrame) float64 {
	a, b := f(p.pts[i]), f(p.pts[(i+1)%len(p.pts)])
	d := math.Sqrt((b[0]-a[0])*(b[0]-a[0]) + (b[1]-a[1])*(b[1]-a[1]) + (b[2]-a[2])*(b[2]-a[2]))
	if !p.lit[i] {
		d /= beamBlankSpeed
	}
	return d
}

// length is the time round the circuit, as a distance at the beam's speed.
func (p *beamPath) length(f beamFrame) float64 {
	t := 0.0
	for i := range p.pts {
		t += p.segLen(i, f)
	}
	return t
}

// walk moves w dist along p, measured in frame f, calling visit with each
// lit stretch it crosses as two points of the path and their fractions of
// the way along their segment. A walk of a whole circuit or more draws the
// whole figure once.
func (w *beamWalker) walk(p *beamPath, f beamFrame, dist float64, visit func(seg int, f0, f1 float64)) {
	w.advance(p, f, dist, p.length(f), visit)
}

// advance is walk with the circuit's length already measured, for a caller
// that moves the beam a sample at a time and cannot afford to measure the
// whole figure for each.
func (w *beamWalker) advance(p *beamPath, f beamFrame, dist, total float64, visit func(seg int, f0, f1 float64)) {
	n := len(p.pts)
	if n < 2 || !(dist > 0) || !(total > 0) {
		return
	}
	if w.seg >= n {
		w.seg, w.frac = 0, 0
	}
	if dist >= total {
		if visit != nil {
			for i := range n {
				if p.lit[i] {
					visit(i, 0, 1)
				}
			}
		}
		dist = math.Mod(dist, total)
	}
	// Bounded: dist is under one circuit, so this passes each segment at
	// most once and a zero-length one costs a step.
	for range n + 1 {
		l := p.segLen(w.seg, f)
		left := l * (1 - w.frac)
		if dist < left {
			f1 := w.frac + dist/l
			if visit != nil && p.lit[w.seg] {
				visit(w.seg, w.frac, f1)
			}
			w.frac = f1
			return
		}
		if visit != nil && p.lit[w.seg] && w.frac < 1 {
			visit(w.seg, w.frac, 1)
		}
		dist -= left
		w.seg, w.frac = (w.seg+1)%n, 0
	}
}

// at is where the beam is, in frame f.
func (w *beamWalker) at(p *beamPath, f beamFrame) [3]float64 {
	n := len(p.pts)
	if n == 0 {
		return [3]float64{}
	}
	if w.seg >= n {
		w.seg, w.frac = 0, 0
	}
	a, b := f(p.pts[w.seg]), f(p.pts[(w.seg+1)%n])
	t := w.frac
	return [3]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}
