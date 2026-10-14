package scopegames

import "math"

// Pong is Scope Pong — an homage to the glensstuff.com analog Oscilloscope
// Pong, where the whole game is a voltage pair driving a scope's X/Y inputs.
// A side with no human input for ~10 s hands its paddle back to the machine,
// so the game boots as a self-playing attract demo. First to 9 resets the
// match.
type Pong struct {
	BallSpeed float32
	PaddleH   float32 // paddle full height
	AISkill   float32
	BX, BY    float64 // ball position
	VX, VY    float64 // ball direction (unit-ish)
	PadL      float64
	PadR      float64
	ScoreL    int
	ScoreR    int
	Serve     int // frames until serve (pause after a point)
	HumanL    int // frames of human control left on each side
	HumanR    int
	KeyW      bool
	KeyS      bool
	KeyUp     bool
	KeyDn     bool

	// Rand is a number in [0, 1); Beep sounds a blip of hz for ms, nil for
	// none.
	Rand func() float64
	Beep func(hz float64, ms int)
}

// The court.
const (
	PongW    = 1.5  // court half-width
	PongH    = 1.0  // court half-height
	PongPadX = 1.38 // paddle |x|
	PongBall = 0.05 // ball diamond radius
	// PongHuman is how many frames a hand keeps a paddle after it lets go.
	PongHuman = 600
)

// NewPong is a game at the knobs' defaults.
func NewPong(rand func() float64) Pong {
	return Pong{BallSpeed: 1, PaddleH: 0.42, AISkill: 0.7, VX: 0.6, VY: 0.23, Rand: rand}
}

func (p *Pong) beep(hz float64, ms int) {
	if p.Beep != nil {
		p.Beep(hz, ms)
	}
}

// Step advances one frame of game state; k is the Speed control's sub-steps
// times its dt scale, as the integrators take it.
func (p *Pong) Step(k float64) {
	ph := float64(p.PaddleH) / 2
	// Paddles: human while recently touched, machine otherwise.
	move := func(pad *float64, up, dn bool, human *int, aiming bool) {
		if *human > 0 {
			*human--
			// Keys nudge; a pointer or a pot writes the pad position
			// directly, so with no key held this just clamps.
			d := 0.0
			if up {
				d += 0.032
			}
			if dn {
				d -= 0.032
			}
			*pad += d * k
		} else {
			// Machine: chase the ball when it's incoming, drift home when not.
			target := 0.0
			if aiming {
				target = p.BY
			}
			maxSpd := (0.006 + 0.030*float64(p.AISkill)) * k
			d := target - *pad
			if d > maxSpd {
				d = maxSpd
			}
			if d < -maxSpd {
				d = -maxSpd
			}
			*pad += d
		}
		if *pad > PongH-ph {
			*pad = PongH - ph
		}
		if *pad < -(PongH - ph) {
			*pad = -(PongH - ph)
		}
	}
	move(&p.PadL, p.KeyW, p.KeyS, &p.HumanL, p.VX < 0)
	move(&p.PadR, p.KeyUp, p.KeyDn, &p.HumanR, p.VX > 0)
	if p.Serve > 0 {
		p.Serve--
		return
	}
	sp := 0.020 * float64(p.BallSpeed) * k
	p.BX += p.VX * sp
	p.BY += p.VY * sp
	// Wall bounce.
	if p.BY > PongH-PongBall && p.VY > 0 || p.BY < -(PongH-PongBall) && p.VY < 0 {
		p.VY = -p.VY
		p.beep(226, 90)
	}
	// Paddle bounce: reflect at the paddle plane when the ball face covers it,
	// with english from the hit offset and a little speed-up.
	hit := func(pad float64) bool { return math.Abs(p.BY-pad) <= ph+PongBall }
	if p.BX > PongPadX-PongBall && p.VX > 0 && hit(p.PadR) {
		p.VX = -math.Abs(p.VX) * 1.04
		p.VY += (p.BY - p.PadR) / ph * 0.35
		p.beep(459, 90)
	}
	if p.BX < -(PongPadX-PongBall) && p.VX < 0 && hit(p.PadL) {
		p.VX = math.Abs(p.VX) * 1.04
		p.VY += (p.BY - p.PadL) / ph * 0.35
		p.beep(459, 90)
	}
	if v := math.Abs(p.VX); v > 1.6 { // keep returns playable
		p.VX = p.VX / v * 1.6
	}
	if v := math.Abs(p.VY); v > 1.2 {
		p.VY = p.VY / v * 1.2
	}
	// Point scored: tick the winner, serve toward the loser.
	if p.BX > PongW+0.25 {
		p.ScoreL++
		p.ServeBall(-1)
	}
	if p.BX < -(PongW + 0.25) {
		p.ScoreR++
		p.ServeBall(1)
	}
	if p.ScoreL > 9 || p.ScoreR > 9 {
		p.ScoreL, p.ScoreR = 0, 0
	}
}

// ServeBall re-centers the ball and aims it at dir (±1) after a pause.
func (p *Pong) ServeBall(dir float64) {
	p.BX, p.BY = 0, 0
	p.VX = dir * 0.6
	p.VY = (p.Rand() - 0.5) * 0.8
	p.Serve = 45
	p.beep(490, 220)
}

// Restart is a new match: the Restart switch.
func (p *Pong) Restart() {
	p.ScoreL, p.ScoreR = 0, 0
	p.ServeBall(1)
}

// Enter sets the game up as the model is chosen: a fresh match, the paddles
// home and the machine on both.
func (p *Pong) Enter() {
	p.ScoreL, p.ScoreR = 0, 0
	p.PadL, p.PadR = 0, 0
	p.HumanL, p.HumanR = 0, 0
	p.ServeBall(1)
}

// Seize puts a hand on a paddle: left or right, at court height pos (-1..1
// of the half-height), kept for the human window.
func (p *Pong) Seize(left bool, pos float64) {
	if left {
		p.PadL, p.HumanL = pos*PongH, PongHuman
	} else {
		p.PadR, p.HumanR = pos*PongH, PongHuman
	}
}

// Strokes is the frame as blanked-beam strokes, each a polyline of x, y
// pairs: court border, a dashed net, detached score marks, paddles, ball —
// and nothing between them; the retrace is blanked, as a real scope's z-axis
// would be. They are appended to dst.
func (p *Pong) Strokes(dst [][]float64) [][]float64 {
	ph := float64(p.PaddleH) / 2
	// Court border (one closed polyline).
	dst = append(dst, []float64{
		-PongW, PongH, PongW, PongH, PongW, -PongH, -PongW, -PongH, -PongW, PongH})
	// Net: real dashes down the middle.
	for y := PongH - 0.04; y > -PongH; y -= 0.12 {
		dst = append(dst, []float64{0, y, 0, y - 0.06})
	}
	// Score marks: detached ticks hanging under the top edge.
	for i := range p.ScoreL {
		x := -(0.18 + 0.11*float64(i))
		dst = append(dst, []float64{x, PongH - 0.03, x, PongH - 0.11})
	}
	for i := range p.ScoreR {
		x := 0.18 + 0.11*float64(i)
		dst = append(dst, []float64{x, PongH - 0.03, x, PongH - 0.11})
	}
	// Paddles: slim closed bars.
	paddle := func(x, pad float64) []float64 {
		const w = 0.016
		return []float64{x - w, pad - ph, x - w, pad + ph, x + w, pad + ph, x + w, pad - ph, x - w, pad - ph}
	}
	dst = append(dst, paddle(-PongPadX, p.PadL), paddle(PongPadX, p.PadR))
	// Ball diamond (blinks while serving).
	if p.Serve == 0 || p.Serve%10 < 5 {
		dst = append(dst, []float64{
			p.BX - PongBall, p.BY, p.BX, p.BY + PongBall,
			p.BX + PongBall, p.BY, p.BX, p.BY - PongBall,
			p.BX - PongBall, p.BY})
	}
	return dst
}

// PongBlipGain is a pong blip's level at its start: a square, falling to
// nothing over the blip.
const PongBlipGain = 0.08
