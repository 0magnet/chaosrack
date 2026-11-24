package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/chester"
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
	return globeMesh(lat, lon, spiral, rev, twist, verts, idx, func(phi, theta float64) (x, y, z float64) {
		return math.Sin(phi) * math.Cos(theta), math.Sin(phi) * math.Sin(theta), math.Cos(phi)
	})
}

// GlobeFlatMesh is the globe one dimension down, its DIM 2: the circle, with
// the graticule as the plane's own polar grid. A point phi from the north
// pole lies phi/π of the way to the rim, so the parallels are concentric
// circles evenly spaced, the meridians are spokes (spirals when twisted, as
// they are helices on the sphere), the spiral parallel is Archimedes'
// spiral, and the south pole, a point on the sphere, opens out into the rim:
// the circle itself, drawn whatever the knobs say.
func GlobeFlatMesh(lat, lon int, spiral, rev bool, twist float64, verts []float32, idx []uint16) ([]float32, []uint16) {
	at := func(phi, theta float64) (x, y, z float64) {
		r := phi / math.Pi
		return r * math.Cos(theta), r * math.Sin(theta), 0
	}
	verts, idx = globeMesh(lat, lon, spiral, rev, twist, verts, idx, at)
	return appendLoop(verts, idx, globePoints, func(t float64) (x, y, z float64) { return at(math.Pi, t) })
}

// globePoints is how many straight pieces a circle of the globe is drawn in.
const globePoints = 60

// globeMesh is the globe's wireframe with each point (phi down from the
// pole, theta round) placed by at.
func globeMesh(lat, lon int, spiral, rev bool, twist float64, verts []float32, idx []uint16, at func(phi, theta float64) (x, y, z float64)) ([]float32, []uint16) {
	const pts = globePoints
	put := func(phi, theta float64) {
		x, y, z := at(phi, theta)
		verts = append(verts, float32(x), float32(y), float32(z))
	}
	if spiral && lat > 0 {
		steps := pts * lat
		base := uint16(len(verts) / 3) //nolint:gosec // bounded: pts*lat is far under uint16 at lat's cap
		wind := 1.0
		if rev {
			wind = -1
		}
		for k := 0; k <= steps; k++ {
			t := float64(k) / float64(steps)
			put(t*math.Pi, wind*t*2*math.Pi*float64(lat))
			if k > 0 {
				idx = append(idx, base+uint16(k-1), base+uint16(k)) //nolint:gosec // as above
			}
		}
	} else {
		for i := 1; i < lat; i++ {
			phi := float64(float32(i) * float32(math.Pi) / float32(lat))
			base := uint16(len(verts) / 3) //nolint:gosec // a mesh index, bounded by the knobs' caps
			for j := 0; j <= pts; j++ {
				put(phi, float64(float32(j)*2.0*float32(math.Pi)/float32(pts)))
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
			phi := float64(float32(i) * float32(math.Pi) / float32(pts))
			put(phi, float64(float32(theta0+twist*phi)))
			if i > 0 {
				idx = append(idx, base+uint16(i-1), base+uint16(i)) //nolint:gosec // as above
			}
		}
	}
	return verts, idx
}

// appendLoop appends a closed curve of n pieces, the point at parameter
// t in [0, 2π) placed by at.
func appendLoop(verts []float32, idx []uint16, n int, at func(t float64) (x, y, z float64)) ([]float32, []uint16) {
	base := uint16(len(verts) / 3) //nolint:gosec // a mesh index, bounded by the knobs' caps
	for k := range n {
		x, y, z := at(float64(k) * 2 * math.Pi / float64(n))
		verts = append(verts, float32(x), float32(y), float32(z))
		idx = append(idx, base+uint16(k), base+uint16((k+1)%n)) //nolint:gosec // as above
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

// TorusFlatMesh is the torus one dimension down, its DIM 2: the annulus, the
// band a segment sweeps turning about a point as the torus is the surface a
// circle sweeps turning about an axis. Its grid is the torus's own: stacks
// spokes across the band where the torus has its tube's circles, and slices
// rings round it where the torus has rings round its axis, each ring a
// polygon of stacks sides as each of the torus's is, so 4 stacks is a square
// band. The edges of the band are always drawn; between them, the rings flow
// outward with roll and wrap round, as the tube's surface turns over. A tube
// wider than the hole leaves no hole: the band starts at the center.
func TorusFlatMesh(major, minor float32, stacks, slices int, roll float32) ([]float32, []uint16) {
	inner := math.Max(float64(major-minor), 0)
	outer := float64(major + minor)
	stacks, slices = max(stacks, 3), max(slices, 1)
	var verts []float32
	var idx []uint16
	ring := func(r float64) {
		verts, idx = appendLoop(verts, idx, stacks, func(t float64) (x, y, z float64) {
			return r * math.Cos(t), r * math.Sin(t), 0
		})
	}
	ring(inner)
	ring(outer)
	shift := float64(roll) / (2 * math.Pi)
	for j := 1; j < slices; j++ {
		f := float64(j)/float64(slices) + shift
		f -= math.Floor(f)
		ring(inner + f*(outer-inner))
	}
	for i := range stacks {
		t := float64(i) * 2 * math.Pi / float64(stacks)
		base := uint16(len(verts) / 3) //nolint:gosec // a mesh index, bounded by the knobs' caps
		verts = append(verts,
			float32(inner*math.Cos(t)), float32(inner*math.Sin(t)), 0,
			float32(outer*math.Cos(t)), float32(outer*math.Sin(t)), 0)
		idx = append(idx, base, base+1)
	}
	return verts, idx
}

// SquaresMesh is the Nested Cube one dimension down, its DIM 2: a square
// within a square, the cubes' faces toward the viewer, at the same sizes.
func SquaresMesh() ([]float32, []uint16) {
	var verts []float32
	var idx []uint16
	for _, h := range []float64{1, 0.5} {
		verts, idx = appendLoop(verts, idx, 4, func(t float64) (x, y, z float64) {
			// The corners at 45°, 135°, …, out to ±h.
			return h * math.Sqrt2 * math.Cos(t+math.Pi/4), h * math.Sqrt2 * math.Sin(t+math.Pi/4), 0
		})
	}
	return verts, idx
}

// PolyShape is the Polyhedron as its knobs make it, DIM included: at 2 the
// polygon {p}, the face of {p,q} (conway.Polygon), and otherwise PolySolid.
func PolyShape(p, q int, morph float64, op int, kis float64, dim int) (conway.Solid, bool) {
	if dim == 2 {
		return conway.Polygon(p, morph, kis), false
	}
	return PolySolid(p, q, morph, op, kis)
}

// ChestKnobs are the Chestahedron's knobs, and chestDefaults their
// defaults: the solid, as Chester shows it, with the first of what is
// inside or around it (chester_js.go).
var (
	ChestKnobs    = [3]string{"chestahedron-form", "chestahedron-morph", "chestahedron-detail"}
	chestDefaults = [3]float32{0, 1, 1}
)

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
	// DIM 2 only when the knob says 2: a rack whose catalog predates the
	// knob reads it as 0, and keeps the solid.
	flat := func(model string) bool { return round(model+"-dim") == 2 }
	switch key {
	case "globe":
		mesh := GlobeMesh
		if flat(key) {
			mesh = GlobeFlatMesh
		}
		v, i := mesh(int(knob("globe-lat")), int(knob("globe-lon")),
			knob("globe-par") >= 0.5, knob("globe-rev") >= 0.5, knob("globe-twist"), nil, nil)
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "torus":
		stacks, slices := int(knob("torus-stacks")), int(knob("torus-slices"))
		if stacks < 1 || slices < 1 {
			return Figure{}, false
		}
		mesh := TorusMesh
		if flat(key) {
			mesh = TorusFlatMesh
		}
		v, i := mesh(float32(knob("torus-R")), float32(knob("torus-r")), stacks, slices, float32(roll))
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "nestedcube":
		if flat(key) {
			v, i := SquaresMesh()
			return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
		}
		return linesFigure(geom.Lines{Vertices: verticesCube, Indices: indicesCube}), true
	case "polyhedron":
		s, _ := PolyShape(round("polyhedron-p"), round("polyhedron-q"),
			knob("polyhedron-morph"), round("polyhedron-op"), knob("polyhedron-kis"), round("polyhedron-dim"))
		return polyFigure(s), true
	case "chestahedron":
		return linesFigure(chester.Form(round("chestahedron-form"),
			knob("chestahedron-morph"), knob("chestahedron-detail")).Geom()), true
	case "lissajou":
		a, b, c := knob("lissajou-a"), knob("lissajou-b"), knob("lissajou-c")
		if a <= 0 || b <= 0 || c <= 0 {
			return Figure{}, false
		}
		return Figure{Kind: FigurePath, Points: lissajousOf(a, b, c, 20000)}, true
	}
	return Figure{}, false
}
