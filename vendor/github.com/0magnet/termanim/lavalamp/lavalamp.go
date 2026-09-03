// Package lavalamp is wax rising and sinking in a heated glass vessel.
//
// The blobs are drawn as a metaball field, the same summed inverse-square
// falloff this repository's metaballs package uses: where two blobs approach,
// their fields add and the surface between them bulges out and joins, which is
// what makes wax look like wax rather than like circles. The physics is what
// differs. Every blob carries a temperature. The element at the base of the
// lamp heats whatever settles on it, hot wax expands and floats up; the glass
// near the top loses heat to the room, cold wax contracts and sinks; and the
// round trip that falls out of those two facts is the whole animation. The
// blobs are held inside a vessel that is narrow at the neck and full at the
// base, because that silhouette, rather than the field, is what makes the
// effect read as a lava lamp instead of as metaballs in a box.
//
// Written from that description; no implementation of a lava lamp was
// consulted. The field evaluation follows this repository's own metaballs.
package lavalamp

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

type blob struct {
	x, y   float64 // position in pixels
	vx, vy float64 // velocity in pixels per second
	r0     float64 // radius at neutral temperature
	r      float64 // current radius: how far this blob's influence reaches
	temp   float64 // 0 is cold and sinking, 1 is hot and rising
}

// LavaLamp is the animation. The zero value is not usable; call New.
type LavaLamp struct {
	w, h    float64
	cx      float64 // the axis of the vessel: blobs are held either side of it
	maxHalf float64 // the vessel's half width at its widest
	blobs   []blob
	rng     *rand.Rand

	// Count is how many blobs of wax. Zero, the default, picks a number from
	// the height of the vessel. Cost is Count multiplies per pixel per frame,
	// and too many blobs merge into one permanent mass with nothing to watch.
	Count int
	// Buoyancy scales the acceleration a blob gets from being off neutral
	// temperature, in pixels per second squared.
	Buoyancy float64
	// HeatRate and CoolRate are how fast temperature changes per second in the
	// element's zone at the base and against the cold glass at the neck.
	// Together with Buoyancy and Drag they set the period of the lamp: too fast
	// and the wax twitches, too slow and nothing appears to happen at all.
	HeatRate, CoolRate float64
	// Ambient is how fast a blob in the middle of the column gives up its heat
	// to the oil around it, per second. It has to be slow — much slower than
	// one trip up the lamp — or the wax equilibrates before it arrives and
	// hangs in the middle. What it prevents is a blob that happens to be at
	// exactly neutral temperature sitting there forever: the oil away from the
	// element is cooler than neutral, so wax left alone always cools, sinks,
	// and is reheated at the base. Heat is added in one place only, which is
	// why a real lamp circulates at all.
	Ambient float64
	// Drag is how fast the oil takes speed out of the wax, per second. Wax in
	// oil is a viscous, drag-dominated system, not a ballistic one: a blob
	// reaches a terminal speed almost at once and holds it, which is why the
	// motion looks heavy and unhurried rather than bouncy.
	//
	// It is quoted as the rate of an exponential decay and applied as
	// exp(-Drag·dt), rather than as a fraction of the speed kept each frame.
	// The obvious form — multiply the velocity by 0.9 every frame — is not a
	// property of the oil at all but of the frame rate: the same wax would be
	// twice as viscous when drawn twice as often. This is the same viscosity at
	// any frame rate, and reduces to the same thing at the rate it was tuned at.
	Drag float64
	// Palette colours the field. It should be a hot ramp — the animation is
	// literally about temperature, and the eye reads a black-red-yellow
	// sequence as heat without being told.
	Palette canvas.Palette
}

// New returns a lava lamp. seed of 0 gives a fixed arrangement, which makes
// tests repeatable.
func New(seed int64) *LavaLamp {
	return &LavaLamp{
		rng:      rand.New(rand.NewSource(seed)),
		Buoyancy: 36,
		HeatRate: 0.6,
		CoolRate: 0.48,
		Ambient:  0.045,
		// -30·ln(0.9): the decay rate that loses a tenth of the speed every
		// thirtieth of a second, which is where these numbers were tuned.
		Drag:    3.162,
		Palette: canvas.Fire,
	}
}

const (
	// neutral is the temperature at which a blob neither rises nor sinks. Wax
	// spends most of its time either side of this, so putting it in the middle
	// of the range gives the same headroom to each half of the cycle.
	neutral = 0.5
	// heatZone and coolZone are the fractions of the vessel's height, measured
	// from the top, outside which the element heats and the glass cools. The
	// gap between them is the long middle stretch where a blob simply coasts on
	// the temperature it left with — that coasting is what makes the wax appear
	// to drift rather than oscillate.
	heatZone = 0.82
	coolZone = 0.28
	// ambientTemp is the temperature of the oil away from the element. It sits
	// below neutral because the only heat in the lamp enters at the base: wax
	// floats on borrowed heat and sinks once it has given it back.
	ambientTemp = 0.15
	// neckFrac is the vessel's half width at the neck as a fraction of its
	// widest. A lava lamp is a cone that has been rounded off; without a
	// distinctly narrow top the silhouette is a rectangle and the illusion goes.
	neckFrac = 0.42
	// wallGap keeps a blob's centre off the glass, as a fraction of the widest
	// half width, so wax flattens against the wall instead of half of it
	// disappearing through the outside of the vessel.
	wallGap = 0.12
	// cutoff is the field value below which a pixel is left unpainted. An
	// inverse-square field never reaches zero, so without a cutoff every pixel
	// inside the vessel ends up faintly lit and the blobs stop floating in
	// anything. It sits above the far-field level and well below the value at a
	// blob's own radius, which is about 64. metaballs uses the same number for
	// the same reason.
	cutoff = 32
)

// Resize builds the vessel and fills it with wax.
func (l *LavaLamp) Resize(w, h int) {
	l.w, l.h = float64(w), float64(h)
	if w <= 0 || h <= 0 {
		l.blobs = nil
		return
	}
	l.cx = l.w / 2
	// The vessel is tall and narrow whatever shape the window is: capping the
	// half width by the height as well as by the width stops a wide terminal
	// producing a lamp that is wider than it is tall, which reads as a fish
	// tank.
	l.maxHalf = math.Min(0.42*l.w, 0.34*l.h)

	n := l.Count
	if n <= 0 {
		// Blobs share one column, so it is the height of the lamp that decides
		// how many fit, not its area. Fewer than three is a lonely bubble;
		// beyond about seven the fields never come apart again and the lamp is
		// one permanent mass.
		n = 3 + int(l.h/40)
		if n > 7 {
			n = 7
		}
	}

	// Radii scale with the vessel so the wax occupies the same fraction of it
	// at any window size, and are limited by the height as well as the width so
	// that blobs can pass each other vertically and the field does not fill the
	// glass. The visible edge of a blob is about half again its radius — that
	// is where the falloff crosses the cutoff — so the radius has to look small
	// against the vessel for the wax to end up looking right in it.
	scale := math.Min(0.45*l.maxHalf, 0.12*l.h)
	l.blobs = make([]blob, n)
	for i := range l.blobs {
		r := (0.5 + l.rng.Float64()*0.4) * scale
		y := (0.1 + l.rng.Float64()*0.8) * l.h
		b := blob{
			y:    y,
			r0:   r,
			temp: l.rng.Float64(),
		}
		// Start each blob somewhere legal for its height, not merely somewhere
		// inside the bounding box: the vessel is much narrower than the window.
		lim := l.limit(y)
		b.x = l.cx + (l.rng.Float64()*2-1)*lim
		b.r = r
		l.blobs[i] = b
	}
}

// smoothstep is a 0..1 ramp with zero slope at both ends, used for the vessel
// profile so the neck meets the body without a visible kink.
func smoothstep(edge0, edge1, x float64) float64 {
	t := (x - edge0) / (edge1 - edge0)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

// halfWidth is the vessel's half width in pixels at pixel row y.
func (l *LavaLamp) halfWidth(y float64) float64 {
	t := y / l.h // 0 at the top, 1 at the base
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	// The body: the neck opens out to the full width by about three quarters of
	// the way down, and stays there.
	hw := l.maxHalf * (neckFrac + (1-neckFrac)*smoothstep(0.04, 0.76, t))
	// Round the rim and the base. Cut off flat they read as a truncated cone;
	// curved in they read as blown glass. A circular arc over the last few per
	// cent of the height is enough to sell it.
	const round = 0.05
	if t < round {
		hw *= math.Sqrt(1 - ((round-t)/round)*((round-t)/round))
	} else if t > 1-round {
		hw *= math.Sqrt(1 - ((t-(1-round))/round)*((t-(1-round))/round))
	}
	return hw
}

// limit is how far from the axis a blob's centre may sit at height y.
func (l *LavaLamp) limit(y float64) float64 {
	lim := l.halfWidth(y) - wallGap*l.maxHalf
	if lim < 0 {
		return 0
	}
	return lim
}

// Frame advances the wax and draws the field. dt is the seconds elapsed since
// the previous frame.
func (l *LavaLamp) Frame(s *canvas.Surface, dt float64) {
	if len(l.blobs) == 0 || dt <= 0 {
		return
	}
	l.physics(dt)
	l.draw(s)
}

// physics runs dt seconds of heating, buoyancy and confinement.
//
// Everything with a rate in it is scaled by dt: the heating, the cooling, the
// buoyant acceleration and the velocities. Everything that is a shape or a
// ratio is not — the vessel profile, the coupling between temperature and
// radius, and the fraction of speed a blob keeps when it meets the glass.
func (l *LavaLamp) physics(dt float64) {
	// Viscosity as a decay over the elapsed time rather than per frame; see
	// the comment on Drag.
	damp := math.Exp(-l.Drag * dt)
	for i := range l.blobs {
		b := &l.blobs[i]
		t := b.y / l.h

		// Heat from the element, cooling at the glass. Both act at their full
		// rate anywhere in their zone rather than ramping in from the edge of
		// it: the element keeps a pool of oil at the base at an even
		// temperature and the neck sits at room temperature, so what matters is
		// whether the wax is in the pool, not how far in.
		//
		// The difference is not cosmetic. Ramped from zero, the heat a blob
		// meets on the way down is proportional to how far down it has got, and
		// a blob that has only just entered the zone is turned around by a
		// nudge before it ever reaches the base. The lamp then oscillates about
		// the boundary with a tiny amplitude and never circulates.
		if t > heatZone {
			b.temp += l.HeatRate * dt
		}
		if t < coolZone {
			b.temp -= l.CoolRate * dt
		}
		b.temp += l.Ambient * (ambientTemp - b.temp) * dt
		if b.temp < 0 {
			b.temp = 0
		} else if b.temp > 1 {
			b.temp = 1
		}

		// Hot wax is less dense than the oil around it and rises; cold wax
		// sinks. Up the screen is negative y.
		b.vy -= l.Buoyancy * (b.temp - neutral) / neutral * dt
		// A little sideways nudge, from convection in the oil. Without it the
		// blobs run up and down a single column and the lamp looks mechanical.
		b.vx += (l.rng.Float64()*2 - 1) * l.Buoyancy * 0.35 * dt

		// Blobs that have almost merged push apart a little. The field already
		// makes overlapping blobs look like one mass, which is wanted; what is
		// not wanted is every blob ending up at the same coordinates, after
		// which the lamp holds one blob forever.
		for j := range l.blobs {
			if j == i {
				continue
			}
			o := &l.blobs[j]
			dx, dy := b.x-o.x, b.y-o.y
			d2 := dx*dx + dy*dy
			near := 0.5 * (b.r + o.r)
			if d2 > near*near || d2 == 0 {
				continue
			}
			d := math.Sqrt(d2)
			push := l.Buoyancy * 0.5 * (1 - d/near) * dt
			b.vx += dx / d * push
			b.vy += dy / d * push
		}

		b.vx *= damp
		b.vy *= damp
		b.x += b.vx * dt
		b.y += b.vy * dt

		// Hot wax expands and cold wax contracts. This is not decoration: the
		// change in radius is what makes a blob visibly swell before it lifts
		// off the base and shrink as it gathers at the neck.
		b.r = b.r0 * (0.82 + 0.36*b.temp)

		// Confinement. The floor and ceiling of the vessel absorb rather than
		// bounce — wax arriving at the base settles on it and heats, which it
		// could not do if it rebounded.
		if b.y < 0 {
			b.y, b.vy = 0, 0
		} else if b.y > l.h {
			b.y, b.vy = l.h, 0
		}
		lim := l.limit(b.y)
		if b.x < l.cx-lim {
			b.x = l.cx - lim
			b.vx = -b.vx * 0.3
		} else if b.x > l.cx+lim {
			b.x = l.cx + lim
			b.vx = -b.vx * 0.3
		}
	}
}

// draw paints the field, the glass and the lit base.
func (l *LavaLamp) draw(s *canvas.Surface) {
	s.Clear()
	_, h := s.Size()
	// The glass itself: dim enough to be a container rather than a competing
	// bright object.
	glass := tcell.NewRGBColor(90, 60, 40)

	for y := 0; y < h; y++ {
		fy := float64(y) + 0.5
		hw := l.halfWidth(fy)
		x0 := int(math.Ceil(l.cx - hw))
		x1 := int(math.Floor(l.cx + hw))
		if x1 < x0 {
			continue
		}
		// Outside the vessel nothing is drawn at all, which is what gives the
		// silhouette: only the rows' interiors are visited.
		for x := x0; x <= x1; x++ {
			fx := float64(x) + 0.5
			var field, hot float64
			for i := range l.blobs {
				b := &l.blobs[i]
				dx, dy := fx-b.x, fy-b.y
				// Inverse-square falloff in units of the blob's radius. The
				// squared distance is used directly: a square root per blob per
				// pixel would be the most expensive thing here and would change
				// nothing about how it looks.
				d2 := (dx*dx + dy*dy) / (b.r * b.r)
				if d2 < 0.0001 {
					d2 = 0.0001
				}
				c := 1 / d2
				field += c
				// Carry the temperature along with the field so a pixel can be
				// coloured by whose wax it is standing in, weighted by how much
				// each blob contributes there. Two blobs merging blend their
				// heat across the join instead of meeting at a hard edge.
				hot += c * b.temp
			}
			// field is unbounded near a core; compress it so the cores do not
			// all saturate to one flat colour.
			v := 255 * field / (field + 3)
			if v < cutoff {
				// Unlit oil. The glass wall and the glow of the element are the
				// only things allowed here, so the wax has something to float
				// in and against.
				if x == x0 || x == x1 {
					s.Set(x, y, glass)
				} else if g := l.baseGlow(fy); g > 0 {
					s.Set(x, y, l.Palette[g])
				}
				continue
			}
			// Modulate by temperature: cold wax settles to a dull red, hot wax
			// to yellow and white. The geometry decides the shape, the heat
			// decides the colour.
			v *= 0.68 + 0.5*(hot/field)
			i := int(v)
			if i > 255 {
				i = 255
			} else if i < 1 {
				i = 1
			}
			s.Set(x, y, l.Palette[i])
		}
	}
}

// baseGlow is the palette index for the light at the foot of the lamp, or 0 for
// none. It is deliberately dimmer than the cutoff: it should suggest where the
// heat comes from without lighting the oil enough to spoil the floating.
func (l *LavaLamp) baseGlow(y float64) int {
	const zone = 0.08 // the bottom twelfth of the vessel
	t := y / l.h
	if t < 1-zone {
		return 0
	}
	return int(float64(cutoff-6) * (t - (1 - zone)) / zone)
}

// Run runs a lava lamp on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
