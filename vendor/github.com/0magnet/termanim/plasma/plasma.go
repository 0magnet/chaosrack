// Package plasma is the drifting coloured field of the demoscene.
//
// The method predates the terminal by decades: sum a few sine waves of the
// pixel coordinates, offset each by time, and use the total as an index into a
// looping colour ramp. Because the sines never align the same way twice the
// field appears to seethe without ever repeating visibly, and because the ramp
// closes on itself there is no seam where the sum wraps.
//
// Written from that description. libcaca's cacademo shows one of these; no
// code is taken from it.
package plasma

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Plasma is the animation. The zero value is not usable; call New.
type Plasma struct {
	w, h int
	t    float64

	// sinTab is a sine lookup over one full turn, in the same 0..255 units the
	// palette is indexed by. Trigonometry per pixel per frame is the one thing
	// that makes this effect expensive; a table makes it addition.
	sinTab [tabSize]float64

	// Speed scales how fast the field drifts. 1 is a slow seethe.
	Speed float64
	// Palette must loop — its last entry should match its first, or a seam
	// appears wherever the summed value wraps past 255.
	Palette canvas.Palette
}

const tabSize = 1024

// New returns a plasma.
func New() *Plasma {
	p := &Plasma{Speed: 1, Palette: canvas.Plasma}
	for i := range p.sinTab {
		p.sinTab[i] = math.Sin(float64(i) / tabSize * 2 * math.Pi)
	}
	return p
}

// sin looks up sin(turns) where turns is in revolutions rather than radians,
// wrapping automatically.
func (p *Plasma) sin(turns float64) float64 {
	i := int(turns*tabSize) % tabSize
	if i < 0 {
		i += tabSize
	}
	return p.sinTab[i]
}

// Resize records the surface size. Nothing is allocated: the field is a pure
// function of position and time, so there is no buffer to keep.
func (p *Plasma) Resize(w, h int) { p.w, p.h = w, h }

// Frame advances time and draws the field.
func (p *Plasma) Frame(s *canvas.Surface, dt float64) {
	// 0.12 turns per second: the old 0.004 per frame at 30fps.
	p.t += 0.12 * p.Speed * dt
	if p.w == 0 || p.h == 0 {
		return
	}

	// Scale so the pattern has roughly the same number of bands whatever the
	// window size, rather than becoming a single blur when small and a fine
	// stipple when large.
	fw := 6 / float64(p.w)
	fh := 6 / float64(p.h)

	for y := 0; y < p.h; y++ {
		fy := float64(y) * fh
		// The two terms that depend only on y are lifted out of the inner
		// loop; at this pixel count that is most of the work.
		a := p.sin(fy + p.t*1.3)
		b := p.sin(fy*0.5 - p.t*0.7)
		for x := 0; x < p.w; x++ {
			fx := float64(x) * fw
			v := a +
				p.sin(fx+p.t) +
				b +
				// A diagonal term, so the field is not merely the sum of a
				// horizontal and a vertical wave — that reads as a grid.
				p.sin((fx+fy)*0.4+p.t*1.7)
			// v is in -4..4. Fold it into 0..255.
			i := int((v + 4) * 255 / 8)
			if i < 0 {
				i = 0
			} else if i > 255 {
				i = 255
			}
			s.Set(x, y, p.Palette[i])
		}
	}
}

// Run draws plasma on the screen until the user quits.
func Run(screen tcell.Screen) error {
	return canvas.Run(screen, New(), canvas.Options{})
}
