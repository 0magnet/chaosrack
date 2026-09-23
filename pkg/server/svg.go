//go:build !js

package server

import (
	"fmt"
	"math"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
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
	// Same fit as the raster path, for the same reason: an attractor is not
	// modeled around its own middle, so it has to be moved there.
	pts = attractor.Centered(pts)
	a := [3]float64{}
	copy(a[:], renderSpin)
	sinX, cosX := math.Sin(a[0]), math.Cos(a[0])
	sinY, cosY := math.Sin(a[1]), math.Cos(a[1])
	sinZ, cosZ := math.Sin(a[2]), math.Cos(a[2])

	// Rz, then Ry, then Rx — the same order rasterview applies, so the SVG and
	// the PNG show the same view from the same angles.
	proj := make([][2]float64, len(pts))
	maxLen := 0.0
	for i, p := range pts {
		x1, y1 := p[0]*cosZ-p[1]*sinZ, p[0]*sinZ+p[1]*cosZ
		x2, z2 := x1*cosY+p[2]*sinY, -x1*sinY+p[2]*cosY
		y3 := y1*cosX - z2*sinX
		proj[i] = [2]float64{x2, y3}
		if l := math.Hypot(math.Hypot(p[0], p[1]), p[2]); l > maxLen {
			maxLen = l
		}
	}
	if maxLen == 0 {
		maxLen = 1
	}
	w, h := float64(renderW), float64(renderH)
	cx, cy := w/2, h/2
	r := math.Min(cx, cy) * 0.85 / maxLen

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		renderW, renderH, renderW, renderH)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#0a0d14"/>`, renderW, renderH)
	b.WriteString(`<polyline fill="none" stroke="#4ad8ff" stroke-width="0.6" stroke-opacity="0.85" points="`)
	for i, p := range proj {
		if i > 0 {
			b.WriteByte(' ')
		}
		// Two decimals: at these sizes it is under a hundredth of a pixel, and
		// full precision triples the file for a difference nothing can show.
		fmt.Fprintf(&b, "%.2f,%.2f", cx+p[0]*r, cy-p[1]*r)
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}
