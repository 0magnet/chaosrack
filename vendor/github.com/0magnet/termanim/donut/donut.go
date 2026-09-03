// Package donut is a lit torus turning in space.
//
// A torus is two circles: sweep a small circle of radius r (the tube, walked by
// theta) around a larger circle of radius R (the hole, walked by phi). Every
// (theta, phi) names one point on the surface, and — because the tube is a
// circle — the outward normal at that point is simply the direction from the
// tube's centre to it. Getting the normal for free is why this shape, rather
// than any other, is the one everybody draws: shading needs a normal, and here
// it falls out of the parametrisation.
//
// The surface points are rotated by two angles that advance with elapsed
// time, divided by their distance from the camera for perspective, and dropped
// into pixels. Two things turn that from a smear into a solid: a z-buffer, so the
// far wall of the tube cannot paint over the near wall, and a dot product
// between each rotated normal and a fixed light, so the side facing the light
// is bright and the side facing away is not. Without the z-buffer the shape is
// unreadable; without the lighting it is a flat silhouette.
//
// Written from that description of the technique. No existing implementation
// was consulted; in particular none of the well-known obfuscated C version is
// reproduced here.
package donut

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// camDist is how far the camera sits from the centre of the torus, in the same
// units as the radii below. Close enough that the perspective divide visibly
// tapers the far side, far enough that the near side does not balloon and
// swallow the frame.
const camDist = 4.0

// Donut is the animation. The zero value is not usable; call New.
type Donut struct {
	w, h     int
	cx, cy   float64   // centre of the surface, in pixels
	scale    float64   // pixels per unit of lateral offset per unit of depth
	a, b     float64   // the two rotation angles, in radians
	rateA    float64   // radians per second
	rateB    float64   // radians per second
	zbuf     []float64 // one inverse depth per pixel; see Frame
	thetaSin []float64 // sin/cos of every sampled angle, built once in Resize
	thetaCos []float64
	phiSin   []float64
	phiCos   []float64

	// Speed multiplies the tumble rates. 1 is about one turn every ten seconds
	// on the slower axis.
	Speed float64
	// Fill is the fraction of the shorter side of the surface the torus spans
	// at its widest. Read in Resize.
	Fill float64
	// TubeRatio is the tube radius as a fraction of the hole radius. Below
	// about 0.2 the tube is a thread; above about 0.8 the hole closes up.
	// Read in Resize.
	TubeRatio float64
	// Light is the direction the light comes from, in the torus's own frame
	// after rotation: +y is up the screen and -z is towards the camera. It
	// should be unit length, or the shading will clip or never reach full
	// brightness.
	Light [3]float64
	// Ambient is the brightness of the unlit side, 0 to 1. Zero would leave
	// half the torus at palette index 0 and indistinguishable from the
	// background, which loses the silhouette.
	Ambient float64
	// Palette shades the surface, dark at the terminator and bright where the
	// light is head on.
	Palette canvas.Palette
}

// New returns a torus. The seed picks the two tumble rates, so different seeds
// present the shape from a different sequence of angles; a given seed always
// produces the same animation.
func New(seed int64) *Donut {
	rng := rand.New(rand.NewSource(seed))
	return &Donut{
		// Radians per second, not per frame: the pose advances by elapsed time,
		// so the torus turns at the same rate whatever the frame rate and
		// whatever the terminal is doing to the tick.
		//
		// Rates deliberately incommensurate: if the two were a simple ratio
		// the torus would retrace the same path and the motion would read as a
		// short loop rather than as tumbling.
		rateA: 0.63 + rng.Float64()*0.42,
		rateB: 0.33 + rng.Float64()*0.27,

		// At a=0 the torus lies in the plane the camera looks along and is seen
		// exactly edge on, as a bar with no hole. Starting part way round means
		// the very first frame already reads as a donut. b is a spin in the
		// plane of the screen and can start anywhere.
		a: 0.9 + rng.Float64()*0.8,
		b: rng.Float64() * 2 * math.Pi,

		Speed:     1,
		Fill:      0.9,
		TubeRatio: 0.4,
		// Up, to the left and towards the camera. Straight-on light flattens
		// the shape because the brightest point lands in the middle of the
		// silhouette; off-axis light puts the highlight on one flank and lets
		// the eye read which way the surface is curving.
		Light:   [3]float64{-0.4082, 0.8165, -0.4082},
		Ambient: 0.12,
		Palette: canvas.Fire,
	}
}

// radii returns the hole and tube radii for the current TubeRatio, normalised
// so their sum is 1. Fixing the outer extent means changing TubeRatio changes
// the shape without changing how much of the window it occupies.
func (d *Donut) radii() (bigR, tubeR float64) {
	t := d.TubeRatio
	if t < 0.05 {
		t = 0.05
	} else if t > 0.95 {
		t = 0.95
	}
	return 1 / (1 + t), t / (1 + t)
}

// Resize allocates the z-buffer and the angle tables. Called before the first
// frame and on every resize.
//
// Everything here depends only on the size, so Frame can be pure arithmetic:
// the tables mean no trigonometry per surface point, and the z-buffer is the
// one big allocation this effect needs.
func (d *Donut) Resize(w, h int) {
	d.w, d.h = w, h
	if w <= 0 || h <= 0 {
		return
	}
	d.cx, d.cy = float64(w)/2, float64(h)/2
	d.zbuf = make([]float64, w*h)

	bigR, tubeR := d.radii()
	fill := d.Fill
	if fill <= 0 {
		fill = 0.9
	}

	// The torus is scaled to a target half-width in pixels. The projected
	// half-width is not simply scale*(R+r)/camDist: a point swung towards the
	// camera loses depth faster than it loses lateral offset, so the widest the
	// silhouette ever gets is a little more than that. Dividing by the true
	// maximum keeps the shape inside the window at every angle instead of
	// clipping once per turn.
	half := fill * math.Min(float64(w), float64(h)) / 2
	d.scale = half / maxProjRatio(bigR+tubeR, camDist)

	// Sample spacing. The surface is drawn as points, so if consecutive samples
	// land more than a pixel apart the tube fills with holes and the light
	// shows through. pxPerUnit is measured at the centre depth; sampling three
	// times per pixel there leaves nearly two per pixel on the near face, where
	// the perspective divide magnifies everything.
	pxPerUnit := d.scale / camDist
	nPhi := clampInt(int(3*2*math.Pi*(bigR+tubeR)*pxPerUnit), 96, 1024)
	nTheta := clampInt(int(3*2*math.Pi*tubeR*pxPerUnit), 48, 512)

	d.phiSin, d.phiCos = angleTable(nPhi)
	d.thetaSin, d.thetaCos = angleTable(nTheta)
}

// angleTable returns sin and cos sampled at n evenly spaced angles over a full
// turn.
func angleTable(n int) (sin, cos []float64) {
	sin, cos = make([]float64, n), make([]float64, n)
	for i := 0; i < n; i++ {
		sin[i], cos[i] = math.Sincos(2 * math.Pi * float64(i) / float64(n))
	}
	return sin, cos
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

// maxProjRatio returns the largest lateral-offset-over-depth that a point at
// radius rad can reach under any rotation, with the camera at dist.
//
// Such a point sits at depth dist-c*rad with a lateral offset of
// rad*sqrt(1-c*c), for some c in -1..1, and the projection is the ratio of the
// two. The maximum is not at c=0; it is somewhere in front of the centre, which
// is why the naive scale factor lets a shape spill off the edges as it turns.
// Sampled rather than solved because this runs once per resize.
func maxProjRatio(rad, dist float64) float64 {
	const steps = 512
	best := 0.0
	for i := 0; i <= steps; i++ {
		c := float64(i) / steps
		depth := dist - c*rad
		if depth <= 0.05 {
			break
		}
		if v := rad * math.Sqrt(1-c*c) / depth; v > best {
			best = v
		}
	}
	return best
}

// Frame advances the rotation by dt seconds and draws the torus.
func (d *Donut) Frame(s *canvas.Surface, dt float64) {
	if d.w == 0 || d.h == 0 || len(d.zbuf) == 0 {
		return
	}
	d.a += d.rateA * d.Speed * dt
	d.b += d.rateB * d.Speed * dt

	s.Clear()
	// Zero means "nothing here yet": the buffer holds inverse depths, which are
	// strictly positive for anything in front of the camera, and larger for
	// nearer points. Comparing inverse depths rather than depths avoids a
	// divide per comparison and makes the empty case the natural zero.
	for i := range d.zbuf {
		d.zbuf[i] = 0
	}

	bigR, tubeR := d.radii()
	sinA, cosA := math.Sincos(d.a)
	sinB, cosB := math.Sincos(d.b)
	lx, ly, lz := d.Light[0], d.Light[1], d.Light[2]
	amb := d.Ambient

	for pi := range d.phiCos {
		sp, cp := d.phiSin[pi], d.phiCos[pi]
		for ti := range d.thetaCos {
			st, ct := d.thetaSin[ti], d.thetaCos[ti]

			// A point on the tube, then swung around the hole by phi. ring is
			// the point's distance from the torus's axis of symmetry.
			ring := bigR + tubeR*ct
			x0 := ring * cp
			y0 := tubeR * st
			z0 := ring * sp

			// The outward normal is the same construction with the tube's
			// centre at the origin, which leaves it unit length for free.
			nx0 := ct * cp
			ny0 := st
			nz0 := ct * sp

			// Rotate about x, then about z. Two axes are enough to bring every
			// part of the surface into view; a third would only re-spin the
			// image in the plane of the screen.
			y1 := y0*cosA - z0*sinA
			z1 := y0*sinA + z0*cosA
			ny1 := ny0*cosA - nz0*sinA
			nz1 := ny0*sinA + nz0*cosA

			x2 := x0*cosB - y1*sinB
			y2 := x0*sinB + y1*cosB
			nx2 := nx0*cosB - ny1*sinB
			ny2 := nx0*sinB + ny1*cosB

			// Rotation about z leaves z alone, so z1 is the final depth.
			depth := z1 + camDist
			if depth <= 0.05 {
				continue
			}
			ooz := 1 / depth

			sx := int(d.cx + d.scale*x2*ooz)
			sy := int(d.cy - d.scale*y2*ooz) // screen y grows downwards
			if sx < 0 || sy < 0 || sx >= d.w || sy >= d.h {
				continue
			}
			i := sy*d.w + sx
			// The far wall of the tube projects onto the same pixels as the
			// near wall and, for half the sweep, is visited later. Without this
			// test it would overwrite the near wall and the torus would look
			// inside out.
			if ooz <= d.zbuf[i] {
				continue
			}
			d.zbuf[i] = ooz

			// How square-on this bit of surface faces the light. Negative means
			// it faces away, and gets the ambient floor rather than going to
			// black, so the unlit limb still reads as part of the shape.
			lum := nx2*lx + ny2*ly + nz1*lz
			if lum < 0 {
				lum = 0
			}
			v := int((amb + (1-amb)*lum) * 255)
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			s.Set(sx, sy, d.Palette[v])
		}
	}
}

// Run draws the torus on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
