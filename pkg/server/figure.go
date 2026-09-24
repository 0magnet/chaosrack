//go:build !js

package server

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/gifenc"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The models render draws that are not flows: a map's iterates as points,
// and wireframes as their edges. A flow is still a path, and still goes
// through trajectoryFor and the path writers in svg.go and animate.go; this
// file only adds the two other ways a figure is joined.

// figureFor builds whatever a model draws, or says why it cannot.
func figureFor(model string) (attractor.Figure, error) {
	if dynamics.HasFlow(model) {
		return attractor.Figure{Kind: attractor.FigurePath, Points: trajectoryFor(model)}, nil
	}
	if f, ok := attractor.StaticFigure(model, renderPts); ok {
		return f, nil
	}
	return attractor.Figure{}, fmt.Errorf("no model named %q that can be drawn without a browser; chaosrack models lists them", model)
}

// drawableKeys is every model render can draw, in catalog order, then any
// registered flow the catalog does not list.
func drawableKeys() []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range attractor.CatalogKeys() {
		if attractor.Drawable(k) {
			out = append(out, k)
			seen[k] = true
		}
	}
	for _, k := range dynamics.Keys() {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// writeFigure writes a still of any figure.
func writeFigure(name string, f attractor.Figure) error {
	if f.Kind == attractor.FigurePath {
		return writeModel(name, f.Points)
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".svg":
		return os.WriteFile(name, []byte(svgFrames(f, frameViews(1))), 0o600)
	case ".png", "":
		file, err := os.Create(name) //nolint:gosec // the path is the user's own argument
		if err != nil {
			return err
		}
		defer file.Close() //nolint:errcheck // the encode error below is the one that matters
		return png.Encode(file, attractor.DrawFigure(f, 0, len(f.Points), drawOptions()))
	default:
		return fmt.Errorf("%s: unknown format — use .png or .svg", filepath.Ext(name))
	}
}

// writeFigureAnimation writes an animation of any figure. The camera turns
// as it does for a flow; a map's points also accumulate over the run, the
// way the page fills one in, while a wireframe is whole in every frame.
func writeFigureAnimation(name string, f attractor.Figure) error {
	if f.Kind == attractor.FigurePath {
		return writeAnimation(name, f.Points)
	}
	views := frameViews(renderFrames)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".gif":
		frames := make([]*image.RGBA, 0, len(views))
		for i, a := range views {
			o := drawOptions()
			o.View.AngleX, o.View.AngleY, o.View.AngleZ = a[0], a[1], a[2]
			frames = append(frames, attractor.DrawFigure(f, 0, figureReveal(f, i, len(views)), o))
		}
		file, err := os.Create(name) //nolint:gosec // the path is the user's own argument
		if err != nil {
			return err
		}
		defer file.Close() //nolint:errcheck // the encode error below is the one that matters
		delay := int(math.Round(100 / float64(renderFPS)))
		if delay < 1 {
			delay = 1
		}
		return gifenc.EncodeRGBA(file, frames, delay)
	case ".svg":
		return os.WriteFile(name, []byte(svgFrames(f, views)), 0o600)
	default:
		return fmt.Errorf("%s: --frames writes .gif or .svg", filepath.Ext(name))
	}
}

// figureReveal is how many of a figure's points frame i of n shows.
func figureReveal(f attractor.Figure, i, n int) int {
	if f.Kind != attractor.FigurePoints || n < 2 {
		return len(f.Points)
	}
	return len(f.Points) * (i + 1) / n
}

// svgFrames writes a points or lines figure as SVG: one frame is a still,
// more are shown one after another like animatedSVG's.
func svgFrames(f attractor.Figure, views [][3]float64) string {
	if len(f.Points) == 0 || len(views) == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg"/>`
	}
	pts := attractor.Centered(f.Points)
	r, mx, my := svgFit(pts, views)
	g := gradientFor()
	min, max := rasterview.ModelBounds(attractor.Vertices(pts))

	var body strings.Builder
	dur := float64(len(views)) / float64(renderFPS)
	for i, a := range views {
		if len(views) > 1 {
			vals := make([]string, len(views))
			for k := range vals {
				vals[k] = "0"
			}
			vals[i] = "1"
			fmt.Fprintf(&body, `<g opacity="0"><animate attributeName="opacity" calcMode="discrete" dur="%gs" repeatCount="indefinite" values="%s"/>`,
				dur, strings.Join(vals, ";"))
		}
		writeSegments(&body, f, pts, figureReveal(f, i, len(views)), a, r, mx, my, g, min, max)
		if len(views) > 1 {
			body.WriteString(`</g>`)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		renderW, renderH, renderW, renderH)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0a0d14"/>`, renderW, renderH)
	b.WriteString(body.String())
	b.WriteString(`</svg>`)
	return b.String()
}

// writeSegments draws a figure's points (as dots) or edges (as lines), one
// <path> per quantized color so the file stays a manageable size. An edge
// takes the color at its midpoint.
func writeSegments(b *strings.Builder, f attractor.Figure, pts [][3]float64, hi int, a [3]float64, r, mx, my float64, g rasterview.Gradient, min, max [3]float32) {
	cx, cy := float64(renderW)/2, float64(renderH)/2
	at := func(p [3]float64) (float64, float64) {
		x, y := project(p, a)
		return cx + (x-mx)*r, cy - (y-my)*r
	}
	color := func(p [3]float64, i int) string {
		age := float32(0)
		if len(pts) > 1 {
			age = float32(i) / float32(len(pts)-1)
		}
		return quantHex(g.ColorAt(float32(p[0]), float32(p[1]), float32(p[2]), min, max, age))
	}
	paths := map[string]*strings.Builder{}
	add := func(hex, d string) {
		p, ok := paths[hex]
		if !ok {
			p = &strings.Builder{}
			paths[hex] = p
		}
		p.WriteString(d)
	}

	width := "0.6"
	if f.Kind == attractor.FigurePoints {
		width = "1.2"
		if hi > len(pts) {
			hi = len(pts)
		}
		for i, p := range pts[:hi] {
			x, y := at(p)
			add(color(p, i), fmt.Sprintf("M%.1f %.1fh0", x, y))
		}
	} else {
		for i := 0; i+1 < len(f.Edges); i += 2 {
			ia, ib := int(f.Edges[i]), int(f.Edges[i+1])
			if ia >= len(pts) || ib >= len(pts) {
				continue
			}
			p, q := pts[ia], pts[ib]
			mid := [3]float64{(p[0] + q[0]) / 2, (p[1] + q[1]) / 2, (p[2] + q[2]) / 2}
			x1, y1 := at(p)
			x2, y2 := at(q)
			add(color(mid, ia), fmt.Sprintf("M%.2f %.2fL%.2f %.2f", x1, y1, x2, y2))
		}
	}

	hexes := make([]string, 0, len(paths))
	for h := range paths {
		hexes = append(hexes, h)
	}
	sort.Strings(hexes)
	for _, h := range hexes {
		fmt.Fprintf(b, `<path fill="none" stroke="%s" stroke-width="%s" stroke-opacity="0.85" stroke-linecap="round" d="%s"/>`,
			h, width, paths[h].String())
	}
}
