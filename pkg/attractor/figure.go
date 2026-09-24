package attractor

import (
	"image"
	"image/draw"
	"math"

	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/geom"
)

// The models that can be drawn with no browser, beyond the flows.
//
// headless.go draws a flow's trajectory. The catalog has more than flows that
// need nothing a browser provides: a map's iterates are arithmetic, the
// polyhedra and the sphere, torus, globe and magnetosphere are line lists from
// pkg/geom and conway.go, and the Lissajous figure is three sines. Each is
// built here from the same code the page's generator uses, so `chaosrack
// render` draws what the page draws.

// FigureKind is how a figure's points are joined.
type FigureKind int

const (
	// FigurePath joins the points in order: a trajectory or a curve.
	FigurePath FigureKind = iota
	// FigurePoints leaves them unjoined: a map's iterates, which are
	// far apart on the attractor and would draw a hairball if joined.
	FigurePoints
	// FigureLines joins the pairs listed in Edges: a wireframe.
	FigureLines
)

// Figure is a model's geometry, ready to draw.
type Figure struct {
	Kind   FigureKind
	Points [][3]float64
	Edges  []uint16 // endpoint pairs into Points, for FigureLines
}

// StaticFigure builds a model that is not a flow. points is the point budget
// for a map or a curve; wireframes have a fixed size and ignore it. It reports
// false for flows (see dynamics.Trajectory) and for models that need a
// browser.
func StaticFigure(key string, points int) (Figure, bool) {
	if pts := dynamics.MapPoints(key, points); pts != nil {
		return Figure{Kind: FigurePoints, Points: pts}, true
	}
	for i, s := range polySeeds {
		if s.Name == key {
			return polyFigure(conwaySolid(i, 0)), true
		}
	}
	switch key {
	case "nestedcube":
		return linesFigure(geom.Lines{Vertices: verticesCube, Indices: indicesCube}), true
	case "sphere":
		v, i := geom.Sphere(1, 30, 30, 0)
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "torus":
		v, i := geom.Torus(1.5, 0.5, 30, 30, 0)
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "globe":
		return linesFigure(geom.Globe(18, 36, 60)), true
	case "magnetosphere":
		return linesFigure(geom.Magnetosphere()), true
	case "lissajou":
		return Figure{Kind: FigurePath, Points: lissajous(points)}, true
	}
	return Figure{}, false
}

// Drawable reports whether a model can be drawn with no browser.
func Drawable(key string) bool {
	if dynamics.HasFlow(key) {
		return true
	}
	_, ok := StaticFigure(key, 2)
	return ok
}

func polyFigure(p polyhedron) Figure {
	f := Figure{Kind: FigureLines}
	for _, v := range p.Verts {
		f.Points = append(f.Points, [3]float64{v.X, v.Y, v.Z})
	}
	for _, e := range p.Edges() {
		f.Edges = append(f.Edges, uint16(e[0]), uint16(e[1])) //nolint:gosec // a polyhedron has far fewer than 65536 vertices
	}
	return f
}

func linesFigure(l geom.Lines) Figure {
	f := Figure{Kind: FigureLines, Edges: l.Indices}
	for i := 0; i+2 < len(l.Vertices); i += 3 {
		f.Points = append(f.Points, [3]float64{
			float64(l.Vertices[i]), float64(l.Vertices[i+1]), float64(l.Vertices[i+2]),
		})
	}
	return f
}

// lissajous is the page's figure at its default 3:2:5 ratios and zero phase:
// one full period, which is a closed curve.
func lissajous(n int) [][3]float64 {
	if n < 2 {
		n = 2
	}
	out := make([][3]float64, n)
	for i := range out {
		t := 2 * math.Pi * float64(i) / float64(n-1)
		out[i] = [3]float64{math.Sin(3 * t), math.Sin(2 * t), math.Sin(5 * t)}
	}
	return out
}

// DrawFigure renders a figure into a new image. For FigurePoints only the
// first hi points are drawn, and for FigurePath points lo..hi, so an animation
// can reveal a figure while the frame stays fitted to all of it.
func DrawFigure(f Figure, lo, hi int, o DrawOptions) *image.RGBA {
	if f.Kind == FigurePath {
		return DrawSpan(f.Points, lo, hi, o)
	}
	o = o.withDefaults()
	img := image.NewRGBA(image.Rect(0, 0, o.Width, o.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{o.Background}, image.Point{}, draw.Src)
	v, view := o.place(f.Points)
	idx := f.Edges
	if f.Kind == FigurePoints {
		idx = PointIndices(len(f.Points), hi)
	}
	view.Render(img, v, idx, o.Gradient)
	return img
}

// PointIndices draws each of the first hi of n points as a zero-length
// segment, which the renderer plots as a single pixel.
func PointIndices(n, hi int) []uint16 {
	if hi > n {
		hi = n
	}
	if hi > 65536 {
		hi = 65536
	}
	out := make([]uint16, 0, hi*2)
	for i := 0; i < hi; i++ {
		out = append(out, uint16(i), uint16(i)) //nolint:gosec // clamped to 65536 above
	}
	return out
}
