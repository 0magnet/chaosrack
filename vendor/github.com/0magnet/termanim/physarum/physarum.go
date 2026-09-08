// Package physarum is slime mold, the transport networks of Physarum
// polycephalum grown out of an agent rule three lines long.
//
// Thousands of particles crawl over a chemical field. Each one drops a little
// of the chemical where it stands, then looks ahead at three points — one to
// the left, one straight on, one to the right — and turns toward whichever
// smells strongest. The field blurs outward a little and evaporates a little
// every step. That is the whole model; there is no path finding in it and no
// agent knows about any other.
//
// What comes out is a network. A particle that wanders onto a trail is turned
// along it and reinforces it, so trails recruit; evaporation kills the ones
// that stop being used, so weak trails die. Between those two the population
// pulls itself into veins, the veins compete, and the picture keeps
// reorganizing — anastomosing, pruning, thickening the routes that carry
// traffic — for as long as it runs. It is the same behavior that has the real
// organism solving mazes and reproducing the Tokyo rail network out of oat
// flakes.
//
// Written from Jeff Jones' description of the model (Artificial Life 16(2),
// 2010) — the three-sensor rule and roughly his parameters — not from any
// implementation of it.
//
// The parameters are the whole animation, and the one that decides what it
// looks like is the ratio of sensor distance to step size. Sensing barely past
// its own nose a particle can only find a trail it is nearly on, and the
// result is a fine crazed mesh; sensing three times as far as the default it
// commits to trails it is nowhere near, and the veins widen into broad smooth
// ribbons. Everything else is milder than that. They are exported so this is
// arguable rather than baked in.
//
// The simulation runs on a fixed step and the frame budget buys a whole
// number of them, in the manner of langton and sand. The diffusion kernel is
// a fixed 3x3 average and the decay is a fixed fraction, neither of which has
// a meaningful "half a step" — scaling them by an arbitrary elapsed time
// would change the equilibrium the network settles at whenever the frame rate
// wobbled, which is exactly the thing that must not depend on the frame rate.
package physarum

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Slime is the default ramp: near black through a cold blue-green into the
// yellow-white of a heavily traveled vein.
//
// The dark end has a trace of blue in it rather than being flat black, because
// the faint haze of un-recruited wandering between the veins is half of what
// makes this look like an organism instead of a wire diagram, and pure black
// throws it away.
var Slime = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 2, G: 6, B: 14},
	canvas.Stop{At: 0.25, R: 10, G: 52, B: 72},
	canvas.Stop{At: 0.55, R: 40, G: 160, B: 120},
	canvas.Stop{At: 0.80, R: 190, G: 232, B: 96},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 224},
)

// agent is one particle: where it is, and which way it faces. It carries no
// memory at all — everything it knows is read back out of the field.
type agent struct {
	x, y float64 // position in pixels
	dir  float64 // heading in radians
}

// Physarum is the animation. The zero value is not usable; call New.
type Physarum struct {
	w, h   int
	fw, fh float64

	agents []agent

	// field is the chemical, one value per pixel, and next is the buffer the
	// blur writes into. Diffusion has to read the whole neighborhood as it was
	// at the start of the step — smearing in place would let a cell diffuse
	// into a neighbor and then read its own deposit back out on the same
	// sweep, which walks the field sideways.
	field, next []float32

	// taken marks which pixels hold a particle, because at most one may.
	//
	// This is not decoration and it is not collision detection. Without
	// exclusion the population piles into whichever direction happens to win
	// early, every particle reinforcing the same trail from inside it, and the
	// whole surface turns into one broad wave marching diagonally off the
	// screen — no branching, no competition, no network. Exclusion is what
	// makes a vein have a capacity, and a vein with a capacity is what forces
	// the traffic that will not fit to find or make another route. It is the
	// single rule the interesting behavior rests on.
	taken []bool

	// acc is unspent elapsed time, carried between frames so a step rate that
	// does not divide into the frame rate still averages out.
	acc float64

	// steps counts simulation steps run, which is the age of the network in a
	// unit that does not depend on how often it was drawn.
	steps int

	rng *rand.Rand

	// Density is particles per thousand pixels of surface. Counting by area
	// rather than fixing a number keeps the same crowding in a small pane and
	// a full-screen terminal, and crowding is what decides whether a network
	// forms: too few and the trails evaporate faster than they are
	// reinforced, too many and the whole surface saturates into one sheet.
	Density float64

	// StepsPerSecond is how many simulation steps run each second. A particle
	// moves Speed pixels on each one, so this and Speed together are how fast
	// the thing crawls.
	StepsPerSecond float64

	// Speed is how far a particle moves per step, in pixels.
	Speed float64

	// SensorDist is how far ahead the three sensors reach, in pixels, and
	// SensorAngle is how far the outer two are splayed off the heading, in
	// radians. Their ratio to Speed is the single most important number here:
	// sensing much further than it steps makes a particle commit to a distant
	// trail and gives broad, smooth veins, while sensing barely past its own
	// nose gives a fine crazed mesh.
	SensorDist  float64
	SensorAngle float64

	// TurnAngle is how far a particle swings when a sensor wins, in radians
	// per step. Well below SensorAngle a particle cannot turn fast enough to
	// lock onto a trail it has found, and the field settles into a broad even
	// smear with no veins in it at all; well above, the veins survive but come
	// out coarse and curled. The default is Jones' figure of twice the sensor
	// angle.
	TurnAngle float64

	// Deposit is how much chemical a particle leaves per step, and Decay is
	// the fraction of the field lost per step. Their quotient sets the
	// brightness the veins settle at; Decay alone sets how long an abandoned
	// trail stays visible, and it is the only thing stopping the picture from
	// filling in.
	Deposit float64
	Decay   float64

	// Diffuse is how much of each cell is replaced by the average of its
	// neighbors per step, from 0 for none to 1 for a full box blur. Some is
	// necessary: with none a trail is one pixel wide, a sensor that is not
	// exactly on it cannot find it, and what comes out is a speckled patchwork
	// rather than continuous veins.
	Diffuse float64

	// Contrast is the field value that maps to the middle of the palette. The
	// field is unbounded — a vein carrying heavy traffic can sit an order of
	// magnitude above the haze around it — so it is compressed rather than
	// scaled, and this is where the compression puts its knee.
	Contrast float64

	// Palette colors the field, dark where nothing goes and bright along the
	// veins.
	Palette canvas.Palette
}

// New returns a slime mold. seed of 0 gives a fixed scatter, which makes tests
// repeatable.
//
// The defaults are close to Jones' published set, converted from his grid
// units: sensors a few pixels out at about 22 degrees, a 45 degree turn, and
// a step of a pixel. They are worth leaving alone until you have watched what
// each one does on its own.
func New(seed int64) *Physarum {
	return &Physarum{
		rng:            rand.New(rand.NewSource(seed)), //nolint:gosec
		Density:        110,
		StepsPerSecond: 60,
		Speed:          1.0,
		SensorDist:     5.5,
		SensorAngle:    22.5 * math.Pi / 180,
		TurnAngle:      45 * math.Pi / 180,
		Deposit:        5,
		Decay:          0.08,
		Diffuse:        0.55,
		Contrast:       4,
		Palette:        Slime,
	}
}

// Resize allocates the field and scatters the particles. Called before the
// first frame and on every resize.
func (p *Physarum) Resize(w, h int) {
	p.w, p.h = w, h
	p.fw, p.fh = float64(w), float64(h)
	p.acc, p.steps = 0, 0
	p.field = make([]float32, w*h)
	p.next = make([]float32, w*h)
	p.taken = make([]bool, w*h)

	if p.Density <= 0 {
		p.Density = 1
	}
	n := int(p.Density * float64(w*h) / 1000)
	if n < 1 {
		n = 1
	}
	// Exclusion means the surface cannot hold more particles than it has
	// pixels, and a population that fills it has nowhere to move to. Half the
	// pixels is well clear of that.
	if max := w * h / 2; n > max {
		n = max
	}
	p.agents = make([]agent, n)
	for i := range p.agents {
		// Scattered uniformly with random headings rather than seeded in a
		// disc or a ring. A symmetric start gives a symmetric first few
		// seconds, and the interesting part of this is that structure appears
		// out of nothing — handing it structure to begin with is cheating.
		a := &p.agents[i]
		a.dir = p.rng.Float64() * 2 * math.Pi
		if w <= 0 || h <= 0 {
			continue
		}
		// Placed one to a pixel from the start, or the first step would have
		// to resolve a pile-up that the rest of the run cannot create.
		for {
			a.x = p.rng.Float64() * p.fw
			a.y = p.rng.Float64() * p.fh
			c := int(a.y)*w + int(a.x)
			if !p.taken[c] {
				p.taken[c] = true
				break
			}
		}
	}
}

// maxStepsPerFrame caps how much one frame will catch up by. A fully clamped
// frame at the default rate is six steps, so this is slack rather than a limit
// that normally bites; it stops a large StepsPerSecond turning one hitch into
// an unbounded burst.
const maxStepsPerFrame = 32

// Frame advances the colony for dt seconds and draws the field.
func (p *Physarum) Frame(s *canvas.Surface, dt float64) {
	if p.w == 0 || p.h == 0 {
		return
	}
	if p.StepsPerSecond > 0 {
		interval := 1 / p.StepsPerSecond
		p.acc += dt
		for n := 0; p.acc >= interval; n++ {
			if n >= maxStepsPerFrame {
				// Too far behind to catch up. Drop the backlog rather than let
				// it compound into ever longer frames.
				p.acc = 0
				break
			}
			p.step()
			p.acc -= interval
		}
	}
	p.draw(s)
}

// step is one whole tick of the model: everyone senses and turns, everyone
// moves and deposits, then the field blurs and evaporates.
func (p *Physarum) step() {
	p.steps++
	for i := range p.agents {
		a := &p.agents[i]

		// Three samples, taken from where the particle is now. A particle that
		// sensed from where it is about to be would be steering by the trail
		// it is on the point of laying itself.
		fl := p.sense(a, -p.SensorAngle)
		fc := p.sense(a, 0)
		fr := p.sense(a, +p.SensorAngle)

		switch {
		case fc > fl && fc > fr:
			// Straight ahead is best: hold the heading. This is the case that
			// makes a trail a trail — once on one, a particle stays on it.
		case fc < fl && fc < fr:
			// Both flanks beat the center, which happens on the far side of a
			// vein or in the trough between two. Committing to the larger one
			// would make the choice deterministic and let the population
			// funnel into whichever side rounding favored; a coin toss is what
			// keeps the two branches of a fork both fed.
			if p.rng.Intn(2) == 0 {
				a.dir -= p.TurnAngle
			} else {
				a.dir += p.TurnAngle
			}
		case fl > fr:
			a.dir -= p.TurnAngle
		case fr > fl:
			a.dir += p.TurnAngle
		}
		a.dir = wrapAngle(a.dir)

		nx := wrapf(a.x+math.Cos(a.dir)*p.Speed, p.fw)
		ny := wrapf(a.y+math.Sin(a.dir)*p.Speed, p.fh)
		from := int(a.y)*p.w + int(a.x)
		to := int(ny)*p.w + int(nx)
		if from != to {
			if p.taken[to] {
				// Blocked. The particle stays where it is and takes a new
				// heading at random rather than queueing: a queue would let
				// the jam propagate backwards along the vein as a shock wave,
				// and what should happen instead is that the traffic that does
				// not fit goes looking for another way round.
				a.dir = p.rng.Float64() * 2 * math.Pi
				continue
			}
			p.taken[from] = false
			p.taken[to] = true
		}
		a.x, a.y = nx, ny

		// Deposit after moving, so the trail is behind the particle rather
		// than under the sensors it is about to read.
		p.field[to] += float32(p.Deposit)
	}
	p.diffuseDecay()
}

// sense reads the field at one sensor, off the heading by da radians.
func (p *Physarum) sense(a *agent, da float64) float32 {
	ang := a.dir + da
	x := int(wrapf(a.x+math.Cos(ang)*p.SensorDist, p.fw))
	y := int(wrapf(a.y+math.Sin(ang)*p.SensorDist, p.fh))
	return p.field[y*p.w+x]
}

// diffuseDecay blurs the field into next, evaporates it and swaps the buffers.
//
// The edges wrap. A closed boundary would pile chemical up against the walls
// and grow a bright frame around the picture, because nothing there ever
// diffuses away outward; on a torus a vein that leaves one side arrives at the
// other and the network is seamless.
func (p *Physarum) diffuseDecay() {
	w, h := p.w, p.h
	keep := float32(1 - p.Diffuse)
	spread := float32(p.Diffuse / 9)
	survive := float32(1 - p.Decay)
	for y := 0; y < h; y++ {
		up := ((y - 1 + h) % h) * w
		mid := y * w
		down := ((y + 1) % h) * w
		for x := 0; x < w; x++ {
			l := (x - 1 + w) % w
			r := (x + 1) % w
			sum := p.field[up+l] + p.field[up+x] + p.field[up+r] +
				p.field[mid+l] + p.field[mid+x] + p.field[mid+r] +
				p.field[down+l] + p.field[down+x] + p.field[down+r]
			p.next[mid+x] = (p.field[mid+x]*keep + sum*spread) * survive
		}
	}
	p.field, p.next = p.next, p.field
}

// draw paints the field. Nothing about the particles is drawn: a particle is
// a point with no extent, and what the eye is following is the chemical they
// have left behind, which is the object the model is actually about.
func (p *Physarum) draw(s *canvas.Surface) {
	c := float32(p.Contrast)
	if c <= 0 {
		c = 1
	}
	for y := 0; y < p.h; y++ {
		row := y * p.w
		for x := 0; x < p.w; x++ {
			f := p.field[row+x]
			// Compressed, not scaled: a vein sits far above the haze, and a
			// linear map either clips the veins flat or leaves everything else
			// black. This is monotonic, saturates gently and never overflows
			// the ramp.
			v := int(255 * f / (f + c))
			if v > 255 {
				v = 255
			}
			s.Set(x, y, p.Palette[v])
		}
	}
}

// wrapAngle brings a heading back into one turn, so it cannot drift out to
// where the float has no precision left after an hour of turning.
func wrapAngle(a float64) float64 {
	for a < 0 {
		a += 2 * math.Pi
	}
	for a >= 2*math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

// wrapf brings a coordinate back onto the torus. The loops handle the only
// case that arises — a particle steps at most Speed or senses at most
// SensorDist past an edge — and cost nothing when it does not.
func wrapf(v, m float64) float64 {
	for v < 0 {
		v += m
	}
	for v >= m {
		v -= m
	}
	return v
}

// Run grows a slime mold on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
