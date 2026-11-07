package ripple

import "math"

// Surface is a water surface the lens can look through: Field, which steps
// the wave equation, or Tank, which steps water.
type Surface interface {
	// Drop raises (or, with amp below 0, lowers) a smooth bump at x, y.
	Drop(x, y, radius, amp float32)
	// Line drops along a segment: a wake.
	Line(x0, y0, x1, y1, radius, amp float32)
	// Step runs the surface on a step.
	Step()
	// Still empties it.
	Still()
	// Height reads one cell, clamped to the surface.
	Height(x, y int) float32
	// Heights is the surface, row-major.
	Heights() []float32
	// Size is its width and height in cells.
	Size() (w, h int)
	// Tune sets its knobs: speed, damping, viscous spread and what the
	// edges send back, each as the surface reads it.
	Tune(speed, damping, spread, reflect float32)
	// Drift is how far it carries what floats on it at x, y in a step, in
	// cells.
	Drift(x, y float32) (dx, dy float32)
	// StepSeconds is how much of the surface's own time the last step
	// covered, in seconds.
	StepSeconds() float32
	// CellAspect is a cell's width over its height: what a distance across
	// is multiplied by to be measured as one up and down is.
	CellAspect() float32
}

var (
	_ Surface = (*Field)(nil)
	_ Surface = (*Tank)(nil)
)

// drop adds a raised cosine of the given radius and height at x, y to the
// w by h grid g, whose cells are aspect times as wide as they are tall: the
// radius is in cell heights, so the bump is round on the screen.
//
// The profile is a raised cosine rather than a spike. A single-cell impulse
// contains every wavelength the grid can represent, including the ones at the
// stencil's resolution limit, and those do not propagate — they sit and
// shimmer. A smooth bump of a few cells radiates a ring, which is what a
// disturbance in water actually does.
func drop(g []float32, w, h int, aspect, x, y, radius, amp float32) {
	if radius < 1 {
		radius = 1
	}
	r, rx := int(radius)+1, int(radius/aspect)+1
	cx, cy := int(x), int(y)
	for dy := -r; dy <= r; dy++ {
		for dx := -rx; dx <= rx; dx++ {
			px, py := cx+dx, cy+dy
			if px < 0 || py < 0 || px >= w || py >= h {
				continue
			}
			d := float32(math.Hypot(float64(float32(dx)*aspect), float64(dy)))
			if d > radius {
				continue
			}
			// cos ramp: full amplitude at the center, zero and flat at the rim.
			k := 0.5 * (1 + float32(math.Cos(float64(d/radius)*math.Pi)))
			g[py*w+px] += amp * k
		}
	}
}

// line drops along the segment from one point to another, which is what a
// finger DRAGGED through water leaves — a wake, not a series of dots. Called
// with the pointer's previous and current position, it stays continuous
// however fast the pointer moves; dropping at the current position alone
// leaves gaps at speed, and the gaps read as a dotted line.
func line(g []float32, w, h int, aspect, x0, y0, x1, y1, radius, amp float32) {
	if radius < 1 {
		radius = 1
	}
	dx, dy := float64((x1-x0)*aspect), float64(y1-y0)
	steps := int(math.Hypot(dx, dy)/float64(radius)*2) + 1
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		drop(g, w, h, aspect, x0+(x1-x0)*t, y0+(y1-y0)*t, radius, amp/float32(steps+1)*2)
	}
}
