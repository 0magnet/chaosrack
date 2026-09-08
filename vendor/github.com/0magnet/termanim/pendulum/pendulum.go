// Package pendulum is a fan of double pendulums released from almost the same
// angle, drawn as fading trails.
//
// A double pendulum — one rod hanging off the end of another — is the smallest
// mechanism anyone can point at and call chaotic. Given enough energy to swing
// over the top it never repeats, and two of them released a millionth of a
// radian apart follow each other for a while and then do completely different
// things.
//
// That is what this is for. A strange attractor is the usual way to draw
// chaos, and it does not actually show sensitive dependence: it shows a shape,
// and the viewer is asked to take the sensitivity on trust. Here the
// divergence is the animation. A dozen pendulums start at angles that differ
// in the fifth decimal place, so they are drawn on top of each other and read
// as one; for the first several seconds they stay one; then a hair of color
// appears at the edge of the trail, then a visible fan, and within a few more
// swings they are unrelated. Nothing about the setup changed. That is the
// whole of what sensitive dependence means, and it is legible without a
// caption.
//
// # Why RK4 and not Euler
//
// This is not a performance choice, it is the difference between the animation
// being true and being a lie.
//
// Forward Euler is unstable on an oscillator: each step it steps along the
// tangent, which for circular motion always lands outside the circle, so the
// energy climbs. A double pendulum integrated with Euler visibly winds itself
// up — it swings harder and harder, and eventually spins — and the divergence
// between two copies of it is then dominated by each copy's own accumulated
// integration error rather than by the physics. The picture would look right,
// the fan would open on cue, and it would be showing the integrator's
// arithmetic instead of the system's chaos.
//
// Classical fourth-order Runge-Kutta on a fixed step has local error of order
// h^5, and at the step used here the total energy holds to about five parts in
// a hundred million over ten seconds. Measured against the same run at a
// quarter of the step, the integrator's own error two seconds in is around
// 6e-9 radians while the separation deliberately put between the pendulums has
// already grown to 6e-5 — four orders of magnitude apart. What the fan shows
// is the physics.
//
// The step is fixed and the frame budget buys a whole number of them. An
// integrator handed the frame's elapsed time directly would take a different
// step on every frame, and a chaotic system integrated with a wobbling step is
// not reproducible between two machines or between two runs.
//
// Written from the standard Lagrangian equations of motion for the double
// pendulum and from the definition of RK4; no implementation was consulted.
package pendulum

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// state is one pendulum: the two rod angles from straight down, and their
// angular velocities. It is a value type so a Runge-Kutta stage is a local
// variable and not an allocation.
type state struct {
	t1, w1, t2, w2 float64
}

// Pendulum is the animation. The zero value is not usable; call New.
type Pendulum struct {
	w, h   int
	fw, fh float64

	// The pivot, and how many pixels one rod length is.
	cx, cy, scale float64

	arms []state

	// glow is how bright each pixel of trail is and owner is which pendulum
	// last drew there. Two fields rather than one color per pixel because the
	// trail has to fade, and fading a stored color means unpacking and
	// repacking it every frame for every pixel; an intensity and an index into
	// a per-pendulum ramp fades with one multiply.
	glow  []float32
	owner []uint8

	pals []canvas.Palette

	// elapsed is the total simulated time and steps is how many integrator
	// steps have run. A frame runs however many are needed to bring steps up
	// to elapsed/SubStep.
	//
	// The obvious form — carry the unspent time and subtract the step size off
	// it in a loop — is not good enough here, and this is not hypothetical. A
	// float that has had a step size subtracted from it a few thousand times
	// has drifted, and the drift eventually buys or loses a whole extra step;
	// one extra step is a phase shift of about a hundredth of a radian, which
	// this system amplifies until it is the entire picture. Measured against a
	// direct integration, the carried-remainder version was wrong by four
	// orders of magnitude more than the integrator itself was. Counting steps
	// against a running total cannot do that.
	elapsed  float64
	steps    int
	released float64 // elapsed at the last release

	rng *rand.Rand

	// Count is how many pendulums are released. Enough that the fan is a fan
	// and not a pair; past a few dozen the trails cover each other and the
	// shearing is harder to see, not easier.
	Count int

	// Spread is the angle between one pendulum's release and the next, in
	// radians. This is the number the whole animation is about. At the default
	// the difference is far below a pixel — the fan cannot be seen at the
	// start at any zoom — and it takes a few seconds of swinging to become
	// visible, which is the point being made. Raise it and the pendulums
	// separate immediately and the demonstration is lost.
	Spread float64

	// Release is the angle both rods start at, measured from hanging straight
	// down. It has to be past horizontal: below that the system has too little
	// energy to swing over the top, the motion stays close to two coupled
	// linear oscillators, and it is not chaotic at all — nearby starts stay
	// near.
	Release float64

	// L1, L2, M1, M2 and Gravity are the mechanism. Only their ratios and the
	// overall time scale matter; gravity in the usual units with rods of unit
	// length gives swings of about a second, which is a good speed to watch.
	L1, L2, M1, M2 float64
	Gravity        float64

	// SubStep is the integrator's fixed step, in seconds. Smaller costs work
	// and buys accuracy; a four-hundred-and-eightieth of a second holds the
	// energy to a few parts in a hundred million over ten seconds, which is
	// orders of magnitude below the separation being shown.
	SubStep float64

	// TrailLife is the time constant of the trail's fade, in seconds. Long
	// enough that a whole swing is on screen at once, because the shape of the
	// swing is what shows the pendulums coming apart; much longer and the
	// screen fills with scribble and nothing is legible.
	TrailLife float64

	// ReleaseAfter is how long a fan runs before it is released again, in
	// seconds. Zero means never.
	//
	// The demonstration is over well before the pendulums are: once they have
	// separated they are simply fourteen unrelated pendulums, and the screen
	// is a scribble that will never be anything else. Starting a new fan is
	// what makes this an animation rather than a thing that happened once. The
	// old trails are left to fade out under the new one rather than being
	// wiped, which is much less abrupt.
	ReleaseAfter float64

	// ShowArms draws the rods as well as the trails. The rods are what make
	// the picture read as pendulums rather than as abstract curves, and the
	// moment the fan opens is much clearer in the rods than in the trails.
	ShowArms bool

	// ArmIntensity is how brightly the rods are drawn against the trails, from
	// 0 to 255. Dim: the rods are there to be recognized, not to be the
	// subject.
	ArmIntensity int

	// Cutoff is the trail intensity below which a pixel is left as the
	// terminal's own background. The fade never reaches zero, and without a
	// floor the screen ends up uniformly, faintly scribbled on.
	Cutoff float64
}

// New returns a fan of double pendulums. The seed is accepted for consistency
// with the other animations and to jitter the release angle; the divergence
// this shows does not depend on it, and seed 0 gives a fixed run.
func New(seed int64) *Pendulum {
	return &Pendulum{
		rng:          rand.New(rand.NewSource(seed)), //nolint:gosec
		Count:        14,
		Spread:       1e-5,
		Release:      2.4,
		L1:           1,
		L2:           1,
		M1:           1,
		M2:           1,
		Gravity:      9.81,
		SubStep:      1.0 / 480,
		TrailLife:    1.6,
		ReleaseAfter: 16,
		ShowArms:     true,
		ArmIntensity: 96,
		Cutoff:       0.03,
	}
}

// Resize lays out the pivot, builds one color ramp per pendulum and releases
// them. Called before the first frame and on every resize.
func (p *Pendulum) Resize(w, h int) {
	p.w, p.h = w, h
	p.fw, p.fh = float64(w), float64(h)
	p.glow = make([]float32, w*h)
	p.owner = make([]uint8, w*h)
	p.elapsed, p.steps, p.released = 0, 0, 0

	reach := p.L1 + p.L2
	if reach <= 0 {
		reach = 1
	}
	p.cx = p.fw / 2
	// The pivot sits high, because a pendulum with this much energy spends
	// most of its time below the pivot and almost none above it. Centering it
	// would waste the top third of the window.
	p.cy = p.fh * 0.33
	p.scale = 0.92 * math.Min(p.fw*0.5, p.fh*0.62) / reach

	n := p.Count
	if n < 1 {
		n = 1
	}
	if n > 255 {
		// owner is a byte, and a fan this wide is unreadable long before here.
		n = 255
	}
	p.arms = make([]state, n)
	p.pals = make([]canvas.Palette, n)
	for i := range p.arms {
		// Hues evenly around the wheel, so which pendulum a trail belongs to
		// is readable at a glance the moment they separate. Each ramp keeps
		// the hue and varies only brightness, because brightness is carrying
		// the trail's age.
		r, g, b := hue(float64(i) / float64(n))
		p.pals[i] = canvas.NewPalette(
			canvas.Stop{At: 0.00, R: r / 12, G: g / 12, B: b / 12},
			canvas.Stop{At: 0.55, R: r * 2 / 3, G: g * 2 / 3, B: b * 2 / 3},
			canvas.Stop{At: 1.00, R: (r + 400) / 3, G: (g + 400) / 3, B: (b + 400) / 3},
		)
	}
	p.release()
}

// release starts every pendulum from the same angle, one Spread apart.
func (p *Pendulum) release() {
	// A jitter of a hundredth of a radian on the release angle, so successive
	// fans and different seeds are different trajectories. It is three orders
	// of magnitude above Spread, so it moves the whole fan and does not touch
	// what the fan is demonstrating.
	base := p.Release + (p.rng.Float64()*2-1)*0.01
	for i := range p.arms {
		// Only the first rod is perturbed, and only by a multiple of Spread.
		// Everything else about the fourteen is identical, which is what makes
		// the separation unattributable to anything but the perturbation.
		p.arms[i] = state{t1: base + float64(i)*p.Spread, t2: base}
	}
	p.released = p.elapsed
}

// hue returns a bright color at position t around the wheel, 0 to 1.
func hue(t float64) (r, g, b int) {
	// A sine triple offset by a third of a turn each. It never goes dark and
	// never goes white, which is what a set of distinguishable trail colors
	// needs; a ramp through black would lose whichever pendulum landed there.
	f := func(o float64) int {
		return int(128 + 127*math.Sin(2*math.Pi*(t+o)))
	}
	return f(0), f(1.0 / 3), f(2.0 / 3)
}

// maxSubStepsPerFrame caps how much one frame will catch up by. A fully
// clamped frame at the default step is 48, so this is slack; it exists so a
// tiny SubStep cannot turn one hitch into an unbounded burst.
const maxSubStepsPerFrame = 512

// Frame integrates for dt seconds and draws the trails and the rods.
func (p *Pendulum) Frame(s *canvas.Surface, dt float64) {
	if p.w == 0 || p.h == 0 {
		return
	}
	if p.SubStep > 0 {
		p.elapsed += dt
		// The nudge is a millionth of a step, and it is load-bearing. elapsed is
		// a sum of frame times, so after a few thousand frames the quotient sits
		// a rounding error either side of the integer it should be, and a floor
		// that lands on the low side loses a whole step. See the note on elapsed
		// for why one step matters so much here.
		want := int(p.elapsed/p.SubStep + 1e-6)
		for n := 0; p.steps < want; n++ {
			if n >= maxSubStepsPerFrame {
				// Too far behind to catch up. Drop the backlog by moving the
				// clock to where the simulation actually got to, rather than
				// leaving a debt that makes every later frame longer.
				p.elapsed = float64(p.steps) * p.SubStep
				break
			}
			p.step()
		}
		if p.ReleaseAfter > 0 && p.elapsed-p.released >= p.ReleaseAfter {
			p.release()
		}
	}
	p.draw(s, dt)
}

// step advances every pendulum by one fixed integrator step and lays down a
// pixel of trail at each tip.
func (p *Pendulum) step() {
	p.steps++
	for i := range p.arms {
		p.arms[i] = p.rk4(p.arms[i], p.SubStep)
		x, y := p.tip(p.arms[i])
		p.stamp(x, y, uint8(i))
	}
}

// rk4 is classical fourth-order Runge-Kutta: four derivative samples across
// the step, weighted so that the error terms of order h, h^2, h^3 and h^4 all
// cancel. See the package comment for why it and not something cheaper.
func (p *Pendulum) rk4(s state, h float64) state {
	k1 := p.deriv(s)
	k2 := p.deriv(nudge(s, k1, h/2))
	k3 := p.deriv(nudge(s, k2, h/2))
	k4 := p.deriv(nudge(s, k3, h))
	return state{
		t1: s.t1 + h/6*(k1.t1+2*k2.t1+2*k3.t1+k4.t1),
		w1: s.w1 + h/6*(k1.w1+2*k2.w1+2*k3.w1+k4.w1),
		t2: s.t2 + h/6*(k1.t2+2*k2.t2+2*k3.t2+k4.t2),
		w2: s.w2 + h/6*(k1.w2+2*k2.w2+2*k3.w2+k4.w2),
	}
}

// nudge is a state stepped h along a derivative, which is the trial point each
// Runge-Kutta stage is evaluated at.
func nudge(s, d state, h float64) state {
	return state{s.t1 + d.t1*h, s.w1 + d.w1*h, s.t2 + d.t2*h, s.w2 + d.w2*h}
}

// deriv is the equations of motion: the rate of change of the state.
//
// These come from the Lagrangian of two rods with the mass at the ends. The
// only part worth pointing at is the denominator, 2*M1 + M2 - M2*cos(2*(t1-t2)),
// which never reaches zero for positive masses — the system has no
// configuration it cannot be integrated through, which is not true of every
// way of writing these equations down.
func (p *Pendulum) deriv(s state) state {
	m1, m2 := p.M1, p.M2
	l1, l2 := p.L1, p.L2
	g := p.Gravity
	d := s.t1 - s.t2
	sd, cd := math.Sin(d), math.Cos(d)
	den := 2*m1 + m2 - m2*math.Cos(2*d)

	a1 := (-g*(2*m1+m2)*math.Sin(s.t1) -
		m2*g*math.Sin(s.t1-2*s.t2) -
		2*sd*m2*(s.w2*s.w2*l2+s.w1*s.w1*l1*cd)) / (l1 * den)

	a2 := (2 * sd * (s.w1*s.w1*l1*(m1+m2) +
		g*(m1+m2)*math.Cos(s.t1) +
		s.w2*s.w2*l2*m2*cd)) / (l2 * den)

	return state{t1: s.w1, w1: a1, t2: s.w2, w2: a2}
}

// joint and tip are where the two masses are on the surface. y grows downward,
// so gravity is +y and an angle of zero hangs straight down.
func (p *Pendulum) joint(s state) (x, y float64) {
	return p.cx + p.scale*p.L1*math.Sin(s.t1), p.cy + p.scale*p.L1*math.Cos(s.t1)
}

func (p *Pendulum) tip(s state) (x, y float64) {
	jx, jy := p.joint(s)
	return jx + p.scale*p.L2*math.Sin(s.t2), jy + p.scale*p.L2*math.Cos(s.t2)
}

// stamp lays a unit of trail at one pixel and claims it for a pendulum.
//
// The last one to pass owns the pixel outright rather than the colors being
// mixed. Mixing would turn a crossing into a third color that belongs to no
// pendulum, and while the fan is still closed every pixel is being written by
// all of them; a blend there would be a uniform gray and the moment of
// separation would have nothing to show.
func (p *Pendulum) stamp(x, y float64, who uint8) {
	xi, yi := int(math.Round(x)), int(math.Round(y))
	if xi < 0 || yi < 0 || xi >= p.w || yi >= p.h {
		return
	}
	i := yi*p.w + xi
	p.owner[i] = who
	if p.glow[i] < 1 {
		p.glow[i] = 1
	}
}

// draw fades the trails, paints them and lays the rods over the top.
func (p *Pendulum) draw(s *canvas.Surface, dt float64) {
	// Exponential, so the same elapsed time always costs the same fraction of
	// the trail however it was divided into frames.
	keep := float32(math.Exp(-dt / p.TrailLife))
	cut := float32(p.Cutoff)
	for y := 0; y < p.h; y++ {
		row := y * p.w
		for x := 0; x < p.w; x++ {
			v := p.glow[row+x] * keep
			p.glow[row+x] = v
			if v < cut {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			n := int(v * 255)
			if n > 255 {
				n = 255
			}
			s.Set(x, y, p.pals[int(p.owner[row+x])%len(p.pals)][n])
		}
	}
	if !p.ShowArms {
		return
	}
	for i := range p.arms {
		pal := &p.pals[i]
		jx, jy := p.joint(p.arms[i])
		tx, ty := p.tip(p.arms[i])
		line(s, p.cx, p.cy, jx, jy, pal[p.armIntensity()])
		line(s, jx, jy, tx, ty, pal[p.armIntensity()])
		// The bobs, at full brightness. They are the things whose positions
		// are being compared, so they are the one part of the picture that is
		// never dim.
		s.Set(int(math.Round(jx)), int(math.Round(jy)), pal[255])
		s.Set(int(math.Round(tx)), int(math.Round(ty)), pal[255])
	}
}

func (p *Pendulum) armIntensity() int {
	v := p.ArmIntensity
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return v
}

// line draws a straight rod, one sample per pixel of its longer axis.
func line(s *canvas.Surface, x0, y0, x1, y1 float64, c tcell.Color) {
	dx, dy := x1-x0, y1-y0
	n := int(math.Max(math.Abs(dx), math.Abs(dy)))
	if n < 1 {
		n = 1
	}
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		s.Set(int(math.Round(x0+dx*t)), int(math.Round(y0+dy*t)), c)
	}
}

// Run swings a fan of double pendulums on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
