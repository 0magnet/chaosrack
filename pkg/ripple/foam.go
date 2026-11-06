package ripple

import (
	"math"
	"math/rand/v2"
)

// Foam is the froth on a surface: bubbles that ride it rather than waves
// that cross it.
//
// A wave leaves; foam stays. What a moving thing churns into water spreads
// away as ripples and is gone in a second, but the bubbles it whips up float
// on, carried by the waves, and burst one by one over several seconds. That
// difference is the whole reason this is a list of bubbles and not more of
// the height field: a bump in the field is a wave, and a wave cannot sit
// still.
//
// A bubble has a beginning and an end that can be seen. It is thrown off
// the thing that made it and inflates as it goes; it rides the waves, rocking
// as each passes under it; and it bursts — the film tears and the tear runs
// round the rim in a few frames — leaving a small ring on the water where it
// was. A bubble that only faded would just be a dot that dimmed.
//
// Bubbles cling. Floating ones pull together until they touch, by the same
// surface tension that gathers cereal in a bowl, and touching ones move as
// one, so foam lies in rafts and strings that drift and turn whole rather
// than as a sprinkle of separate dots.
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
	// The neighbor search's buckets, kept between steps: head is the first
	// bubble in each bucket, next the one after each bubble; r each one's
	// radius as it stands, dv the pull the others put on it.
	head, next []int32
	r          []float32
	dv         [][2]float32
}

// Bubble is one bubble on the surface.
type Bubble struct {
	X, Y   float32 // where it floats, in cells
	VX, VY float32 // its drift, cells a step
	R      float32 // its radius when full, in cells
	Age    int     // steps since it was made
	Life   int     // steps it lasts before it bursts
	Tear   float32 // where on the rim its film tears, radians
}

// Foam's constants, in the field's units and steps (a step is a frame).
const (
	// A floating bubble is pushed down the surface's slope, so a passing wave
	// rocks it to and fro and it gathers in the troughs, as foam lines up
	// between waves on real water. Per unit slope, in cells a step.
	foamDrift = 0.6
	// How much of its drift a bubble keeps a step. Enough that it is carried
	// a way by the throw it was born with and by the waves; not so much that
	// it coasts across the water like a puck.
	foamDrag = 0.95
	// The jostle of the water too fine for the field to carry, in cells a
	// step: without it a bubble on flat water is a fixed dot.
	foamJostle = 0.02
	// Most bubbles are small and a few are large: a radius is foamMinR plus
	// foamSpanR times the cube of a uniform draw.
	foamMinR  = 0.15
	foamSpanR = 0.9
	// How long a bubble lasts, in steps: two to ten seconds at sixty frames,
	// the large ones toward the short end, as a big film drains to its
	// bursting point sooner than a small one.
	foamMinLife  = 120
	foamSpanLife = 480
	// A bubble inflates to its full size over its first foamGrow steps.
	foamGrow = 18
	// Its burst takes foamBurst steps: the tear opening round the rim.
	foamBurst = 7
	// The ring a burst leaves on the water, per cell of the bubble's radius.
	foamPop = 0.012
	// Two bubbles feel each other from touching out to foamReach cells
	// further, pulled together at foamCling cells a step per step; the pull
	// tapers off only over the last quarter of that, as a pull that faded
	// linearly would lose to the jostle well inside its reach.
	foamReach = 1.5
	foamCling = 0.006
	// Closer than touching they are pushed apart, foamPack per cell of
	// overlap, so a raft packs rather than piling into one blot.
	foamPack = 0.08
	// Touching bubbles share this much of their difference in drift a step:
	// a raft moves as one.
	foamStick = 0.15
	// The most the others change one bubble's drift in a step, in cells a
	// step. Pulls add, and a bubble in the middle of a fresh crowd feels
	// dozens at once: summed freely they flung the whole crowd off the
	// surface in a second. The water a raft sits in holds it back.
	foamClingMax = 0.01
	// The neighbor search's bucket side, in cells: wider than the largest
	// pair of radii plus the reach, so a bubble's neighbors are all in the
	// buckets next to its own.
	foamBucket = 4
	// A bubble feels no more than foamNeighbors of the others a step. In a
	// raft the ones touching it are all that matter, and without a bound a
	// thick raft costs the square of its size.
	foamNeighbors = 12
)

// NewFoam returns an empty surface that carries no more than most bubbles.
func NewFoam(most int, seed uint64) *Foam {
	return &Foam{Max: most, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} //nolint:gosec // where bubbles land on a decorative surface
}

// Spawn makes a bubble at x, y, a little scattered and thrown off at vx, vy,
// if the surface has room.
func (fm *Foam) Spawn(x, y, vx, vy float32) {
	if len(fm.Bubbles) >= fm.Max {
		return
	}
	u := fm.rng.Float32()
	big := u * u * u
	// A throw is never quite the same twice: each bubble leaves at 50–150%
	// of it.
	k := 0.5 + fm.rng.Float32()
	fm.Bubbles = append(fm.Bubbles, Bubble{
		X: x + fm.rng.Float32() - 0.5, Y: y + fm.rng.Float32() - 0.5,
		VX: vx * k, VY: vy * k,
		R:    foamMinR + foamSpanR*big,
		Life: foamMinLife + int(float32(fm.rng.IntN(foamSpanLife))*(1-0.6*big)),
		Tear: fm.rng.Float32() * 6.2831855,
	})
}

// Chance reports true with probability p, from the foam's own generator, so
// a caller deciding whether to make a bubble draws from the same sequence.
func (fm *Foam) Chance(p float32) bool { return fm.rng.Float32() < p }

// Step moves every bubble one step on surface f, starts the bursts whose time
// is up, and clears the ones done bursting. A bursting bubble stops riding the
// water, and the moment it tears it leaves a small ring on f. A bubble carried
// off the surface is gone with it.
func (fm *Foam) Step(f *Field) {
	fm.cling(f)
	kept := fm.Bubbles[:0]
	for i, b := range fm.Bubbles {
		dx, dy := fm.dv[i][0], fm.dv[i][1]
		if l := float32(math.Sqrt(float64(dx*dx + dy*dy))); l > foamClingMax {
			dx, dy = dx/l*foamClingMax, dy/l*foamClingMax
		}
		b.VX += dx
		b.VY += dy
		b.Age++
		if b.Age >= b.Life+foamBurst {
			continue
		}
		if b.Age == b.Life {
			f.Drop(b.X, b.Y, 1+b.R, -foamPop*(0.3+b.R))
		}
		if b.Age < b.Life {
			gx, gy := f.Gradient(int(b.X), int(b.Y))
			b.VX = b.VX*foamDrag - gx*foamDrift + (fm.rng.Float32()-0.5)*foamJostle
			b.VY = b.VY*foamDrag - gy*foamDrift + (fm.rng.Float32()-0.5)*foamJostle
			b.X += b.VX
			b.Y += b.VY
		}
		if b.X < 0 || b.Y < 0 || b.X >= float32(f.W) || b.Y >= float32(f.H) {
			continue
		}
		kept = append(kept, b)
	}
	fm.Bubbles = kept
}

// cling works out the pull each whole bubble feels from the whole ones near
// it, into fm.dv. A bursting bubble neither pulls nor is pulled: its film is
// already gone.
func (fm *Foam) cling(f *Field) {
	bw, bh := (f.W+foamBucket-1)/foamBucket, (f.H+foamBucket-1)/foamBucket
	if len(fm.head) != bw*bh {
		fm.head = make([]int32, bw*bh)
	}
	for i := range fm.head {
		fm.head[i] = -1
	}
	n := len(fm.Bubbles)
	if cap(fm.next) < n {
		fm.next = make([]int32, n, fm.Max)
		fm.r = make([]float32, n, fm.Max)
		fm.dv = make([][2]float32, n, fm.Max)
	}
	fm.next, fm.r, fm.dv = fm.next[:n], fm.r[:n], fm.dv[:n]
	clear(fm.dv)
	bucket := func(b *Bubble) (int, int) {
		return min(max(int(b.X)/foamBucket, 0), bw-1), min(max(int(b.Y)/foamBucket, 0), bh-1)
	}
	for i := range fm.Bubbles {
		b := &fm.Bubbles[i]
		if b.Age >= b.Life {
			continue
		}
		fm.r[i] = b.R * b.Size()
		bx, by := bucket(b)
		fm.next[i] = fm.head[by*bw+bx]
		fm.head[by*bw+bx] = int32(i)
	}
	for i := range fm.Bubbles {
		a := &fm.Bubbles[i]
		if a.Age >= a.Life {
			continue
		}
		ra := fm.r[i]
		bx, by := bucket(a)
		felt := 0
	near:
		for y := max(by-1, 0); y <= min(by+1, bh-1); y++ {
			for x := max(bx-1, 0); x <= min(bx+1, bw-1); x++ {
				// A bucket lists its bubbles newest first, so past the
				// first one at or before i are only pairs already taken.
				for j := fm.head[y*bw+x]; j > int32(i); j = fm.next[j] {
					b := &fm.Bubbles[j]
					dx, dy := b.X-a.X, b.Y-a.Y
					touch := ra + fm.r[j]
					d2 := dx*dx + dy*dy
					if d2 >= (touch+foamReach)*(touch+foamReach) || d2 < 1e-8 {
						continue
					}
					felt++
					if felt > foamNeighbors {
						break near
					}
					d := float32(math.Sqrt(float64(d2)))
					var push float32
					if d < touch {
						push = -foamPack * (touch - d)
						// Touching: share the drift.
						sx, sy := (b.VX-a.VX)*foamStick*0.5, (b.VY-a.VY)*foamStick*0.5
						fm.dv[i][0] += sx
						fm.dv[i][1] += sy
						fm.dv[j][0] -= sx
						fm.dv[j][1] -= sy
					} else {
						push = foamCling * min(1, 4*(1-(d-touch)/foamReach))
					}
					ux, uy := dx/d*push, dy/d*push
					fm.dv[i][0] += ux
					fm.dv[i][1] += uy
					fm.dv[j][0] -= ux
					fm.dv[j][1] -= uy
				}
			}
		}
	}
}

// Clear bursts every bubble at once.
func (fm *Foam) Clear() { fm.Bubbles = fm.Bubbles[:0] }

// Size is how far a bubble has inflated, 0..1 of R. It swells fast and then
// settles, as a film blown out of water does.
func (b Bubble) Size() float32 {
	t := min(float32(b.Age)/foamGrow, 1)
	return 1 - (1-t)*(1-t)
}

// Burst is how far through its burst a bubble is: 0 while it is whole, then
// rising to 1 as the tear runs round the rim.
func (b Bubble) Burst() float32 {
	if b.Age < b.Life {
		return 0
	}
	return min(float32(b.Age-b.Life+1)/foamBurst, 1)
}
