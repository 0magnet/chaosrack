package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/conway"
	"github.com/0magnet/chaosrack/pkg/geom"
)

// The geometry models' shapes as their knobs make them, shared by the page
// (sphere.go, conway_js.go, which upload them) and a native rack, which draws
// them from its own knobs (ShapeFigure).

// GlobeMesh is the globe's wireframe, appended to verts and idx: lat
// parallels, as rings or as one spiral pole to pole (wound the other way
// with rev), and lon meridians, each leaning twist radians round per radian
// down. sphere.go says why each is drawn as it is.
func GlobeMesh(lat, lon int, spiral, rev bool, twist float64, verts []float32, idx []uint16) ([]float32, []uint16) {
	const pts = 60 // points per circle
	if spiral && lat > 0 {
		steps := pts * lat
		base := uint16(len(verts) / 3) //nolint:gosec // bounded: pts*lat is far under uint16 at lat's cap
		wind := 1.0
		if rev {
			wind = -1
		}
		for k := 0; k <= steps; k++ {
			t := float64(k) / float64(steps)
			phi := t * math.Pi
			theta := wind * t * 2 * math.Pi * float64(lat)
			verts = append(verts,
				float32(math.Sin(phi)*math.Cos(theta)),
				float32(math.Sin(phi)*math.Sin(theta)),
				float32(math.Cos(phi)))
			if k > 0 {
				idx = append(idx, base+uint16(k-1), base+uint16(k)) //nolint:gosec // as above
			}
		}
	} else {
		for i := 1; i < lat; i++ {
			phi := float32(i) * float32(math.Pi) / float32(lat)
			base := uint16(len(verts) / 3) //nolint:gosec // a mesh index, bounded by the knobs' caps
			for j := 0; j <= pts; j++ {
				theta := float32(j) * 2.0 * float32(math.Pi) / float32(pts)
				verts = append(verts,
					float32(math.Sin(float64(phi)))*float32(math.Cos(float64(theta))),
					float32(math.Sin(float64(phi)))*float32(math.Sin(float64(theta))),
					float32(math.Cos(float64(phi))))
				if j > 0 {
					idx = append(idx, base+uint16(j-1), base+uint16(j)) //nolint:gosec // as above
				}
			}
		}
	}
	for j := range lon {
		theta0 := float64(j) * 2.0 * math.Pi / float64(lon)
		base := uint16(len(verts) / 3) //nolint:gosec // a mesh index, bounded by the knobs' caps
		for i := 0; i <= pts; i++ {
			phi := float32(i) * float32(math.Pi) / float32(pts)
			theta := float32(theta0 + twist*float64(phi))
			verts = append(verts,
				float32(math.Sin(float64(phi)))*float32(math.Cos(float64(theta))),
				float32(math.Sin(float64(phi)))*float32(math.Sin(float64(theta))),
				float32(math.Cos(float64(phi))))
			if i > 0 {
				idx = append(idx, base+uint16(i-1), base+uint16(i)) //nolint:gosec // as above
			}
		}
	}
	return verts, idx
}

// TorusMesh is the torus's wireframe, its tube rolled roll radians about its
// core circle (sphere.go's torusShape says which way round that is).
func TorusMesh(major, minor float32, stacks, slices int, roll float32) ([]float32, []uint16) {
	vertices := make([]float32, 0, (stacks+1)*(slices+1)*3)
	indices := make([]uint16, 0, stacks*slices*4)
	for i := 0; i <= stacks; i++ {
		theta := float32(i) * 2.0 * math.Pi / float32(stacks)
		for j := 0; j <= slices; j++ {
			phi := float32(j)*2.0*math.Pi/float32(slices) + roll
			xv := (major + minor*float32(math.Cos(float64(phi)))) * float32(math.Cos(float64(theta)))
			yv := (major + minor*float32(math.Cos(float64(phi)))) * float32(math.Sin(float64(theta)))
			zv := minor * float32(math.Sin(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := range stacks {
		for j := range slices {
			cur := uint16(i*(slices+1) + j)       //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
			below := uint16((i+1)*(slices+1) + j) //nolint:gosec // as above
			indices = append(indices, cur, cur+1, cur, below)
		}
	}
	return vertices, indices
}

// PolySolid is the Polyhedron's solid, or the tiling {p,q} is, and whether
// it is a solid: morph, op and kis act only on a solid (conway_js.go).
func PolySolid(p, q int, morph float64, op int, kis float64) (conway.Solid, bool) {
	s := conway.Regular(p, q)
	if conway.CurvatureOf(p, q) != conway.Spherical {
		return s, false
	}
	s = conway.Morph(s, morph).Apply(op)
	if kis != 0 {
		s = s.Kis(kis)
	}
	return s, true
}

// ShapeFigure is a geometry model drawn from its knobs (knob is a control's
// value) rather than at its defaults, and whether key is one that has them.
// roll is the torus tube's phase, which its ROLL knob turns over time.
func ShapeFigure(key string, knob func(id string) float64, roll float64) (Figure, bool) {
	round := func(id string) int { return int(knob(id) + 0.5) }
	switch key {
	case "globe":
		v, i := GlobeMesh(int(knob("globe-lat")), int(knob("globe-lon")),
			knob("globe-par") >= 0.5, knob("globe-rev") >= 0.5, knob("globe-twist"), nil, nil)
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "torus":
		stacks, slices := int(knob("torus-stacks")), int(knob("torus-slices"))
		if stacks < 1 || slices < 1 {
			return Figure{}, false
		}
		v, i := TorusMesh(float32(knob("torus-R")), float32(knob("torus-r")), stacks, slices, float32(roll))
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "polyhedron":
		s, _ := PolySolid(round("polyhedron-p"), round("polyhedron-q"),
			knob("polyhedron-morph"), round("polyhedron-op"), knob("polyhedron-kis"))
		return polyFigure(s), true
	case "lissajou":
		a, b, c := knob("lissajou-a"), knob("lissajou-b"), knob("lissajou-c")
		if a <= 0 || b <= 0 || c <= 0 {
			return Figure{}, false
		}
		return Figure{Kind: FigurePath, Points: lissajousOf(a, b, c, 20000)}, true
	}
	return Figure{}, false
}
