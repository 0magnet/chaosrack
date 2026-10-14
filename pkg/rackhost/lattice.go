package rackhost

import (
	"math"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/lattice"
)

// The Lattice, natively: the page's stack of terminals (attractor.
// LatticeStack), every program in a real terminal of its own, turned with
// the view; each character of their screens drawn as a point in its own
// color, since at a native scene's size a character is a pixel or two.

// latticeFigure runs the Lattice's programs on to now and is every lit cell
// of the sheets it shows, framed to the volume. Called with r.mu held.
func (r *Rack) latticeFigure() (attractor.Figure, [][3]float64) {
	st := r.lattice
	for _, id := range attractor.LatticeKnobs {
		st.Set(id, float32(r.number(id)))
	}
	now := r.sound.now()
	nowMs := float64(now.UnixNano()) / 1e6
	n, _ := st.Ensure(nil, nowMs)
	rot := rotation(r.viewAngle)
	square := st.Pose(quatOf(r.viewAngle))
	// Toward the viewer, in the stack's frame: the view's third row, or
	// straight out with the stack held square.
	look := lattice.Vec3{rot[2][0], rot[2][1], rot[2][2]}
	if square {
		look = lattice.Vec3{0, 0, 1}
	}
	on := st.Families(look)
	st.Run(nowMs, time.Now(), on, 6*time.Millisecond)

	f := attractor.Figure{Kind: attractor.FigurePoints}
	var glyphs []([2][3]float64)
	st.Cells(n, on, func(x, y, z, cr, cg, cb float32, background bool) {
		p := [3]float64{float64(x), float64(y), float64(z)}
		if square {
			p = turnBack(rot, p) // the view turns it; held square, it is turned back
		}
		c := [3]float64{float64(cr), float64(cg), float64(cb)}
		if background {
			// The backgrounds first, so each glyph is drawn over its cell.
			f.Points = append(f.Points, p)
			f.Colors = append(f.Colors, [3]float32{cr, cg, cb})
			return
		}
		glyphs = append(glyphs, [2][3]float64{p, c})
	})
	for _, g := range glyphs {
		f.Points = append(f.Points, g[0])
		f.Colors = append(f.Colors, [3]float32{float32(g[1][0]), float32(g[1][1]), float32(g[1][2])})
	}
	e := attractor.LatticeExtent
	return f, [][3]float64{{-e, -e, -e}, {e, e, e}}
}

// rotation is the view's turn as rasterview applies it, Rx·Ry·Rz, a row per
// screen axis.
func rotation(a [3]float64) [3][3]float64 {
	sx, cx := math.Sin(a[0]), math.Cos(a[0])
	sy, cy := math.Sin(a[1]), math.Cos(a[1])
	sz, cz := math.Sin(a[2]), math.Cos(a[2])
	rx := [3][3]float64{{1, 0, 0}, {0, cx, -sx}, {0, sx, cx}}
	ry := [3][3]float64{{cy, 0, sy}, {0, 1, 0}, {-sy, 0, cy}}
	rz := [3][3]float64{{cz, -sz, 0}, {sz, cz, 0}, {0, 0, 1}}
	return mul3(mul3(rx, ry), rz)
}

func mul3(a, b [3][3]float64) (m [3][3]float64) {
	for i := range 3 {
		for j := range 3 {
			for k := range 3 {
				m[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return m
}

// turnBack is p turned by the inverse of rot.
func turnBack(rot [3][3]float64, p [3]float64) (q [3]float64) {
	for i := range 3 {
		for k := range 3 {
			q[i] += rot[k][i] * p[k]
		}
	}
	return q
}

// quatOf is the view's turn as a quaternion, w first: qx·qy·qz, the order
// rotation composes them in.
func quatOf(a [3]float64) lattice.Quat {
	half := func(t float64) (float64, float64) { return math.Cos(t / 2), math.Sin(t / 2) }
	cx, sx := half(a[0])
	cy, sy := half(a[1])
	cz, sz := half(a[2])
	qx := lattice.Quat{cx, sx, 0, 0}
	qy := lattice.Quat{cy, 0, sy, 0}
	qz := lattice.Quat{cz, 0, 0, sz}
	return qmul(qmul(qx, qy), qz)
}

func qmul(a, b lattice.Quat) lattice.Quat {
	return lattice.Quat{
		a[0]*b[0] - a[1]*b[1] - a[2]*b[2] - a[3]*b[3],
		a[0]*b[1] + a[1]*b[0] + a[2]*b[3] - a[3]*b[2],
		a[0]*b[2] - a[1]*b[3] + a[2]*b[0] + a[3]*b[1],
		a[0]*b[3] + a[1]*b[2] - a[2]*b[1] + a[3]*b[0],
	}
}
