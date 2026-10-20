package attractor

import (
	"image"
	"image/draw"
	"math"

	"github.com/0magnet/chaosrack/pkg/conway"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/geom"
)

// The models that can be drawn with no browser, beyond the flows.
//
// headless.go draws a flow's trajectory. The catalog has more than flows that
// need nothing a browser provides: a map's iterates are arithmetic, the
// polyhedra and the sphere, torus, globe and magnetosphere are line lists from
// pkg/geom and pkg/conway, and the Lissajous figure is three sines. Each is
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
	// Ages is each point's place along the trail, 0..1, which a gradient
	// sourced from the trail colors by; none, and it is the point's order.
	Ages []float32
	// Colors is each point's own color, for a picture whose colors are its
	// own: none, and the gradient colors it.
	Colors [][3]float32
}

// StaticFigure builds a model that is not a flow. points is the point budget
// for a map or a curve; wireframes have a fixed size and ignore it. It reports
// false for flows (see dynamics.Trajectory) and for models that need a
// browser.
func StaticFigure(key string, points int) (Figure, bool) {
	if pts := dynamics.MapPoints(key, points); pts != nil {
		return Figure{Kind: FigurePoints, Points: pts}, true
	}
	switch key {
	case "polyhedron":
		// At its defaults, {3,3}: the tetrahedron. See conway_js.go.
		return polyFigure(conway.Regular(3, 3)), true
	case "nestedcube":
		return linesFigure(geom.Lines{Vertices: verticesCube, Indices: indicesCube}), true
	case "torus":
		v, i := geom.Torus(1.5, 0.5, 30, 30, 0)
		return linesFigure(geom.Lines{Vertices: v, Indices: i}), true
	case "globe":
		return linesFigure(geom.Globe(18, 36, 60)), true
	case "magnetosphere":
		return linesFigure(geom.Magnetosphere()), true
	case "lissajou":
		return Figure{Kind: FigurePath, Points: lissajous(points)}, true
	case "graphicartist":
		g := NewGraphicArtist()
		for _, id := range GraphicArtistKnobs {
			if v, ok := figureSets[id]; ok {
				g.Set(id, v)
			}
		}
		f := Figure{Kind: FigurePath, Points: make([][3]float64, max(points, 2))}
		g.Trace(len(f.Points), func(i int, x, y, z float64) { f.Points[i] = [3]float64{x, y, z} })
		return f, true
	case "sprottmorph":
		// Parked on one system, D by default: a still is one wiring.
		sp := NewSprottMorph(func() float64 { return 0.5 })
		sp.Set("smorph-rate", 0)
		if v, ok := figureSets["smorph-sys"]; ok {
			sp.Set("smorph-sys", v)
		}
		n := max(points, 2)
		sp.Enter(n)
		f := Figure{Kind: FigurePath, Points: make([][3]float64, n)}
		sp.Trail(n, func(k int, x, y, z float64) { f.Points[k] = [3]float64{x, y, z} })
		return f, true
	case "turtle":
		return turtleFigure(points), true
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

func polyFigure(p conway.Solid) Figure {
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
func lissajous(n int) [][3]float64 { return lissajousOf(3, 2, 5, n) }

// lissajousOf is the figure at ratios a:b:c and zero phase, over one period
// of the base: closed, as the knobs' whole ratios make it.
func lissajousOf(a, b, c float64, n int) [][3]float64 {
	if n < 2 {
		n = 2
	}
	out := make([][3]float64, n)
	for i := range out {
		t := 2 * math.Pi * float64(i) / float64(n-1)
		out[i] = [3]float64{math.Sin(a * t), math.Sin(b * t), math.Sin(c * t)}
	}
	return out
}

// DrawFigure renders a figure into a new image. For FigurePoints and
// FigurePath only points lo..hi are drawn, so an animation
// can reveal a figure while the frame stays fitted to all of it.
func DrawFigure(f Figure, lo, hi int, o DrawOptions) *image.RGBA {
	if f.Kind == FigurePath && len(f.Ages) != len(f.Points) && len(f.Colors) != len(f.Points) {
		return DrawSpan(f.Points, lo, hi, o)
	}
	o = o.withDefaults()
	img := image.NewRGBA(image.Rect(0, 0, o.Width, o.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{o.Background}, image.Point{}, draw.Src)
	v, view := o.place(f.Points)
	idx := f.Edges
	if f.Kind == FigurePoints {
		idx = PointIndices(len(f.Points), lo, hi)
	}
	if f.Kind == FigurePath {
		idx = SpanIndices(len(f.Points), lo, hi)
	}
	view.RenderColors(img, v, idx, o.Gradient, f.Ages, f.Colors)
	return img
}

// PointIndices draws points lo..hi of n each as a zero-length segment, which
// the renderer plots as a single pixel.
func PointIndices(n, lo, hi int) []uint16 {
	if hi > n {
		hi = n
	}
	if hi > 65536 {
		hi = 65536
	}
	lo = max(lo, 0)
	out := make([]uint16, 0, max(hi-lo, 0)*2)
	for i := lo; i < hi; i++ {
		out = append(out, uint16(i), uint16(i)) //nolint:gosec // clamped to 65536 above
	}
	return out
}

// turtleFigure is the Turtle Path at its defaults, walked far enough to be the
// figure the page shows: the page extends the walk at the Speed knob's rate
// forever; this walks it five seconds of the page's frames, about as long as
// a visitor has watched it, then takes the trail.
func turtleFigure(points int) Figure {
	var r TurtleRun
	budget := max(points, 2)
	knobs := figureTurtleKnobs()
	f := Figure{Kind: FigurePath}
	for range 5 {
		r.Step(knobs, budget, 60, 1, 1, true, nil, func(_, _, _, _ float32) {})
	}
	r.Step(knobs, budget, 0, 1, 1, false, nil, func(x, y, z, tint float32) {
		f.Points = append(f.Points, [3]float64{float64(x), float64(y), float64(z)})
		f.Ages = append(f.Ages, tint)
	})
	return f
}

// figureSets are the knobs the scope models are drawn at, beyond their
// defaults: a renderer's --set, by the page's ids.
var figureSets = map[string]float32{}

// FigureKnobs are the knobs of the models StaticFigure draws from the page's
// own state rather than a parameter registry — the Turtle Path, the Graphic
// Artist and the Sprott Morph — each with its default.
func FigureKnobs() []struct {
	ID  string
	Def float32
} {
	var out []struct {
		ID  string
		Def float32
	}
	add := func(id string, def float32) {
		out = append(out, struct {
			ID  string
			Def float32
		}{id, def})
	}
	for i, id := range TurtleKnobs {
		add(id, TurtleDefaults[i])
	}
	g := NewGraphicArtist()
	for _, id := range GraphicArtistKnobs {
		add(id, g.value(id))
	}
	add("smorph-sys", 3)
	return out
}

// SetFigureKnob turns one of FigureKnobs for the figures StaticFigure draws
// from then on, reporting whether id is one.
func SetFigureKnob(id string, v float32) bool {
	for _, k := range FigureKnobs() {
		if k.ID == id {
			figureSets[id] = v
			return true
		}
	}
	return false
}

// figureTurtleKnobs is TurtleDefaults with figureSets over it.
func figureTurtleKnobs() [9]float32 {
	k := TurtleDefaults
	for i, id := range TurtleKnobs {
		if v, ok := figureSets[id]; ok {
			k[i] = v
		}
	}
	return k
}
