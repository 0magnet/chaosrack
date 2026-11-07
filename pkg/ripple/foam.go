package ripple

import (
	"math"
	"math/rand/v2"
)

// Foam is the froth on a surface: bubbles that ride it rather than waves
// that cross it.
//
// A wave leaves; foam stays. What a moving thing churns into water spreads
// away as ripples and is gone in a second, but the bubbles it brings up
// float on, carried by the water, and burst one by one over seconds. That
// difference is the whole reason this is a list of bubbles and not more of
// the height field: a bump in the field is a wave, and a wave cannot sit
// still.
//
// What is here is what floating bubbles are measured to do:
//
//   - They ride the water's own flow (Surface.Drift): round with each wave
//     that passes and a little forward each time. They have next to no mass
//     of their own, so they go where the surface goes and coast nowhere.
//   - They pull together (the "Cheerios effect"): each lifts the water round
//     it into a meniscus, and two menisci lean toward each other. The pull
//     dies away over the capillary length, about 2.7 mm, so bubbles gather
//     into rafts and strings only once they have drifted close.
//   - Most are tiny and a few are large. Measured sizes fall off steeply
//     with radius, so a size is drawn from a power law.
//   - They last anywhere from a second to a quarter of a minute, measured
//     on clean water: a film drains until it is thin enough to fail, and
//     when that is is close to chance. Lives are drawn evenly in their
//     logarithm, and counted on the surface's clock.
//   - They burst at once. The film's hole runs round at meters a second, so
//     a bubble a few millimeters across is gone well inside a frame; what is
//     left to see is the ring of ripples where it was.
//
// Positions are in the surface's cells and radii in cell heights, and the
// distance between two bubbles is measured as on the screen, a cell across
// being CellAspect of one up; so a bubble rides the
// same surface the refraction reads.
type Foam struct {
	Bubbles []Bubble
	// Max is the most bubbles the surface carries. Past it a new bubble is
	// not made: a sheet of foam that thick is opaque however many more are
	// added, and every one is drawn each frame.
	Max int
	rng *rand.Rand
	// The neighbor search's buckets, kept between steps: head is the first
	// bubble in each bucket, next the one after each bubble.
	head, next []int32
	// steps counts Step's calls. The others' pull on a bubble is worked out
	// every other step and kept on it for the step between: it changes as
	// slowly as bubbles drift, and the search for it was most of a step
	// (2 ms of a frame in wasm, for the page's 1500 bubbles).
	steps int
}

// Bubble is one bubble on the surface.
type Bubble struct {
	X, Y float32    // where it floats, in cells
	R    float32    // its radius, in cells
	Age  float32    // seconds of the surface's clock since it came up
	Life float32    // seconds it lasts before it bursts
	pull [2]float32 // how fast the others draw it, cell heights a second
}

// Foam's constants, in the surface's cells and seconds of its clock. A cell
// is a tank's (tankSide/128, 2.3 mm).
const (
	// The smallest bubble's radius, and the largest's; between them the
	// share larger than r falls as r^-foamSizes.
	foamMinR  = 0.2
	foamMaxR  = 2.5
	foamSizes = 2.0
	// A life is foamMinLife to foamMaxLife seconds, even in its logarithm.
	foamMinLife = 1.0
	foamMaxLife = 15.0
	// foamRise is how long a bubble takes to come up through the surface,
	// in seconds: it is drawn growing into place rather than appearing.
	foamRise = 0.08
	// The ring a burst leaves on the water, per cell of the bubble's radius.
	foamPop = 0.02
	// foamCapillary is the capillary length in cells: how far a bubble's
	// meniscus reaches, and so how far its pull on another does.
	foamCapillary = 1.2
	// foamReach is how far apart, gap between rims, two bubbles still feel
	// each other: the pull is down to a fiftieth there.
	foamReach = 4 * foamCapillary
	// foamCling is how fast two bubbles whose rims touch close on each
	// other, in cells a second (about 7 mm/s, as millimeter bubbles on
	// clean water are seen to).
	foamCling = 3
	// foamPack is how fast two that overlap are pushed apart, in cells a
	// second per cell of overlap, so a raft packs rather than piling into
	// one blot.
	foamPack = 6
	// foamClingMax is the fastest the others move one bubble, in cells a
	// second. Pulls add, and in the middle of a fresh crowd one bubble feels
	// dozens at once: summed freely they flung the crowd off the surface.
	foamClingMax = 4
	// The neighbor search's bucket side, in cells: wider than the largest
	// pair of radii plus the reach, so a bubble's neighbors are all in the
	// buckets next to its own.
	foamBucket = 11 // 2·foamMaxR + foamReach, 9.8, rounded up
	// A bubble feels no more than foamNeighbors of the others a step, and
	// looks at no more than foamLooks to find them. In a raft the ones
	// touching it are all that matter, and without the first bound a thick
	// raft costs the square of its size; without the second, so does a long
	// string of foam, whose bubbles mostly share buckets without being in
	// reach (the waterline's, 4 ms a frame).
	foamNeighbors = 12
	foamLooks     = 64
)

// NewFoam returns an empty surface that carries no more than most bubbles.
func NewFoam(most int, seed uint64) *Foam {
	return &Foam{Max: most, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} //nolint:gosec // where bubbles land on a decorative surface
}

// Spawn brings a bubble up at x, y, a little scattered, if the surface has
// room.
func (fm *Foam) Spawn(x, y float32) {
	if len(fm.Bubbles) >= fm.Max {
		return
	}
	// The power law's tail inverted: r = min·u^(-1/α), drawn again past
	// the largest rather than piled up at it.
	r := float32(math.MaxFloat32)
	for r > foamMaxR {
		r = foamMinR * float32(math.Pow(1-fm.rng.Float64(), -1/foamSizes))
	}
	life := foamMinLife * math.Pow(foamMaxLife/foamMinLife, fm.rng.Float64())
	fm.Bubbles = append(fm.Bubbles, Bubble{
		X: x + fm.rng.Float32() - 0.5, Y: y + fm.rng.Float32() - 0.5,
		R: r, Life: float32(life),
	})
}

// Chance reports true with probability p, from the foam's own generator, so
// a caller deciding whether to make a bubble draws from the same sequence.
func (fm *Foam) Chance(p float32) bool { return fm.rng.Float32() < p }

// Step moves every bubble one step on surface s: carried by its flow and
// drawn toward the others. A bubble whose time is up bursts, leaving a
// small ring on s, and one carried off the surface is gone with it.
func (fm *Foam) Step(s Surface) {
	dt := s.StepSeconds()
	w, h := s.Size()
	asp := s.CellAspect()
	if fm.steps%2 == 0 {
		fm.cling(w, h, asp)
	}
	fm.steps++
	kept := fm.Bubbles[:0]
	for _, b := range fm.Bubbles {
		b.Age += dt
		if b.Age >= b.Life {
			s.Drop(b.X, b.Y, 1+b.R, -foamPop*b.R)
			continue
		}
		vx, vy := b.pull[0], b.pull[1]
		if l := float32(math.Hypot(float64(vx), float64(vy))); l > foamClingMax {
			vx, vy = vx/l*foamClingMax, vy/l*foamClingMax
		}
		dx, dy := s.Drift(b.X, b.Y)
		b.X += dx + vx/asp*dt
		b.Y += dy + vy*dt
		if b.X < 0 || b.Y < 0 || b.X >= float32(w) || b.Y >= float32(h) {
			continue
		}
		kept = append(kept, b)
	}
	fm.Bubbles = kept
}

// cling works out how fast the others draw each bubble, into its pull, on a
// surface w by h cells each asp times as wide as tall. Speeds are in cell
// heights a second.
func (fm *Foam) cling(w, h int, asp float32) {
	// A bucket is foamBucket cell heights each way, so fewer cells across
	// where they are wide.
	across := max(int(math.Ceil(float64(foamBucket/asp))), 1)
	bw, bh := (w+across-1)/across, (h+foamBucket-1)/foamBucket
	if len(fm.head) != bw*bh {
		fm.head = make([]int32, bw*bh)
	}
	for i := range fm.head {
		fm.head[i] = -1
	}
	n := len(fm.Bubbles)
	if cap(fm.next) < n {
		fm.next = make([]int32, n, max(fm.Max, n))
	}
	fm.next = fm.next[:n]
	for i := range fm.Bubbles {
		fm.Bubbles[i].pull = [2]float32{}
	}
	bucket := func(b *Bubble) (int, int) {
		return min(max(int(b.X)/across, 0), bw-1), min(max(int(b.Y)/foamBucket, 0), bh-1)
	}
	for i := range fm.Bubbles {
		bx, by := bucket(&fm.Bubbles[i])
		fm.next[i] = fm.head[by*bw+bx]
		fm.head[by*bw+bx] = int32(i) //nolint:gosec // fewer bubbles than Max
	}
	for i := range fm.Bubbles {
		a := &fm.Bubbles[i]
		bx, by := bucket(a)
		felt, looked := 0, 0
	near:
		for y := max(by-1, 0); y <= min(by+1, bh-1); y++ {
			for x := max(bx-1, 0); x <= min(bx+1, bw-1); x++ {
				// A bucket lists its bubbles newest first, so past the
				// first one at or before i are only pairs already taken.
				for j := fm.head[y*bw+x]; j > int32(i); j = fm.next[j] { //nolint:gosec // fewer bubbles than Max
					looked++
					if looked > foamLooks {
						break near
					}
					b := &fm.Bubbles[j]
					dx, dy := (b.X-a.X)*asp, b.Y-a.Y
					touch := a.R + b.R
					d2 := dx*dx + dy*dy
					if d2 >= (touch+foamReach)*(touch+foamReach) || d2 < 1e-8 {
						continue
					}
					felt++
					if felt > foamNeighbors {
						break near
					}
					d := float32(math.Sqrt(float64(d2)))
					var pull float32
					if d < touch {
						pull = -foamPack * (touch - d)
					} else {
						pull = foamCling * float32(math.Exp(float64(-(d-touch)/foamCapillary)))
					}
					ux, uy := dx/d*pull, dy/d*pull
					a.pull[0] += ux
					a.pull[1] += uy
					b.pull[0] -= ux
					b.pull[1] -= uy
				}
			}
		}
	}
}

// Clear bursts every bubble at once.
func (fm *Foam) Clear() { fm.Bubbles = fm.Bubbles[:0] }

// Risen is how far a bubble has come up through the surface, 0..1.
func (b Bubble) Risen() float32 { return min(b.Age/foamRise, 1) }
