// Package julia draws a Julia set whose shape will not hold still.
//
// # THE SET
//
// Pick a complex number c. For every point z on the screen, iterate
// z ← z² + c and count how many steps it takes to escape past a radius of
// two, which is the point of no return. Points that never escape are the
// filled Julia set of c; points that do are colored by how long they lasted,
// and those escape counts pile up into bands that trace the boundary.
//
// # WHY c WALKS
//
// One c is one picture, and a still picture is a poster. What is animated here
// is c itself, walked all the way around the boundary of the main cardioid of
// the Mandelbrot set — because the Mandelbrot set is exactly the map of which
// c give a connected Julia set, so its edge is where the shapes are richest and
// where a small move in c changes the picture most. Round that circuit the set
// passes through spirals, rabbits, dendrites and near-circles, and comes back.
// Nothing is morphing between drawings: there is one formula and one moving
// parameter, and every shape it passes through is the true Julia set of the c
// at that instant.
//
// The path runs a few hundredths INSIDE the edge rather than on it. On the edge
// itself the set is a dendrite with no interior — at terminal resolution a
// smooth wash of escape bands with no silhouette in it — and outside, it
// shatters into a dust that looks like nothing at all. Just inside, there is
// always a solid core with a recognizable outline, and it is the outline that
// carries the animation. A third harmonic on how far inside it runs makes the
// path a closed curve rather than a retraced cardioid, so the core breathes
// thin and thick three times a circuit. See Inset and Wobble.
//
// # THE TWO THINGS THAT MAKE IT AFFORDABLE
//
// z² + c is even in z, so the picture is symmetric under a half turn about the
// origin: z and −z escape in exactly the same number of steps. Half the pixels
// are therefore computed and the other half copied, which is not an
// approximation — it is exact, and it is worth a factor of two.
//
// The other half of the budget is the iteration limit, and it is adaptive. The
// cost of a frame is pixels times iterations, and a terminal can be twenty
// times the area of another one, so a limit that suits a small window either
// starves a large one of detail or stalls it. Instead the limit starts from
// the pixel count and is then steered by how long frames are actually taking:
// falling behind lowers it, keeping up raises it. What that trades away is the
// fineness of the boundary — a lower limit paints more of the near-boundary
// filigree as interior — which is the right thing to lose, because motion at
// the frame rate reads better than a crisper edge that stutters.
//
// Written from the description of the escape-time algorithm. No code is taken
// from any existing implementation.
package julia

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// baseWalk is how fast c goes around its loop at Speed 1, in radians per
// second. A full circuit takes a little over a minute and a half, which is
// slow enough that the shape is never seen to jump and long enough that the
// same picture does not come round while anyone is still watching.
// bailout is the escape radius squared. Larger than the 4 the mathematics
// needs, because the smooth escape count below is only accurate well outside
// the radius; the extra couple of iterations per pixel are what stop the color
// bands from rippling.
const bailout = 256

const baseWalk = 0.068

// baseExtent is the half-height of the view in the complex plane at Zoom 1.
// A filled Julia set for a c on this path fits inside a radius of about 1.1,
// so this leaves a margin of escaping points around it — those are the colored
// bands, and cropping them off would leave the shape with no context.
const baseExtent = 1.15

// Julia is the animation. The zero value is not usable; call New.
type Julia struct {
	w, h    int
	t       float64 // seconds, which is the phase of the walk
	cycle   float64 // palette rotation, in entries
	iter    float64 // the current iteration limit, carried as a float so the
	areaCap float64 // controller can move it by percentages rather than steps

	// Speed scales how fast c walks its loop, and so how fast the shape
	// morphs. Negative walks the other way round.
	Speed float64

	// Inset is how far inside the cardioid the path runs, as a fraction of the
	// distance from the origin. Zero puts c exactly on the boundary of the
	// Mandelbrot set, where the Julia set is a dendrite with no interior left
	// at all; a few hundredths inside keeps a solid core in the middle of the
	// picture, which is what gives the shape a silhouette to recognize.
	Inset float64

	// Wobble is a third harmonic on that inset, which makes the path a closed
	// curve rather than a retraced cardioid: it breathes toward the boundary
	// and away from it three times a circuit, and the core thins and thickens
	// with it. Kept smaller than Inset — past that the path crosses outside
	// the set, the Julia set falls apart into dust, and at a terminal's
	// resolution a dust is a smooth wash with nothing in it to look at.
	Wobble float64

	// Zoom magnifies the view. Above about three the boundary detail outruns
	// any iteration limit that fits in a frame and the picture goes flat.
	Zoom float64

	// Bands is how many palette entries one escape step advances by. Small is
	// a wide smooth gradient across the escape bands; large is a tight
	// contour map of them.
	Bands float64

	// CycleRate is how many times a second the palette rotates through
	// itself. The bands are level sets of the escape count, so rotating the
	// ramp makes them appear to flow outward from the boundary without
	// anything about the set having changed.
	CycleRate float64

	// MinIter and MaxIter bound the adaptive iteration limit. The floor
	// matters more than the ceiling: below about twenty steps the bands are
	// too few to show a boundary at all, and the picture stops being a Julia
	// set and becomes a blob.
	MinIter, MaxIter int

	// PixelBudget is how many pixel-iterations one frame may cost, which is
	// what the iteration limit is derived from when the surface is sized. It
	// is the honest unit here: a frame costs pixels times iterations and
	// nothing else.
	PixelBudget float64

	// FrameBudget is how long a frame is allowed to take, in seconds. The
	// controller watches the elapsed time it is handed and lowers the
	// iteration limit when frames run longer than this. The default sits a
	// little above a thirtieth of a second, so a loop that is keeping up
	// reads as comfortable and one that is dropping ticks does not.
	FrameBudget float64

	// Palette colors a point by its escape count. It must loop — the count is
	// taken modulo the ramp — or a seam appears at every multiple of 256
	// escape steps, and the seam moves as the palette cycles.
	Palette canvas.Palette
}

// New returns a Julia animation.
//
// There is no seed: everything here is a function of elapsed time, so two runs
// started at the same moment show the same thing. The set has no need of
// randomness — its complexity is in the arithmetic.
func New() *Julia {
	return &Julia{
		Speed:       1,
		Inset:       0.035,
		Wobble:      0.025,
		Zoom:        1,
		Bands:       40,
		CycleRate:   0.06,
		MinIter:     24,
		MaxIter:     260,
		PixelBudget: 2.4e6,
		FrameBudget: 0.04,
		Palette:     canvas.Plasma,
	}
}

// Resize records the surface and sets the starting iteration limit from its
// area. A first guess, not a verdict: the controller in Frame moves it from
// here once there is evidence about how long frames really take.
func (j *Julia) Resize(w, h int) {
	j.w, j.h = w, h
	if w == 0 || h == 0 {
		return
	}
	j.areaCap = j.PixelBudget / float64(w*h)
	j.iter = j.clampIter(j.areaCap)
}

// clampIter holds the limit inside its bounds.
func (j *Julia) clampIter(v float64) float64 {
	lo, hi := float64(j.MinIter), float64(j.MaxIter)
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// param is c at the current phase: a point just inside the boundary of the
// main cardioid of the Mandelbrot set.
//
// The cardioid c = e^{ia}/2 − e^{2ia}/4 IS that boundary, so walking it is not
// an approximation to a path near the boundary — it is the boundary, pulled a
// few hundredths toward the origin. The region is star-shaped about the
// origin, so scaling a boundary point down moves it inside and nowhere else,
// which is what makes one multiply enough to control how near the edge the
// walk runs.
func (j *Julia) param() (re, im float64) {
	a := j.t * baseWalk * j.Speed
	s1, c1 := math.Sincos(a)
	s2, c2 := math.Sincos(2 * a)
	rho := 1 - j.Inset + j.Wobble*math.Sin(3*a)
	return rho * (c1/2 - c2/4), rho * (s1/2 - s2/4)
}

// Period is how long c takes to come back to where it started, in seconds.
// Exported because it is the one number that says how long the animation is
// before it repeats, and because the path being closed is the property the
// whole design rests on.
func (j *Julia) Period() float64 {
	if j.Speed == 0 {
		return math.Inf(1)
	}
	return 2 * math.Pi / (baseWalk * math.Abs(j.Speed))
}

// Frame walks c, steers the iteration limit and draws the set.
func (j *Julia) Frame(s *canvas.Surface, dt float64) {
	if j.w == 0 || j.h == 0 {
		return
	}
	j.t += dt
	j.cycle += dt * j.CycleRate * 256
	// Kept in range rather than allowed to grow: it is added to an escape
	// count and truncated to an int, and a float64 large enough to have lost
	// its low bits would make the cycling jerk.
	j.cycle = math.Mod(j.cycle, 256)

	// The controller. Down hard and up gently, which is the usual shape for
	// anything steering against a deadline: overshooting the budget costs a
	// dropped frame now, while being under it costs only detail, so the cheap
	// mistake is the one to make repeatedly.
	switch {
	case dt > j.FrameBudget:
		j.iter *= 0.88
	case dt < j.FrameBudget*0.9:
		j.iter *= 1.03
	}
	j.iter = j.clampIter(j.iter)
	maxIter := int(j.iter)

	cre, cim := j.param()

	// The view. Scaled by the shorter side so the set keeps its proportions
	// in a window of any shape, and centered on the exact middle of the pixel
	// grid so that the half-turn symmetry below is exact rather than off by
	// half a pixel.
	zoom := j.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	scale := 2 * baseExtent / (zoom * float64(min(j.w, j.h)))
	cx := float64(j.w-1) / 2
	cy := float64(j.h-1) / 2

	bands := j.Bands
	cycle := j.cycle

	// Half the rows, and the other half copied through the origin. See the
	// package comment: z² + c is even, so the two are equal by construction.
	rows := (j.h + 1) / 2
	for y := 0; y < rows; y++ {
		zy0 := (float64(y) - cy) * scale
		for x := 0; x < j.w; x++ {
			zx, zy := (float64(x)-cx)*scale, zy0

			n := 0
			var mag2 float64
			for ; n < maxIter; n++ {
				x2, y2 := zx*zx, zy*zy
				// Escaped, and by a long way: see bailout.
				if mag2 = x2 + y2; mag2 > bailout {
					break
				}
				zy = 2*zx*zy + cim
				zx = x2 - y2 + cre
			}

			var col tcell.Color
			if n >= maxIter {
				// Never escaped: this is the filled set. Left as the
				// terminal's own background rather than painted black, so the
				// interior reads as a hole in the picture — which is what it
				// is — on a light terminal as well as a dark one.
				col = tcell.ColorDefault
			} else {
				// Smooth escape count. The integer count is not enough here:
				// four fifths of the exterior escapes within three steps, so
				// an integer index gives four flat colors over most of the
				// screen and the bands read as poster edges. Correcting by how
				// far past the bailout the point actually landed turns that
				// into a continuous gradient. It costs one logarithm per
				// escaping pixel — cheap against the iteration that preceded
				// it, and the single biggest difference to how this looks.
				mu := float64(n) - math.Log2(0.5*math.Log(mag2))
				// Floor rather than a plain conversion, which truncates toward
				// zero and would put a seam wherever the index crosses it. The
				// mask then wraps a negative index the way a cyclic palette
				// wants.
				col = j.Palette[int(math.Floor(mu*bands+cycle))&255]
			}
			s.Set(x, y, col)
			s.Set(j.w-1-x, j.h-1-y, col)
		}
	}
}

// Run draws a Julia set on the screen until the user quits.
func Run(screen tcell.Screen) error {
	return canvas.Run(screen, New(), canvas.Options{})
}
