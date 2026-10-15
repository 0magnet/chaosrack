package rackhost

import (
	"math"
	"strconv"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// The Style module's phosphor, natively: with one chosen, the model is
// traced on that tube — drawn in its color, fading channel by channel
// instead of clearing, and darkening toward the corners — as the page's CRT
// look draws it (phosphor_js.go). The scopes' beams are that phosphor too.

// crtState is the tube's last frame, for the afterglow.
type crtState struct {
	last *rackpic.Image
	at   time.Time
}

// phosphor is the phosphor chosen, and whether one is. Called with r.mu
// held.
func (r *Rack) phosphor() (attractor.Phosphor, bool) {
	i, err := strconv.Atoi(r.value("phosphor"))
	if err != nil || i <= 0 || i >= len(attractor.Phosphors) {
		return attractor.Phosphor{}, false
	}
	return attractor.Phosphors[i], true
}

// crt traces frame m on phosphor p: each pixel's brightness in the
// phosphor's color, over what is left of the last frame after its
// retention, which the page applies sixty times a second and so is applied
// here as often as that for the time since the last.
func (c *crtState) crt(m *rackpic.Image, p attractor.Phosphor, now time.Time) {
	frames := 1.0
	if c.last != nil && c.last.W == m.W && c.last.H == m.H && !c.at.IsZero() {
		frames = math.Min(now.Sub(c.at).Seconds()*60, 600)
	} else {
		c.last = nil
	}
	keep := [3]float64{math.Pow(p.KR, frames), math.Pow(p.KG, frames), math.Pow(p.KB, frames)}
	tint := [3]float64{p.TR, p.TG, p.TB}
	back := [3]float64{float64(sceneBack.R), float64(sceneBack.G), float64(sceneBack.B)}
	cx, cy := float64(m.W)/2, float64(m.H)/2
	for y := range m.H {
		for x := range m.W {
			i := 3 * (y*m.W + x)
			px := m.Px[i : i+3]
			lum := 0.0
			if px[0] != sceneBack.R || px[1] != sceneBack.G || px[2] != sceneBack.B {
				lum = float64(max(px[0], px[1], px[2])) / 255
			}
			dx, dy := (float64(x)-cx)/cx, (float64(y)-cy)/cy
			vignette := 1 - 0.35*min(dx*dx+dy*dy, 1)
			for ch := range 3 {
				v := tint[ch] * lum * 255 * vignette
				if c.last != nil {
					v = math.Max(v, float64(c.last.Px[i+ch])*keep[ch])
				}
				// The glass is dark where nothing has been traced: the
				// panel's own near-black, not a phosphor's.
				px[ch] = uint8(math.Max(v, back[ch]*vignette)) //nolint:gosec // within 0..255
			}
		}
	}
	c.last = &rackpic.Image{W: m.W, H: m.H, Px: append([]byte(nil), m.Px...)}
	c.at = now
}
