// Package fireworks is shells launching and bursting over the terminal.
//
// There are only two kinds of thing here and they obey the same physics. A
// shell is fired from the floor with an upward velocity, gravity pulls it back,
// and at the top of its arc — where it has run out of climb — it bursts. The
// burst replaces it with a hundred particles thrown outwards in every
// direction at a random speed, each of which is then carried by the same
// gravity: fast ones fly far and arc over, slow ones barely leave the centre,
// and the sphere of the burst sags into the drooping shape a real one has.
//
// Three details do most of the work. Every particle of one burst shares a
// colour, so a firework reads as a single object rather than confetti. Each
// particle has a life that decays as it flies and dims it to nothing, so the
// burst thins out instead of switching off. And the burst itself starts with a
// white flash, because the eye expects the light before the sparks. The rising
// shell leaves a trail of short-lived embers, which is what makes the launch
// visible at all — a single moving pixel is not.
//
// Written from that description, not from any existing implementation.
package fireworks

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Colours are the shell colours, one per firework. They are saturated and
// bright because everything drawn from them is a fade towards black: a muted
// starting colour has no range to fade through and dies immediately.
var Colours = []tcell.Color{
	tcell.NewRGBColor(255, 80, 80),   // red
	tcell.NewRGBColor(255, 190, 60),  // gold
	tcell.NewRGBColor(120, 255, 130), // green
	tcell.NewRGBColor(120, 190, 255), // blue
	tcell.NewRGBColor(230, 130, 255), // violet
	tcell.NewRGBColor(255, 255, 220), // white
}

// dragPerSecond is what a spark's outward speed is multiplied by over one
// second of flight. Sparks lose their sideways speed quickly and then simply
// fall, which is what turns the expanding ball into the drooping willow shape
// of a real burst. It is applied as a power of the elapsed time, so the decay
// is the same however the second is divided into frames.
const dragPerSecond = 0.16

// flashDecay is how much of a burst's flash is spent per second.
const flashDecay = 7.5

type shell struct {
	x, y   float64
	vx, vy float64 // pixels per second; vy is negative while climbing
	trail  float64 // embers owed: see Frame
	colour tcell.Color
}

type particle struct {
	x, y   float64
	vx, vy float64 // pixels per second
	life   float64 // 1 at birth, 0 when gone
	decay  float64 // life lost per second
	colour tcell.Color
}

// flash is the light of a burst: no motion, just a disc that fades over a few
// frames. It is kept apart from the particles because it does not move and
// does not answer to gravity.
type flash struct {
	x, y   float64
	life   float64
	radius float64
	colour tcell.Color
}

// Fireworks is the animation. The zero value is not usable; call New.
type Fireworks struct {
	w, h      float64
	shells    []shell
	particles []particle
	flashes   []flash
	burst     int // particles per burst, worked out from the surface size
	rng       *rand.Rand

	// Gravity is the downward pull in surface heights per second squared.
	// Expressing it as a fraction of the height rather than in pixels means a
	// shell takes the same time to arc over in any window; a fixed pixel value
	// makes a tall window feel like the moon.
	Gravity float64
	// Rockets is how many shells may be climbing at once. More than a handful
	// and the sky is never dark, which is what the bursts are seen against.
	Rockets int
	// LaunchRate is how many shells go up a second when there is room for them.
	// A little over one keeps something in the air without the whole display
	// going up as one volley.
	LaunchRate float64
	// TrailRate is how many embers a climbing shell drops a second. The trail
	// is what makes the launch visible at all — a single moving pixel is not —
	// and thirty is dense enough to look continuous at any frame rate.
	TrailRate float64
	// Burst is the number of particles in a burst on a surface of about eighty
	// by fifty; the actual count is scaled from the real size so a burst covers
	// the same fraction of any window.
	Burst int
}

// New returns a fireworks animation. seed of 0 gives a fixed display, which
// makes tests repeatable.
func New(seed int64) *Fireworks {
	return &Fireworks{
		rng:        rand.New(rand.NewSource(seed)),
		Gravity:    0.54,
		Rockets:    3,
		LaunchRate: 1.2,
		TrailRate:  30,
		Burst:      90,
	}
}

// Resize allocates the shells and particles. Called before the first frame and
// on every resize.
func (f *Fireworks) Resize(w, h int) {
	f.w, f.h = float64(w), float64(h)
	if f.Rockets < 1 {
		f.Rockets = 1
	}
	if f.Burst < 1 {
		f.Burst = 1
	}
	// Scale the burst by the square root of the area, not the area itself: a
	// burst is a ring, so its pixel count grows with the linear size of the
	// window and scaling by area would make a large terminal a solid disc.
	scale := math.Sqrt(float64(w*h) / 4000)
	f.burst = int(float64(f.Burst) * scale)
	if f.burst < 12 {
		f.burst = 12
	}

	f.shells = make([]shell, 0, f.Rockets)
	// Every rocket in the air may burst at once, and the previous bursts are
	// still fading, so allow two full rounds. Frame never grows these slices:
	// a burst that does not fit is trimmed instead, which loses a few sparks in
	// the worst case and never allocates mid-animation.
	f.particles = make([]particle, 0, f.Rockets*f.burst*2)
	f.flashes = make([]flash, 0, f.Rockets*2)
}

// launch fires a shell from the floor. It is aimed to reach somewhere in the
// upper half of the surface: v = sqrt(2*g*rise) is the speed that just runs out
// of climb at that height, which is where it will burst.
func (f *Fireworks) launch() {
	g := f.Gravity * f.h
	rise := f.h * (0.45 + f.rng.Float64()*0.35)
	f.shells = append(f.shells, shell{
		// Keep launches away from the very edges, or half of the burst is off
		// screen and the firework looks clipped rather than distant.
		x: f.w * (0.1 + f.rng.Float64()*0.8),
		y: f.h - 1,
		// A little sideways drift, so shells do not all rise in parallel lines.
		vx:     (f.rng.Float64()*2 - 1) * f.h * 0.12,
		vy:     -math.Sqrt(2 * g * rise),
		colour: Colours[f.rng.Intn(len(Colours))],
	})
}

// explode replaces a shell with a ring of particles and a flash.
func (f *Fireworks) explode(s shell) {
	if len(f.flashes) < cap(f.flashes) {
		f.flashes = append(f.flashes, flash{
			x: s.x, y: s.y, life: 1,
			// The flash is small next to the burst it starts: it is the light
			// at the centre, not the shape of the explosion.
			radius: f.h * 0.08,
			colour: s.colour,
		})
	}
	for i := 0; i < f.burst; i++ {
		if len(f.particles) >= cap(f.particles) {
			break
		}
		a := f.rng.Float64() * 2 * math.Pi
		// Speeds are spread across the whole range instead of being equal, so
		// the burst is a filled ball of sparks rather than a hollow ring
		// expanding as one shell.
		v := f.h * 0.6 * (0.25 + f.rng.Float64()*0.75)
		f.particles = append(f.particles, particle{
			x: s.x, y: s.y,
			// Inherit the shell's motion. A burst from a shell still drifting
			// sideways should drift with it.
			vx: math.Cos(a)*v + s.vx*0.5,
			vy: math.Sin(a)*v + s.vy*0.5,
			// The colour of the shell, so one firework is one colour.
			colour: s.colour,
			life:   1,
			// Life lost per second: a spark lasts between one and a half and
			// three seconds. Lives differ by a factor of two or so, which is
			// what makes the burst thin out unevenly and hang in the air
			// instead of all of it going dark at once.
			decay: 0.3 + f.rng.Float64()*0.36,
		})
	}
}

// Frame advances everything and draws it. dt is the seconds since the last
// frame; every velocity, rate and lifetime here is per second and is scaled by
// it, so a firework arcs and bursts the same way at any frame rate.
func (f *Fireworks) Frame(s *canvas.Surface, dt float64) {
	if f.w == 0 || f.h == 0 {
		return
	}
	g := f.Gravity * f.h
	// Drag is a decay over time, so it compounds: half a second of it is the
	// square root of a second of it, not half of it. One Pow for the whole
	// frame, not one per spark.
	drag := math.Pow(dragPerSecond, dt)
	// How far a dragged spark actually travels in dt. Its speed is decaying the
	// whole time, so the distance is the integral of that decay and not simply
	// speed times dt: for v(t) = v0*k^t that integral is v0*(1-k^dt)/-ln k.
	// Multiplying by dt and then applying the drag would leave the distance
	// depending on how the second was cut into frames, which is the one thing
	// this is all meant to avoid.
	dragMove := (1 - drag) / -math.Log(dragPerSecond)

	if len(f.shells) < f.Rockets && f.rng.Float64() < f.LaunchRate*dt {
		f.launch()
	}

	keptShells := f.shells[:0]
	for _, sh := range f.shells {
		// Midpoint integration: the position advances by the average of the
		// speed at the start and end of the step, not the speed at the start.
		// Under constant acceleration that is exact at any step size, which is
		// what keeps the shell bursting at the same height whether the frame
		// rate is thirty, sixty or stuttering.
		sh.x += sh.vx * dt
		sh.y += (sh.vy + 0.5*g*dt) * dt
		sh.vy += g * dt
		// The trail: dim embers dropped at a fixed rate a second, dying fast,
		// so the shell draws a short tail behind it rather than a line all the
		// way down. The debt carries the fraction of an ember left over, so the
		// tail has the same density however often this runs.
		sh.trail += f.TrailRate * dt
		for sh.trail >= 1 {
			sh.trail--
			if len(f.particles) >= cap(f.particles) {
				break
			}
			f.particles = append(f.particles, particle{
				x: sh.x, y: sh.y,
				// Embers sink out of the shell's wake rather than hanging.
				vy:     f.h * 0.04,
				colour: sh.colour,
				life:   0.7,
				decay:  2.7,
			})
		}
		// Burst at the top of the arc, where the climb has run out. Waiting for
		// it to start falling instead would show the shell hanging and sinking
		// first, which reads as a dud.
		if sh.vy >= 0 || sh.y <= 0 {
			f.explode(sh)
			continue
		}
		keptShells = append(keptShells, sh)
	}
	f.shells = keptShells

	keptParticles := f.particles[:0]
	for _, p := range f.particles {
		p.x += p.vx * dragMove
		// Midpoint again, so a spark's arc does not depend on the frame rate.
		p.y += (p.vy + 0.5*g*dt) * dt
		p.vy += g * dt
		// Air drag, applied to the horizontal only. See dragPerSecond.
		p.vx *= drag
		p.life -= p.decay * dt
		// Drop a particle once it is dark or has fallen off the bottom. Without
		// this the slice fills with things that will never be seen again.
		if p.life > 0 && p.y < f.h {
			keptParticles = append(keptParticles, p)
		}
	}
	f.particles = keptParticles

	keptFlashes := f.flashes[:0]
	for _, fl := range f.flashes {
		// The whole flash goes in about an eighth of a second, which is roughly
		// how long one actually registers.
		fl.life -= flashDecay * dt
		if fl.life > 0 {
			keptFlashes = append(keptFlashes, fl)
		}
	}
	f.flashes = keptFlashes

	f.draw(s)
}

// fade dims a colour towards black by a factor from 0 to 1. Every particle is
// drawn through this: fading the colour rather than swapping in a ramp keeps
// each firework the one colour it was launched with.
func fade(c tcell.Color, v float64) tcell.Color {
	if v <= 0 {
		return tcell.ColorDefault
	}
	if v > 1 {
		v = 1
	}
	r, g, b := c.RGB()
	return tcell.NewRGBColor(
		int32(float64(r)*v),
		int32(float64(g)*v),
		int32(float64(b)*v),
	)
}

func (f *Fireworks) draw(s *canvas.Surface) {
	s.Clear()

	for _, fl := range f.flashes {
		r := int(fl.radius * (1.4 - fl.life))
		if r < 1 {
			r = 1
		}
		cx, cy := int(fl.x), int(fl.y)
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				d := math.Hypot(float64(dx), float64(dy))
				if d > float64(r) {
					continue
				}
				// White at the core shading out to the shell's colour at the
				// rim: a flash is hot enough to wash its own colour out.
				v := fl.life * (1 - d/float64(r+1))
				c := fade(fl.colour, v)
				if d < float64(r)/2 {
					c = fade(tcell.NewRGBColor(255, 255, 255), v)
				}
				if v > 0 {
					s.Set(cx+dx, cy+dy, c)
				}
			}
		}
	}

	for _, p := range f.particles {
		// The square root holds a spark near full brightness for most of its
		// life and then drops it away at the end. A linear fade spends the
		// whole burst in a dull mid tone.
		s.Set(int(p.x), int(p.y), fade(p.colour, math.Sqrt(p.life)))
	}

	for _, sh := range f.shells {
		// The shell itself is the brightest thing in the sky until it bursts.
		s.Set(int(sh.x), int(sh.y), tcell.NewRGBColor(255, 255, 255))
	}
}

// Run sets off fireworks on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
