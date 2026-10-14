// Package scopegames is the rack's two scope games — the Bouncing Ball and
// Scope Pong — as rules alone: state, a step, and the blips a step makes,
// shared by the page, which draws them with WebGL and blips Web Audio, and
// the native rack, which draws and blips them itself.
package scopegames

// Ball is the Bouncing Ball — the classic analog-computer demo (Telefunken
// shipped it with their RA-series machines to sell integrators): two
// integrator chains (velocity → position) under constant gravity, a
// comparator that flips the vertical velocity at the floor with a
// restitution loss, and wall reflections for the horizontal drift. When the
// bounces decay away, the machine re-kicks the ball — the loop that ran
// unattended in trade-show windows. Each floor hit blips at a pitch set by
// the impact speed.
type Ball struct {
	Grav   float32 // gravity
	Rest   float32 // restitution (bounce energy keep)
	Drift  float32 // horizontal speed
	Height float32 // drop height: where the next Drop releases the ball
	X, Y   float64
	VX, VY float64
	Kicks  int  // machine re-kicks since the count was last zeroed
	Warm   bool // a warmup run: no blips, no kicks counted

	// Rand is a number in [0, 1); Beep sounds a blip of hz for ms, nil for
	// none.
	Rand func() float64
	Beep func(hz float64, ms int)
}

// The ball's court.
const (
	BallFloor = -1.0
	BallCeil  = 1.0
	BallWall  = 1.4
	// BallStartX is where a drop releases the ball across.
	BallStartX = -1.2
)

// NewBall is the ball at the knobs' defaults, ready to drop.
func NewBall(rand func() float64) Ball {
	return Ball{Grav: 12, Rest: 0.88, Drift: 0.7, Height: 0.9, X: BallStartX, Y: 0.9, VX: 0.7, Rand: rand}
}

func (b *Ball) beep(hz float64, ms int) {
	if b.Beep != nil && !b.Warm {
		b.Beep(hz, ms)
	}
}

// Step advances the ball by one integrator tick of dt.
func (b *Ball) Step(dt float64) {
	b.VY -= float64(b.Grav) * dt
	b.X += b.VX * dt
	b.Y += b.VY * dt
	// Comparator: floor bounce with restitution, plus a blip whose pitch
	// follows the impact speed (harder hit = higher blip).
	if b.Y < BallFloor && b.VY < 0 {
		b.Y = BallFloor
		imp := -b.VY
		b.VY = imp * float64(b.Rest)
		b.beep(180+90*imp, 45)
		// Re-kick when the bounces have decayed away, like the unattended
		// trade-show loop: fresh upward velocity, fresh drift direction.
		if b.VY < 0.25 {
			b.VY = 3.6 + 1.4*b.Rand()
			b.VX = float64(b.Drift) * (0.4 + 0.6*b.Rand())
			if b.Rand() < 0.5 {
				b.VX = -b.VX
			}
			if !b.Warm {
				b.Kicks++
			}
			b.beep(490, 90)
		}
	}
	if b.Y > BallCeil && b.VY > 0 {
		b.Y = BallCeil
		b.VY = -b.VY * float64(b.Rest)
		b.beep(320, 40)
	}
	if b.X > BallWall && b.VX > 0 || b.X < -BallWall && b.VX < 0 {
		b.VX = -b.VX
		b.beep(226, 40)
	}
	// The drift knob retunes the horizontal speed live (sign preserved).
	if b.VX > 0 {
		b.VX = float64(b.Drift)
	} else if b.VX < 0 {
		b.VX = -float64(b.Drift)
	}
}

// DropHeight is the height knob's setting: court y for a release.
func (b *Ball) DropHeight() float64 {
	if b.Height > 0 {
		return float64(b.Height)
	}
	return 0.9
}

// Drop releases the ball from the drop height, drifting either way: the
// Launcher's Drop switch.
func (b *Ball) Drop() {
	b.X, b.Y = BallStartX, b.DropHeight()
	b.VY = 0
	b.VX = float64(b.Drift)
	if b.Rand() < 0.5 {
		b.VX = -b.VX
	}
}

// Enter sets the ball up as the model is chosen: dropped face-on from the
// height, drifting right, the kicks counted from none.
func (b *Ball) Enter() {
	b.Kicks = 0
	b.X, b.Y = BallStartX, b.DropHeight()
	b.VX, b.VY = float64(b.Drift), 0
}

// BallFrameDT is one frame's integrator tick at Speed 1.
const BallFrameDT = 0.016

// BallBlipGain is a ball blip's level at its start: a sine, falling to
// nothing over the blip.
const BallBlipGain = 0.1
