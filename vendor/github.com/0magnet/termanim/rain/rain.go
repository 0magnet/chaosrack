// Package rain is falling rain with depth.
//
// A field of drops falls down the surface at a slight slant. Each drop is given
// a depth, and depth drives everything: a near drop falls faster, is drawn
// brighter and leaves a longer streak, a far one crawls and is barely visible.
// That parallax is the whole trick. Drops all moving at one speed read as
// falling dashes; spread the speeds and brightnesses over a range and the eye
// separates them into layers and calls it rain.
//
// Each drop is drawn as a streak rather than a dot for the same reason a camera
// shows one: in the time a frame covers, a drop has moved several pixels, and
// the streak is that motion. Where a drop reaches the floor it leaves a splash,
// a small mark that brightens the bottom edge for a few frames and fades, which
// is what makes the rain land somewhere instead of vanishing.
//
// Written from that description, not from any existing implementation.
package rain

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Water is the default ramp: a nearly black blue for the far drops through a
// mid blue to a cold near-white at the head of the nearest ones. Rain has no
// colour of its own, it takes the light, so the ramp is a brightness ramp with
// a blue cast rather than a hue sweep.
var Water = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 0, G: 4, B: 12},
	canvas.Stop{At: 0.35, R: 20, G: 50, B: 90},
	canvas.Stop{At: 0.75, R: 90, G: 150, B: 210},
	canvas.Stop{At: 1.00, R: 225, G: 245, B: 255},
)

const (
	// splashDepth is how near a drop must be to leave a splash. See Frame.
	splashDepth = 0.6
	// exposure is the shutter time the streaks stand for, in seconds. A drop's
	// streak is how far it travels in this long, which is what motion blur is;
	// a twentieth of a second is enough that consecutive frames overlap and the
	// fall looks continuous rather than strobed.
	//
	// It is a time and not a frame count on purpose: tying it to the frame
	// would stretch every streak whenever the frame rate dropped, which is the
	// one moment the rain should not visibly change.
	exposure = 0.05
	// crownSpread is how fast a splash opens outwards, in pixels a second.
	// Roughly a pixel every other frame at sixty: any faster and each splash is
	// a wide bar, and a floor of wide bars is a line rather than a scatter of
	// impacts.
	crownSpread = 15
)

type drop struct {
	x, y   float64 // position of the head, in pixels
	vy     float64 // fall speed in pixels per second
	slant  float64 // horizontal pixels travelled per vertical pixel
	length float64 // streak length in pixels: how far it moves in one exposure
	depth  float64 // 0 is far away, 1 is right in front of you
}

// splash is a drop that has hit the floor. It has no velocity: it is an
// expanding, fading mark, and age alone says how wide and how bright.
type splash struct {
	x   float64
	age float64 // seconds since it landed
}

// Rain is the animation. The zero value is not usable; call New.
type Rain struct {
	w, h     float64
	drops    []drop
	splashes []splash
	rng      *rand.Rand

	// Density is drops per thousand pixels of surface. Counting by area rather
	// than fixing a number keeps the downpour the same weight in a small pane
	// and a full-screen terminal; a fixed count is a drizzle in one and a wall
	// of water in the other.
	Density float64
	// Slant is the wind, in horizontal pixels per vertical pixel. Vertical rain
	// looks like a test pattern; a little lean makes it weather. Each drop
	// varies slightly around this so the field is not one rigid comb.
	Slant float64
	// SplashLife is how long a splash lasts, in seconds. Long enough to see,
	// short enough that the floor is not a permanent bright line.
	SplashLife float64
	// Palette colours the drops by depth, dim at 0 and bright at 255.
	Palette canvas.Palette
}

// New returns a rain animation. seed of 0 gives a fixed downpour, which makes
// tests repeatable.
func New(seed int64) *Rain {
	return &Rain{
		rng:        rand.New(rand.NewSource(seed)),
		Density:    30,
		Slant:      0.28,
		SplashLife: 0.2,
		Palette:    Water,
	}
}

// Resize allocates the drops and scatters them. Called before the first frame
// and on every resize.
func (r *Rain) Resize(w, h int) {
	r.w, r.h = float64(w), float64(h)
	if r.Density <= 0 {
		r.Density = 1
	}
	n := int(r.Density * float64(w*h) / 1000)
	if n < 1 {
		n = 1
	}
	r.drops = make([]drop, n)
	for i := range r.drops {
		r.drops[i] = r.newDrop()
		// Start scattered down the whole surface instead of all at the top,
		// or the first second of the animation is an empty screen followed by
		// a single curtain of water arriving together.
		r.drops[i].y = r.rng.Float64() * r.h
	}
	// At most one splash per drop can be alive at a time, since a drop makes a
	// splash only when it recycles and a splash outlives that by a few frames.
	// Sizing for a couple of generations means Frame never has to grow this.
	r.splashes = make([]splash, 0, n)
}

// newDrop makes a drop at the top with a fresh depth. Speed, streak length and
// brightness all follow from that one number, which is what keeps the layers
// consistent: nothing dim ever falls fast.
func (r *Rain) newDrop() drop {
	depth := r.rng.Float64()
	// Speeds are given as surface heights per second, so a drop takes the same
	// time to cross the screen whatever its size and whatever the frame rate.
	// The near drop crosses in about half a second, the far one in nearly two.
	vy := r.h * (0.6 + depth*1.35)
	return drop{
		x:     r.rng.Float64() * r.w,
		y:     -r.rng.Float64() * r.h * 0.2,
		vy:    vy,
		slant: r.Slant * (0.7 + depth*0.6),
		// Speed times shutter time: the streak is exactly the distance covered
		// while the shutter was open, which is what makes it read as blur
		// rather than as a drawn line of some arbitrary length.
		length: vy * exposure,
		depth:  depth,
	}
}

// brightness is the intensity of a drop's head, 0 to 1. Depth is squashed
// towards the bright end so the near drops stand clearly out of the haze
// instead of the whole field sitting at a uniform mid grey.
func (r *Rain) brightness(d drop) float64 {
	return 0.12 + d.depth*d.depth*0.88
}

func (r *Rain) colour(v float64) tcell.Color {
	i := int(v * 255)
	if i < 0 {
		i = 0
	}
	if i > 255 {
		i = 255
	}
	return r.Palette[i]
}

// Frame advances the drops and splashes and draws them. dt is the seconds
// since the last frame; every speed here is per second and is scaled by it, so
// the rain falls at the same rate however often this is called.
func (r *Rain) Frame(s *canvas.Surface, dt float64) {
	if r.w == 0 || r.h == 0 {
		return
	}
	s.Clear()

	for i := range r.drops {
		d := &r.drops[i]
		d.y += d.vy * dt
		d.x += d.vy * d.slant * dt
		// Wrap sideways rather than respawning: a drop blown off the right edge
		// is still rain, and wrapping keeps the density even across the width.
		if d.x < 0 {
			d.x += r.w
		} else if d.x >= r.w {
			d.x -= r.w
		}
		if d.y-d.length > r.h {
			// Landed and gone. Splash where it hit, then send it round again
			// with a new depth so the layers keep reshuffling.
			//
			// Only the near drops splash. Every drop splashing puts marks
			// along the floor faster than they fade, which merges into a solid
			// bright bar; splashing the front layer alone keeps them separate
			// enough to read as individual impacts, and the far drops are
			// landing somewhere behind you anyway.
			if d.depth > splashDepth && len(r.splashes) < cap(r.splashes) {
				r.splashes = append(r.splashes, splash{x: d.x})
			}
			*d = r.newDrop()
			d.y = -d.length
		}
	}

	kept := r.splashes[:0]
	for _, sp := range r.splashes {
		sp.age += dt
		if sp.age < r.SplashLife {
			kept = append(kept, sp)
		}
	}
	r.splashes = kept

	r.draw(s)
}

func (r *Rain) draw(s *canvas.Surface) {
	h := int(r.h)

	for _, d := range r.drops {
		head := r.brightness(d)
		n := int(d.length)
		if n < 1 {
			n = 1
		}
		for t := 0; t <= n; t++ {
			f := float64(t)
			// The tail fades to two fifths of the head. Fading to nothing loses
			// the streak's length; not fading at all draws a bar with no sense
			// of which end is moving.
			v := head * (1 - 0.6*f/float64(n))
			// Floor rather than truncate: truncation rounds the pixel above
			// the surface up onto row zero, which draws a line of drops along
			// the top edge that never falls.
			y := int(math.Floor(d.y - f))
			if y < 0 || y >= h {
				continue
			}
			x := int(math.Floor(d.x - f*d.slant))
			// The tail may have wrapped through the edge along with the head.
			if x < 0 {
				x += int(r.w)
			}
			s.Set(x, y, r.colour(v))
		}
	}

	for _, sp := range r.splashes {
		// A splash opens outwards and dims as it does, so the mark reads as
		// water thrown sideways rather than a blinking dot.
		f := 1 - sp.age/r.SplashLife
		c := r.colour(0.5 + f*0.5)
		x := int(sp.x)
		s.Set(x, h-1, c)
		// The crown widens at a fixed rate in pixels a second, so it opens the
		// same way however often this is drawn.
		if e := int(sp.age * crownSpread); e > 0 {
			s.Set(x-e, h-1, c)
			s.Set(x+e, h-1, c)
			// Early on the thrown water is still above the floor.
			if sp.age < r.SplashLife/2 {
				s.Set(x-e, h-2, c)
				s.Set(x+e, h-2, c)
			}
		}
	}
}

// Run rains on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
