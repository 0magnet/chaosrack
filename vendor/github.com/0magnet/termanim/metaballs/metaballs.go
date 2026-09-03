// Package metaballs is the merging-blob effect.
//
// Each ball contributes a field that falls off with distance; the field at a
// pixel is the sum of every ball's contribution. Where two balls approach, the
// sums add and the surface between them bulges out and joins, which is what
// makes them look like liquid rather than circles. Colouring by field strength
// instead of thresholding gives the soft halo.
//
// Written from that description. libcaca's cacademo shows one of these; no
// code is taken from it.
package metaballs

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

type ball struct {
	x, y   float64 // position in pixels
	dx, dy float64 // velocity in pixels per frame
	r      float64 // radius: how far this ball's influence reaches
}

// Metaballs is the animation. The zero value is not usable; call New.
type Metaballs struct {
	w, h  float64
	balls []ball
	rng   *rand.Rand

	// Count is how many balls to simulate. More is denser and slower; the
	// cost is Count multiplies per pixel per frame.
	Count int
	// Palette colours the field, dim at the edges and bright in the cores.
	Palette canvas.Palette
}

// New returns a metaballs animation. seed of 0 gives a fixed arrangement,
// which makes tests repeatable.
func New(seed int64) *Metaballs {
	return &Metaballs{
		rng:     rand.New(rand.NewSource(seed)),
		Count:   6,
		Palette: canvas.Plasma,
	}
}

// Resize places the balls. Called before the first frame and on every resize.
func (m *Metaballs) Resize(w, h int) {
	m.w, m.h = float64(w), float64(h)
	if m.Count < 1 {
		m.Count = 1
	}
	m.balls = make([]ball, m.Count)
	for i := range m.balls {
		// Radius scales with the surface so the blobs occupy the same
		// fraction of the screen whatever the window size.
		r := (0.18 + m.rng.Float64()*0.12) * math.Min(m.w, m.h)
		// Velocities are pixels per second: a blob crosses the window in
		// roughly seven seconds at full tilt. These were w/200 per frame when
		// motion was counted in frames at 30 fps.
		m.balls[i] = ball{
			x:  m.rng.Float64() * m.w,
			y:  m.rng.Float64() * m.h,
			dx: (m.rng.Float64()*2 - 1) * m.w * 30 / 200,
			dy: (m.rng.Float64()*2 - 1) * m.h * 30 / 200,
			r:  r,
		}
	}
}

// Frame moves the balls and draws the field.
func (m *Metaballs) Frame(s *canvas.Surface, dt float64) {
	if m.w == 0 || m.h == 0 {
		return
	}
	// Move, bouncing off the edges. Reflecting rather than wrapping keeps the
	// blobs on screen, where a wrap would make one vanish and reappear.
	for i := range m.balls {
		b := &m.balls[i]
		b.x += b.dx * dt
		b.y += b.dy * dt
		if b.x < 0 {
			b.x, b.dx = 0, -b.dx
		} else if b.x > m.w {
			b.x, b.dx = m.w, -b.dx
		}
		if b.y < 0 {
			b.y, b.dy = 0, -b.dy
		} else if b.y > m.h {
			b.y, b.dy = m.h, -b.dy
		}
	}

	w, h := int(m.w), int(m.h)
	for y := 0; y < h; y++ {
		fy := float64(y)
		for x := 0; x < w; x++ {
			fx := float64(x)
			var field float64
			for i := range m.balls {
				b := &m.balls[i]
				dx, dy := fx-b.x, fy-b.y
				// Inverse-square falloff, in units of the ball's radius. The
				// squared distance is used directly: taking a square root per
				// ball per pixel would be the most expensive thing here and
				// changes nothing about how it looks.
				d2 := (dx*dx + dy*dy) / (b.r * b.r)
				if d2 < 0.0001 {
					d2 = 0.0001
				}
				field += 1 / d2
			}
			// field is unbounded near a core. Compress it so the cores do not
			// all saturate to the same flat colour.
			v := int(255 * field / (field + 3))
			if v > 255 {
				v = 255
			}
			// An inverse-square field never reaches zero, so without a cutoff
			// every pixel on the screen ends up faintly lit and the blobs stop
			// floating. This is set above the far-field level but well below
			// the value at a ball's own radius, which is about 64.
			if v < 32 {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			s.Set(x, y, m.Palette[v])
		}
	}
}

// Run draws metaballs on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
