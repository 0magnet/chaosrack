package ripple

import "math/rand/v2"

// Foam is the froth on a surface: bubbles that ride it rather than waves
// that cross it.
//
// A wave leaves; foam stays. What a moving thing churns into water spreads
// away as ripples and is gone in a second, but the bubbles it whips up float
// where they were made, drift on the surface, gather in its hollows and burst
// one by one over several seconds. That difference is the whole reason this
// is a list of bubbles and not more of the height field: a bump in the field
// is a wave, and a wave cannot sit still.
//
// Positions and radii are in the field's cells, so a bubble rides the same
// surface the refraction reads.
type Foam struct {
	Bubbles []Bubble
	// Max is the most bubbles the surface carries. Past it a new bubble is
	// not made: a sheet of foam that thick is opaque however many more are
	// added, and every one is a point drawn per frame.
	Max int
	rng *rand.Rand
}

// Bubble is one bubble on the surface.
type Bubble struct {
	X, Y   float32 // where it floats, in cells
	VX, VY float32 // its drift, cells a step
	R      float32 // its radius, in cells
	Age    int     // steps since it was made
	Life   int     // steps it lasts before it bursts
}

// Foam's constants, in the field's units and steps (a step is a frame).
const (
	// A floating bubble slides down the surface's slope, so it gathers in the
	// troughs, as foam lines up between waves on real water. Per unit slope,
	// in cells a step.
	foamDrift = 0.25
	// How much of its drift a bubble keeps a step: water holds it back, so it
	// follows the waves under it rather than coasting across them.
	foamDrag = 0.9
	// The jostle of the water too fine for the field to carry, in cells a
	// step: without it a bubble on flat water is a fixed dot.
	foamJostle = 0.02
	// Most bubbles are small and a few are large: a radius is foamMinR plus
	// foamSpanR times the cube of a uniform draw.
	foamMinR  = 0.15
	foamSpanR = 0.9
	// How long a bubble lasts, in steps: two to ten seconds at sixty frames.
	foamMinLife  = 120
	foamSpanLife = 480
)

// NewFoam returns an empty surface that carries no more than most bubbles.
func NewFoam(most int, seed uint64) *Foam {
	return &Foam{Max: most, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} //nolint:gosec // where bubbles land on a decorative surface
}

// Spawn makes a bubble at x, y, a little scattered, if the surface has room.
func (fm *Foam) Spawn(x, y float32) {
	if len(fm.Bubbles) >= fm.Max {
		return
	}
	u := fm.rng.Float32()
	fm.Bubbles = append(fm.Bubbles, Bubble{
		X: x + fm.rng.Float32() - 0.5, Y: y + fm.rng.Float32() - 0.5,
		R:    foamMinR + foamSpanR*u*u*u,
		Life: foamMinLife + fm.rng.IntN(foamSpanLife),
	})
}

// Chance reports true with probability p, from the foam's own generator, so
// a caller deciding whether to make a bubble draws from the same sequence.
func (fm *Foam) Chance(p float32) bool { return fm.rng.Float32() < p }

// Step moves every bubble one step on surface f and bursts the ones whose
// time is up. A bubble carried off the surface is gone with it.
func (fm *Foam) Step(f *Field) {
	kept := fm.Bubbles[:0]
	for _, b := range fm.Bubbles {
		b.Age++
		if b.Age >= b.Life {
			continue
		}
		gx, gy := f.Gradient(int(b.X), int(b.Y))
		b.VX = b.VX*foamDrag - gx*foamDrift + (fm.rng.Float32()-0.5)*foamJostle
		b.VY = b.VY*foamDrag - gy*foamDrift + (fm.rng.Float32()-0.5)*foamJostle
		b.X += b.VX
		b.Y += b.VY
		if b.X < 0 || b.Y < 0 || b.X >= float32(f.W) || b.Y >= float32(f.H) {
			continue
		}
		kept = append(kept, b)
	}
	fm.Bubbles = kept
}

// Clear bursts every bubble at once.
func (fm *Foam) Clear() { fm.Bubbles = fm.Bubbles[:0] }

// Fade is how present a bubble is, 0..1: it grows in over its first few
// steps and thins over the last fifth of its life before it bursts, rather
// than appearing and vanishing in one frame.
func (b Bubble) Fade() float32 {
	in := min(float32(b.Age)/8, 1)
	out := min(float32(b.Life-b.Age)/(float32(b.Life)*0.2), 1)
	return min(in, out)
}
