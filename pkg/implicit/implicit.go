// Package implicit draws a surface given as an equation, F(x, y, z) = 0, as
// lines.
//
// A surface written this way has no vertices or edges to draw: it is wherever
// F changes sign. The rack draws in lines, so the surface is drawn the way a
// drafter or a topographic map would draw one, by its contours: it is cut by
// evenly spaced planes across each axis, and where each plane meets it is
// traced (marching squares). A sphere is rings of circles, a cube squares,
// a gyroid woven curves; the lines are the shape without a mesh in between.
package implicit

import "math"

// Field is the surface's function. Negative is inside, positive outside, and
// the surface is where it is zero.
type Field func(x, y, z float64) float64

// MaxVerts caps the drawing so its indices fit in 16 bits.
const MaxVerts = 65535

// Contours cuts the cube [-r, r]³ by slices planes across each axis and traces
// where each meets the surface on a res×res grid. It returns line segments as
// vertex pairs (x, y, z each), stopping early rather than passing MaxVerts.
func Contours(f Field, r float64, slices, res int) []float32 {
	if slices < 1 || res < 2 || !(r > 0) {
		return nil
	}
	var out []float32
	step := 2 * r / float64(res)
	grid := make([]float64, (res+1)*(res+1))
	// The planes sit between the grid's own lines, so a cut never falls
	// exactly on a face of a box aligned with the axes, where a solid's side
	// would be all zeros and trace as noise.
	for axis := range 3 {
		for s := range slices {
			c := -r + 2*r*(float64(s)+0.5)/float64(slices)
			// The plane's two in-plane axes, u and v, and the point at (u, v).
			at := func(u, v float64) (float64, float64, float64) {
				switch axis {
				case 0:
					return c, u, v
				case 1:
					return u, c, v
				default:
					return u, v, c
				}
			}
			for j := 0; j <= res; j++ {
				for i := 0; i <= res; i++ {
					x, y, z := at(-r+float64(i)*step, -r+float64(j)*step)
					grid[j*(res+1)+i] = f(x, y, z)
				}
			}
			for j := range res {
				for i := range res {
					out = square(out, grid, res, i, j, r, step, at)
					if len(out)/3 > MaxVerts-4 {
						return out
					}
				}
			}
		}
	}
	return out
}

// square traces the surface across one grid cell: the classic marching
// squares, with the saddle case split by the cell's center value.
func square(out []float32, g []float64, res, i, j int, r, step float64, at func(u, v float64) (float64, float64, float64)) []float32 {
	w := res + 1
	// Corners counterclockwise from the bottom left.
	v := [4]float64{g[j*w+i], g[j*w+i+1], g[(j+1)*w+i+1], g[(j+1)*w+i]}
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return out
		}
	}
	u0, v0 := -r+float64(i)*step, -r+float64(j)*step
	corner := [4][2]float64{{u0, v0}, {u0 + step, v0}, {u0 + step, v0 + step}, {u0, v0 + step}}
	// Where the surface crosses edge k, from corner k to corner k+1.
	cross := func(k int) (float64, float64) {
		a, b := v[k], v[(k+1)%4]
		t := a / (a - b)
		p, q := corner[k], corner[(k+1)%4]
		return p[0] + t*(q[0]-p[0]), p[1] + t*(q[1]-p[1])
	}
	idx := 0
	for k := range 4 {
		if v[k] < 0 {
			idx |= 1 << k
		}
	}
	seg := func(e1, e2 int) {
		ua, va := cross(e1)
		ub, vb := cross(e2)
		xa, ya, za := at(ua, va)
		xb, yb, zb := at(ub, vb)
		out = append(out, float32(xa), float32(ya), float32(za), float32(xb), float32(yb), float32(zb))
	}
	switch idx {
	case 0, 15:
	case 1, 14:
		seg(3, 0)
	case 2, 13:
		seg(0, 1)
	case 3, 12:
		seg(3, 1)
	case 4, 11:
		seg(1, 2)
	case 6, 9:
		seg(0, 2)
	case 7, 8:
		seg(2, 3)
	case 5, 10:
		// A saddle: two opposite corners inside. Which pair of crossings
		// joins is decided by the center, as the surface there decides it.
		center := (v[0] + v[1] + v[2] + v[3]) / 4
		if (center < 0) == (idx == 5) {
			seg(3, 2)
			seg(0, 1)
		} else {
			seg(3, 0)
			seg(1, 2)
		}
	}
	return out
}
