// Package moire is the interference pattern of two overlapping ripples.
//
// Two sets of concentric rings, drawn from centers that drift apart and
// together, and combined. Where crests coincide the sum is bright, where a
// crest meets a trough they cancel — and because the ring spacing is fixed
// while the centers move, the bands of coincidence sweep across the screen far
// faster than either center does. That illusion is the whole effect.
//
// Written from that description. libcaca's cacademo shows one of these; no
// code is taken from it.
package moire

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Moire is the animation. The zero value is not usable; call New.
type Moire struct {
	w, h int
	t    float64

	// dist caches the distance from each pixel to a center, recomputed per
	// frame per center. Two square roots per pixel per frame is the cost of
	// this effect; there is no way around it that still looks like rings.
	scratch []float64

	// Spacing is the distance between rings, in pixels. Smaller is a finer
	// pattern and a faster-moving interference.
	Spacing float64
	// Speed scales how fast the centers drift.
	Speed float64
	// Palette colors the combined amplitude.
	Palette canvas.Palette
}

// New returns a moire animation.
func New() *Moire {
	return &Moire{Spacing: 3, Speed: 1, Palette: canvas.Plasma}
}

// Resize records the size and allocates the scratch row.
func (m *Moire) Resize(w, h int) {
	m.w, m.h = w, h
	m.scratch = make([]float64, w)
}

// Frame advances the centers and draws the interference.
func (m *Moire) Frame(s *canvas.Surface, dt float64) {
	if m.w == 0 || m.h == 0 {
		return
	}
	// 0.6 radians per second: the old 0.02 per frame at 30fps.
	m.t += 0.6 * m.Speed * dt

	fw, fh := float64(m.w), float64(m.h)
	// Two centers orbiting the middle in opposite directions, on ellipses so
	// they do not simply rotate about each other at a fixed separation.
	ax := fw/2 + math.Cos(m.t)*fw*0.3
	ay := fh/2 + math.Sin(m.t*1.3)*fh*0.3
	bx := fw/2 - math.Cos(m.t*0.7)*fw*0.3
	by := fh/2 - math.Sin(m.t)*fh*0.3

	for y := 0; y < m.h; y++ {
		fy := float64(y)
		day, dby := fy-ay, fy-by
		day2, dby2 := day*day, dby*dby
		for x := 0; x < m.w; x++ {
			fx := float64(x)
			dax, dbx := fx-ax, fx-bx
			ra := math.Sqrt(dax*dax + day2)
			rb := math.Sqrt(dbx*dbx + dby2)
			// Each ripple is a sine of distance. Their sum runs -2..2.
			v := math.Sin(ra/m.Spacing) + math.Sin(rb/m.Spacing)
			i := int((v + 2) * 255 / 4)
			if i < 0 {
				i = 0
			} else if i > 255 {
				i = 255
			}
			s.Set(x, y, m.Palette[i])
		}
	}
}

// Run draws a moire pattern on the screen until the user quits.
func Run(screen tcell.Screen) error {
	return canvas.Run(screen, New(), canvas.Options{})
}
