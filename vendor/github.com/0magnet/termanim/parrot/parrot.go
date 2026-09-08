// Package parrot is the party parrot: a bird's head that rolls in a circle
// while its color runs once around the hue wheel per roll.
//
// # WHERE THIS COMES FROM, AND WHY NONE OF IT IS COPIED
//
// The party parrot began as a Slack emoji made from footage of Sirocco, a
// kākāpō, and reached terminals through `curl parrot.live` and `ascii.live`.
// Neither of those can be used here. ascii-live is GPL-3.0 and this repository
// is MIT, so vendoring its frames would relicense the repository; parrot.live
// and the nyancat server declare no license at all, which reserves every right
// by default. So this is written from a description of the effect and nothing
// else — the same footing as metaballs, which describes an effect libcaca also
// has and takes no code from it.
//
// # WHY IT IS DRAWN RATHER THAN PLAYED
//
// The original is ten fixed frames of sprite art. Ten frames is a filmstrip:
// it has one size, one palette cycle, and one bird, and at a terminal's
// aspect ratio it is either tiny or blocky.
//
// Drawing the bird instead — an ellipse for the head, a wedge for the beak, a
// few strokes for the crest, all in a local space that is scaled and rotated
// per frame — costs about the same code and gives three things a filmstrip
// cannot. It fills whatever window it is given. The roll is continuous rather
// than ten-position, so it does not strobe on a fast terminal. And because
// position and phase are parameters, a whole wall of parrots offset in phase
// costs one more loop, which is the form people actually want: not a parrot,
// a colonnade of them doing the wave.
//
// # THE MOTION
//
// The bird does not spin. It roll-bobs: the head tilts one way, dips, tilts
// back, dips again — two dips per full hue cycle — which is what reads as
// dancing rather than as a rotating asset. Everything below is a function of
// one phase angle, so the whole animation is seekable and the tests can ask
// for a specific pose.
package parrot

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Parrot is the animation. The zero value is not usable; call New.
type Parrot struct {
	w, h  float64
	phase float64 // 0..1 through one roll

	// Count is how many parrots to draw across the window. One is the emoji;
	// more is the wall, each offset in phase so the row does a wave. Zero or
	// less is treated as one.
	Count int

	// Period is the seconds for one full roll and one full turn of hue.
	// Around a second reads as dancing; much slower reads as breathing.
	Period float64

	// Spread is how far apart in phase neighboring parrots are, as a fraction
	// of a full roll. 0 makes the row move as one bird; the default staggers
	// them into a traveling wave.
	Spread float64

	// Saturation and Value are the HSV constants the cycling hue is combined
	// with. Full saturation is the emoji; dropping it gives pastel birds.
	Saturation, Value float64
}

// New returns a party parrot. It takes a seed for signature compatibility with
// the other animations, and ignores it: there is nothing random here. The
// motion is a pure function of elapsed time, which is what makes a wall of
// them line up into a wave instead of a scatter.
func New(_ int64) *Parrot {
	return &Parrot{
		Count:      1,
		Period:     0.9,
		Spread:     0.12,
		Saturation: 1,
		Value:      1,
	}
}

// Resize records the surface size. There is nothing to allocate: the bird is
// rasterized by testing pixels, so it has no buffers of its own.
func (p *Parrot) Resize(w, h int) { p.w, p.h = float64(w), float64(h) }

// Frame draws one frame.
func (p *Parrot) Frame(s *canvas.Surface, dt float64) {
	if p.w == 0 || p.h == 0 {
		return
	}
	period := p.Period
	if period <= 0 {
		period = 0.9
	}
	p.phase += dt / period
	// Kept in 0..1 rather than allowed to grow, so the float stays precise
	// over a long run — this animation is a plausible screensaver and may be
	// left up for hours.
	p.phase -= math.Floor(p.phase)

	s.Clear()

	n := p.Count
	if n < 1 {
		n = 1
	}

	// Each parrot gets an equal column. The head is sized to the smaller of
	// the column width and the full height so a wide row of them does not
	// produce birds taller than the window.
	colW := p.w / float64(n)
	scale := math.Min(colW, p.h) * 0.32

	for i := 0; i < n; i++ {
		cx := colW*(float64(i)+0.5) + 0.02*scale
		cy := p.h * 0.5
		ph := p.phase + float64(i)*p.Spread
		ph -= math.Floor(ph)
		p.draw(s, cx, cy, scale, ph)
	}
}

// draw rasterizes one bird at a center, size and phase.
//
// The bird is defined once in a local space where the head is a unit circle,
// and each pixel of the bounding box is transformed back into that space and
// tested. That is the opposite of the usual order — shapes to pixels — and it
// is chosen because it makes rotation free (it is one rotation of the sample
// point, not of every shape) and because it allocates nothing, which Frame is
// required not to do.
func (p *Parrot) draw(s *canvas.Surface, cx, cy, scale, ph float64) {
	th := ph * 2 * math.Pi

	// The roll. Tilt leads, the dip runs at twice the rate so the bird bobs
	// down at each extreme of the tilt rather than once per cycle.
	tilt := 0.30 * math.Sin(th)
	dipY := 0.13 * (1 - math.Cos(2*th)) * 0.5
	swayX := 0.10 * math.Sin(th)

	// The hue turns once per roll: that is the whole joke.
	body := hsv(ph, p.Saturation, p.Value)
	// The crest is the same hue held a little ahead, so it reads as a
	// separate feather group rather than as part of the skull.
	crest := hsv(math.Mod(ph+0.08, 1), p.Saturation, p.Value*0.82)

	sin, cos := math.Sin(tilt), math.Cos(tilt)

	// Bounding box in surface space, generous enough for the beak and crest
	// at any tilt. Clipped to the surface so an oversized parrot costs
	// nothing off-screen.
	sw, sh := s.Size()
	ox := cx + swayX*scale
	oy := cy + dipY*scale
	x0, x1 := clampRange(ox-2.1*scale, ox+2.1*scale, sw)
	y0, y1 := clampRange(oy-2.1*scale, oy+2.1*scale, sh)

	for y := y0; y < y1; y++ {
		// Pixels are square on this surface (it is twice the terminal's row
		// count, one pixel per half block), so no aspect correction is needed
		// here — that is the whole reason canvas works in half blocks.
		dy := (float64(y) + 0.5 - oy) / scale
		for x := x0; x < x1; x++ {
			dx := (float64(x) + 0.5 - ox) / scale

			// Into the bird's own frame.
			lx := dx*cos + dy*sin
			ly := -dx*sin + dy*cos

			if c, ok := shade(lx, ly, body, crest); ok {
				s.Set(x, y, c)
			}
		}
	}
}

// shade is the bird itself: which part of it, if any, covers a point in local
// space, and what color that part is.
//
// Ordered front to back — eye, then beak, then crest, then skull — so the
// first hit wins and nothing has to be drawn twice. The bird faces left,
// because the emoji does.
func shade(x, y float64, body, crest tcell.Color) (tcell.Color, bool) {
	// Eye: a dark disc with a highlight up and to the left, which is what
	// makes it read as wet rather than as a hole.
	ex, ey := x+0.30, y+0.28
	if ex*ex+ey*ey < 0.20*0.20 {
		hx, hy := ex+0.07, ey+0.07
		if hx*hx+hy*hy < 0.07*0.07 {
			return tcell.NewRGBColor(255, 255, 255), true
		}
		return tcell.NewRGBColor(20, 18, 26), true
	}

	// Beak: a blunt wedge off the front of the face, pale cream against
	// whatever the body hue currently is. Two half-planes and a cap, rather
	// than a triangle test, so the tip is rounded like a parrot's and not
	// sharp like a finch's.
	if x < -0.42 && x > -1.32 {
		// Half-height of the wedge, tapering toward the tip.
		t := (x + 1.32) / 0.90 // 0 at the tip, 1 at the face
		half := 0.10 + 0.34*t*t
		if y > -0.30 && y < -0.30+2*half {
			// The lower mandible is shaded a little darker so the two halves
			// of the beak are distinguishable at small sizes.
			if y > -0.30+1.35*half {
				return tcell.NewRGBColor(196, 176, 132), true
			}
			return tcell.NewRGBColor(238, 222, 178), true
		}
	}

	// Crest: three feathers standing off the crown and swept back.
	//
	// Each is a long thin ellipse whose major axis points radially outward
	// from the top of the skull, at a widening angle from vertical. They are
	// rooted just inside the head (at radius 0.95 against a skull of about
	// 1.0) so they emerge from it rather than floating above it, and they are
	// tested before the skull so they paint over it where they overlap.
	//
	// Ellipses rather than drawn strokes because a stroke needs a line
	// rasterizer and a buffer, and Frame is required to allocate nothing.
	for i := 0; i < 3; i++ {
		// Radians from straight up, leaning back over the bird. Up is -y and
		// back is +x, so the outward direction is (sin a, -cos a).
		a := 0.12 + float64(i)*0.40
		dx2, dy2 := math.Sin(a), -math.Cos(a)
		fcx, fcy := 0.08+1.02*dx2, 1.02*dy2

		px, py := x-fcx, y-fcy
		u := px*dx2 + py*dy2  // along the feather
		v := -px*dy2 + py*dx2 // across it
		if (u*u)/(0.46*0.46)+(v*v)/(0.15*0.15) < 1 {
			return crest, true
		}
	}

	// Skull: slightly wider than tall, which is what stops it reading as a
	// ball with a beak stuck on.
	//
	// Flat, with no cheek highlight or shading. The first version had both,
	// and at this resolution a soft shade is not available — the highlight
	// rendered as a hard-edged lighter disc sitting on the face, which read
	// as two overlapping circles rather than as a lit cheek. The original
	// emoji is flat-colored anyway, so this is both the better picture and
	// the more faithful one.
	if (x*x)/(1.02*1.02)+(y*y)/(0.94*0.94) < 1 {
		return body, true
	}

	return 0, false
}

// clampRange turns a float span into the loop bounds that stay on a surface of
// the given extent.
func clampRange(lo, hi float64, n int) (int, int) {
	a := int(math.Floor(lo))
	b := int(math.Ceil(hi))
	if a < 0 {
		a = 0
	}
	if b > n {
		b = n
	}
	if b < a {
		b = a
	}
	return a, b
}

// hsv converts a hue in 0..1 with saturation and value in 0..1 to a color.
//
// Written out rather than pulled from a color library because it is eight
// lines and this package would otherwise have no dependencies beyond tcell and
// canvas — and because it runs per parrot per frame, not per pixel, so there
// is nothing to gain from a faster one.
func hsv(h, s, v float64) tcell.Color {
	h = h - math.Floor(h)
	if s < 0 {
		s = 0
	} else if s > 1 {
		s = 1
	}
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	i := math.Floor(h * 6)
	f := h*6 - i
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return tcell.NewRGBColor(int32(r*255), int32(g*255), int32(b*255))
}

// lighten moves a color toward white for positive amounts and toward black for
// negative ones. Used for the cheek highlight and the shadow under it, which
// are what keep a flat-hue head from reading as a sticker.
func lighten(c tcell.Color, amt float64) tcell.Color {
	r, g, b := c.RGB()
	adj := func(v int32) int32 {
		f := float64(v)
		if amt >= 0 {
			f += (255 - f) * amt
		} else {
			f += f * amt
		}
		if f < 0 {
			f = 0
		} else if f > 255 {
			f = 255
		}
		return int32(f)
	}
	return tcell.NewRGBColor(adj(r), adj(g), adj(b))
}

// Run draws party parrots on the given screen until the user stops it.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
