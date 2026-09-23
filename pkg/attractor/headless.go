package attractor

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// Drawing a model with no browser.
//
// Almost all of this already existed and none of it was joined up: Trajectory
// integrates any registered flow headlessly (transient dropped, decimated to a
// point budget, 4-D systems included), and pkg/rasterview is a pure software
// renderer with tests and — until now — no caller outside them. What was
// missing was the twenty lines between them.
//
// It matters for more than convenience. A whole class of bug is invisible to
// the tests this package has: TestEveryDynamicalModeMeasuresChaotic walks the
// catalog asking for a Lyapunov exponent, but the Sprott morph's flow is a
// blended coefficient table rather than a registered deriv, so it answers
// "n/a", the test skips it, and ten of nineteen systems shipped drawing a
// single motionless point. A picture cannot be skipped that way, and an
// attractor that is not attracting has no extent — which is a thing a test can
// assert without a browser, a GPU or an eye.
//
// What this does NOT do is the rack. Module widths, bays and panel layout are
// measured from real text in a real font in a real flex container; no software
// renderer answers those. The browser is still where the layout lives. This is
// for the models.

// DrawOptions is how to draw a trajectory.
type DrawOptions struct {
	Width, Height int
	View          rasterview.View
	Gradient      rasterview.Gradient
	// Background is painted before the trail. It has to be, and that is the
	// renderer's contract rather than an oversight: Render deliberately does
	// NOT clear its destination (TestRenderDoesNotClear pins it), so that a
	// caller can compose several passes into one image. A fresh image.RGBA is
	// transparent black, so a caller that forgets gets a picture that is
	// invisible on anything that shows transparency as white.
	Background color.RGBA
}

func (o DrawOptions) withDefaults() DrawOptions {
	if o.Width <= 0 {
		o.Width = 1000
	}
	if o.Height <= 0 {
		o.Height = 1000
	}
	// Colors 0 would render every vertex black on black, which is the same
	// picture a broken model draws — the one case the output must never be
	// ambiguous about.
	if o.Gradient.Colors == 0 {
		o.Gradient = rasterview.DefaultGradient()
	}
	if o.Background.A == 0 {
		o.Background = color.RGBA{10, 13, 20, 255} // the panel's own near-black
	}
	return o
}

// Vertices flattens a trajectory into the x,y,z triples the renderer takes.
func Vertices(pts [][3]float64) []float32 {
	out := make([]float32, 0, len(pts)*3)
	for _, p := range pts {
		out = append(out, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	return out
}

// TrailIndices joins the points into one polyline.
//
// rasterview draws LINE SEGMENTS from pairs of indices — it is a wireframe
// renderer, and a mesh hands it edges. A trail has none: it is one path
// through the points in order, so the edges are every consecutive pair. Passing
// nil draws a perfectly correct picture of no edges at all, which is a
// background and nothing else.
func TrailIndices(n int) []uint16 {
	if n < 2 {
		return nil
	}
	// uint16 is the renderer's index type, so a trail longer than 65535 points
	// cannot be joined end to end. Stopping is better than wrapping round to
	// index 0 and drawing a line from the end of the attractor to its start.
	if n > 65535 {
		n = 65535
	}
	out := make([]uint16, 0, (n-1)*2)
	for i := 0; i+1 < n; i++ {
		out = append(out, uint16(i), uint16(i+1))
	}
	return out
}

// Draw renders a trajectory into a new image.
func Draw(pts [][3]float64, o DrawOptions) *image.RGBA {
	return DrawSpan(pts, 0, len(pts), o)
}

// DrawSpan renders only pts[lo:hi], with the picture still framed by ALL of
// pts. It is how a trail is drawn sweeping along a path without the path
// moving underneath it.
//
// Every frame is handed the whole trajectory and differs only in which
// segments it joins. That is not an optimization, it is the thing that makes
// the animation hold still: Render measures its fit and normalizes the
// gradient over the vertices it is given, so drawing each window on its own
// would re-center and re-scale to that window — and a trail sweeping a fixed
// path would come out as a fixed trail sweeping a path that lurched and
// breathed. The colors would crawl too, a window's own min and max not being
// the run's.
func DrawSpan(pts [][3]float64, lo, hi int, o DrawOptions) *image.RGBA {
	o = o.withDefaults()
	img := image.NewRGBA(image.Rect(0, 0, o.Width, o.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{o.Background}, image.Point{}, draw.Src)
	v := Vertices(Centered(pts))
	o.View.Render(img, v, SpanIndices(len(v)/3, lo, hi), o.Gradient)
	return img
}

// SpanIndices joins points lo..hi of an n-point trail end to end.
func SpanIndices(n, lo, hi int) []uint16 {
	// uint16 is the renderer's index type, so nothing past 65535 can be
	// addressed at all. Clamping rather than wrapping: a wrap would draw a
	// line from the end of the attractor back to its start.
	if n > 65536 {
		n = 65536
	}
	if lo < 0 {
		lo = 0
	}
	if hi > n {
		hi = n
	}
	if hi-lo < 2 {
		return nil
	}
	out := make([]uint16, 0, (hi-lo-1)*2)
	for i := lo; i+1 < hi; i++ {
		out = append(out, uint16(i), uint16(i+1)) //nolint:gosec // clamped to 65536 above
	}
	return out
}

// Extent is a trajectory's size along each axis.
//
// The cheapest thing worth asserting about a model, and the one that catches
// what a picture is drawn to catch: an attractor that is not attracting spans
// nothing. Exported so a command can say "it drew nothing" instead of writing
// a black PNG and leaving it to the eye.
func Extent(pts [][3]float64) (dx, dy, dz float64) {
	if len(pts) == 0 {
		return 0, 0, 0
	}
	mn, mx := pts[0], pts[0]
	for _, p := range pts {
		for a := 0; a < 3; a++ {
			if p[a] < mn[a] {
				mn[a] = p[a]
			}
			if p[a] > mx[a] {
				mx[a] = p[a]
			}
		}
	}
	return mx[0] - mn[0], mx[1] - mn[1], mx[2] - mn[2]
}

// Centered moves a trajectory so its bounding box sits on the origin.
//
// The renderer fits the picture by the furthest point FROM THE ORIGIN, which
// is the right rule for a mesh — a globe, a polyhedron, anything modeled
// around its own middle. An attractor is not modeled around anything: Lorenz
// lives around z ≈ 27, so fitting it from the origin pushes it into a corner
// and shrinks it to make room for the empty half of the frame it is being
// measured across.
//
// The gradient is unaffected. It normalizes against the trajectory's own
// min/max, and shifting every point by the same amount shifts both ends
// equally, so a coordinate's position within its range does not move.
func Centered(pts [][3]float64) [][3]float64 {
	if len(pts) == 0 {
		return pts
	}
	mn, mx := pts[0], pts[0]
	for _, p := range pts {
		for a := 0; a < 3; a++ {
			if p[a] < mn[a] {
				mn[a] = p[a]
			}
			if p[a] > mx[a] {
				mx[a] = p[a]
			}
		}
	}
	var mid [3]float64
	for a := 0; a < 3; a++ {
		mid[a] = (mn[a] + mx[a]) / 2
	}
	out := make([][3]float64, len(pts))
	for i, p := range pts {
		out[i] = [3]float64{p[0] - mid[0], p[1] - mid[1], p[2] - mid[2]}
	}
	return out
}
