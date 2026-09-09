// Package magnetosphere animates magnetosphere.net's logo: a waisted funnel
// standing in a field of stripes, the funnel scrolling one way and the field
// the other.
//
// The logo is a still image — op art of the kind where a page of horizontal
// lines bends around something that is not there. Straight stripes at the left
// and right edges rise as they near the middle and fan into flares above and
// below a narrow throat, and the throat itself is banded with rings that open
// into big ellipses as it widens.
//
// # THE FUNNEL IS TRACED, NOT DRAWN
//
// The rings are not arcs picked to look right. They are a hyperboloid of one
// sheet standing on the vertical axis, banded at constant height, and each
// pixel asks which band a ray through it lands on:
//
//	surface   x² + (z-Depth)² = a² + (y·Slope)²
//	ray       P(k) = k·(u, v, 1)          camera at the origin, plane at z=1
//
// which is a quadratic in k. Written the stable way — dividing the constant
// term by the larger root rather than subtracting two close numbers — the near
// hit is
//
//	A = u² + 1 - (v·Slope)²      C = Depth² - a²
//	k = C / (Depth + √(Depth² - A·C))
//
// and the height of the hit is k·v. One square root per sample, no branch, and
// no z-buffer: a ray meets this surface twice and the near root is wanted
// every time.
//
// The discriminant is what says whether the ray hits at all, and the shape of
// that condition is the silhouette: Depth² - A·C ≥ 0 works out to
// u² ≤ (v·Slope)² + Waist², a hyperbola. So the funnel's outline, the waist,
// and the flares are all one expression, and the picture cannot end up with an
// edge in a place the geometry does not put one.
//
// Doing it properly rather than bowing flat rings by hand is what makes the
// flare read: an approximate bow is the near half of a circle, and the near
// half of a circle much wider than the frame is a straight line. The
// perspective divide is the whole effect — the near side of a ring is close,
// so it swings far below the far side, and the ring opens into the ellipse the
// logo draws.
//
// # THE FIELD
//
// Outside the silhouette there is no surface, so the field is a rule. The
// stripe a point belongs to is its height divided by how near the funnel it is:
//
//	t = W / max(|u|, W)     1 against the funnel, small out at the frame edge
//	S = v / (1 + Bend·t³)
//
// A band is a curve of constant S, so v = S·(1 + Bend·t³): out at the edges it
// sits at its own height and is straight, and as it nears the funnel it climbs
// and thickens. Both flares fall out of that one expression, and so does the
// fact that a band peaks exactly where it meets the funnel.
//
// The lift is a function of u alone, deliberately. Feeding v back in through
// the silhouette — so a band lifted into a wider part of the funnel is lifted
// further still — closes the constant-S curves into ovals around the waist
// instead of sweeping them off the top of the frame.
//
// # WHY IT COUNTER-SCROLLS
//
// Because that is the animation asked for, and not because a surface would do
// it. Slide the funnel's phase one way and the field's the other and the
// picture reads as a column rising through a descending field — an illusion
// the still image sets up and never delivers. One surface scrolled rigidly
// would move everything the same way, which is the ordinary thing.
//
// # ANTIALIASING IS NOT OPTIONAL HERE
//
// A band at the waist is a pixel or two across on a half-block surface, and
// hard black-and-white bands that narrow reduce to a moire of whatever the
// sampling grid happens to hit — the picture crawls, and the crawl is not the
// motion. Each pixel is therefore sampled four times and averaged, which turns
// the too-fine bands into the grey they should be.
package magnetosphere

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Magnetosphere is the animation. The zero value is not usable; call New.
type Magnetosphere struct {
	w, h     int
	cx, cy   float64
	invHalfH float64

	// Derived in Resize: the hyperboloid's throat radius in world units, and
	// Depth² - a², which is the quadratic's constant term.
	throat float64
	cTerm  float64
	wide   float64 // the horizontal scale the field bends over

	fldBands float64 // FieldPitch resolved against the window height
	funBands float64 // FunnelPitch, likewise
	funPhase float64 // bands, added to the funnel's stripe coordinate
	fldPhase float64 // bands, added to the field's

	// Waist is the funnel's half-width on screen at its narrowest, in units of
	// half the window height.
	Waist float64
	// Slope is how fast the funnel opens out: its half-width at height v is
	// hypot(v*Slope, Waist), so far from the waist the silhouette is a cone of
	// this gradient. At the logo's value the funnel has opened past the side
	// of a three-by-two frame by about two thirds of the way to the top.
	Slope float64
	// Depth is how far the camera stands from the funnel's axis, in the same
	// units. It is what decides how strongly a ring's near side swings below
	// its far side, so it is the difference between rings that read as
	// ellipses and rings that read as flat bars.
	Depth float64
	// Bend is how hard the field's stripes are pulled up towards the funnel.
	// Zero leaves them straight, which is a page of lines with a funnel on it
	// and none of the illusion.
	Bend float64
	// BendWidth is the horizontal scale the field bends over, in half-heights.
	// Zero derives it as three waists.
	BendWidth float64
	// FieldPitch is how tall a band of the field is out at the frame edge, in
	// pixels of the surface — so half a character cell tall each.
	//
	// Pixels and not a count of bands over the height, which is what the logo
	// itself is drawn with. A count is resolution-independent in the wrong
	// direction here: the logo has around twenty-five bands over its height,
	// and twenty-five bands over a thirty-row terminal is under three pixels
	// each, which is not a picture of anything. Holding the pitch instead
	// keeps every band legible and shows fewer of them in a small window,
	// which is the trade a terminal wants.
	FieldPitch float64
	// FunnelPitch is the same, measured at the waist. Perspective does the
	// rest, so the bands thicken into the flare on their own.
	FunnelPitch float64
	// FunnelSpeed is bands per second, positive upward.
	FunnelSpeed float64
	// FieldSpeed is bands per second, positive downward — so both defaults are
	// positive and the two move against each other.
	FieldSpeed float64
	// Duty is the fraction of each cycle the light band occupies. A half is
	// the logo; lower thins the light bands towards a line drawing.
	Duty float64
	// Light and Dark are the two colors of the field, and FunnelLight and
	// FunnelDark the funnel's. All four default to the logo's white on black.
	Light, Dark             [3]float64
	FunnelLight, FunnelDark [3]float64
}

// New returns the logo. There is nothing to randomize — the picture is a rule,
// not a process — so the seed is accepted for the sake of the common signature
// and ignored.
func New(_ int64) *Magnetosphere {
	return &Magnetosphere{
		// Measured off the logo: the throat is a little over a fifth of the
		// half-height across, and the funnel has opened past the side of the
		// frame by three quarters of the way to the top.
		Waist: 0.24,
		Slope: 1.15,
		// Close. Standing back flattens the rings towards the flat bars an
		// approximation would have drawn, which is the one thing this is doing
		// the geometry for.
		Depth: 2.0,
		// Enough that the outermost stripes are within a few percent of
		// straight and the innermost have climbed most of the way to the top.
		Bend:        3.0,
		FieldPitch:  7,
		FunnelPitch: 5,
		// Slow, and not a round ratio: at 3:2 the two fields would drift back
		// into alignment every few seconds and the eye latches onto that
		// instead of onto the counter-motion.
		FunnelSpeed: 0.62,
		FieldSpeed:  0.41,
		Duty:        0.5,
		Light:       [3]float64{236, 236, 240},
		Dark:        [3]float64{10, 10, 14},
		FunnelLight: [3]float64{236, 236, 240},
		FunnelDark:  [3]float64{10, 10, 14},
	}
}

// Resize records the window and works out the constants the ray test needs.
func (m *Magnetosphere) Resize(w, h int) {
	m.w, m.h = w, h
	if w <= 0 || h <= 0 {
		return
	}
	m.cx, m.cy = float64(w)/2, float64(h)/2
	// Both axes are measured in half-heights, so the funnel keeps its
	// proportions and a wider window simply shows more field either side of
	// it — which is what widening the logo's frame would do.
	m.invHalfH = 2 / float64(h)

	// Waist is given on screen; the throat radius that projects to it from
	// Depth away is a = Depth*Waist/sqrt(1+Waist²), which comes straight out
	// of setting the silhouette condition equal to u² = Waist² at v = 0.
	m.throat = m.Depth * m.Waist / math.Sqrt(1+m.Waist*m.Waist)
	m.cTerm = m.Depth*m.Depth - m.throat*m.throat

	// Turn the two pitches into the band counts the stripe coordinates want.
	//
	// One unit of v is half the window in pixels, and out at the frame edge the
	// field's lift is near enough one to ignore, so a field band is
	// (h/2)/fldBands pixels tall.
	halfPx := float64(h) / 2
	m.fldBands = halfPx / math.Max(m.FieldPitch, 0.5)
	// The funnel's coordinate is the height of the ray's hit, which at the
	// waist advances k0 per unit of v — the near root with the ray straight
	// ahead, where the discriminant is just the throat radius squared.
	k0 := m.cTerm / (m.Depth + m.throat)
	m.funBands = halfPx / (k0 * math.Max(m.FunnelPitch, 0.5))

	m.wide = m.BendWidth
	if m.wide <= 0 {
		// Three waists out, which is about where the funnel has opened enough
		// to be the thing the field bends around. Its width at the very top is
		// not the scale to use: that is wider than the frame, and a bend
		// measured against something wider than the frame lifts the whole
		// field by the same amount — a page of straight lines again.
		m.wide = m.Waist * 3
	}
	if m.wide <= 0 {
		m.wide = 1e-6
	}
}

// Frame advances both phases and redraws.
func (m *Magnetosphere) Frame(s *canvas.Surface, dt float64) {
	if m.w == 0 || m.h == 0 {
		return
	}
	// Negated: a band sits where its stripe coordinate is constant, so raising
	// the phase lowers the band. Storing the phase the way it is added keeps
	// the sign out of sample, and the surprise out of the exported fields.
	m.funPhase = wrap1(m.funPhase - m.FunnelSpeed*dt)
	m.fldPhase = wrap1(m.fldPhase + m.FieldSpeed*dt)

	w, h := s.Size()
	if w > m.w {
		w = m.w
	}
	if h > m.h {
		h = m.h
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b float64
			// Four samples on a half-pixel grid. See the package comment: at
			// the waist a band is about a pixel across, and one sample per
			// pixel of a black-and-white pattern that fine is a moire.
			for _, o := range [4][2]float64{
				{0.25, 0.25}, {0.75, 0.25}, {0.25, 0.75}, {0.75, 0.75},
			} {
				u := (float64(x) + o[0] - m.cx) * m.invHalfH
				v := (m.cy - float64(y) - o[1]) * m.invHalfH
				c := m.sample(u, v)
				r += c[0]
				g += c[1]
				b += c[2]
			}
			s.Set(x, y, tcell.NewRGBColor(
				clamp255(r/4), clamp255(g/4), clamp255(b/4)))
		}
	}
}

// disc is the ray-surface discriminant at one screen point. Non-negative means
// the ray meets the funnel, so its sign is the silhouette and its size says
// how squarely the surface faces the camera there.
func (m *Magnetosphere) disc(u, v float64) float64 {
	vs := v * m.Slope
	return m.Depth*m.Depth - (u*u+1-vs*vs)*m.cTerm
}

// sample returns the color at one point, in half-height units with v up.
func (m *Magnetosphere) sample(u, v float64) [3]float64 {
	if disc := m.disc(u, v); disc >= 0 {
		// The ray meets the funnel. k is the near hit, written so that it
		// stays accurate as a passes through zero — which it does, at the
		// height where the ray runs parallel to the surface's asymptote.
		k := m.cTerm / (m.Depth + math.Sqrt(disc))
		s := k * v * m.funBands

		// How much of a band one pixel sideways covers.
		//
		// Towards the silhouette the surface turns away from the camera and
		// the bands crowd without limit — a striped surface seen exactly edge
		// on has infinitely many bands under the last pixel of it. No amount
		// of supersampling fixes that; four samples of a thousand bands is
		// still noise. Where a pixel spans more than about a third of a band,
		// fade towards the average of the two colors, which is what that pixel
		// would average to if it could be sampled properly.
		width := 1.0
		if d2 := m.disc(u+m.invHalfH, v); d2 >= 0 {
			k2 := m.cTerm / (m.Depth + math.Sqrt(d2))
			width = math.Abs(k2-k) * math.Abs(v) * m.funBands
		}
		b := band(s+m.funPhase, m.Duty)
		sharp := 1 - smoothstep((width-0.2)/0.4)
		return mix(m.FunnelDark, m.FunnelLight, 0.5+(b-0.5)*sharp)
	}
	// The field, outside the silhouette.
	//
	// Cubes over a sum rather than a clamped ratio: a clamp leaves the lift
	// flat everywhere inside the scale it clamps at, and the field there runs
	// dead horizontal — a shelf beside the funnel that the logo does not have.
	w3 := pow3(m.wide)
	lift := 1 + m.Bend*w3/(pow3(math.Abs(u))+w3)
	s := v / lift * m.fldBands
	return mix(m.Dark, m.Light, band(s+m.fldPhase, m.Duty))
}

// pow3 is x cubed. The bend is concentrated near the funnel rather than spread
// across the picture: in the logo the outer half of the field is straight and
// all of it happens in the last stretch before the flare.
func pow3(x float64) float64 { return x * x * x }

// band returns how light the stripe at coordinate s is, 0 to 1.
//
// The edges are not hard. A band boundary that snapped from black to white
// would land wherever the sample grid put it and jitter by a whole pixel as
// the phase moved; softening it over a fraction of a band lets the boundary
// move smoothly across a pixel instead, which is what makes slow scrolling
// look continuous rather than stepped.
func band(s, duty float64) float64 {
	const soft = 0.16
	if duty <= 0 {
		return 0
	}
	if duty >= 1 {
		return 1
	}
	f := s - math.Floor(s)
	// Rising edge at 0, falling edge at duty.
	return smoothstep(f/soft) * smoothstep((duty-f)/soft)
}

// smoothstep clamps x to 0..1 and eases it.
func smoothstep(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	return x * x * (3 - 2*x)
}

func mix(dark, light [3]float64, k float64) [3]float64 {
	return [3]float64{
		dark[0] + (light[0]-dark[0])*k,
		dark[1] + (light[1]-dark[1])*k,
		dark[2] + (light[2]-dark[2])*k,
	}
}

// wrap1 keeps a phase in [0,1) so it cannot drift out to where float64 stops
// resolving a frame's worth of movement.
func wrap1(v float64) float64 {
	return v - math.Floor(v)
}

func clamp255(v float64) int32 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return int32(v)
}

// Run draws the logo on the given screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
