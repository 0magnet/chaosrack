//go:build js && wasm

package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/glctx"
)

// sphereShape is the sphere model's knobs.
type sphereShape struct {
	radius  float32
	stacksF float32
	slicesF float32
}

var sphere = sphereShape{
	radius:  1.0,
	stacksF: 30,
	slicesF: 30,
}

// torusShape is the torus model: its knobs and the phase of its poloidal
// roll.
type torusShape struct {
	major   float32
	minor   float32
	stacksF float32
	slicesF float32

	// rollF is the POLOIDAL spin rate: the tube turning about the torus's
	// own core circle, the circle running through the middle of the tube body.
	// Signed, and zero by default so the torus is still until asked.
	rollF   float32 // poloidal rate; see torus.generate
	rollPhi float32 // accumulated poloidal angle
}

var torus = torusShape{
	major:   1.5,
	minor:   0.5,
	stacksF: 30,
	slicesF: 30,
}

// globeShape is the globe model: its knobs and the buffers its mesh is
// refilled into.
type globeShape struct {
	latF float32

	// spiralF picks how the parallels are drawn: 0 rings, 1 one spiral.
	spiralF float32

	// revF winds that spiral the other way round; twistF skews the
	// meridians into helices, its SIGN choosing which way they lean.
	revF   float32
	twistF float32
	lonF   float32

	// globe.generate runs once per frame, and until now built its mesh into two
	// fresh slices every time. Profiling the TinyGo build in the browser put 72%
	// of all allocation in this one function, and TinyGo's conservative collector
	// then spent about 39% of the CPU scanning for the result — enough to blow the
	// frame budget roughly one frame in six, which showed as a visible stutter.
	//
	// The mesh is the same size on every frame the knobs hold still, so the
	// backing arrays are kept and refilled rather than reallocated. The contents
	// are rewritten from scratch each time, so nothing downstream can tell the
	// difference; only the collector can.
	vertBuf []float32
	idxBuf  []uint16
}

var globe = globeShape{
	latF: 18,
	lonF: 36,
}

func sphereVerticesIndices(radius float32, stacks, slices int, baseIdx uint16) ([]float32, []uint16) {
	// Sized up front. These are called per frame, and a slice grown from nil
	// reallocates about log2(n) times on the way up, every one of which is
	// garbage for a collector that is already the bottleneck here.
	vertices := make([]float32, 0, (stacks+1)*(slices+1)*3)
	indices := make([]uint16, 0, stacks*slices*6)
	for i := 0; i <= stacks; i++ {
		phi := float32(i) * float32(math.Pi) / float32(stacks)
		for j := 0; j <= slices; j++ {
			theta := float32(j) * 2.0 * float32(math.Pi) / float32(slices)
			xv := radius * float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := radius * float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := radius * float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := 0; i < stacks; i++ {
		for j := 0; j <= slices; j++ {
			indices = append(indices, baseIdx+uint16(i*(slices+1)+j), baseIdx+uint16((i+1)*(slices+1)+j)) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
		}
	}
	return vertices, indices
}

// torusVerticesIndices builds the wireframe. roll is a POLOIDAL phase: an
// offset added to phi, which walks each point around the tube's cross-section
// while leaving the torus itself exactly where it is.
//
// The two angles are the two ways round a torus and they have names. theta is
// TOROIDAL — the long way, about the axis through the hole, which is what every
// other spin control already turns. phi is POLOIDAL — the short way, about the
// CORE CIRCLE, the circle drawn through the middle of the tube body. Turning
// phi is the motion a smoke ring makes: the surface rolls through itself and
// the ring stays put.
func torusVerticesIndices(R, r float32, stacks, slices int, baseIdx uint16, roll float32) ([]float32, []uint16) {
	// Sized up front, as above.
	vertices := make([]float32, 0, (stacks+1)*(slices+1)*3)
	indices := make([]uint16, 0, stacks*slices*6)
	for i := 0; i <= stacks; i++ {
		theta := float32(i) * 2.0 * math.Pi / float32(stacks)
		for j := 0; j <= slices; j++ {
			phi := float32(j)*2.0*math.Pi/float32(slices) + roll
			xv := (R + r*float32(math.Cos(float64(phi)))) * float32(math.Cos(float64(theta)))
			yv := (R + r*float32(math.Cos(float64(phi)))) * float32(math.Sin(float64(theta)))
			zv := r * float32(math.Sin(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
		}
	}
	for i := 0; i < stacks; i++ {
		for j := 0; j < slices; j++ {
			cur := baseIdx + uint16(i*(slices+1)+j) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
			next := cur + 1
			below := baseIdx + uint16((i+1)*(slices+1)+j) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
			// Horizontal ring edge
			indices = append(indices, cur, next)
			// Vertical edge
			indices = append(indices, cur, below)
		}
	}
	return vertices, indices
}

func generateSphere() {
	if gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	stacks := int(sphere.stacksF)
	slices := int(sphere.slicesF)
	vertices, indices := sphereVerticesIndices(sphere.radius, stacks, slices, 0)
	gpu.uploadBuffersIndexed(vertices, indices, glctx.Types.Line)
}

func (t *torusShape) generate() {
	stacks := int(t.stacksF)
	slices := int(t.slicesF)
	// The roll advances here rather than in the render loop because it is a
	// property of the GEOMETRY, not of the pose: every other spin turns the
	// model by a matrix and leaves its vertices alone, while this one moves the
	// vertices and leaves the model facing where it was. That also means the
	// mesh has to be rebuilt while it turns, which is why the dirty flag is set
	// only when the rate is non-zero — a still torus stays a cached upload.
	if t.rollF != 0 {
		t.rollPhi += t.rollF / 20
		gpu.staticDirty = true
	}
	if gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	vertices, indices := torusVerticesIndices(t.major, t.minor, stacks, slices, 0, t.rollPhi)
	gpu.uploadBuffersIndexed(vertices, indices, glctx.Types.Line)
}

func (g *globeShape) generate() {
	if gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	lat := int(g.latF)
	lon := int(g.lonF)
	vertices := g.vertBuf[:0]
	indices := g.idxBuf[:0]
	pts := 60 // points per circle

	// The parallels, as rings or as ONE SPIRAL.
	//
	// Rings are what a globe has: lat-1 separate closed circles, none of which
	// joins the next. The spiral is the same journey pole to pole made without
	// lifting the pen — phi runs from one pole to the other while theta winds
	// round lat times — so it is a helix on the sphere rather than a stack of
	// hoops. A globe drawn that way is a ball of string rather than a cage, and
	// the count knob reads as "how many times round" instead of "how many
	// rings".
	if g.spiral() && lat > 0 {
		steps := pts * lat
		base := uint16(len(vertices) / 3) //nolint:gosec // bounded: pts*lat is far under uint16 at lat's cap
		wind := 1.0
		if g.revF >= 0.5 {
			wind = -1 // the same helix wound the other way round
		}
		for k := 0; k <= steps; k++ {
			t := float64(k) / float64(steps)
			phi := t * math.Pi                             // pole to pole, once
			theta := wind * t * 2 * math.Pi * float64(lat) // winding lat times on the way
			vertices = append(vertices,
				float32(math.Sin(phi)*math.Cos(theta)),
				float32(math.Sin(phi)*math.Sin(theta)),
				float32(math.Cos(phi)))
			if k > 0 {
				indices = append(indices, base+uint16(k-1), base+uint16(k)) //nolint:gosec // as above
			}
		}
	} else {
		for i := 1; i < lat; i++ {
			phi := float32(i) * float32(math.Pi) / float32(lat)
			base := uint16(len(vertices) / 3) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
			for j := 0; j <= pts; j++ {
				theta := float32(j) * 2.0 * float32(math.Pi) / float32(pts)
				xv := float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
				yv := float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
				zv := float32(math.Cos(float64(phi)))
				vertices = append(vertices, xv, yv, zv)
				if j > 0 {
					indices = append(indices, base+uint16(j-1), base+uint16(j))
				}
			}
		}
	}

	// The meridians, straight or TWISTED.
	//
	// A meridian normally holds one theta all the way from pole to pole. Twist
	// adds a turn as it descends — theta = theta0 + twist*phi — so each line
	// becomes a helix of its own and the cage weaves instead of caging. Zero is
	// the ordinary globe; the SIGN is which way they lean, which is the whole
	// answer to wanting the spiral to go the other way. This is the meridians'
	// version of what the parallels get from par+dir, and the two compose: a
	// spiral of parallels through a twisted cage is a ball of string.
	twist := float64(g.twistF)
	for j := 0; j < lon; j++ {
		theta0 := float64(j) * 2.0 * math.Pi / float64(lon)
		base := uint16(len(vertices) / 3) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
		for i := 0; i <= pts; i++ {
			phi := float32(i) * float32(math.Pi) / float32(pts)
			theta := float32(theta0 + twist*float64(phi))
			xv := float32(math.Sin(float64(phi))) * float32(math.Cos(float64(theta)))
			yv := float32(math.Sin(float64(phi))) * float32(math.Sin(float64(theta)))
			zv := float32(math.Cos(float64(phi)))
			vertices = append(vertices, xv, yv, zv)
			if i > 0 {
				indices = append(indices, base+uint16(i-1), base+uint16(i))
			}
		}
	}

	g.vertBuf, g.idxBuf = vertices, indices
	gpu.uploadBuffersIndexed(vertices, indices, glctx.Types.Line)
}

// generateMagnetosphere runs per frame and, like globe.generate, used to build
// its mesh into fresh slices each time. Same treatment: keep the arrays and
// refill them.
var (
	magVertBuf []float32
	magIdxBuf  []uint16
)

func generateMagnetosphere() {
	if gpu.staticGeomCached(glctx.Types.Line) {
		return
	}
	allVerts := magVertBuf[:0]
	allIdx := magIdxBuf[:0]

	// Central sphere
	sv, si := sphereVerticesIndices(0.5, 16, 16, 0)
	allVerts = append(allVerts, sv...)
	allIdx = append(allIdx, si...)

	// Magnetic field lines — dipole field: r = R*cos²(θ)
	nLines := 12
	ptsPerLine := 80
	for i := 0; i < nLines; i++ {
		angle := float32(i) * 2.0 * math.Pi / float32(nLines)
		base := uint16(len(allVerts) / 3) //nolint:gosec // a mesh index, bounded by the stack/slice counts a few lines up
		R := float32(3.0)
		for j := 0; j <= ptsPerLine; j++ {
			theta := float32(-math.Pi/2) + float32(j)*float32(math.Pi)/float32(ptsPerLine)
			ct := float32(math.Cos(float64(theta)))
			r := R * ct * ct
			xv := r * ct * float32(math.Cos(float64(angle)))
			yv := r * ct * float32(math.Sin(float64(angle)))
			zv := r * float32(math.Sin(float64(theta)))
			allVerts = append(allVerts, xv, yv, zv)
			if j > 0 {
				allIdx = append(allIdx, base+uint16(j-1), base+uint16(j))
			}
		}
	}

	magVertBuf, magIdxBuf = allVerts, allIdx
	gpu.uploadBuffersIndexed(allVerts, allIdx, glctx.Types.Line)
}

// spiral reports whether the parallels are drawn as a single pole-to-pole
// spiral instead of as separate rings.
func (g *globeShape) spiral() bool { return g.spiralF >= 0.5 }
