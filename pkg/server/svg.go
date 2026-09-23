//go:build !js

package server

import (
	"fmt"
	"math"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// A trail as SVG, which is what a trail already is.
//
// The raster path goes through the same software renderer the page's depth
// shading comes from, and is the right answer when the picture is the point.
// This is the right answer when the DATA is: a polyline scales to any size,
// diffs line by line against the last one, and can be read with an editor.
//
// Projected here rather than by rasterview because rasterview draws pixels —
// it shades by depth and blends per fragment, and none of that survives being
// turned into a path. The projection is the same rotation in both, so the two
// pictures are the same picture.

// svgOf projects a trajectory and writes it as one polyline.
func svgOf(pts [][3]float64) string {
	if len(pts) == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg"/>`
	}
	a := [3]float64{}
	copy(a[:], renderSpin)
	pts = attractor.Centered(pts)
	r, mx, my := svgFit(pts, [][3]float64{a})

	g := gradientFor()
	min, max := rasterview.ModelBounds(attractor.Vertices(pts))
	var defs, body strings.Builder
	writeColoredTrail(&defs, &body, "g0", pts, a, r, mx, my, g, min, max, 0, len(pts))

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		renderW, renderH, renderW, renderH)
	if defs.Len() > 0 {
		b.WriteString(`<defs>` + defs.String() + `</defs>`)
	}
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0a0d14"/>`, renderW, renderH)
	b.WriteString(body.String())
	b.WriteString(`</svg>`)
	return b.String()
}

// project turns one centered model point into frame coordinates.
//
// Rz, then Ry, then Rx — the same order rasterview applies, so the SVG and the
// PNG show the same view from the same angles.
func project(p [3]float64, a [3]float64) (x, y float64) {
	sinX, cosX := math.Sin(a[0]), math.Cos(a[0])
	sinY, cosY := math.Sin(a[1]), math.Cos(a[1])
	sinZ, cosZ := math.Sin(a[2]), math.Cos(a[2])
	x1, y1 := p[0]*cosZ-p[1]*sinZ, p[0]*sinZ+p[1]*cosZ
	x2, z2 := x1*cosY+p[2]*sinY, -x1*sinY+p[2]*cosY
	return x2, y1*cosX - z2*sinX
}

// svgFit is the scale and the middle that put pts in the frame as seen from
// ANY of views: pixels per model unit, and the projected point to draw at the
// center of the picture.
//
// ONE FIT FOR EVERY FRAME, measured rather than assumed, and it took two goes.
//
// Fitting each frame to its own projection makes the model pump in and out
// once a turn: a flat figure seen edge-on projects narrow and would then be
// magnified to fill the frame. Fitting instead to the bounding SPHERE — the
// furthest point's distance from the middle, which no rotation changes —
// avoids the pumping and was what this did, but it wastes most of the picture,
// because an attractor is not a ball: the sphere around Lorenz is several
// times the figure, and the drawing came out about a third of the size the
// raster path produced from the same trajectory.
//
// The projected bounding box over the WHOLE TURN is both. It cannot change
// between frames, because it is the same box for all of them, and it is the
// smallest such box, so the figure is as large as a constant scale allows.
//
// The center has to be measured the same way, and separately from the 3-D
// centering. attractor.Centered puts the middle of the model's bounding BOX at
// the origin, which does not put the middle of its PROJECTION there: rotate a
// box and the shadow's middle moves. That is why the picture sat left of
// center in the frame — the drawing was right, the frame was hung wrong.
func svgFit(pts [][3]float64, views [][3]float64) (r, midX, midY float64) {
	loX, hiX := math.Inf(1), math.Inf(-1)
	loY, hiY := math.Inf(1), math.Inf(-1)
	for _, a := range views {
		for _, p := range pts {
			x, y := project(p, a)
			loX, hiX = math.Min(loX, x), math.Max(hiX, x)
			loY, hiY = math.Min(loY, y), math.Max(hiY, y)
		}
	}
	w, h := hiX-loX, hiY-loY
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	// The tighter of the two fits, so neither axis is cropped.
	r = math.Min(float64(renderW)/w, float64(renderH)/h) * 0.94
	return r, (loX + hiX) / 2, (loY + hiY) / 2
}

// writePoints writes one view's projected coordinates as polyline points, at a
// fit the caller decided so that every frame of an animation shares it.
// pts must already be centered.
func writePoints(b *strings.Builder, pts [][3]float64, a [3]float64, r, midX, midY float64) {
	cx, cy := float64(renderW)/2, float64(renderH)/2
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		x, y := project(p, a)
		// Two decimals: at these sizes it is under a hundredth of a pixel, and
		// full precision triples the file for a difference nothing can show.
		fmt.Fprintf(b, "%.2f,%.2f", cx+(x-midX)*r, cy-(y-midY)*r)
	}
}

// writeTrail writes the trail as polylines colored by the SAME gradient the
// raster path uses.
//
// SVG has no per-vertex color: a polyline is one stroke. The picture is
// therefore cut into runs that quantize to the same color and each run is its
// own polyline, sharing its end point with the next so the line stays joined.
//
// THIS IS THE FALLBACK. Run-length encoding a gradient works when the color
// changes slowly along the path and does not here: the palette follows a MODEL
// AXIS, and a trajectory oscillates across that axis constantly, so the color
// runs back and forth rather than sweeping. Measured on Lorenz, 6,000 points
// came out as 4,461 runs even after quantizing to 138 colors — the file was
// most of a megabyte for a still and the color still changes every second or
// third segment. svgAxisGradient does the job properly for every view that
// admits it; this covers the ones that do not.
func writeTrail(b *strings.Builder, pts [][3]float64, a [3]float64, r, midX, midY float64, g rasterview.Gradient, min, max [3]float32, ageOff, ageTotal int) {
	if len(pts) < 2 {
		return
	}
	var run strings.Builder
	cur := ""
	flush := func() {
		if cur == "" || run.Len() == 0 {
			return
		}
		b.WriteString(svgStroke)
		fmt.Fprintf(b, ` stroke="%s" points="%s"/>`, cur, run.String())
		run.Reset()
	}
	cx, cy := float64(renderW)/2, float64(renderH)/2
	prev := ""
	for i, p := range pts {
		// Age over the WHOLE run, not over this window: the raster path colors
		// from every vertex it is handed, so a frame of the GIF and the same
		// frame of the SVG have to measure the same way or the two animations
		// are colored differently.
		age := float32(0)
		if ageTotal > 1 {
			age = float32(ageOff+i) / float32(ageTotal-1)
		}
		c := g.ColorAt(float32(p[0]), float32(p[1]), float32(p[2]), min, max, age)
		hex := quantHex(c)
		x, y := project(p, a)
		pt := fmt.Sprintf("%.2f,%.2f", cx+(x-midX)*r, cy-(y-midY)*r)
		if hex != cur && i > 0 {
			// Close the old run ON this point, then start the new one from it,
			// so there is no gap where the color changes.
			run.WriteByte(' ')
			run.WriteString(pt)
			flush()
			cur = hex
			run.WriteString(prev)
			run.WriteByte(' ')
			run.WriteString(pt)
		} else {
			if cur == "" {
				cur = hex
			} else if run.Len() > 0 {
				run.WriteByte(' ')
			}
			run.WriteString(pt)
		}
		prev = pt
	}
	flush()
}

func clamp01f(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// svgStroke is the opening tag every trail is drawn with.
//
// round joins and caps, because a trajectory is a curve and the polyline is a
// sampling of it: at a few thousand points across a frame the segments meet at
// visible angles, and a miter join draws each of those corners as a spike. The
// curve is still faceted — that is what --points buys — but it stops being
// faceted AND spiky.
const svgStroke = `<polyline fill="none" stroke-width="0.6" ` +
	`stroke-opacity="0.85" stroke-linejoin="round" stroke-linecap="round"`

// gradientFor is the palette the flags ask for, shared by the raster and the
// vector writers so --colors means the same thing in a PNG and an SVG.
func gradientFor() rasterview.Gradient {
	g := rasterview.DefaultGradient()
	if renderColors > 0 {
		g.Colors = renderColors
	}
	switch renderGrad {
	case "x":
		g.Source = 0
	case "y":
		g.Source = 1
	case "trail", "age", "t":
		g.Source = rasterview.SourceTrail
	default:
		g.Source = 2
	}
	return g
}

// svgColorLevels is how many steps each channel is rounded to before a run is
// cut. 24 is far below what an eye resolves on a hairline stroke and far above
// what it takes for the runs to be long.
const svgColorLevels = 24

func quantHex(c [3]float32) string {
	q := func(v float32) int {
		n := int(clamp01f(v)*float32(svgColorLevels-1) + 0.5)
		return n * 255 / (svgColorLevels - 1)
	}
	return fmt.Sprintf("#%02x%02x%02x", q(c[0]), q(c[1]), q(c[2]))
}

// svgAxisGradient writes a <linearGradient> that reproduces the palette
// exactly, and reports whether it could.
//
// The trick is that this projection is ORTHOGRAPHIC and the gradient is a
// function of one model axis. An orthographic projection is linear, so a model
// coordinate maps to a linear function of screen position: the set of points
// sharing a value of the source axis is a straight line on screen, and those
// lines are parallel. That is exactly what an SVG linear gradient is. One
// polyline stroked with it is the whole trail, correctly colored, instead of
// thousands of separately stroked runs.
//
// It fails in one case and says so: looking straight down the source axis. Then
// the axis projects to a point, every color lands on top of every other, and no
// screen-space gradient can express it — the information went into depth, which
// a flat picture does not have. writeTrail's run-length path handles those.
func svgAxisGradient(b *strings.Builder, id string, a [3]float64, r, midX, midY float64, g rasterview.Gradient, min, max [3]float32) bool {
	if g.Source == rasterview.SourceTrail {
		// Along the trail is not a direction on screen. The path wanders; a
		// linear gradient is a straight ramp across the picture, and the two
		// only coincide for a straight trajectory. writeTrail encodes it, and
		// encodes it WELL, because this parameter is monotonic.
		return false
	}
	axis := [3]float64{}
	axis[min3Index(g.Source)] = 1
	ex, ey := project(axis, a)
	cx, cy := float64(renderW)/2, float64(renderH)/2
	// The endpoints of the gradient: where the low and high ends of the source
	// axis land, carried along the axis's own projected direction.
	lo, hi := float64(min[min3Index(g.Source)]), float64(max[min3Index(g.Source)])
	x1, y1 := cx+(ex*lo-midX)*r, cy-(ey*lo-midY)*r
	x2, y2 := cx+(ex*hi-midX)*r, cy-(ey*hi-midY)*r
	if math.Hypot(x2-x1, y2-y1) < 1 {
		return false // the axis is pointing at the camera
	}
	fmt.Fprintf(b, `<linearGradient id="%s" gradientUnits="userSpaceOnUse" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f">`,
		id, x1, y1, x2, y2)
	// 64 stops: the rainbow is the fussiest palette and a 64-step hue ramp is
	// smooth at any size this is drawn at, while the stops cost 64 short tags
	// against the thousands of polylines they replace.
	const stops = 64
	for i := 0; i <= stops; i++ {
		t := float64(i) / stops
		var p [3]float64
		p[min3Index(g.Source)] = lo + (hi-lo)*t
		c := g.ColorAt(float32(p[0]), float32(p[1]), float32(p[2]), min, max, float32(t))
		fmt.Fprintf(b, `<stop offset="%.4g" stop-color="%s"/>`, t, quantHex(c))
	}
	b.WriteString(`</linearGradient>`)
	return true
}

// min3Index maps Gradient.Source to an axis index, with the same default
// rasterview uses: anything not 0 or 1 is Z.
func min3Index(source int) int {
	if source == 0 || source == 1 {
		return source
	}
	return 2
}

// writeColoredTrail emits the trail into body, preferring one polyline stroked
// with a gradient in defs and falling back to per-color runs.
func writeColoredTrail(defs, body *strings.Builder, id string, pts [][3]float64, a [3]float64, r, mx, my float64, g rasterview.Gradient, min, max [3]float32, ageOff, ageTotal int) {
	if len(pts) < 2 {
		return
	}
	if svgAxisGradient(defs, id, a, r, mx, my, g, min, max) {
		body.WriteString(svgStroke)
		fmt.Fprintf(body, ` stroke="url(#%s)" points="`, id)
		writePoints(body, pts, a, r, mx, my)
		body.WriteString(`"/>`)
		return
	}
	writeTrail(body, pts, a, r, mx, my, g, min, max, ageOff, ageTotal)
}
