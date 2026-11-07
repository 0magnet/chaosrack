package ripple

import (
	"math"
	"math/bits"
)

// Tank is a water surface stepped as water moves, not as a drum head does.
//
// # WHAT FIELD GETS WRONG
//
// Field steps the wave equation, in which every wave travels at one speed.
// That is a drum head, a sheet of rubber: drop something on it and one ring
// runs out and keeps its shape. Water does not do that. How fast a wave on
// water travels depends on how long it is (dispersion):
//
//	ω² = k·tanh(kD)·(g + σk²/ρ)
//
// Long waves are carried by gravity and the longer they are the faster they
// go; short ones, ripples under about 1.7 cm, are carried by surface tension
// and the SHORTER they are the faster they go. So a pebble's splash spreads
// into a train: the finest ripples race out ahead, the long swells follow,
// and the slowest waves of all, about 1.7 cm long, are left behind in the
// middle. The rings multiply as they go. That is the look of water, and no
// one speed can make it.
//
// # HOW IT IS STEPPED
//
// Tessendorf's eWave (2014): the surface's height h and its velocity
// potential φ (the flow under the surface is the gradient of φ) are taken
// apart into waves by an FFT, and each wave is turned on by exactly its own
// frequency,
//
//	h' =  cos(ωt)·h + (A/ω)·sin(ωt)·φ      A = k·tanh(kD)
//	φ' =  cos(ωt)·φ − (B/ω)·sin(ωt)·h      B = g + σk²/ρ
//
// which is the exact solution, not an approximation of it, for any step. So
// there is no speed past which it blows up — Field's c² ≤ ½ — and the speed
// knob is simply how fast the tank's clock runs.
//
// What the FFT cannot do is edges: it treats the surface as if it wrapped
// round. So after each step the walls and the open water are put back in on
// the grid itself: a wall is a border where the water is held still, which a
// wave turns back from (iWave's obstruction mask), and open water is a band
// inside it that soaks a wave up before it gets there.
//
// # WHY φ
//
// φ is the water moving, and its gradient is how fast the surface flows
// under a thing floating on it: round in a circle as each wave passes, a
// little forward each time (Stokes drift). Foam riding that drifts the way
// foam on water does, which a slope could only imitate.
//
// # UNITS AND THE SHAPE OF A CELL
//
// The grid is square, N by N, but the picture it covers is not, so a cell
// is Aspect times as wide as it is tall. The physics is worked in meters, a
// cell being tankSide/N tall, so a wave is as long across as up and down and
// a drop spreads in a circle on the screen, not an ellipse stretched with
// it. Heights are in cell heights, so a bubble on a wave of height 0.5 rides
// a circle half a cell across; positions are in cells.
type Tank struct {
	W, H     int       // cells across and down
	h, phi   []float32 // the surface, on the grid
	src      []float32 // what has been dropped on it since the last Begin
	zr, zi   []float32 // h + iφ, transformed
	or, oi   []float32 // the step's result, before it is transformed back
	fft      *fft2
	kw       []float32 // each wave's wavenumber, radians a meter, as the spectrum is laid out
	top      []float32 // each wave's share of the finest the grid holds, 0..1
	kwFor    float32   // the Aspect kw was made for
	edge     []float32 // what the edges keep of each cell a step, for the Reflect and steps it was made for
	edgeFor  [2]float32
	coef     []float32 // c, A·s/ω, B·s/ω for each wave, for the step they were made for
	coefDt   float32
	coefNu   float32
	coefKeep float32
	coefTop  int  // the steps the top's share was taken over
	flight   bool // a step is begun and not finished

	// Speed is how fast the tank's clock runs: 1 is real time, a step being
	// a sixtieth of a second. Water this size is quick — a ripple crosses it
	// in about a second — and slowed down it is easier to watch.
	Speed float32
	// Damping is the fraction of every wave kept for each sixtieth of a
	// second of the tank's time: the loss to the walls and the bottom and
	// whatever is on the water, which a real tank has more of than viscosity
	// alone. It goes by the tank's clock, so slowing the clock slows the
	// fading with it, and a wave travels as far before it is gone.
	Damping float32
	// Spread is how viscous the water is, as a share of 0.12 being water's.
	// Viscosity takes the short waves first — at the rate 2νk², so a ripple
	// half the length dies four times as fast — which is why real water is
	// smooth between the waves anyone meant to make.
	Spread float32
	// Reflect is how much of a wave the edges send back, 0..1: 1 is a tank
	// with walls, 0 open water.
	Reflect float32
	// Aspect is a cell's width over its height: the picture's, over the
	// grid's.
	Aspect float32
	// Dt is a sixtieth of a second of the tank's clock, in seconds: what a
	// step of one covers, and what each of a page's frames does.
	Dt float32
}

// The water, and the tank it is in.
const (
	// tankSide is the tank's height on the screen, in meters. At this size a
	// 128-cell grid is 2.3 mm a cell: the finest ripples it can hold are
	// about 5 mm long, the slowest waves (1.7 cm) are seven cells, and long
	// waves up to the tank's size are carried by gravity — both kinds of
	// wave, and the turn between them, are on the screen at once.
	tankSide = 0.3
	// tankDepth is the water's depth, in meters: a shallow tray, as a
	// ripple tank is. Waves much longer than this feel the bottom and stop
	// spreading out, which keeps the tank's long waves from outrunning
	// everything.
	tankDepth = 0.02
	gravity   = 9.81    // m/s²
	tension   = 7.28e-5 // surface tension over density, m³/s²
	viscosity = 1.0e-6  // water's kinematic viscosity, m²/s
	// edgeBand is how deep into the tank, in cells, open water soaks up a
	// wave; edgeSoak how much of it the band's outer cells take a step.
	edgeBand = 12
	edgeSoak = 0.25
	// tankWall is how thick a wall is, in cells, each side. A wave is not
	// turned by its next cell alone but by the water a few cells round — a
	// spectral step reaches further than a stencil — and through a wall one
	// cell thick each side 2% of a wave came out on the far side.
	tankWall = 2
	// stepSeconds is a step of the tank's clock at Speed 1: one of the
	// page's frames.
	stepSeconds = 1.0 / 60
	// tankKnobClock is the clock at the speed knob's 1. Water this size in
	// real time is too quick to watch: a ripple is across the screen in a
	// second and a splash's train spreads out within a few frames. The
	// knob's 0.5 runs it a fifth of real time, slow motion, every wave
	// still at its own speed beside the others.
	tankKnobClock = 0.4
	// The finest waves the grid holds, two cells long, cannot be drawn: on
	// the screen they are a stair-step, not a ripple. And the grid holds
	// finer ones some ways than others: up and down more than across, where
	// its cells are wide, and finest of all on the diagonals, at the corners
	// of the square of waves an FFT has. The finest ripples are the fastest,
	// so they lead a splash's front, and a front made of those was a square,
	// running ahead where the grid was finest. So the tank keeps only waves
	// no finer than the grid holds every way — two of its widest cells — and
	// each step keeps exp(-topCut·q^topSharp) of a wave q of the way from
	// none to that (a spectral filter): past it gone within a step or two,
	// the last sixth of the way thinned, and the rest left alone.
	topCut   = 2
	topSharp = 12
)

// NewTank returns a still tank w cells across and h down, each rounded up
// to a power of two, as the FFT needs.
func NewTank(w, h int) *Tank {
	w, h = pow2(w), pow2(h)
	n := w * h
	return &Tank{
		W: w, H: h,
		h: make([]float32, n), src: make([]float32, n),
		phi: make([]float32, n),
		zr:  make([]float32, n), zi: make([]float32, n),
		or: make([]float32, n), oi: make([]float32, n),
		fft: newFFT2(w, h), kw: make([]float32, n), top: make([]float32, n),
		edge:  make([]float32, n),
		coef:  make([]float32, 3*n),
		kwFor: -1, edgeFor: [2]float32{-1, -1},
		Speed: 0.5, Damping: 0.985, Spread: 0.12, Reflect: 1, Aspect: 1,
	}
}

// pow2 is n, at least 8, rounded up to a power of two.
func pow2(n int) int {
	n = max(n, 8)
	if n&(n-1) != 0 {
		n = 1 << bits.Len(uint(n))
	}
	return n
}

// freq is the signed frequency at index i of an n-point transform.
func freq(i, n int) float32 {
	if i > n/2 {
		return float32(i - n)
	}
	return float32(i)
}

// cell is a cell's height and width, in meters.
func (t *Tank) cell() (dy, dx float32) {
	dy = tankSide / float32(t.H)
	return dy, dy * t.aspect()
}

func (t *Tank) aspect() float32 {
	if t.Aspect <= 0 {
		return 1
	}
	return t.Aspect
}

// waves works out each wave's wavenumber for the shape the cells are. The
// forward transform leaves the spectrum transposed, so the row is the
// frequency across.
func (t *Tank) waves() {
	a := t.aspect()
	if a == t.kwFor {
		return
	}
	t.kwFor = a
	t.coefDt = -1 // made for other waves
	w, h := t.W, t.H
	dy, dx := t.cell()
	for i := range w {
		for j := range h {
			fx, fy := freq(i, w), freq(j, h)
			kx, ky := 2*math.Pi*float64(fx)/(float64(w)*float64(dx)), 2*math.Pi*float64(fy)/(float64(h)*float64(dy))
			t.kw[i*h+j] = float32(math.Hypot(kx, ky))
			// The finest wave held every way: two of the widest cells.
			q := float64(t.kw[i*h+j]) * float64(max(dx, dy)) / math.Pi
			t.top[i*h+j] = float32(math.Exp(-topCut * math.Pow(min(q, 1.5), topSharp)))
		}
	}
}

// Size is the tank's width and height in cells.
func (t *Tank) Size() (int, int) { return t.W, t.H }

// CellAspect is a cell's width over its height.
func (t *Tank) CellAspect() float32 { return t.aspect() }

// Heights is the surface as last stepped, row-major, for uploading as a
// texture.
func (t *Tank) Heights() []float32 { return t.h }

// Height reads one cell, clamped to the tank, with what has been dropped
// there since the last step.
func (t *Tank) Height(x, y int) float32 {
	x, y = min(max(x, 0), t.W-1), min(max(y, 0), t.H-1)
	return t.h[y*t.W+x] + t.src[y*t.W+x]
}

// Drop raises (or lowers) a smooth bump at x, y, round on the screen, its
// radius in cell heights; see drop. It is never narrower than a cell is
// wide: on wide cells a smaller bump is one column of the grid, which the
// grid can only hold as its finest stair-step.
func (t *Tank) Drop(x, y, radius, amp float32) {
	drop(t.src, t.W, t.H, t.aspect(), x, y, max(radius, t.aspect()), amp)
}

// Line drops along a segment, a wake, as wide as Drop's; see line.
func (t *Tank) Line(x0, y0, x1, y1, radius, amp float32) {
	line(t.src, t.W, t.H, t.aspect(), x0, y0, x1, y1, max(radius, t.aspect()), amp)
}

// Tune sets the knobs: the speed knob (the clock, in shares of
// tankKnobClock), what is kept a sixtieth of a second, viscosity as a share
// of 0.12 being water's, and what the edges send back.
func (t *Tank) Tune(speed, damping, spread, reflect float32) {
	t.Speed, t.Damping, t.Spread, t.Reflect = speed*tankKnobClock, damping, spread, reflect
}

// Still empties the tank.
func (t *Tank) Still() {
	clear(t.h)
	clear(t.src)
	clear(t.phi)
}

// Drift is how far the surface carries what floats on it at x, y in a step,
// in cells: the flow at the surface, which is the gradient of φ. Heights
// are in cell heights, so φ is too, and a cell's width turns its slope
// across into meters a second and those into cells.
func (t *Tank) Drift(x, y float32) (dx, dy float32) {
	w := t.W
	i, j := min(max(int(x), 1), w-2), min(max(int(y), 1), t.H-2)
	p := func(a, b int) float32 { return t.phi[b*w+a] }
	ch, cw := t.cell()
	gx, gy := (p(i+1, j)-p(i-1, j))*0.5, (p(i, j+1)-p(i, j-1))*0.5
	return gx * ch / (cw * cw) * t.Dt, gy / ch * t.Dt
}

// Energy is the summed square of the surface's height, with what has been
// dropped on it since the last step.
func (t *Tank) Energy() float64 {
	var e float64
	for i, v := range t.h {
		v += t.src[i]
		e += float64(v) * float64(v)
	}
	return e
}

// dispersion is the frequency of a wave of wavenumber k (radians a meter),
// with A and B: dh/dt = A·φ and dφ/dt = −B·h for that wave.
func dispersion(k float32) (omega, a, b float32) {
	kk := float64(k)
	a = float32(kk * math.Tanh(kk*tankDepth))
	b = float32(gravity + tension*kk*kk)
	return float32(math.Sqrt(float64(a * b))), a, b
}

// coefs works out each wave's turn for a step of dt seconds, viscosity nu
// (m² a second) and keep kept a step.
func (t *Tank) coefs(dt, nu, keep float32, steps int) {
	if dt == t.coefDt && nu == t.coefNu && keep == t.coefKeep && steps == t.coefTop {
		return
	}
	t.coefDt, t.coefNu, t.coefKeep, t.coefTop = dt, nu, keep, steps
	for i, k := range t.kw {
		omega, a, b := dispersion(k)
		top := float32(math.Pow(float64(t.top[i]), float64(steps)))
		fade := keep * top * float32(math.Exp(float64(-2*nu*k*k*dt)))
		c, sa, sb := fade, float32(0), float32(0)
		if omega > 0 {
			s := float32(math.Sin(float64(omega * dt)))
			c = fade * float32(math.Cos(float64(omega*dt)))
			sa, sb = fade*a*s/omega, fade*b*s/omega
		}
		t.coef[3*i], t.coef[3*i+1], t.coef[3*i+2] = c, sa, sb
	}
}

// edges works out what the walls and the open water keep of each cell a
// step. The outermost tankWall cells each side are the wall (twice that
// where the FFT wraps one side round to the other), where the water is held
// still and a wave turns back; inside it, the band soaks a wave up by as
// much as the walls do not send back.
func (t *Tank) edges(reflect float32, steps int) {
	if reflect == t.edgeFor[0] && float32(steps) == t.edgeFor[1] {
		return
	}
	t.edgeFor = [2]float32{reflect, float32(steps)}
	w, h := t.W, t.H
	soak := (1 - min(max(reflect, 0), 1)) * edgeSoak
	for y := range h {
		for x := range w {
			d := min(x, y, w-1-x, h-1-y)
			keep := float32(1)
			switch {
			case d < tankWall:
				keep = 0
			case d < tankWall+edgeBand:
				u := 1 - float32(d-tankWall)/edgeBand
				keep = 1 - soak*u*u
			}
			t.edge[y*w+x] = float32(math.Pow(float64(keep), float64(steps)))
		}
	}
}

// Step runs the tank on a sixtieth of a second of its clock, times Speed:
// Begin and Finish together.
func (t *Tank) Step() {
	t.Begin(1)
	t.Finish()
}

// Begin starts a step of so many sixtieths of a second of the tank's
// clock: what was dropped since the last goes in, the surface is taken
// apart into waves and each is turned on. Until Finish, the surface is as
// it was. A Begin before the last step's Finish finishes it first.
//
// The two halves are for a page to run on frames of their own. A step of
// a 256-by-128 tank is 6 ms of a frame in wasm; begun on one frame and
// finished on the next, each frame takes half, and the surface is new
// every other frame, which with the clock at a fifth of real time is less
// than a cell for the fastest ripple. A step of two sixtieths is the same
// water as two of one: each wave is turned exactly, however far.
func (t *Tank) Begin(steps int) {
	if t.flight {
		t.Finish()
	}
	steps = max(steps, 1)
	speed := max(t.Speed, 0)
	t.Dt = float32(stepSeconds) * speed
	dt := t.Dt * float32(steps)
	nu := viscosity * max(t.Spread, 0) / 0.12
	keep := float32(math.Pow(float64(min(max(t.Damping, 0), 1)), float64(speed)*float64(steps)))
	t.waves()
	t.coefs(dt, nu, keep, steps)
	t.edges(t.Reflect, steps)

	w, h := t.W, t.H
	for i, v := range t.src {
		t.zr[i] = t.h[i] + v
	}
	clear(t.src)
	copy(t.zi, t.phi)
	t.fft.forward(t.zr, t.zi)
	// h and φ are both real, so one complex transform carries the two: the
	// spectrum of h is the even part of z's, and φ's the odd part over i.
	// The spectrum is transposed: w rows of h, a row to a frequency across.
	for i := range w {
		mi := (w - i) & (w - 1)
		for j := range h {
			mj := (h - j) & (h - 1)
			p, q := i*h+j, mi*h+mj
			zr, zi, wr, wi := t.zr[p], t.zi[p], t.zr[q], t.zi[q]
			hr, hi := (zr+wr)*0.5, (zi-wi)*0.5 // h at this wave
			fr, fi := (zi+wi)*0.5, (wr-zr)*0.5 // φ at this wave
			c, sa, sb := t.coef[3*p], t.coef[3*p+1], t.coef[3*p+2]
			nhr, nhi := c*hr+sa*fr, c*hi+sa*fi
			nfr, nfi := c*fr-sb*hr, c*fi-sb*hi
			t.or[p], t.oi[p] = nhr-nfi, nhi+nfr // h + iφ again
		}
	}
	t.flight = true
}

// Finish ends the step Begin started: the waves back into a surface, and
// the walls and open water put in. Without a step begun it does nothing.
func (t *Tank) Finish() {
	if !t.flight {
		return
	}
	t.flight = false
	t.fft.inverse(t.or, t.oi)
	for i, keep := range t.edge {
		t.h[i] = t.or[i] * keep
		t.phi[i] = t.oi[i] * keep
	}
}

// StepSeconds is a sixtieth of a second of the tank's clock: what each of
// the page's frames covers, however the steps are split across them.
func (t *Tank) StepSeconds() float32 { return t.Dt }
