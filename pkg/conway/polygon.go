package conway

import "math"

// The polygon is the solid one dimension down. Drop the last number of a
// Schläfli symbol and {p,q}, q faces of p sides at every corner, is {p}: the
// face itself. The tetrahedron {3,3} becomes the triangle, the cube {4,3}
// the square, the dodecahedron {5,3} the pentagon. This is the Polyhedron's
// DIM 2, drawn by the same rules as its solids, one dimension down:
//
//   - MORPH cuts the corners. 0 is the polygon, 1 cuts them back to the
//     midpoints of the edges (the rectified polygon, which for a polygon is
//     the same polygon turned by half a step), and 2 is the dual, which for
//     a polygon is itself again, turned. Every point is put back on the unit
//     circle after, as the solids' are put back on the sphere, so the truncated
//     polygon between is a 2p-gon inscribed in the same circle.
//   - KIS raises a point on every edge, outward or (below zero) in: the
//     pyramid on every face of a solid is a triangle on every edge of a
//     polygon, and a polygon with a triangle on every edge is a star.
//
// It is returned as a Solid with one face, so the Polyhedron's wireframe,
// counts and native figure draw it with no code of their own.

// Polygon is the regular polygon {p} in the plane z = 0, morphed and with
// kis applied, as the Polyhedron's knobs make it. p below 3 is a triangle.
func Polygon(p int, morph, kis float64) Solid {
	p = max(p, 3)
	step := 2 * math.Pi / float64(p)
	corner := func(offset float64, k int) [2]float64 {
		a := offset + float64(k)*step
		return [2]float64{math.Cos(a), math.Sin(a)}
	}
	var pts [][2]float64
	switch {
	case math.Abs(morph-1) < 1e-3:
		// The rectified polygon: the edge midpoints, once each (cut from both
		// ends, every midpoint is reached twice).
		for k := range p {
			pts = append(pts, corner(step/2, k))
		}
	default:
		offset, cut := 0.0, morph/2
		if morph > 1 {
			offset, cut = step/2, (2-morph)/2 // the dual's corners, cut back
		}
		for k := range p {
			v := corner(offset, k)
			if cut < 1e-3 {
				pts = append(pts, v)
				continue
			}
			prev, next := corner(offset, k-1), corner(offset, k+1)
			pts = append(pts, onCircle(lerp2(v, prev, cut)), onCircle(lerp2(v, next, cut)))
		}
	}
	if kis != 0 {
		// A triangle on every edge: its apex over the edge's midpoint, kis
		// out (or in) along the radius through it.
		star := make([][2]float64, 0, 2*len(pts))
		for i, a := range pts {
			b := pts[(i+1)%len(pts)]
			m := lerp2(a, b, 0.5)
			r := math.Hypot(m[0], m[1])
			if r == 0 {
				star = append(star, a)
				continue
			}
			s := (r + kis) / r
			star = append(star, a, [2]float64{m[0] * s, m[1] * s})
		}
		pts = star
	}
	out := Solid{Faces: [][]int{make([]int, len(pts))}}
	for i, v := range pts {
		out.Verts = append(out.Verts, Vert{v[0], v[1], 0})
		out.Faces[0][i] = i
	}
	return out
}

// PolygonAngle is the interior angle of {p} in degrees, (p − 2)·180/p.
func PolygonAngle(p int) float64 { return float64(max(p, 3)-2) * 180 / float64(max(p, 3)) }

func lerp2(a, b [2]float64, t float64) [2]float64 {
	return [2]float64{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1])}
}

func onCircle(v [2]float64) [2]float64 {
	r := math.Hypot(v[0], v[1])
	if r == 0 {
		return v
	}
	return [2]float64{v[0] / r, v[1] / r}
}
