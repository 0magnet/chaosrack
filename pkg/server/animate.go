//go:build !js

package server

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/gifenc"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// A model running, written to a file, with no browser anywhere.
//
// The still picture already went through pkg/rasterview and pkg/gifenc was
// already here with an adaptive palette and a tested encoder, written for the
// uitool GIFs that are captured through CDP. Neither knew about the other: the
// frames the encoder wanted were being screenshotted out of a browser that was
// drawing the same trajectory this process can integrate itself.
//
// WHAT MOVES IS THE TRAJECTORY. The first version of this turned the camera
// around a finished attractor, which is a 3-D portrait of a system at rest: it
// shows the shape and says nothing whatever about the dynamics, and a model
// that is not moving is the one thing an attractor is not. A trail sweeps
// along the path instead, the way the rack draws it on screen — a length of
// recent history that advances — so the picture is the system evolving.
//
// The camera can turn as well, and does not by default. A turn on top of the
// run reads as a second motion competing with the one that matters, and at any
// speed fast enough to complete in a few seconds it is the only motion the eye
// picks up. --turn asks for it.

// frameViews are the angles for each frame: the still picture's angles, plus a
// share of --turn, which is zero unless asked for.
//
// The last frame is deliberately NOT the first one again. A loop of n frames
// covering a full turn has to advance by turn/n each time, so that frame n-1
// is one step short of the start; stepping by turn/(n-1) would make the last
// frame identical to the first and the loop stutter on one repeated image.
func frameViews(n int) [][3]float64 {
	base := [3]float64{}
	copy(base[:], renderSpin)
	turn := [3]float64{}
	copy(turn[:], renderTurn)
	out := make([][3]float64, n)
	for i := range out {
		f := float64(i) / float64(n)
		out[i] = [3]float64{base[0] + turn[0]*f, base[1] + turn[1]*f, base[2] + turn[2]*f}
	}
	return out
}

// frameSpan is the slice of the trajectory visible in frame i of n: a trailing
// window whose end sweeps the whole run.
//
// A WINDOW, not the whole path so far. Showing everything up to the moment
// draws the attractor filling in and then sitting there, which is the portrait
// again with a slow start. A fixed length of recent history that moves is what
// the rack shows on screen and what makes the motion legible — the trail is
// the system's recent past and it is always the same age.
//
// The window is renderTrail points, defaulting to a quarter of the run. The
// end advances from the window's own length to the end of the path, so frame 0
// is a full trail rather than an empty frame and the last frame ends on the
// last point integrated.
func frameSpan(total, i, n int) (lo, hi int) {
	win := renderTrail
	if win <= 0 {
		win = total / 4
	}
	if win < 2 {
		win = 2
	}
	if win > total {
		win = total
	}
	if n < 2 {
		return total - win, total
	}
	hi = win + (total-win)*i/(n-1)
	return hi - win, hi
}

// writeAnimation writes an animated GIF or SVG of the model running.
func writeAnimation(name string, pts [][3]float64) error {
	views := frameViews(renderFrames)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".gif":
		return writeGIF(name, pts, views)
	case ".svg":
		// 0600 for the same reason the still path uses it: a tool should not
		// widen permissions on the user's behalf.
		return os.WriteFile(name, []byte(animatedSVG(pts, views)), 0o600)
	default:
		return fmt.Errorf("%s: --frames writes .gif or .svg", filepath.Ext(name))
	}
}

func writeGIF(name string, pts [][3]float64, views [][3]float64) error {
	frames := make([]*image.RGBA, 0, len(views))
	for i, a := range views {
		o := drawOptions()
		o.View.AngleX, o.View.AngleY, o.View.AngleZ = a[0], a[1], a[2]
		lo, hi := frameSpan(len(pts), i, len(views))
		frames = append(frames, attractor.DrawSpan(pts, lo, hi, o))
	}
	f, err := os.Create(name) //nolint:gosec // the path is the user's own argument
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the encode error below is the one that matters
	// GIF delays are hundredths of a second, and the format cannot express a
	// rate that is not one: 20 fps is 5, 30 fps is 3.33 and becomes 3, which
	// is 33 fps. Rounding is the honest thing the format allows.
	delay := max(int(math.Round(100/float64(renderFPS))), 1)
	return gifenc.EncodeRGBA(f, frames, delay)
}

// animatedSVG writes every frame as its own polyline and shows one at a time.
//
// A flipbook, and it has to be one. SVG can animate a transform cheaply, but
// no transform expresses what is happening here: the trail advances ALONG the
// path, so each frame is a different set of points and not the same points
// moved. Each frame carries its own window — a quarter of the run by default,
// so the file is roughly the run's points over four, times the frame count.
//
// The fit comes from svgFit over the whole run, so the path does not lurch or
// breathe under the trail; the raster path holds still for the same reason,
// by handing Render every vertex and varying only the indices.
//
// calcMode="discrete" because a frame is either showing or it is not:
// interpolated opacity would cross-fade consecutive views into a blur.
func animatedSVG(pts [][3]float64, views [][3]float64) string {
	if len(pts) == 0 || len(views) == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg"/>`
	}
	dur := float64(len(views)) / float64(renderFPS)
	pts = attractor.Centered(pts)
	r, mx, my := svgFit(pts, views)
	g := gradientFor()
	min, max := rasterview.ModelBounds(attractor.Vertices(pts))
	// One gradient serves every frame when the camera does not move, which is
	// the default: the palette is fixed to the model's axes, so it only has to
	// be re-derived when the view does.
	still := true
	for _, v := range views[1:] {
		if v != views[0] {
			still = false
			break
		}
	}
	var defs, body strings.Builder
	for i, a := range views {
		// values is this frame's slot lit and every other dark, which is one
		// character per frame per frame — a few kilobytes at any frame count
		// worth using, against the points themselves which are the real size.
		vals := make([]string, len(views))
		for k := range vals {
			vals[k] = "0"
		}
		vals[i] = "1"
		// A GROUP per frame, not a polyline. A trail colored by the gradient is
		// many polylines — one per run of constant color — and they have to
		// appear and vanish together, so the animation goes on the thing that
		// owns all of them.
		fmt.Fprintf(&body, `<g opacity="0"><animate attributeName="opacity" calcMode="discrete" dur="%gs" repeatCount="indefinite" values="%s"/>`,
			dur, strings.Join(vals, ";"))
		lo, hi := frameSpan(len(pts), i, len(views))
		id := fmt.Sprintf("g%d", i)
		into := &defs
		if still {
			id = "g0"
			if i > 0 {
				// Already written; hand the writer somewhere to throw away the
				// duplicate rather than emitting the same stops every frame.
				into = &strings.Builder{}
			}
		}
		writeColoredTrail(into, &body, id, pts[lo:hi], a, r, mx, my, g, min, max, lo, len(pts))
		body.WriteString(`</g>`)
	}

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
