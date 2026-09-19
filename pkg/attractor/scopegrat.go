package attractor

import "github.com/0magnet/chaosrack/pkg/rackspec"

// The graticule of a real oscilloscope — the thing you actually read the
// measurement off.
//
// chaosrack already had a graticule, but it is a GONIOMETER's: two axes, two
// diagonals and a box, which is what you want behind a stereo figure and is
// not what is etched on the face of a 465 or a 1740A. A scope graticule is a
// ruler. It is 8 divisions tall by 10 wide because that is what the industry
// settled on, the two center axes carry minor ticks at a fifth of a division
// so you can interpolate, and it has risetime references at 0% and 100% with
// 10% and 90% marked, because measuring a rise between those two points is
// the job the marks exist for.
//
// Pure geometry, in divisions, with no drawing in it: the GL path scales it
// to the screen, and a faceplate that wants to silkscreen the same figure
// gets the same numbers. It is the part that has to be RIGHT — a graticule
// that is merely grid-shaped is decoration, and decoration is the one thing
// an instrument's screen must not put in front of the signal.

// Scope graticule geometry, in divisions from the center of the screen.
//
// The screen spans ±gratHalfW horizontally and ±gratHalfH vertically, so a
// division is one unit and every number below reads as a scope manual reads.
const (
	// The division counts are rackspec's: the graticule drawn on the model
	// and the one etched on the rack scope's tube are the SAME instrument,
	// and two copies of 10 and 8 is how they would stop being.
	gratDivX  = rackspec.ScopeDivX // horizontal divisions, the time axis
	gratDivY  = rackspec.ScopeDivY // vertical divisions, the amplitude axis
	gratHalfW = gratDivX / 2
	gratHalfH = gratDivY / 2

	// gratTickDiv is the minor tick spacing along the center axes: a fifth
	// of a division, which is what lets a reading be interpolated.
	gratTickDiv = 0.2

	// The risetime references. 0% and 100% sit two divisions either side of
	// center, which is the four-division amplitude a scope asks you to set
	// the pulse to before reading its rise; 10% and 90% then fall 0.4 of a
	// division inside each of them.
	gratRefDiv = 2.0
	gratPctDiv = 0.4
)

// gratWeight says how heavily a line is drawn. A real graticule is not one
// uniform grid: the center axes are the ones you measure against and they
// are cut heavier than the division lines, which are in turn heavier than
// the ticks. Drawing them all the same is the single thing that makes a
// rendered graticule look like a spreadsheet.
type gratWeight int

const (
	gratWeightDiv  gratWeight = iota // the division lines and the border
	gratWeightAxis                   // the two center axes
	gratWeightTick                   // minor ticks and the percent references
)

// gratLine is one segment of the graticule, in divisions.
type gratLine struct {
	X0, Y0, X1, Y1 float32
	W              gratWeight
}

// scopeGraticule returns the whole figure.
//
// Built rather than tabulated because the numbers above are the spec and a
// table would be the spec copied out by hand — and a graticule whose ticks
// have drifted one position out of step with its divisions is wrong in a way
// nobody notices until they trust a reading.
// scopeGraticuleCached is the figure, built once.
//
// It is constant — the same hundred and forty lines every time — and it
// was being rebuilt three times a frame by the canvas drawing, which at
// sixty frames a second is twenty-five thousand allocations a second of
// something that never changes. In wasm that is collector pauses, and
// they were visible: the model hesitated about once a second.
var scopeGraticuleCache []gratLine

func scopeGraticule() []gratLine {
	if scopeGraticuleCache == nil {
		scopeGraticuleCache = buildScopeGraticule()
	}
	return scopeGraticuleCache
}

func buildScopeGraticule() []gratLine {
	out := make([]gratLine, 0, 64)

	// The border and the interior division lines. The border is part of the
	// grid and not a frame around it: on a real tube the outermost division
	// line IS the edge of the usable screen.
	for i := -gratHalfW; i <= gratHalfW; i++ {
		if i == 0 {
			continue // the center axis is drawn heavier, below
		}
		out = append(out, gratLine{float32(i), -gratHalfH, float32(i), gratHalfH, gratWeightDiv})
	}
	for i := -gratHalfH; i <= gratHalfH; i++ {
		if i == 0 {
			continue
		}
		out = append(out, gratLine{-gratHalfW, float32(i), gratHalfW, float32(i), gratWeightDiv})
	}

	// The two center axes, full width and full height.
	out = append(out,
		gratLine{-gratHalfW, 0, gratHalfW, 0, gratWeightAxis},
		gratLine{0, -gratHalfH, 0, gratHalfH, gratWeightAxis},
	)

	// Minor ticks, every fifth of a division along both center axes. They
	// cross the axis rather than sitting on one side of it, which is how you
	// can still read them once the trace is sitting on the axis.
	const tickLen = 0.12
	for _, t := range gratTicks(gratHalfW) {
		out = append(out, gratLine{t, -tickLen, t, tickLen, gratWeightTick})
	}
	for _, t := range gratTicks(gratHalfH) {
		out = append(out, gratLine{-tickLen, t, tickLen, t, gratWeightTick})
	}

	// Risetime references. The 0% and 100% lines run the full width, because
	// you line the pulse up to them; the 10% and 90% marks are short, at the
	// left edge, because you only read the crossing there.
	const pctLen = 0.5
	for _, s := range []float32{-1, 1} {
		y := s * gratRefDiv
		out = append(out, gratLine{-gratHalfW, y, gratHalfW, y, gratWeightTick})
		// 0.4 division INSIDE the reference it belongs to: 10% is just above
		// the 0% line, 90% just below the 100% one.
		p := y - s*gratPctDiv
		out = append(out, gratLine{-gratHalfW, p, -gratHalfW + pctLen, p, gratWeightTick})
	}
	return out
}

// gratTicks returns the minor tick positions along one center axis, out to
// half, skipping the ones that land on a division line — those already have
// a full line through them and a tick on top reads as a thicker division.
func gratTicks(half int) []float32 {
	var out []float32
	steps := int(float32(half)/gratTickDiv + 0.5)
	for i := -steps; i <= steps; i++ {
		// Integer arithmetic for the division test: 0.2*5 in float32 is not
		// exactly 1, and a tick that misses the skip by a rounding error is
		// drawn straight on top of a division line.
		if i%5 == 0 {
			continue
		}
		out = append(out, float32(i)*gratTickDiv)
	}
	return out
}
