package meshstl

import "math"

// Dimensioning: witness lines, arrows and text, built as solid geometry so
// they survive into an STL and render in a wireframe viewer that has no idea
// what an annotation is.
//
// The point is that the rack's dimensions stop living only in a package
// comment and a README table. A dimensioned model carries them: pick it up in
// any viewer or slicer and the panel is labeled 35.06, the 3U height 128.5,
// the Eurocard depth 162, in the same millimeters everything else is drawn in.

// RodRadius is the default half-thickness of an annotation line. Thin enough
// to read as a line, thick enough to be a real solid.
const RodRadius = 0.35

// Rod is a square-section bar from a to b — a line you can print. Four
// segments around a swept tube is a square, which is both the cheapest
// possible rod and the one that renders as clean edges in a wireframe.
func Rod(a, b V3, r float64) Mesh {
	if r <= 0 {
		r = RodRadius
	}
	return Tube([]V3{a, b}, r, 4, true)
}

// Arrow is a cone pointing at tip, coming from the direction of tail.
func Arrow(tip, tail V3, r float64) Mesh {
	if r <= 0 {
		r = RodRadius
	}
	dir := tip.Sub(tail).Norm()
	if dir.Len() == 0 {
		return Mesh{}
	}
	length := 6 * r
	base := tip.Sub(dir.Mul(length))
	// Cone() builds along +Z, so build it there and carry it into place with
	// the same rotation that takes +Z to dir.
	m := Cone(V3{}, 3*r, 0, length, 12)
	return orient(m, dir).Translate(base)
}

// orient rotates a mesh built along +Z so that +Z points along dir.
func orient(m Mesh, dir V3) Mesh {
	z := V3{0, 0, 1}
	d := dir.Norm()
	dot := z.Dot(d)
	switch {
	case dot > 1-1e-12:
		return m
	case dot < -1+1e-12:
		// Antiparallel: any perpendicular axis will do for the half turn.
		return rotateMesh(m, V3{1, 0, 0}, math.Pi)
	}
	return rotateMesh(m, z.Cross(d).Norm(), math.Acos(dot))
}

func rotateMesh(m Mesh, axis V3, ang float64) Mesh {
	out := Mesh{Tris: make([]Tri, len(m.Tris))}
	for i, t := range m.Tris {
		out.Tris[i] = Tri{
			rotateAbout(t.A, axis, ang),
			rotateAbout(t.B, axis, ang),
			rotateAbout(t.C, axis, ang),
		}
	}
	return out
}

// Dimension draws a dimension between two points, offset clear of the part.
//
//	a, b     the two points being measured
//	out      direction to push the dimension line clear of the part
//	gap      how far along out to put the dimension line
//	strokes  the label, as segments in the unit square (see TextStrokes)
//	size     the label's cap height
func Dimension(a, b, out V3, gap float64, strokes [][2]V3, size, r float64) Mesh {
	if r <= 0 {
		r = RodRadius
	}
	o := out.Norm().Mul(gap)
	a2, b2 := a.Add(o), b.Add(o)

	var m Mesh
	// Witness lines, standing a little off the part itself so the annotation
	// is visibly separate from the thing it measures, and running a little
	// past the dimension line as a drawing's do.
	ext := out.Norm().Mul(gap + 3*r*2)
	m.Append(Rod(a.Add(out.Norm().Mul(2*r)), a.Add(ext), r))
	m.Append(Rod(b.Add(out.Norm().Mul(2*r)), b.Add(ext), r))
	// The dimension line, with an arrow at each end pointing outward.
	m.Append(Rod(a2, b2, r))
	m.Append(Arrow(a2, b2, r))
	m.Append(Arrow(b2, a2, r))

	if len(strokes) > 0 && size > 0 {
		// The label sits just off the middle of the dimension line, reading
		// along it, standing off in the same direction as the line.
		along := b2.Sub(a2)
		if along.Len() == 0 {
			return m
		}
		right := along.Norm()
		up := textUp(right)
		width := textWidth(strokes) * size
		mid := a2.Add(along.Mul(0.5))
		// Above the line, on the side the part is — which is where a drawing
		// puts it, and which is why up cannot simply be the offset direction:
		// that points AWAY from the part, and a label drawn along it comes
		// out upside down.
		origin := mid.Sub(right.Mul(width / 2)).Add(up.Mul(2 * r))
		m.Append(Strokes(strokes, origin, right, up, size, r))
	}
	return m
}

// Strokes lays a set of unit-square segments into the plane spanned by right
// and up, scaled to size, as rods.
func Strokes(segs [][2]V3, origin, right, up V3, size, r float64) Mesh {
	var m Mesh
	place := func(p V3) V3 {
		return origin.Add(right.Mul(p[0] * size)).Add(up.Mul(p[1] * size))
	}
	for _, s := range segs {
		m.Append(Rod(place(s[0]), place(s[1]), r))
	}
	return m
}

// textWidth is how wide a stroke set is, in cap heights.
func textWidth(segs [][2]V3) float64 {
	w := 0.0
	for _, s := range segs {
		for _, p := range s {
			if p[0] > w {
				w = p[0]
			}
		}
	}
	return w
}

// BoxDimensions annotates a bounding box on all three axes: width along X
// below it, height along Y to its left, depth along Z below and behind.
//
// labels supplies the strokes for each axis, already formatted by the caller
// — this package has no font and no opinion about units.
func BoxDimensions(min, max V3, labels [3][][2]V3, size, r float64) Mesh {
	var m Mesh
	span := max.Sub(min)
	gap := math.Max(6, math.Max(span[0], span[1])*0.08)

	// Width, measured along X, pushed down in Y.
	m.Append(Dimension(
		V3{min[0], min[1], max[2]}, V3{max[0], min[1], max[2]},
		V3{0, -1, 0}, gap, labels[0], size, r))
	// Height, measured along Y, pushed out in -X.
	m.Append(Dimension(
		V3{min[0], min[1], max[2]}, V3{min[0], max[1], max[2]},
		V3{-1, 0, 0}, gap, labels[1], size, r))
	// Depth, measured along Z, pushed down in Y at the far side.
	m.Append(Dimension(
		V3{max[0], min[1], max[2]}, V3{max[0], min[1], min[2]},
		V3{0, -1, 0}, gap, labels[2], size, r))
	return m
}

// textUp is the up-vector for a label running along right.
//
// Drawing convention: horizontal dimensions read left to right with up as
// +Y; vertical ones read bottom to top, which is the 90° rotation +Z × right
// gives. A dimension running along Z has no such rotation to take — the cross
// product degenerates — so it falls back to +Y and reads flat.
func textUp(right V3) V3 {
	if up := (V3{0, 0, 1}).Cross(right); up.Len() > 1e-9 {
		return up.Norm()
	}
	return V3{0, 1, 0}
}
