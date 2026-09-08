// Package reaction is Gray–Scott reaction–diffusion: two chemicals stirred
// together on a grid, one of which eats the other.
//
// Two concentrations live in every cell. U is fed in from outside at rate F
// and V is drained away at rate F+k, and where they meet, one U and two V
// react to make three V — autocatalytic, because V is both the thing consumed
// and the thing produced. On its own that is a runaway; what stops it is that
// both chemicals also diffuse, and U diffuses twice as fast as V. A patch of V
// can therefore only grow where fresh U can still reach it, so it grows at its
// rim, starves in its middle, splits, and its halves push apart. That
// competition between a reaction that wants to spread and a supply that cannot
// keep up is the entire mechanism, and every stripe and spot on the screen
// comes out of it rather than out of anything drawn.
//
// # THE REGIME
//
// F and k are the whole personality of the system and most of the plane of
// them is dull: the field either starves back to bare substrate or floods
// solid. The defaults sit at F=0.030, k=0.057 — spots that divide until they
// run out of room and then stretch into a labyrinth of worms, which is chosen
// because it never finishes. Left running for a minute past the fill, about a
// quarter of the surface still changes state every thirty seconds: fronts
// merge, dead ends heal, and loops pinch off. That is the thing a screensaver
// needs and it is not the same thing as the prettiest still frame.
//
// The famous mitosis corner, F=0.0367 and k=0.0649, gives rounder and more
// photogenic spots and was the first choice here — but it fills the surface in
// half a minute and then stops: measured at a fortieth of the churn of the
// defaults. Both are one assignment away; see Feed and Kill for the others.
//
// Written from the published description of the model rather than from
// anyone's implementation.
package reaction

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Chem runs from bare substrate through the halo at the rim of a growing front
// to the saturated core.
//
// The dark end is where the reaction is not happening, so it has to be the
// quietest color on the screen: what should catch the eye is the fronts, which
// are the only part actually doing anything.
var Chem = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 6, G: 8, B: 24},
	canvas.Stop{At: 0.30, R: 20, G: 36, B: 96},
	canvas.Stop{At: 0.55, R: 24, G: 132, B: 148},
	canvas.Stop{At: 0.78, R: 214, G: 188, B: 78},
	canvas.Stop{At: 1.00, R: 255, G: 250, B: 232},
)

// defaultStepRate is reaction steps per second. The model's rates are per step
// and its natural time scale is thousands of them, so this is what sets how
// fast a front actually moves: at 300 a fresh seed has worked its way across a
// terminal in well under a minute, which is slow enough to watch a spot divide
// and fast enough that the screen is not the same picture a minute later.
const defaultStepRate = 300

// maxCatchUp bounds how many steps one frame may run, for the same reason
// canvas clamps its elapsed time: a stall must cost smoothness, not a burst of
// a hundred full-grid sweeps.
const maxCatchUp = 24

// maxDiff is the stability limit of this explicit step.
//
// The nine-point Laplacian below has eigenvalue -1.6 at the checkerboard mode
// — the shortest wavelength the grid can hold — so a cell is multiplied by
// 1-1.6D each step at that mode, and past D of 1.25 that factor exceeds one in
// magnitude and the mode grows without bound. It does not look like fast
// diffusion, it looks like the screen filling with grid-scale hash in a couple
// of seconds.
//
// step clamps to 1 rather than to 1.25, a quarter inside the linear bound,
// because the reaction terms sit on top of the diffusion and a mode that is
// merely neutral under diffusion alone gets pushed over the line by them:
// measured at 1.25, a spot's substrate ran away to fifty times what it is fed.
const maxDiff = 1

// Reaction is the animation. The zero value is not usable; call New.
type Reaction struct {
	w, h int

	// The two concentration fields and the buffers they are stepped into.
	// Double buffered because every cell reads its neighbors: updating in
	// place would let a cell see half of its neighbors already advanced,
	// which is a different and much less isotropic system.
	u, v, nu, nv []float32

	// Wrapped neighbor indices, built once per resize. The Laplacian needs
	// eight neighbors per cell and the surface is a torus, so without these
	// the inner loop would carry eight bounds tests and eight modulos — more
	// work than the arithmetic they surround.
	xm, xp []int
	ym, yp []int

	rng *rand.Rand
	acc float64 // leftover fraction of a step from the last frame

	// Feed is the rate fresh U is supplied at and Kill the extra rate V is
	// drained at. Together they pick the regime, and the interesting ones lie
	// in a narrow band: 0.0367/0.0649 divides into round spots and then
	// settles, 0.055/0.062 is a tighter maze, 0.014/0.054 is a sparse field of
	// blobs that crawl and collide, and a little way outside the band in any
	// direction the pattern either dies or floods.
	Feed, Kill float64

	// DiffU and DiffV are how fast each chemical spreads. Their RATIO is what
	// matters: a pattern needs the inhibitor to outrun the activator, and at
	// equal rates there is nothing here but a uniform wash. Clamped to the
	// stability limit — see maxDiff — so the knob cannot be turned into hash.
	DiffU, DiffV float64

	// StepRate is reaction steps per second: how fast the chemistry runs
	// against the wall clock, and nothing else. It does not change which
	// pattern forms, only how long it takes to arrive.
	StepRate float64

	// Seeds is how many patches of V the surface is inoculated with on resize.
	// One is enough — it would divide its way across the screen on its own —
	// but several fronts started apart meet at angles, and those collisions
	// are the most interesting thing in the first minute.
	Seeds int

	// SeedRadius is how wide each starting patch is, in pixels. Below about
	// three, a patch is smaller than the pattern's own wavelength and simply
	// dissolves back into substrate.
	SeedRadius float64

	// Gain scales V to color. V saturates a little under 0.4 rather than at 1,
	// so without this the picture would use a third of the ramp.
	Gain float64

	// Palette colors the surface by the concentration of V.
	Palette canvas.Palette
}

// New returns a reaction. seed of 0 places the starting patches in a fixed
// arrangement, which makes tests repeatable.
func New(seed int64) *Reaction {
	return &Reaction{
		rng:        rand.New(rand.NewSource(seed)), //nolint:gosec
		Feed:       0.030,
		Kill:       0.057,
		DiffU:      1.0,
		DiffV:      0.5,
		StepRate:   defaultStepRate,
		Seeds:      5,
		SeedRadius: 5,
		Gain:       2.6,
		Palette:    Chem,
	}
}

// Resize allocates the fields and inoculates them.
//
// A resize starts the chemistry over rather than stretching what was there.
// The pattern has an intrinsic wavelength measured in cells, so a stretched
// field would be at the wrong scale for its new grid and would spend a minute
// visibly re-spacing itself.
func (r *Reaction) Resize(w, h int) {
	r.w, r.h = w, h
	n := w * h
	r.u = make([]float32, n)
	r.v = make([]float32, n)
	r.nu = make([]float32, n)
	r.nv = make([]float32, n)
	r.acc = 0

	r.xm, r.xp = make([]int, w), make([]int, w)
	for x := 0; x < w; x++ {
		r.xm[x], r.xp[x] = (x-1+w)%w, (x+1)%w
	}
	r.ym, r.yp = make([]int, h), make([]int, h)
	for y := 0; y < h; y++ {
		r.ym[y], r.yp[y] = ((y-1+h)%h)*w, ((y+1)%h)*w
	}

	r.seed()
}

// seed fills the surface with substrate and drops patches of V into it.
//
// The patches are noisy rather than clean discs. A perfectly symmetric patch
// divides symmetrically for a long time and the result looks manufactured; a
// little noise at the start is what knocks the spots off the lattice they
// would otherwise fall into.
func (r *Reaction) seed() {
	for i := range r.u {
		r.u[i] = 1
		r.v[i] = 0
	}
	if r.w == 0 || r.h == 0 {
		return
	}
	rad := r.SeedRadius
	if rad < 1 {
		rad = 1
	}
	n := r.Seeds
	if n < 1 {
		n = 1
	}
	for k := 0; k < n; k++ {
		cx, cy := r.rng.Intn(r.w), r.rng.Intn(r.h)
		ir := int(rad)
		for dy := -ir; dy <= ir; dy++ {
			for dx := -ir; dx <= ir; dx++ {
				if float64(dx*dx+dy*dy) > rad*rad {
					continue
				}
				i := wrap(cy+dy, r.h)*r.w + wrap(cx+dx, r.w)
				r.u[i] = 0.5 + float32(r.rng.Float64())*0.1
				r.v[i] = 0.25 + float32(r.rng.Float64())*0.1
			}
		}
	}
}

// Frame runs however much chemistry the elapsed time is worth and draws it.
func (r *Reaction) Frame(s *canvas.Surface, dt float64) {
	if r.w == 0 || r.h == 0 {
		return
	}
	rate := r.StepRate
	if rate <= 0 {
		rate = defaultStepRate
	}
	r.acc += dt * rate
	if r.acc > maxCatchUp {
		r.acc = maxCatchUp
	}
	for r.acc >= 1 {
		r.step()
		r.acc--
	}
	r.draw(s)
}

// The nine-point Laplacian weights. A five-point stencil is cheaper and gives
// visibly square spots: the pattern's wavelength here is only a few cells, so
// the stencil's own anisotropy is the same size as the feature it is carrying.
// The diagonals cost four more reads per cell and buy round fronts.
const (
	lapOrtho = 0.2
	lapDiag  = 0.05
)

// step advances the chemistry by one tick.
func (r *Reaction) step() {
	du, dv := r.DiffU, r.DiffV
	if du > maxDiff {
		du = maxDiff // see maxDiff: past this it is not fast, it is hash
	} else if du < 0 {
		du = 0
	}
	if dv > maxDiff {
		dv = maxDiff
	} else if dv < 0 {
		dv = 0
	}
	du32, dv32 := float32(du), float32(dv)
	f, k := float32(r.Feed), float32(r.Kill)
	fk := f + k

	for y := 0; y < r.h; y++ {
		row := y * r.w
		up, dn := r.ym[y], r.yp[y]
		for x := 0; x < r.w; x++ {
			i := row + x
			l, rr := r.xm[x], r.xp[x]

			uc, vc := r.u[i], r.v[i]
			lapU := lapOrtho*(r.u[row+l]+r.u[row+rr]+r.u[up+x]+r.u[dn+x]) +
				lapDiag*(r.u[up+l]+r.u[up+rr]+r.u[dn+l]+r.u[dn+rr]) - uc
			lapV := lapOrtho*(r.v[row+l]+r.v[row+rr]+r.v[up+x]+r.v[dn+x]) +
				lapDiag*(r.v[up+l]+r.v[up+rr]+r.v[dn+l]+r.v[dn+rr]) - vc

			// One U and two V make three V. Subtracted from U and added to V,
			// which is what makes V autocatalytic: the more of it there is,
			// the faster it makes more.
			reac := uc * vc * vc

			nu := uc + du32*lapU - reac + f*(1-uc)
			nv := vc + dv32*lapV + reac - fk*vc

			// A concentration cannot be negative. Rounding at the fringe of a
			// front can push one a hair below zero, and a negative V squared is
			// positive, so it would feed the reaction rather than stop it.
			if nu < 0 {
				nu = 0
			}
			if nv < 0 {
				nv = 0
			}
			r.nu[i], r.nv[i] = nu, nv
		}
	}
	r.u, r.nu = r.nu, r.u
	r.v, r.nv = r.nv, r.v
}

// draw colors the surface by the concentration of V.
func (r *Reaction) draw(s *canvas.Surface) {
	gain := r.Gain * 255
	for y := 0; y < r.h; y++ {
		row := y * r.w
		for x := 0; x < r.w; x++ {
			i := int(float64(r.v[row+x]) * gain)
			if i < 0 {
				i = 0
			} else if i > 255 {
				i = 255
			}
			s.Set(x, y, r.Palette[i])
		}
	}
}

// total sums one field, which is what a test of whether the reaction is alive
// wants: a pattern that died leaves no V behind at all.
func total(f []float32) float64 {
	var t float64
	for _, v := range f {
		t += float64(v)
	}
	return t
}

// Run draws a reaction on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}

// wrap folds a coordinate onto the torus. Go's % keeps the sign of its left
// operand, so the obvious v%n is negative for a point off the left edge, and
// adding one n back is only enough when the point is less than one surface
// outside — which a seed radius larger than the window is not.
func wrap(v, n int) int {
	if n <= 0 {
		return 0
	}
	v %= n
	if v < 0 {
		v += n
	}
	return v
}
