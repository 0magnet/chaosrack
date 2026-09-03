package meshstl

import "math"

// Box is an axis-aligned box, wound outward.
func Box(min, max V3) Mesh {
	var m Mesh
	x0, y0, z0 := min[0], min[1], min[2]
	x1, y1, z1 := max[0], max[1], max[2]
	// Each face wound counter-clockwise seen from outside.
	m.AddQuad(V3{x0, y0, z1}, V3{x1, y0, z1}, V3{x1, y1, z1}, V3{x0, y1, z1}) // +Z
	m.AddQuad(V3{x1, y0, z0}, V3{x0, y0, z0}, V3{x0, y1, z0}, V3{x1, y1, z0}) // -Z
	m.AddQuad(V3{x1, y0, z1}, V3{x1, y0, z0}, V3{x1, y1, z0}, V3{x1, y1, z1}) // +X
	m.AddQuad(V3{x0, y0, z0}, V3{x0, y0, z1}, V3{x0, y1, z1}, V3{x0, y1, z0}) // -X
	m.AddQuad(V3{x0, y1, z1}, V3{x1, y1, z1}, V3{x1, y1, z0}, V3{x0, y1, z0}) // +Y
	m.AddQuad(V3{x0, y0, z0}, V3{x1, y0, z0}, V3{x1, y0, z1}, V3{x0, y0, z1}) // -Y
	return m
}

// Cylinder is a closed cylinder along +Z, from base for height h.
func Cylinder(base V3, r, h float64, seg int) Mesh {
	if seg < 3 {
		seg = 3
	}
	var m Mesh
	top := base.Add(V3{0, 0, h})
	ring := func(c V3) []V3 {
		out := make([]V3, seg)
		for i := 0; i < seg; i++ {
			a := 2 * math.Pi * float64(i) / float64(seg)
			out[i] = V3{c[0] + r*math.Cos(a), c[1] + r*math.Sin(a), c[2]}
		}
		return out
	}
	lo, hi := ring(base), ring(top)
	for i := 0; i < seg; i++ {
		j := (i + 1) % seg
		m.AddQuad(lo[i], lo[j], hi[j], hi[i])
		m.Add(Tri{top, hi[i], hi[j]})  // top cap, outward +Z
		m.Add(Tri{base, lo[j], lo[i]}) // bottom cap, outward -Z
	}
	return m
}

// Cone is a truncated cone along +Z — a knob with a taper, or a chamfer.
func Cone(base V3, r0, r1, h float64, seg int) Mesh {
	if seg < 3 {
		seg = 3
	}
	var m Mesh
	top := base.Add(V3{0, 0, h})
	ring := func(c V3, r float64) []V3 {
		out := make([]V3, seg)
		for i := 0; i < seg; i++ {
			a := 2 * math.Pi * float64(i) / float64(seg)
			out[i] = V3{c[0] + r*math.Cos(a), c[1] + r*math.Sin(a), c[2]}
		}
		return out
	}
	lo, hi := ring(base, r0), ring(top, r1)
	for i := 0; i < seg; i++ {
		j := (i + 1) % seg
		m.AddQuad(lo[i], lo[j], hi[j], hi[i])
		m.Add(Tri{top, hi[i], hi[j]})
		m.Add(Tri{base, lo[j], lo[i]})
	}
	return m
}

// UVSphere is a sphere of latitude/longitude quads.
func UVSphere(c V3, r float64, stacks, slices int) Mesh {
	if stacks < 2 {
		stacks = 2
	}
	if slices < 3 {
		slices = 3
	}
	at := func(i, j int) V3 {
		phi := math.Pi * float64(i) / float64(stacks)    // 0..π from +Z
		th := 2 * math.Pi * float64(j) / float64(slices) // around
		return V3{
			c[0] + r*math.Sin(phi)*math.Cos(th),
			c[1] + r*math.Sin(phi)*math.Sin(th),
			c[2] + r*math.Cos(phi),
		}
	}
	var m Mesh
	for i := 0; i < stacks; i++ {
		for j := 0; j < slices; j++ {
			a, b := at(i, j), at(i, j+1)
			cc, d := at(i+1, j+1), at(i+1, j)
			// Wound a-d-c-b, not a-b-c-d: i increases southward and j
			// increases counter-clockwise seen from the north pole, so the
			// natural order is left-handed and gives inward normals — a
			// sphere with negative volume, which is what the volume check
			// caught. The polar rows collapse to a point; Add drops the
			// degenerate half of those quads.
			m.AddQuad(a, d, cc, b)
		}
	}
	return m
}

// Torus is a doughnut in the XY plane: major radius R, tube radius r.
func Torus(c V3, R, r float64, major, minor int) Mesh {
	if major < 3 {
		major = 3
	}
	if minor < 3 {
		minor = 3
	}
	at := func(i, j int) V3 {
		u := 2 * math.Pi * float64(i) / float64(major)
		v := 2 * math.Pi * float64(j) / float64(minor)
		return V3{
			c[0] + (R+r*math.Cos(v))*math.Cos(u),
			c[1] + (R+r*math.Cos(v))*math.Sin(u),
			c[2] + r*math.Sin(v),
		}
	}
	var m Mesh
	for i := 0; i < major; i++ {
		for j := 0; j < minor; j++ {
			m.AddQuad(at(i, j), at(i+1, j), at(i+1, j+1), at(i, j+1))
		}
	}
	return m
}

// Tube sweeps a circle of radius r along a polyline — how a trajectory
// becomes something you can print.
//
// The frame is carried along the path by parallel transport rather than
// rebuilt from a fixed "up" at each point: an attractor doubles back and
// loops through every orientation, and a fixed-up frame flips over as the
// tangent passes vertical, which twists the tube inside out at that point.
// Transporting the previous frame has no such singularity.
func Tube(path []V3, r float64, seg int, capEnds bool) Mesh {
	if len(path) < 2 {
		return Mesh{}
	}
	if seg < 3 {
		seg = 3
	}
	// Drop consecutive duplicates: they give a zero tangent, and the frame
	// would have nothing to transport.
	pts := make([]V3, 0, len(path))
	for _, p := range path {
		if len(pts) == 0 || p.Sub(pts[len(pts)-1]).Len() > 1e-9 {
			pts = append(pts, p)
		}
	}
	if len(pts) < 2 {
		return Mesh{}
	}

	tangent := func(i int) V3 {
		switch {
		case i == 0:
			return pts[1].Sub(pts[0]).Norm()
		case i == len(pts)-1:
			return pts[i].Sub(pts[i-1]).Norm()
		default:
			return pts[i+1].Sub(pts[i-1]).Norm()
		}
	}

	// Seed a normal perpendicular to the first tangent, choosing the world
	// axis least aligned with it so the cross product is well conditioned.
	t0 := tangent(0)
	seed := V3{0, 0, 1}
	if math.Abs(t0[2]) > 0.9 {
		seed = V3{1, 0, 0}
	}
	n := t0.Cross(seed).Norm()

	rings := make([][]V3, len(pts))
	prevT := t0
	for i := range pts {
		t := tangent(i)
		// Parallel transport: rotate the carried normal by the same rotation
		// that takes the previous tangent to this one.
		if axis := prevT.Cross(t); axis.Len() > 1e-12 {
			ang := math.Atan2(axis.Len(), prevT.Dot(t))
			n = rotateAbout(n, axis.Norm(), ang)
		}
		n = n.Sub(t.Mul(n.Dot(t))).Norm() // re-orthogonalize against drift
		b := t.Cross(n)
		ring := make([]V3, seg)
		for j := 0; j < seg; j++ {
			a := 2 * math.Pi * float64(j) / float64(seg)
			ring[j] = pts[i].Add(n.Mul(r * math.Cos(a))).Add(b.Mul(r * math.Sin(a)))
		}
		rings[i] = ring
		prevT = t
	}

	var m Mesh
	for i := 0; i+1 < len(rings); i++ {
		for j := 0; j < seg; j++ {
			k := (j + 1) % seg
			m.AddQuad(rings[i][j], rings[i][k], rings[i+1][k], rings[i+1][j])
		}
	}
	if capEnds {
		first, last := rings[0], rings[len(rings)-1]
		for j := 0; j < seg; j++ {
			k := (j + 1) % seg
			m.Add(Tri{pts[0], first[k], first[j]})
			m.Add(Tri{pts[len(pts)-1], last[j], last[k]})
		}
	}
	return m
}

// rotateAbout rotates v about a unit axis by ang (Rodrigues).
func rotateAbout(v, axis V3, ang float64) V3 {
	s, c := math.Sin(ang), math.Cos(ang)
	return v.Mul(c).Add(axis.Cross(v).Mul(s)).Add(axis.Mul(axis.Dot(v) * (1 - c)))
}

// Polyline sweeps a tube along a path, sized so the whole figure fits a box
// of the given size — the form the app's viewer wants.
func Polyline(path []V3, tubeFrac float64, seg int) Mesh {
	m := Tube(path, 1, seg, true)
	if len(m.Tris) == 0 {
		return m
	}
	// The tube was swept at unit radius; rescale the whole thing so the tube
	// is tubeFrac of the figure's largest dimension.
	s := m.Size()
	biggest := math.Max(s[0], math.Max(s[1], s[2]))
	if biggest <= 0 {
		return m
	}
	return Tube(path, tubeFrac*biggest, seg, true)
}
