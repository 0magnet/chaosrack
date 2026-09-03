// Package starfield flies the viewer forward through a field of stars.
//
// Each star is a point in three dimensions. Every frame its depth shrinks by a
// fixed step, and the point is drawn where a pinhole camera would put it: the
// offset from the axis of travel divided by the depth. That single division is
// what sells the effect. A star far away barely moves; the same star close up
// sweeps across the screen in a few frames, because the divisor is small. The
// eye reads the whole field streaming outward from a fixed point as forward
// motion rather than as drift.
//
// Once a star passes the viewer, or is flung off the edge, it is recycled to a
// new random position at the far plane. The set of stars is therefore allocated
// once and never grows, and the field never thins out.
//
// Written from that description. No code is taken from any existing
// implementation.
package starfield

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// The depth range stars live in. The numbers themselves mean nothing — only
// their ratio does, since everything visible depends on offset divided by
// depth. zNear is deliberately not zero: at zero the division explodes and a
// star would jump from the edge of the screen to somewhere near infinity in a
// single frame.
const (
	zFar  = 1.0
	zNear = 0.05
)

// baseSpeed is how much depth a star loses per second at Speed 1. That is
// about five and a half seconds from the far plane to the viewer, slow enough
// to watch an individual star approach. Expressed per second rather than per
// frame so the field travels at the same rate whatever the frame rate is, and
// so a frame the machine could not deliver on time is skipped over rather
// than turning the whole effect into slow motion.
const baseSpeed = 0.18

type star struct {
	// x and y are the offset from the axis of travel, in units of half the
	// screen: a star at x = 1 sitting at the far plane lands exactly on the
	// right edge. Expressing them this way means the field fills the window
	// whatever its shape, without the aspect ratio appearing anywhere else.
	x, y float64
	// z is the depth, from zFar down to zNear.
	z float64
}

// Starfield is the animation. The zero value is not usable; call New.
type Starfield struct {
	w, h  float64
	stars []star
	rng   *rand.Rand

	// Density is stars per thousand pixels. Counting in pixels rather than
	// fixing a total keeps a maximised window as thick with stars as a small
	// one, instead of leaving a few lonely dots in a large void.
	Density float64
	// Speed multiplies how fast the field approaches. 1 is a comfortable
	// cruise; much above 4 the near stars jump so far between frames that
	// they read as dashes rather than motion.
	Speed float64
	// Palette colours a star by how near it is, dim at the far plane and
	// white as it passes. Its low entries should be dark but not black: a
	// star drawn in black would be a hole in the sky rather than a faint star.
	Palette canvas.Palette
}

// starRamp runs from a dim blue-grey to white. Real stars at the edge of
// vision look colder than bright ones, and the blue tint reads as distance.
var starRamp = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 40, G: 48, B: 72},
	canvas.Stop{At: 0.55, R: 150, G: 160, B: 190},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 255},
)

// New returns a starfield. A seed of 0 gives a fixed sequence, which makes
// tests repeatable.
func New(seed int64) *Starfield {
	return &Starfield{
		rng:     rand.New(rand.NewSource(seed)),
		Density: 70,
		Speed:   1,
		Palette: starRamp,
	}
}

// Resize allocates the star set and scatters it through the whole depth range.
// Scattering rather than starting every star at the far plane matters: born
// together they would arrive together, and the first few seconds would be an
// empty sky followed by a wall.
func (f *Starfield) Resize(w, h int) {
	f.w, f.h = float64(w), float64(h)
	n := int(f.Density * f.w * f.h / 1000)
	if n < 1 {
		n = 1
	}
	f.stars = make([]star, n)
	for i := range f.stars {
		f.spawn(&f.stars[i])
		f.stars[i].z = zNear + f.rng.Float64()*(zFar-zNear)
	}
}

// spawn moves one star to a fresh random position at the far plane.
func (f *Starfield) spawn(st *star) {
	st.x = f.rng.Float64()*2 - 1
	st.y = f.rng.Float64()*2 - 1
	st.z = zFar
}

// project returns where a star lands on the surface, in pixels.
func (f *Starfield) project(st star) (x, y float64) {
	cx, cy := f.w/2, f.h/2
	return cx + st.x/st.z*cx, cy + st.y/st.z*cy
}

// Frame advances the field by dt seconds and draws it.
func (f *Starfield) Frame(s *canvas.Surface, dt float64) {
	if f.w == 0 || f.h == 0 {
		return
	}
	// Stars do not cover the surface, so last frame's sky has to go. Anything
	// short of this leaves trails, and trails at these speeds smear into fog.
	s.Clear()

	step := baseSpeed * f.Speed * dt
	w, h := int(f.w), int(f.h)

	for i := range f.stars {
		st := &f.stars[i]
		st.z -= step
		if st.z <= zNear {
			// Past the viewer.
			f.spawn(st)
		}

		px, py := f.project(*st)
		ix, iy := int(px), int(py)
		if ix < 0 || iy < 0 || ix >= w || iy >= h {
			// Off the edge. Recycling here rather than letting it fly on
			// forever is what keeps the visible count steady; otherwise most
			// of the set would be off-screen at any moment and the sky would
			// look sparse for no apparent reason.
			f.spawn(st)
			continue
		}

		// Brightness falls off with distance the way light does, as one over
		// depth; the square root pulls the far half of the range up out of the
		// floor, where an honest inverse-square would leave every distant star
		// at the same indistinguishable near-black.
		n := math.Sqrt(zNear / st.z)
		v := int(n * 255)
		if v > 255 {
			v = 255
		}
		c := f.Palette[v]
		s.Set(ix, iy, c)

		// A star close enough to rush past subtends more than one pixel. Left
		// as a single dot it would appear to stop growing at the moment it
		// should be filling the view, and the sense of speed goes with it.
		if n > 0.45 {
			s.Set(ix+1, iy, c)
			s.Set(ix, iy+1, c)
			s.Set(ix+1, iy+1, c)
		}
	}
}

// Run draws a starfield on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
