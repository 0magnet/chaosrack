// Package snow is falling snow that settles.
//
// Flakes drift down an order of magnitude slower than rain and sway from side
// to side as they go, each on its own phase so that no two are ever in step —
// a shared phase turns a snowfall into a shoal of fish moving as one body. As
// with rain, depth sets speed and brightness together, so the near flakes are
// large and bright and the far ones are a faint drifting haze.
//
// What makes it snow rather than slow rain is that it stays where it lands. A
// height is kept for every column; a flake that reaches the top of its column
// disappears into it and raises it by a pixel, and the accumulated heights are
// drawn as a bank along the bottom. Left alone the bank would bury the screen,
// so it also settles: a column much taller than its neighbor spills into it,
// which rounds the drifts off, and snow slowly compacts and melts away, which
// bounds the depth without ever emptying the screen.
//
// Written from that description, not from any existing implementation.
package snow

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Snowfall is the default ramp: the far flakes are a dim blue-grey, as though
// seen through the fall itself, and the near ones are white. Snow is white by
// definition, so only the brightness has anywhere to go.
var Snowfall = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 12, G: 16, B: 26},
	canvas.Stop{At: 0.45, R: 90, G: 100, B: 125},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 255},
)

type flake struct {
	x, y  float64 // x is the center it sways about, not where it is drawn
	vy    float64 // fall speed in pixels per second
	phase float64 // where in its sway it currently is
	swayW float64 // how far it swings either side of x, in pixels
	swayV float64 // radians per second
	depth float64 // 0 is far away, 1 is right in front of you
}

// at returns where a flake is drawn: its center plus the current sway.
func (f flake) at() float64 { return f.x + f.swayW*math.Sin(f.phase) }

// Snow is the animation. The zero value is not usable; call New.
type Snow struct {
	w, h   float64
	flakes []flake
	// ground is the depth of settled snow in each column, in pixels, measured
	// up from the bottom row.
	ground []int
	// Settling and melting are things that happen so many times a second, not
	// once a frame. These carry the fraction of an event left over from the
	// last frame, so a slow frame does the work of the frames it replaced
	// instead of the bank settling at whatever rate the machine manages.
	settleDebt float64
	meltDebt   float64
	rng        *rand.Rand

	// Flakes is flakes per thousand pixels of surface. Counting by area keeps
	// the fall the same weight at any window size, where a fixed count is bare
	// in a large window and a whiteout in a small one.
	Flakes float64
	// MaxDepth is how much of the surface height the bank may fill. A quarter
	// leaves the sky to the snow; much more and the animation becomes a rising
	// white rectangle.
	MaxDepth float64
	// MeltRate is how many pixels of snow melt off random columns each second.
	// One a second is far slower than it falls, so banks still build — melting
	// exists to keep old drifts from being permanent, not to fight the
	// snowfall.
	MeltRate float64
	// SettleRate is how many columns are checked for a cliff each second, as a
	// multiple of the number of columns. Checking every column every frame
	// would tie the settling to the frame rate and cost more the wider the
	// window; a few passes a second rounds the drifts off just as fast to the
	// eye.
	SettleRate float64
	// Palette colors the flakes and the bank by depth.
	Palette canvas.Palette
}

// New returns a snow animation. seed of 0 gives a fixed fall, which makes tests
// repeatable.
func New(seed int64) *Snow {
	return &Snow{
		rng:        rand.New(rand.NewSource(seed)),
		Flakes:     18,
		MaxDepth:   0.25,
		MeltRate:   1,
		SettleRate: 4,
		Palette:    Snowfall,
	}
}

// Resize allocates the flakes and the ground. Called before the first frame and
// on every resize.
func (s *Snow) Resize(w, h int) {
	s.w, s.h = float64(w), float64(h)
	if s.Flakes <= 0 {
		s.Flakes = 1
	}
	n := int(s.Flakes * float64(w*h) / 1000)
	if n < 1 {
		n = 1
	}
	s.flakes = make([]flake, n)
	for i := range s.flakes {
		s.flakes[i] = s.newFlake()
		// Scattered through the whole height, or the screen starts empty and
		// the first flakes arrive as one layer.
		s.flakes[i].y = s.rng.Float64() * s.h
	}
	// The bank starts bare. Keeping it across a resize would mean rescaling a
	// height map to a new width, and a fresh fall covers the floor in seconds.
	s.ground = make([]int, w)
	s.settleDebt, s.meltDebt = 0, 0
}

func (s *Snow) newFlake() flake {
	depth := s.rng.Float64()
	return flake{
		x: s.rng.Float64() * s.w,
		y: -s.rng.Float64() * s.h * 0.1,
		// Surface heights per second, so the crossing time depends on neither
		// the window size nor the frame rate: the nearest flake takes about
		// three seconds to fall, the farthest about ten. Rain, for comparison,
		// uses factors five times these and crosses in well under a second.
		vy:    s.h * (0.12 + depth*0.18),
		phase: s.rng.Float64() * 2 * math.Pi,
		// Near flakes swing wider, which reads as them being closer for the
		// same reason a nearer object moves further across your view.
		swayW: 0.8 + depth*3.2,
		// Radians a second: about one swing every three to five seconds. Each
		// flake has its own rate as well as its own phase, because with a
		// shared rate they would drift apart and then, sooner or later, line
		// up again.
		swayV: 1.2 + s.rng.Float64()*1.8,
		depth: depth,
	}
}

func (s *Snow) color(v float64) tcell.Color {
	i := int(v * 255)
	if i < 0 {
		i = 0
	}
	if i > 255 {
		i = 255
	}
	return s.Palette[i]
}

// maxDepth is the hard ceiling on a column, in pixels. Settling and melting
// keep the bank well below this in practice; the clamp is what guarantees it,
// so no run of luck can pile a column into the sky.
func (s *Snow) maxDepth() int {
	d := int(s.MaxDepth * s.h)
	if d < 1 {
		d = 1
	}
	if max := int(s.h) - 1; d > max {
		d = max
	}
	return d
}

// Frame advances the fall, settles the bank and draws both. dt is the seconds
// since the last frame; every rate here is per second and is scaled by it, so
// the snow falls and piles up at the same rate however often this is called.
func (s *Snow) Frame(surf *canvas.Surface, dt float64) {
	if s.w == 0 || s.h == 0 {
		return
	}
	s.fall(dt)
	s.settle(dt)
	s.draw(surf)
}

// fall moves every flake and lands the ones that meet the bank.
func (s *Snow) fall(dt float64) {
	maxD := s.maxDepth()
	for i := range s.flakes {
		f := &s.flakes[i]
		f.y += f.vy * dt
		f.phase += f.swayV * dt

		col := int(math.Floor(f.at()))
		// Sway carries flakes past the edges. Wrapping rather than respawning
		// keeps the fall even across the width and costs nothing.
		if col < 0 {
			f.x += s.w
			col += int(s.w)
		} else if col >= int(s.w) {
			f.x -= s.w
			col -= int(s.w)
		}
		if col < 0 || col >= len(s.ground) {
			continue
		}

		// The surface of the column this flake is over.
		top := s.h - float64(s.ground[col])
		if f.y < top {
			continue
		}
		// Landed. A column at the ceiling swallows the flake without growing,
		// so snow goes on falling and settling visibly on a full drift instead
		// of the fall stopping dead once the bank is deep.
		if s.ground[col] < maxD {
			s.ground[col]++
		}
		*f = s.newFlake()
		f.y = -1
	}
}

// settle spills tall columns into short neighbors and melts the bank slowly.
func (s *Snow) settle(dt float64) {
	// A handful of columns are checked each time rather than all of them. Over
	// a second every column is visited several times, the drifts round off just
	// as fast to the eye, and the cost does not grow with the window.
	s.settleDebt += s.SettleRate * float64(len(s.ground)) * dt
	checks := int(s.settleDebt)
	s.settleDebt -= float64(checks)
	for i := 0; i < checks; i++ {
		x := s.rng.Intn(len(s.ground))
		// A step of two or more pixels between neighbors is a cliff, and snow
		// does not hold a cliff. One pixel of slope is left alone, otherwise
		// the bank flattens into a perfectly level slab.
		for _, dx := range [2]int{-1, 1} {
			n := x + dx
			if n < 0 || n >= len(s.ground) {
				continue
			}
			if s.ground[x]-s.ground[n] >= 2 {
				s.ground[x]--
				s.ground[n]++
				break
			}
		}
	}
	s.meltDebt += s.MeltRate * dt
	for s.meltDebt >= 1 {
		s.meltDebt--
		x := s.rng.Intn(len(s.ground))
		if s.ground[x] > 0 {
			s.ground[x]--
		}
	}
}

func (s *Snow) draw(surf *canvas.Surface) {
	surf.Clear()
	h := int(s.h)

	for x, d := range s.ground {
		for i := 0; i < d; i++ {
			// i counts up from the floor, so d-1-i is how far this pixel lies
			// under the surface of the drift. The surface is the lit part and
			// it darkens downwards into the bank; shading it the other way up
			// lights the buried snow and the drift reads as a flat white bar.
			v := 1 - float64(d-1-i)/float64(s.maxDepth()+2)*0.55
			surf.Set(x, h-1-i, s.color(v))
		}
	}

	for _, f := range s.flakes {
		// Brightness follows depth, held off zero so the farthest flakes are
		// still faintly there rather than missing.
		v := 0.18 + f.depth*0.82
		// Floor rather than truncate: truncation rounds a flake still above the
		// surface up onto row zero, leaving a row of snow stuck to the top.
		y := int(math.Floor(f.y))
		if y < 0 {
			continue
		}
		surf.Set(int(math.Floor(f.at())), y, s.color(v))
	}
}

// Run snows on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
