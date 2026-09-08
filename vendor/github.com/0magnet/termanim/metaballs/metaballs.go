// Package metaballs is the merging-blob effect.
//
// Each ball contributes a field that falls off with distance; the field at a
// pixel is the sum of every ball's contribution. Where two balls approach, the
// sums add and the surface between them bulges out and joins, which is what
// makes them look like liquid rather than circles. Coloring by field strength
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
	// r0 is the radius at rest. r is r0 scaled by this ball's band each
	// frame, so the bulge is always measured from the size the ball was
	// placed at rather than compounding off the last frame's.
	r0 float64
}

// Metaballs is the animation. The zero value is not usable; call New.
type Metaballs struct {
	w, h  float64
	balls []ball
	rng   *rand.Rand

	// Count is how many balls to simulate. More is denser and slower; the
	// cost is Count multiplies per pixel per frame.
	Count int
	// Palette colors the field, dim at the edges and bright in the cores.
	Palette canvas.Palette

	// AudioGain scales how hard sound swells the blobs. 1 is the tuned
	// amount, 0 ignores audio even with a source attached.
	AudioGain float64

	// audio is the last sound handed in and env smooths it. See Listen.
	audio canvas.Audio
	env   canvas.Envelope
}

// audioBulge is how much of its resting radius a ball gains at full level on
// its band.
//
// 0.6, so a band at full makes a ball 1.6 times its resting size. The field a
// ball contributes goes as the square of its radius, so that is two and a half
// times the influence — enough that a bulging ball reaches out and merges with
// a neighbor it was clear of, which is the effect worth having. Much more and
// the loud blobs swallow the quiet ones entirely and there is nothing left to
// tell the bands apart.
const audioBulge = 0.6

// New returns a metaballs animation. seed of 0 gives a fixed arrangement,
// which makes tests repeatable.
func New(seed int64) *Metaballs {
	m := &Metaballs{
		rng:       rand.New(rand.NewSource(seed)), //nolint:gosec
		Count:     6,
		Palette:   canvas.Plasma,
		AudioGain: 1,
	}
	// The shortest decay of the four. A radius is read directly — the blob is
	// the size it is this frame, with no integral to hide a jitter in — so
	// separate hits should read as separate pulses rather than as one long
	// swell, and 180 ms lets them: a sixteenth note at 120bpm is 125 ms, so
	// consecutive hits still fall apart. The attack stays near the default,
	// because a radius driven any faster is where the shivering shows first.
	m.env.Attack = 0.045
	m.env.Decay = 0.180
	return m
}

// Listen takes the sound of the coming frame. See canvas.AudioListener.
// Smoothing happens in Frame, where dt is known.
func (m *Metaballs) Listen(a canvas.Audio) { m.audio = a }

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
			r0: r,
		}
	}
}

// bandLevel is the smoothed level of the band ball i answers to, clamped and
// scaled by the gain. Exactly zero when nothing is listening.
func (m *Metaballs) bandLevel(i int) float64 {
	v := m.env.Band(i%canvas.Bands) * m.AudioGain
	if v <= 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Frame moves the balls and draws the field.
func (m *Metaballs) Frame(s *canvas.Surface, dt float64) {
	// Advanced before the size check so that a window with no area still keeps
	// the envelope in step with the music rather than banking it up.
	m.env.Step(m.audio, dt)
	if m.w == 0 || m.h == 0 {
		return
	}
	// Move, bouncing off the edges. Reflecting rather than wrapping keeps the
	// blobs on screen, where a wrap would make one vanish and reappear.
	for i := range m.balls {
		b := &m.balls[i]
		// The mapping is: each ball takes one frequency band, and that band
		// swells its radius.
		//
		// Bands rather than the overall level because the balls are already
		// separate things, and a field that merges them is the one effect here
		// with somewhere to put a spectrum: the bass ball bulges into its
		// neighbors on a kick while the treble ones stay small, so the picture
		// shows the shape of the sound and not just its size. Ball i takes band
		// i, wrapping, so raising Count past canvas.Bands doubles bands up
		// rather than leaving the extra balls deaf.
		//
		// Recomputed from r0 rather than accumulated onto r, so the size is a
		// function of the current band and cannot drift; and at silence the
		// factor is exactly one, which leaves r bit-for-bit the radius Resize
		// chose.
		b.r = b.r0 * (1 + audioBulge*m.bandLevel(i))
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
			// all saturate to the same flat color.
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
