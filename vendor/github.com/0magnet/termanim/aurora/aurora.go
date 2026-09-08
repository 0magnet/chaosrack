// Package aurora is curtains of light standing in the sky, rippling and
// folding.
//
// # WHAT A CURTAIN ACTUALLY IS
//
// An aurora is not a glowing cloud. It is a thin sheet — electrons falling
// down the field lines into the upper atmosphere, lighting a curtain a few
// hundred meters thick and hundreds of kilometers long — and every feature
// worth drawing follows from its being a sheet seen nearly edge on.
//
// So this does not paint a curtain as a band of color. It walks a line along
// the curtain's ground track, a curve that ripples and doubles back on itself,
// and drops each sample into the screen column it projects onto. Two things
// then come out for free rather than being drawn in:
//
//   - Where the sheet folds, the track turns back and many samples land in one
//     column. That column looks along the sheet rather than through it, so it
//     is several times brighter. Those bright vertical creases are the most
//     recognizable thing about an aurora and nothing here draws them; they are
//     where the curve happens to be perpendicular to the viewer.
//
//   - The rays are a variation in brightness ALONG the curtain, not across the
//     screen. When the curtain folds, its rays fold with it, which is why they
//     bunch up exactly where the creases are.
//
// Vertically the rule is simpler: a sharp lower border, brightest right at it,
// fading exponentially upward as the incoming electrons run out of atmosphere
// to excite. Nothing is drawn below the border at all, and that hard edge
// under a soft top is most of what makes the shape read as an aurora rather
// than as smoke.
//
// # THE COLOR
//
// The palette does the work that in the sky is done by chemistry: atomic
// oxygen glows green where the curtain is brightest and dense, and the thinner
// upper reaches run through cyan into the violets of ionized nitrogen. Because
// brightness here falls with height, indexing a ramp that runs violet → cyan →
// green → white by brightness alone puts each color where it belongs, with no
// second lookup and no per-pixel blending.
//
// Written from a description of the phenomenon. No code is taken from any
// existing implementation.
package aurora

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Borealis runs from the faintest violet fringe at the top of a curtain,
// through blue and cyan, to the green-white of a bright fold at the lower
// border.
//
// It is indexed by brightness and therefore, in this animation, by height. The
// violet end has to start well above black: it is the color of the part of the
// curtain that is only just visible, and a ramp that spends its first fifth
// fading out of the background throws that whole region away.
var Borealis = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 18, G: 0, B: 38},
	canvas.Stop{At: 0.05, R: 66, G: 10, B: 120},
	canvas.Stop{At: 0.11, R: 52, G: 34, B: 178},
	canvas.Stop{At: 0.18, R: 8, G: 110, B: 205},
	canvas.Stop{At: 0.26, R: 0, G: 175, B: 200},
	canvas.Stop{At: 0.38, R: 36, G: 238, B: 112},
	canvas.Stop{At: 0.65, R: 140, G: 250, B: 155},
	canvas.Stop{At: 1.00, R: 235, G: 255, B: 240},
)

// samplesPerColumn is how many points along a curtain are dropped per column
// of the screen.
//
// It has to be several, and it is the one number here that cannot be traded
// away: the folds are made by samples piling into one column, so a curtain
// walked at one sample per column has nothing to pile up and comes out as a
// flat band. Six is where the creases stop getting brighter with more.
const samplesPerColumn = 6

// Aurora is the animation. The zero value is not usable; call New.
type Aurora struct {
	w, h int
	t    float64

	curtains []curtain

	// light is the accumulated brightness at each pixel, cleared per frame.
	// Accumulated rather than drawn straight to the surface because curtains
	// overlap, and two thin sheets one behind the other are brighter where
	// they cross — which is true of the real thing and is what stops the
	// nearer one from looking like a cutout.
	light []float32

	// colWeight and colBase are one curtain's projection onto the screen
	// columns: how much of the sheet landed in each column, and where its
	// lower border sits there. Scratch, refilled per curtain per frame.
	colWeight, colBase []float32

	// colWeight2 and colBase2 are the scratch the three-tap smear reads from,
	// since it rewrites the arrays it is smoothing. Allocated once per resize:
	// Frame must not allocate.
	colWeight2, colBase2 []float32

	rng *rand.Rand

	// Count is how many curtains stand in the sky. One is a clean shape and
	// reads as a diagram; three or four overlap at different depths, which is
	// what gives the display any sense of distance.
	Count int

	// Speed scales the rippling, the folding and the drift of the rays. The
	// real thing moves slowly for minutes and then breaks up in seconds; this
	// stays at the slow end, because a screensaver that lunges is tiring.
	Speed float64

	// Fold is how far the curtain's track wanders across the screen, as a
	// fraction of its own width. Zero is a flat sheet with no creases in it at
	// all; past about a third the track crosses itself so often that the
	// creases stop being separable.
	Fold float64

	// Rays is how many bright striations run the length of a curtain. The
	// count is along the curtain, not across the screen, so a folded curtain
	// shows them bunched where it turns.
	Rays float64

	// Height is how far a curtain reaches above its lower border, as a
	// fraction of the surface — the scale length of the fade, not a hard top,
	// so a bright curtain visibly reaches higher than a faint one.
	Height float64

	// Gain scales the whole display. Turn it down and only the folds are left,
	// which is what a weak aurora looks like.
	Gain float64

	// Floor is the faintest light that is drawn at all, from 0 to 1. Below it
	// the sky is left as the terminal's own background rather than painted the
	// bottom color of the ramp, which would put a violet wash over the whole
	// window.
	Floor float64

	// Palette colors the sky by brightness, which here means by height.
	Palette canvas.Palette
}

// curtain is one sheet: where its track runs, how it ripples, and how bright
// it is.
type curtain struct {
	// x0 and span place the track across the screen. A curtain need not fit
	// in the window — one running off the side is what makes the sky look
	// bigger than the frame.
	x0, span float64

	// The two ripples the track is made of, as amplitude, spatial frequency
	// along the curtain, phase and drift rate. Two rather than one because a
	// single sine folds at evenly spaced places and the regularity is
	// immediately visible.
	a1, f1, p1, s1 float64
	a2, f2, p2, s2 float64

	// The lower border: where it sits, how much it ripples along the curtain,
	// and how fast that ripple travels. The border moving along the curtain is
	// the slow undulation the eye follows when nothing else is happening.
	baseY, baseAmp, baseFreq, basePhase, baseSpeed float64

	// The rays: their phase and how fast it drifts along the curtain.
	rayPhase, raySpeed float64

	// bright is this curtain's own brightness, and fade is its vertical
	// profile: fade[d] is what is left d pixels above the lower border.
	// Tabulated per curtain because the profile is fixed for the life of a
	// resize and an exponential per pixel per curtain per frame is not.
	bright float64
	fade   []float32
}

// New returns an aurora. seed of 0 gives a fixed sky, which makes tests
// repeatable.
func New(seed int64) *Aurora {
	return &Aurora{
		rng:     rand.New(rand.NewSource(seed)), //nolint:gosec
		Count:   3,
		Speed:   1,
		Fold:    0.16,
		Rays:    22,
		Height:  0.28,
		Gain:    1.0,
		Floor:   0.025,
		Palette: Borealis,
	}
}

// Resize builds the curtains and the buffers they are drawn through.
func (a *Aurora) Resize(w, h int) {
	a.w, a.h = w, h
	a.light = make([]float32, w*h)
	a.colWeight = make([]float32, w)
	a.colBase = make([]float32, w)
	a.colWeight2 = make([]float32, w)
	a.colBase2 = make([]float32, w)

	n := a.Count
	if n < 1 {
		n = 1
	}
	fw, fh := float64(w), float64(h)
	a.curtains = make([]curtain, n)
	for i := range a.curtains {
		c := &a.curtains[i]
		// Curtains are placed across the window and allowed to overhang it.
		c.x0 = (a.rng.Float64()*0.5 - 0.25) * fw
		c.span = (0.8 + a.rng.Float64()*0.7) * fw

		c.a1 = a.Fold * (0.7 + a.rng.Float64()*0.6)
		c.f1 = 0.8 + a.rng.Float64()*1.6
		c.p1 = a.rng.Float64() * 2 * math.Pi
		c.s1 = (0.10 + a.rng.Float64()*0.10) * sign(a.rng.Float64()-0.5)
		c.a2 = a.Fold * (0.20 + a.rng.Float64()*0.25)
		c.f2 = 3.5 + a.rng.Float64()*3.5
		c.p2 = a.rng.Float64() * 2 * math.Pi
		c.s2 = (0.18 + a.rng.Float64()*0.16) * sign(a.rng.Float64()-0.5)

		// The lower border sits in the bottom half, further down for the
		// curtains meant to read as nearer.
		c.baseY = fh * (0.62 + a.rng.Float64()*0.3)
		c.baseAmp = fh * (0.02 + a.rng.Float64()*0.05)
		c.baseFreq = 0.7 + a.rng.Float64()*1.5
		c.basePhase = a.rng.Float64() * 2 * math.Pi
		c.baseSpeed = 0.08 + a.rng.Float64()*0.12

		c.rayPhase = a.rng.Float64() * 2 * math.Pi
		c.raySpeed = (0.5 + a.rng.Float64()) * sign(a.rng.Float64()-0.5)

		c.bright = 0.45 + a.rng.Float64()*0.55

		// The vertical profile. Exponential because the electrons are being
		// used up as they descend, so there is no top edge — only a height at
		// which the curtain stops being worth drawing.
		scale := a.Height * fh
		if scale < 1 {
			scale = 1
		}
		c.fade = make([]float32, h+1)
		for d := range c.fade {
			c.fade[d] = float32(math.Exp(-float64(d) / scale))
		}
	}
}

// sign returns -1 or 1, so a curtain can drift either way.
func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// Frame advances the sky and draws it.
func (a *Aurora) Frame(s *canvas.Surface, dt float64) {
	if a.w == 0 || a.h == 0 {
		return
	}
	a.t += dt * a.Speed

	for i := range a.light {
		a.light[i] = 0
	}
	for i := range a.curtains {
		a.draw(&a.curtains[i])
	}

	floor := float32(a.Floor)
	for y := 0; y < a.h; y++ {
		row := y * a.w
		for x := 0; x < a.w; x++ {
			v := a.light[row+x]
			if v <= floor {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			i := int(v * 255)
			if i > 255 {
				i = 255
			}
			s.Set(x, y, a.Palette[i])
		}
	}
}

// draw walks one curtain and adds it to the light buffer.
func (a *Aurora) draw(c *curtain) {
	for i := range a.colWeight {
		a.colWeight[i], a.colBase[i] = 0, 0
	}

	samples := a.w * samplesPerColumn
	// Each sample carries the same amount of curtain, scaled so that a sheet
	// laid flat across the whole screen comes out at brightness 1 whatever the
	// sample count is. A fold that piles four samples into one column is then
	// four times as bright, which is the effect this is all for.
	unit := float32(c.bright * a.Gain * float64(a.w) / float64(samples))
	twoPi := 2 * math.Pi

	for i := 0; i < samples; i++ {
		u := float64(i) / float64(samples-1)
		x := c.x0 + c.span*(u+
			c.a1*math.Sin(twoPi*c.f1*u+c.p1+c.s1*a.t)+
			c.a2*math.Sin(twoPi*c.f2*u+c.p2+c.s2*a.t))
		// The rays, as brightness along the curtain. Kept off zero: a ray
		// pattern that reaches zero cuts the sheet into separate ribbons,
		// where a real curtain is continuous and merely brighter in stripes.
		ray := 0.45 + 0.55*math.Sin(twoPi*a.Rays*u+c.rayPhase+c.raySpeed*a.t)
		base := c.baseY + c.baseAmp*math.Sin(twoPi*c.baseFreq*u+c.basePhase+c.baseSpeed*a.t)

		wt := unit * float32(ray)

		// The sample falls between two columns and is split between them by
		// how far across it landed.
		//
		// Dropping it into one column instead — which is what this did first
		// — renders every curtain as a hard-edged vertical bar. A column's
		// weight is painted uniformly up its whole height, so two neighbors
		// that caught slightly different samples meet at a visible step, and
		// a skyline of those reads as a bar chart rather than as light.
		fxi := math.Floor(x)
		xi := int(fxi)
		fx := float32(x - fxi)
		if xi >= 0 && xi < a.w {
			w0 := wt * (1 - fx)
			a.colWeight[xi] += w0
			a.colBase[xi] += w0 * float32(base)
		}
		if xi+1 >= 0 && xi+1 < a.w {
			w1 := wt * fx
			a.colWeight[xi+1] += w1
			a.colBase[xi+1] += w1 * float32(base)
		}
	}

	// Splitting each sample between two columns removes the worst of the
	// stepping but not all of it, because the border is still quantized to a
	// row per column. A three-tap smear across neighbors costs one pass over
	// a row of floats and takes the remaining staircase out of both the
	// brightness and the border.
	a.smearColumns()

	for x := 0; x < a.w; x++ {
		wt := a.colWeight[x]
		if wt <= 0 {
			continue
		}
		// The border for this column is where the samples that landed here say
		// it is. Weighted, so a column holding one bright fold and a little
		// stray sheet takes the fold's border rather than the average of two.
		base := int(a.colBase[x] / wt)
		if base >= a.h {
			base = a.h - 1
		}
		for y := base; y >= 0; y-- {
			a.light[y*a.w+x] += wt * c.fade[base-y]
		}
	}
}

// smearColumns runs a weighted three-tap blur along the column arrays.
//
// It blurs colBase in its already weight-multiplied form rather than as a
// border position, because that is what keeps a dim column next to a bright
// fold from dragging the fold's border sideways: the sum is reconstituted
// into a position by the same division either way, so the bright column still
// dominates its own border.
func (a *Aurora) smearColumns() {
	copy(a.colWeight2, a.colWeight)
	copy(a.colBase2, a.colBase)
	for x := 0; x < a.w; x++ {
		l := x - 1
		if l < 0 {
			l = 0
		}
		r := x + 1
		if r >= a.w {
			r = a.w - 1
		}
		a.colWeight[x] = 0.25*a.colWeight2[l] + 0.5*a.colWeight2[x] + 0.25*a.colWeight2[r]
		a.colBase[x] = 0.25*a.colBase2[l] + 0.5*a.colBase2[x] + 0.25*a.colBase2[r]
	}
}

// Run draws an aurora on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
