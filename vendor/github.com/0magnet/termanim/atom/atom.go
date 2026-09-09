// Package atom is the textbook atom: electrons on tilted orbits around a
// nucleus, the whole assembly turning slowly in space.
//
// The shape everyone draws is three ellipses crossing at a common center, and
// that shape is a projection rather than a drawing: each orbit here is a real
// circle in three dimensions, tilted about the x-axis and then spun about the
// z-axis by its share of a full turn. Project those circles onto the screen
// and the ellipses fall out — including the fact that an orbit seen edge on
// collapses to a line, which is what makes the assembly read as solid when it
// turns rather than as three overlapping curves.
//
// # WHY IT ACCUMULATES LIGHT INSTEAD OF Z-BUFFERING
//
// donut needs a z-buffer because a torus is opaque: the far wall of the tube
// must not paint over the near one. Nothing here is opaque. An electron is a
// point of light, an orbit is a thread of it, and a nucleus is a cluster of
// glowing spheres, so what is wanted where two of them overlap is the sum,
// not the nearer one. Frame therefore adds into a linear RGB buffer and
// resolves it once at the end, which also gives the trails and the glow for
// free: both are just more light in the same place.
//
// Depth still shows. A point further from the camera projects smaller through
// the perspective divide and is dimmed by an explicit falloff, so an electron
// swinging behind the nucleus recedes and dims even though nothing occludes
// it.
package atom

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// camDist is how far the camera sits from the nucleus, in units of the orbit
// radius. Near enough that the perspective divide visibly separates the near
// and far halves of an orbit, far enough that an electron swinging towards the
// camera does not balloon across the frame.
const camDist = 3.5

// Atom is the animation. The zero value is not usable; call New.
type Atom struct {
	w, h   int
	cx, cy float64
	scale  float64 // pixels per unit of lateral offset per unit of depth

	a, b         float64 // tumble angles, in radians
	rateA, rateB float64 // radians per second

	shells   []shell
	nucleons []nucleon
	t        float64 // elapsed seconds, for the nucleus jitter

	// acc is linear light per pixel, three floats each. Resolved to colors at
	// the end of Frame. Kept across frames only to avoid reallocating.
	acc []float64

	// ringSamples is how finely an orbit is walked, chosen in Resize so
	// consecutive samples land under a pixel apart.
	ringSamples int

	// Speed multiplies every orbital rate. 1 is a little under two seconds for
	// the innermost electron to come round.
	Speed float64
	// Tumble multiplies the rate the whole assembly turns at. 0 holds it
	// still, which is the textbook diagram; 1 turns it slowly enough that the
	// orbits are readable throughout.
	Tumble float64
	// Fill is the fraction of the shorter side of the surface the orbits span
	// at their widest. Read in Resize.
	Fill float64
	// TrailLength is how far behind each electron the trail is drawn, in
	// radians of its own orbit. Zero draws a bare point.
	TrailLength float64
	// OrbitLight is the light an orbit thread contributes at the nearest point
	// of its sweep, before the depth falloff. Low: the orbits are scaffolding
	// and should not compete with what moves along them.
	OrbitLight [3]float64
	// ElectronLight is the light at the center of an electron.
	ElectronLight [3]float64
	// NucleonLight is the two colors nucleons alternate between — the
	// convention that makes a nucleus read as two kinds of particle rather
	// than as one lumpy ball.
	NucleonLight [2][3]float64
}

// shell is one orbit: a circle of the given radius spanned by two orthonormal
// vectors, walked by however many electrons it carries.
type shell struct {
	ux, uy, uz float64
	vx, vy, vz float64
	radius     float64
	rate       float64   // radians per second
	phase      []float64 // one angle per electron on this shell
}

// nucleon is one particle in the nucleus: a rest position, a jitter amplitude
// and a frequency of its own so the cluster shimmers instead of pulsing as a
// unit.
type nucleon struct {
	x, y, z    float64
	ax, ay, az float64
	fx, fy, fz float64
	kind       int
}

// New returns an atom with the classic three orbits and one electron on each.
//
// The seed picks the orbital rates, the tumble, and where the nucleons sit, so
// different seeds give a different atom that behaves the same way; a given
// seed always produces the same animation.
func New(seed int64) *Atom {
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec
	a := &Atom{
		// Radians per second, so the orbits keep their rate whatever the frame
		// rate is doing.
		rateA: 0.21 + rng.Float64()*0.13,
		rateB: 0.13 + rng.Float64()*0.11,
		b:     rng.Float64() * 2 * math.Pi,

		Speed:       1,
		Tumble:      1,
		Fill:        0.95,
		TrailLength: 0.55,

		// Cool for the scaffolding, cooler and much brighter for what runs
		// along it, warm for the nucleus. The nucleus is the only warm thing
		// in the frame, which is what keeps the eye on it.
		OrbitLight:    [3]float64{46, 92, 168},
		ElectronLight: [3]float64{150, 225, 255},
		NucleonLight: [2][3]float64{
			{255, 96, 64},
			{255, 176, 72},
		},
	}
	a.shells = newShells(rng, 3, 1)
	a.nucleons = newNucleus(rng, 14)
	return a
}

// newShells builds n orbits, each tilted the same amount out of the xy-plane
// and spun about z by its share of a half turn.
//
// A half turn and not a full one: an orbit and the same orbit rotated by pi is
// the same circle, so spreading n orbits over 2pi would draw the second half
// of them on top of the first.
func newShells(rng *rand.Rand, n, perShell int) []shell {
	// Far enough from face on that the ellipse is clearly an ellipse, far
	// enough from edge on that it does not spend most of the tumble as a line.
	const tilt = 72 * math.Pi / 180
	sinT, cosT := math.Sincos(tilt)

	shells := make([]shell, n)
	for i := range shells {
		ang := math.Pi * float64(i) / float64(n)
		sinA, cosA := math.Sincos(ang)
		s := shell{
			// The xy-plane basis (1,0,0),(0,1,0) taken through a rotation about
			// x by tilt and then about z by ang.
			ux: cosA, uy: sinA, uz: 0,
			vx: -cosT * sinA, vy: cosT * cosA, vz: sinT,
			radius: 1,
			// Incommensurate rates, so the electrons do not fall into step and
			// turn the motion into a short loop.
			rate:  1.9 + rng.Float64()*1.5,
			phase: make([]float64, perShell),
		}
		// Half the shells run the other way. All of them turning together
		// reads as one rigid object being spun; opposed directions read as
		// separate orbits.
		if i%2 == 1 {
			s.rate = -s.rate
		}
		for j := range s.phase {
			s.phase[j] = rng.Float64()*2*math.Pi + 2*math.Pi*float64(j)/float64(perShell)
		}
		shells[i] = s
	}
	return shells
}

// newNucleus scatters n nucleons in a small ball, each with its own jitter.
//
// The positions are rejection sampled rather than placed on a sphere: a
// hollow shell of nucleons projects to a ring, and a ring is exactly what the
// orbits already are.
func newNucleus(rng *rand.Rand, n int) []nucleon {
	const radius = 0.15
	out := make([]nucleon, 0, n)
	for len(out) < n {
		x := rng.Float64()*2 - 1
		y := rng.Float64()*2 - 1
		z := rng.Float64()*2 - 1
		if x*x+y*y+z*z > 1 {
			continue
		}
		out = append(out, nucleon{
			x: x * radius, y: y * radius, z: z * radius,
			ax:   0.012 + rng.Float64()*0.014,
			ay:   0.012 + rng.Float64()*0.014,
			az:   0.012 + rng.Float64()*0.014,
			fx:   1.1 + rng.Float64()*2.2,
			fy:   1.1 + rng.Float64()*2.2,
			fz:   1.1 + rng.Float64()*2.2,
			kind: len(out) % 2,
		})
	}
	return out
}

// Resize allocates the light buffer and fixes the projection scale. Called
// before the first frame and on every resize.
func (a *Atom) Resize(w, h int) {
	a.w, a.h = w, h
	if w <= 0 || h <= 0 {
		return
	}
	a.cx, a.cy = float64(w)/2, float64(h)/2
	a.acc = make([]float64, w*h*3)

	fill := a.Fill
	if fill <= 0 {
		fill = 0.95
	}
	maxR := 0.0
	for _, s := range a.shells {
		if s.radius > maxR {
			maxR = s.radius
		}
	}
	if maxR <= 0 {
		maxR = 1
	}
	// The widest an orbit ever projects is at the point of it nearest the
	// camera, which is maxR closer than the nucleus.
	half := fill * math.Min(float64(w), float64(h)) / 2
	a.scale = half * (camDist - maxR) / maxR

	// One sample per half pixel around the longest orbit, measured where the
	// perspective divide magnifies most.
	pxPerUnit := a.scale / (camDist - maxR)
	a.ringSamples = clampInt(int(2*2*math.Pi*maxR*pxPerUnit), 180, 3000)
}

// Frame advances the orbits and the tumble by dt and redraws.
func (a *Atom) Frame(s *canvas.Surface, dt float64) {
	if a.w == 0 || a.h == 0 || len(a.acc) == 0 {
		return
	}
	a.t += dt
	a.a += a.rateA * a.Tumble * dt
	a.b += a.rateB * a.Tumble * dt
	for i := range a.shells {
		a.shells[i].advance(a.Speed * dt)
	}

	for i := range a.acc {
		a.acc[i] = 0
	}

	sinA, cosA := math.Sincos(a.a)
	sinB, cosB := math.Sincos(a.b)
	rot := func(x, y, z float64) [3]float64 {
		// About x, then about y. Two axes are enough to bring every orbit
		// through every orientation; a third would only re-spin the image in
		// the plane of the screen.
		y1 := y*cosA - z*sinA
		z1 := y*sinA + z*cosA
		x2 := x*cosB + z1*sinB
		z2 := -x*sinB + z1*cosB
		return [3]float64{x2, y1, z2}
	}

	a.drawOrbits(rot)
	a.drawNucleus(rot)
	a.drawElectrons(rot)
	a.resolve(s)
}

// advance moves every electron on the shell forward, keeping the phases in
// [0, 2pi) so they cannot drift into the range where float64 stops resolving
// small increments.
func (s *shell) advance(dt float64) {
	for i := range s.phase {
		s.phase[i] = wrapTau(s.phase[i] + s.rate*dt)
	}
}

// at returns the point at the given angle on the shell's circle.
func (s *shell) at(ang float64) (x, y, z float64) {
	sn, cs := math.Sincos(ang)
	return s.radius * (cs*s.ux + sn*s.vx),
		s.radius * (cs*s.uy + sn*s.vy),
		s.radius * (cs*s.uz + sn*s.vz)
}

// drawOrbits walks each ring, weighting every sample by how far it moved on
// screen since the one before it.
//
// Without that weighting an orbit seen edge on is a bright white bar. The ring
// is sampled at a fixed number of angles, so foreshortening does not thin the
// samples out — it piles them into fewer pixels, and light adds. Face on they
// spread over the whole ellipse and the thread stays dim; edge on they all
// land on one line and it blows out. Scaling each by the on-screen step makes
// the thread equally bright per unit of its length whatever angle it is seen
// from, which is what a thread of light actually does.
func (a *Atom) drawOrbits(rot func(x, y, z float64) [3]float64) {
	for i := range a.shells {
		s := &a.shells[i]
		var prevX, prevY float64
		havePrev := false
		// One sample past the end, to close the ring.
		for k := 0; k <= a.ringSamples; k++ {
			x, y, z := s.at(2 * math.Pi * float64(k) / float64(a.ringSamples))
			px, py, fall, ok := a.project(rot(x, y, z))
			if !ok {
				havePrev = false
				continue
			}
			if havePrev {
				// Capped at one pixel: a step longer than that is a gap in the
				// thread, not a brighter piece of it.
				step := math.Hypot(px-prevX, py-prevY)
				if step > 1 {
					step = 1
				}
				a.add(int(px), int(py), a.OrbitLight, fall*step)
			}
			prevX, prevY, havePrev = px, py, true
		}
	}
}

func (a *Atom) drawNucleus(rot func(x, y, z float64) [3]float64) {
	for i := range a.nucleons {
		n := &a.nucleons[i]
		// Each nucleon wanders on its own three-frequency Lissajous path. The
		// amplitudes are a fraction of the nucleus radius, so the cluster
		// shimmers without any particle leaving it.
		x := n.x + n.ax*math.Sin(n.fx*a.t)
		y := n.y + n.ay*math.Sin(n.fy*a.t+1.7)
		z := n.z + n.az*math.Sin(n.fz*a.t+3.1)
		a.splat(rot(x, y, z), 0.075, a.NucleonLight[n.kind])
	}
}

func (a *Atom) drawElectrons(rot func(x, y, z float64) [3]float64) {
	trail := a.TrailLength
	if trail < 0 {
		trail = 0
	}
	for i := range a.shells {
		s := &a.shells[i]
		// A trail is drawn behind the electron along the orbit it came from,
		// which is why it bends: it is the orbit, not a straight smear.
		steps := int(trail * 30)
		dir := 1.0
		if s.rate < 0 {
			dir = -1
		}
		for _, ph := range s.phase {
			for k := steps; k >= 1; k-- {
				f := float64(k) / float64(steps+1)
				x, y, z := s.at(ph - dir*trail*f)
				// Cubed, so the trail fades away quickly near its tail and the
				// electron stays clearly the head of it.
				w := (1 - f) * (1 - f) * (1 - f)
				a.splat(rot(x, y, z), 0, scaleLight(a.ElectronLight, w*0.34))
			}
			// A wide dim halo under a tight bright core. One splat cannot do
			// both: at this resolution a disc large enough to be seen from
			// across the frame is a smudge, and one small enough to be a
			// particle disappears against the orbit it is sitting on.
			p := rot(s.at(ph))
			a.splat(p, 0.17, scaleLight(a.ElectronLight, 0.16))
			a.splat(p, 0.06, a.ElectronLight)
		}
	}
}

// project maps a rotated point to the screen and reports how bright it should
// be for its depth. ok is false for anything at or behind the camera.
//
// The falloff is inverse square in the depth, normalized so a point at the
// nucleus is at full brightness. That is the depth cue the whole effect leans
// on: nothing occludes anything here, so the far half of an orbit has to look
// further away rather than be hidden.
func (a *Atom) project(p [3]float64) (px, py, fall float64, ok bool) {
	depth := camDist + p[2]
	if depth <= 0.15 {
		return 0, 0, 0, false
	}
	ooz := 1 / depth
	fall = (camDist * camDist) * ooz * ooz
	if fall > 4 {
		fall = 4
	}
	return a.cx + a.scale*p[0]*ooz, a.cy - a.scale*p[1]*ooz, fall, true
}

// splat adds one point of light to the buffer.
//
// radius is in the same units as the orbit radius and shrinks with distance
// like everything else; zero means a single pixel, which is what a trail
// wants. Anything larger is drawn as a disc whose brightness falls off from
// the middle, because a hard-edged disc a few pixels across on a half-block
// surface reads as a square.
func (a *Atom) splat(p [3]float64, radius float64, light [3]float64) {
	px, py, fall, ok := a.project(p)
	if !ok {
		return
	}

	r := radius * a.scale / (camDist + p[2])
	if r < 0.5 {
		a.add(int(px), int(py), light, fall)
		return
	}
	x0, x1 := int(px-r), int(px+r)+1
	y0, y1 := int(py-r), int(py+r)+1
	inv := 1 / (r * r)
	for yy := y0; yy <= y1; yy++ {
		for xx := x0; xx <= x1; xx++ {
			dx, dy := float64(xx)+0.5-px, float64(yy)+0.5-py
			d2 := (dx*dx + dy*dy) * inv
			if d2 >= 1 {
				continue
			}
			w := 1 - d2
			a.add(xx, yy, light, fall*w*w)
		}
	}
}

func (a *Atom) add(x, y int, light [3]float64, w float64) {
	if x < 0 || y < 0 || x >= a.w || y >= a.h || w <= 0 {
		return
	}
	i := (y*a.w + x) * 3
	a.acc[i] += light[0] * w
	a.acc[i+1] += light[1] * w
	a.acc[i+2] += light[2] * w
}

// resolve turns accumulated light into colors.
//
// A pixel with no light at all is left alone rather than set to black, so the
// animation composites over whatever the host has behind it instead of
// punching a rectangle out of it.
func (a *Atom) resolve(s *canvas.Surface) {
	s.Clear()
	w, h := s.Size()
	if w > a.w {
		w = a.w
	}
	if h > a.h {
		h = a.h
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*a.w + x) * 3
			r, g, b := a.acc[i], a.acc[i+1], a.acc[i+2]
			if r+g+b < 1 {
				continue
			}
			s.Set(x, y, tcell.NewRGBColor(clamp255(r), clamp255(g), clamp255(b)))
		}
	}
}

func scaleLight(l [3]float64, k float64) [3]float64 {
	return [3]float64{l[0] * k, l[1] * k, l[2] * k}
}

func clamp255(v float64) int32 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return int32(v)
}

func wrapTau(v float64) float64 {
	const tau = 2 * math.Pi
	v = math.Mod(v, tau)
	if v < 0 {
		v += tau
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Run draws an atom on the given screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
