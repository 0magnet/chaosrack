// Package cube is a wireframe solid turning in space.
//
// A wireframe is the cheapest honest 3D there is: a list of corners, a list of
// which corners are joined, and nothing else. Each corner is rotated about
// three axes by angles that advance with elapsed time, then divided by its
// distance from the camera so that near corners spread apart and far ones crowd
// together. Joining the projected corners with straight lines is enough — the
// perspective divide alone makes the box turn.
//
// It is not quite enough, though, because a wireframe drawn in one colour is
// ambiguous: the eye cannot tell which face is towards it, and the box appears
// to flip inside out every second or so. Shading each edge by its depth fixes
// that, and is the reason the near edges here are bright and the far ones dark.
//
// The solid is data, not code, so a tetrahedron or any other polyhedron can be
// swapped in by assigning a different Solid.
//
// Written from that description of the technique; no existing implementation
// was consulted.
package cube

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// camDist is the camera's distance from the centre of the solid, in the units
// the vertices are given in. Near enough that the perspective divide is
// obvious, far enough that a corner swinging towards the viewer does not shoot
// off the screen.
const camDist = 4.0

// Solid is a polyhedron as a wireframe: corner positions, and pairs of indices
// into them saying which corners are joined.
//
// Vertices are in arbitrary units — the animation rescales whatever it is given
// to fill the window, so a solid can be written with whatever coordinates are
// clearest.
type Solid struct {
	Name  string
	Verts [][3]float64
	Edges [][2]int
}

// NewCube returns the unit cube: eight corners at every combination of plus and
// minus one, and the twelve edges that join corners differing in exactly one
// coordinate.
func NewCube() Solid {
	s := Solid{Name: "cube"}
	for i := 0; i < 8; i++ {
		// The three bits of i pick the sign of each coordinate, which enumerates
		// the corners without writing eight triples out by hand.
		s.Verts = append(s.Verts, [3]float64{
			float64(i&1)*2 - 1,
			float64(i>>1&1)*2 - 1,
			float64(i>>2&1)*2 - 1,
		})
	}
	for i := 0; i < 8; i++ {
		for bit := 0; bit < 3; bit++ {
			j := i ^ (1 << bit)
			// Only take each pair once, or every edge is drawn twice.
			if j > i {
				s.Edges = append(s.Edges, [2]int{i, j})
			}
		}
	}
	return s
}

// NewTetrahedron returns a regular tetrahedron, as a demonstration that the
// animation is not specific to the cube: four alternating corners of a cube are
// mutually equidistant, and joining all six pairs of them gives the solid.
func NewTetrahedron() Solid {
	return Solid{
		Name: "tetrahedron",
		Verts: [][3]float64{
			{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1},
		},
		Edges: [][2]int{
			{0, 1}, {0, 2}, {0, 3}, {1, 2}, {1, 3}, {2, 3},
		},
	}
}

// Cube is the animation. The zero value is not usable; call New.
type Cube struct {
	w, h       int
	cx, cy     float64
	scale      float64    // pixels of lateral offset per unit of offset-over-depth
	ax, ay, az float64    // the three rotation angles, in radians
	rates      [3]float64 // radians per second, one per axis

	// Per-vertex projection results and per-edge draw order, sized in Resize so
	// that drawing a frame allocates nothing.
	px, py []float64
	ooz    []float64 // inverse depth per vertex: bigger is nearer
	order  []int
	key    []float64

	// oozNear and oozFar bracket the inverse depths the solid can reach, and
	// are what the depth shading is normalised against. Computed from the
	// vertex radii in Resize; using the actual per-frame range instead would
	// make the brightness breathe as the solid turns.
	oozNear, oozFar float64

	// Solid is the polyhedron drawn. Assigning a different one takes effect at
	// the next Resize, which is where the buffers sized from it are allocated.
	Solid Solid
	// Speed multiplies the turn rates. 1 is about one turn every twelve
	// seconds on the slowest axis.
	Speed float64
	// Fill is the fraction of the shorter side of the surface the solid spans
	// at its widest. Read in Resize.
	Fill float64
	// Floor is the palette index the farthest edge is drawn at, 0 to 1. Zero
	// would make the back of the solid the same colour as the background and
	// the wireframe would look broken rather than deep.
	Floor float64
	// Palette shades the edges by depth, dark at the back and bright at the
	// front.
	Palette canvas.Palette
}

// New returns a rotating cube. The seed picks the three turn rates, so
// different seeds tumble differently; a given seed always produces the same
// animation.
func New(seed int64) *Cube {
	rng := rand.New(rand.NewSource(seed))
	c := &Cube{
		Solid:   NewCube(),
		Speed:   1,
		Fill:    0.82,
		Floor:   0.18,
		Palette: canvas.Matrix,
	}
	// Radians per second, not per frame: the attitude advances by elapsed time,
	// so the solid turns at the same rate whatever the frame rate and whatever
	// the terminal is doing to the tick.
	//
	// Three unequal rates, none a simple multiple of another, so the solid does
	// not return to the same attitude on a short cycle.
	c.rates = [3]float64{
		0.51 + rng.Float64()*0.30,
		0.33 + rng.Float64()*0.24,
		0.21 + rng.Float64()*0.18,
	}
	return c
}

// Resize sizes the per-vertex and per-edge buffers and works out the projection
// scale. Called before the first frame and on every resize.
func (c *Cube) Resize(w, h int) {
	c.w, c.h = w, h
	if w <= 0 || h <= 0 || len(c.Solid.Verts) == 0 {
		return
	}
	c.cx, c.cy = float64(w)/2, float64(h)/2

	n := len(c.Solid.Verts)
	c.px = make([]float64, n)
	c.py = make([]float64, n)
	c.ooz = make([]float64, n)
	c.order = make([]int, len(c.Solid.Edges))
	c.key = make([]float64, len(c.Solid.Edges))

	// The farthest corner from the centre sets both the scale and the depth
	// range: rotation can put any corner anywhere on the sphere of its own
	// radius, so that radius is the whole envelope.
	maxR := 0.0
	for _, v := range c.Solid.Verts {
		if r := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]); r > maxR {
			maxR = r
		}
	}
	if maxR == 0 || maxR >= camDist {
		// Degenerate or larger than the camera distance: nothing sensible to
		// draw, and scaling would divide by zero or project through the camera.
		c.scale = 0
		return
	}

	fill := c.Fill
	if fill <= 0 {
		fill = 0.82
	}
	half := fill * math.Min(float64(w), float64(h)) / 2
	c.scale = half / maxProjRatio(maxR, camDist)

	c.oozNear = 1 / (camDist - maxR)
	c.oozFar = 1 / (camDist + maxR)
}

// maxProjRatio returns the largest lateral-offset-over-depth a point at radius
// rad can reach under any rotation, with the camera at dist.
//
// A rotated corner sits at depth dist-t*rad with a lateral offset of
// rad*sqrt(1-t*t) for some t in -1..1, and the projection is the ratio of the
// two. The maximum is not at t=0 — swinging a corner towards the camera costs
// it depth faster than it costs lateral offset — so scaling by the t=0 value
// would let the solid clip through the edges of the window once per turn.
// Sampled rather than solved, because this runs once per resize.
func maxProjRatio(rad, dist float64) float64 {
	const steps = 512
	best := 0.0
	for i := 0; i <= steps; i++ {
		t := float64(i) / steps
		depth := dist - t*rad
		if depth <= 0.05 {
			break
		}
		if v := rad * math.Sqrt(1-t*t) / depth; v > best {
			best = v
		}
	}
	return best
}

// Frame advances the rotation by dt seconds and draws the wireframe.
func (c *Cube) Frame(s *canvas.Surface, dt float64) {
	if c.w == 0 || c.h == 0 || c.scale == 0 {
		return
	}
	c.ax += c.rates[0] * c.Speed * dt
	c.ay += c.rates[1] * c.Speed * dt
	c.az += c.rates[2] * c.Speed * dt

	s.Clear()
	c.project()

	// Draw far edges first so near ones paint over them where they cross. With
	// only a dozen edges an insertion sort over a preallocated index slice is
	// both faster than the sort package and, unlike it, allocation free.
	for i := range c.order {
		c.order[i] = i
		e := c.Solid.Edges[i]
		c.key[i] = c.ooz[e[0]] + c.ooz[e[1]]
	}
	for i := 1; i < len(c.order); i++ {
		v := c.order[i]
		j := i - 1
		for j >= 0 && c.key[c.order[j]] > c.key[v] {
			c.order[j+1] = c.order[j]
			j--
		}
		c.order[j+1] = v
	}

	for _, i := range c.order {
		e := c.Solid.Edges[i]
		c.line(s, e[0], e[1])
	}
}

// project rotates every vertex and works out where it lands on the surface.
func (c *Cube) project() {
	sinX, cosX := math.Sincos(c.ax)
	sinY, cosY := math.Sincos(c.ay)
	sinZ, cosZ := math.Sincos(c.az)

	for i, v := range c.Solid.Verts {
		x, y, z := v[0], v[1], v[2]

		// About x, then y, then z. Three axes rather than two so the solid
		// keeps presenting new corners instead of settling into a spin about a
		// fixed screen axis.
		y, z = y*cosX-z*sinX, y*sinX+z*cosX
		x, z = x*cosY+z*sinY, -x*sinY+z*cosY
		x, y = x*cosZ-y*sinZ, x*sinZ+y*cosZ

		depth := z + camDist
		if depth <= 0.05 {
			// Behind or level with the camera. Clamped rather than dropped so
			// the vertex still has a usable position and its edges do not
			// vanish; the scale factor keeps this from happening in practice.
			depth = 0.05
		}
		ooz := 1 / depth
		c.ooz[i] = ooz
		c.px[i] = c.cx + c.scale*x*ooz
		c.py[i] = c.cy - c.scale*y*ooz // screen y grows downwards
	}
}

// shade maps an inverse depth to a palette index, bright at the near end of the
// solid's depth range and Floor at the far end.
func (c *Cube) shade(ooz float64) int {
	span := c.oozNear - c.oozFar
	t := 0.0
	if span > 0 {
		t = (ooz - c.oozFar) / span
	}
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	v := int((c.Floor + (1-c.Floor)*t) * 255)
	if v < 0 {
		v = 0
	} else if v > 255 {
		v = 255
	}
	return v
}

// line draws the edge between two projected vertices, shading each pixel by the
// depth interpolated along it.
//
// A DDA rather than Bresenham: the loop counter is already the fraction of the
// way along the edge, which is exactly what the depth interpolation needs, so
// the integer form would only have to recover it again.
func (c *Cube) line(s *canvas.Surface, i, j int) {
	x0, y0 := c.px[i], c.py[i]
	x1, y1 := c.px[j], c.py[j]
	z0, z1 := c.ooz[i], c.ooz[j]

	dx, dy := x1-x0, y1-y0
	// Rounded up, not truncated: truncating would make the step along the
	// dominant axis slightly more than one pixel, and every time the fraction
	// rolled over the line would skip a column and come out dashed.
	steps := int(math.Ceil(math.Max(math.Abs(dx), math.Abs(dy))))
	if steps < 1 {
		// The two ends fell on the same pixel: still draw it, or an edge seen
		// end-on disappears entirely.
		s.Set(int(x0), int(y0), c.Palette[c.shade(z0)])
		return
	}
	fs := float64(steps)
	stepX, stepY := dx/fs, dy/fs
	stepZ := (z1 - z0) / fs

	x, y, z := x0, y0, z0
	for k := 0; k <= steps; k++ {
		s.Set(int(x), int(y), c.Palette[c.shade(z)])
		x += stepX
		y += stepY
		// Interpolating inverse depth linearly across the edge is exactly right
		// under perspective — it is the depth itself that does not interpolate
		// linearly — so this costs nothing in accuracy.
		z += stepZ
	}
}

// Run draws the wireframe on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
