package lattice

import (
	"math"
	"sort"
)

// A Shape is a solid given by its signed distance: negative inside, positive
// outside, zero on the surface. Only the surface is drawn.
type Shape func(p Vec3) float64

// Shapes are the built-in solids by name, each reaching about 0.8 of the
// volume's half-width so that a turning one stays inside it.
var Shapes = map[string]Shape{
	"sphere": func(p Vec3) float64 { return p.Len() - 0.8 },
	"torus": func(p Vec3) float64 {
		q := math.Hypot(p[0], p[2]) - 0.55
		return math.Hypot(q, p[1]) - 0.25
	},
	"cube": func(p Vec3) float64 {
		const h = 0.55
		q := Vec3{math.Abs(p[0]) - h, math.Abs(p[1]) - h, math.Abs(p[2]) - h}
		out := Vec3{math.Max(q[0], 0), math.Max(q[1], 0), math.Max(q[2], 0)}.Len()
		return out + math.Min(math.Max(q[0], math.Max(q[1], q[2])), 0)
	},
	"octahedron": func(p Vec3) float64 {
		return (math.Abs(p[0]) + math.Abs(p[1]) + math.Abs(p[2]) - 0.85) / math.Sqrt(3)
	},
	"gyroid": func(p Vec3) float64 {
		const k = 2.2 // cells across the volume
		g := math.Sin(k*math.Pi*p[0])*math.Cos(k*math.Pi*p[1]) +
			math.Sin(k*math.Pi*p[1])*math.Cos(k*math.Pi*p[2]) +
			math.Sin(k*math.Pi*p[2])*math.Cos(k*math.Pi*p[0])
		return math.Max(g/(k*math.Pi*1.5), p.Len()-0.9) // clipped to a ball
	},
}

// ShapeNames are the built-in shapes' names, sorted.
func ShapeNames() []string {
	out := make([]string, 0, len(Shapes))
	for k := range Shapes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Turned is s turned by yaw about the vertical axis and then tilted by
// pitch about the horizontal one, in radians. The point is turned the other
// way, which is the same as turning the solid.
func Turned(s Shape, yaw, pitch float64) Shape {
	cy, sy := math.Cos(yaw), math.Sin(yaw)
	cp, sp := math.Cos(pitch), math.Sin(pitch)
	return func(p Vec3) float64 {
		// undo the pitch (about x), then the yaw (about y)
		y := cp*p[1] + sp*p[2]
		z := -sp*p[1] + cp*p[2]
		x := cy*p[0] - sy*z
		z = sy*p[0] + cy*z
		return s(Vec3{x, y, z})
	}
}

// gradient is the direction s grows fastest at p, by central differences.
func gradient(s Shape, p Vec3, h float64) Vec3 {
	var g Vec3
	for i := range 3 {
		a, b := p, p
		a[i] += h
		b[i] -= h
		g[i] = (s(a) - s(b)) / (2 * h)
	}
	return g
}
