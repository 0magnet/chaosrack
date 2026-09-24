//go:build !js

package server

import (
	"bytes"
	"encoding/xml"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The animation is checked by decoding what was written, not by looking at it.
// A GIF that is one frame repeated and a GIF that turns are the same size and
// the same colors; the difference is whether consecutive frames differ, which
// is a question a test can ask and an eye is bad at.

// withRenderFlags runs f with the render command's package-level flags set to
// something small and puts them back afterwards. They are globals because
// cobra binds them; a test that changed them and left them changed would move
// the next test's picture.
func withRenderFlags(t *testing.T, frames, fps int, f func()) {
	t.Helper()
	oW, oH, oF, oFPS, oT, oP, oS, oTr := renderW, renderH, renderFrames, renderFPS, renderTurn, renderPts, renderSpin, renderTrail
	t.Cleanup(func() {
		renderW, renderH, renderFrames, renderFPS, renderTurn, renderPts, renderSpin, renderTrail = oW, oH, oF, oFPS, oT, oP, oS, oTr
	})
	renderW, renderH = 120, 120
	renderFrames, renderFPS = frames, fps
	renderTrail = 0
	renderTurn = []float64{0, 0, 0} // the default: the trail moves, the camera does not
	renderSpin = []float64{0.6, 0.9, 0}
	renderPts = 400
	f()
}

func lorenzPath(t *testing.T) [][3]float64 {
	t.Helper()
	p := dynamics.Trajectory("lorenz", dynamics.TrajectoryOptions{Transient: 10, Duration: 60, MaxPoints: 400})
	if len(p) == 0 {
		t.Fatal("lorenz integrated to nothing")
	}
	return p
}

func TestAnimatedGIFAdvancesAndLoops(t *testing.T) {
	withRenderFlags(t, 12, 20, func() {
		out := filepath.Join(t.TempDir(), "a.gif")
		if err := writeAnimation(out, lorenzPath(t)); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out) //nolint:gosec // a path this test just made
		if err != nil {
			t.Fatal(err)
		}
		g, err := gif.DecodeAll(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("what was written is not a GIF: %v", err)
		}
		if len(g.Image) != 12 {
			t.Errorf("wrote %d frames, asked for 12", len(g.Image))
		}
		if g.LoopCount != 0 {
			t.Errorf("LoopCount %d; 0 is loop forever, which is what a running model wants", g.LoopCount)
		}
		// 20 fps is 5 hundredths. The format cannot express a rate that is not
		// a whole number of them, so this is the rate asked for exactly.
		for i, d := range g.Delay {
			if d != 5 {
				t.Errorf("frame %d delay %dcs, want 5 for 20fps", i, d)
			}
		}
		// A STILL REPEATED TWELVE TIMES IS ALSO TWELVE FRAMES. This is the
		// check that the trail actually advanced between them.
		for i := 1; i < len(g.Image); i++ {
			if bytes.Equal(g.Image[i-1].Pix, g.Image[i].Pix) {
				t.Errorf("frames %d and %d are identical — the trail is not advancing", i-1, i)
			}
		}
		// And the cycle must not end where it began, or the loop shows the
		// same picture twice running at the seam.
		if bytes.Equal(g.Image[0].Pix, g.Image[len(g.Image)-1].Pix) {
			t.Error("the last frame is the first frame; a full turn over n frames steps by turn/n, not turn/(n-1)")
		}
	})
}

func TestAnimatedSVGIsWellFormedAndShowsOneFrameAtATime(t *testing.T) {
	withRenderFlags(t, 8, 25, func() {
		doc := animatedSVG(lorenzPath(t), frameViews(renderFrames))
		if err := xml.Unmarshal([]byte(doc), new(struct {
			XMLName xml.Name
		})); err != nil {
			t.Fatalf("the SVG does not parse: %v", err)
		}
		if n := strings.Count(doc, "<polyline"); n != 8 {
			t.Errorf("%d polylines, want one per frame (8)", n)
		}
		// Each frame is lit for exactly one slot of the cycle. Two lit slots
		// would draw two views at once; none would drop a frame.
		for _, vals := range valuesAttrs(doc) {
			if got := strings.Count(vals, "1"); got != 1 {
				t.Errorf("a frame is lit in %d slots of the cycle, want 1 (%q)", got, vals)
			}
			if n := len(strings.Split(vals, ";")); n != 8 {
				t.Errorf("the cycle has %d slots, want one per frame (8)", n)
			}
		}
		// 8 frames at 25fps is 0.32s.
		if !strings.Contains(doc, `dur="0.32s"`) {
			t.Error("the cycle is not frames/fps seconds long")
		}
	})
}

func valuesAttrs(doc string) []string {
	var out []string
	for _, part := range strings.Split(doc, `values="`)[1:] {
		if before, _, ok := strings.Cut(part, "\""); ok {
			out = append(out, before)
		}
	}
	return out
}

// THE PATH MUST HOLD STILL WHILE THE TRAIL MOVES ALONG IT.
//
// The trail is a window that advances, so consecutive frames overlap: the
// points at the front of one frame are at the back of the next. Those shared
// points have to land on exactly the same pixels, or the trajectory lurches
// underneath the trail and the animation reads as a shape being shaken rather
// than a system running. Fitting or centering per frame is what breaks it,
// which is why svgFit is measured once over the whole run.
//
// With --turn at its default of no rotation the camera is identical in every
// frame, so any movement at all is the fit moving.
func TestThePathHoldsStillWhileTheTrailMoves(t *testing.T) {
	withRenderFlags(t, 12, 20, func() {
		cpts := attractor.Centered(lorenzPath(t))
		views := frameViews(renderFrames)
		r, mx, my := svgFit(cpts, views)

		coords := func(i int) (lo, hi int, xy []string) {
			lo, hi = frameSpan(len(cpts), i, renderFrames)
			var b strings.Builder
			writePoints(&b, cpts[lo:hi], views[i], r, mx, my)
			return lo, hi, strings.Fields(b.String())
		}
		moved, shared := 0, 0
		for i := 1; i < renderFrames; i++ {
			pLo, pHi, prev := coords(i - 1)
			cLo, cHi, cur := coords(i)
			// The overlap in trajectory indices, expressed in each frame's own
			// coordinate list.
			from, to := max(pLo, cLo), min(pHi, cHi)
			for k := from; k < to; k++ {
				a, b := prev[k-pLo], cur[k-cLo]
				shared++
				if a != b {
					moved++
					if moved < 3 {
						t.Errorf("point %d is at %s in frame %d and %s in frame %d — the path moved under the trail",
							k, a, i-1, b, i)
					}
				}
			}
		}
		if shared == 0 {
			t.Fatal("no frames overlap; the trail is not a sliding window")
		}
		if moved > 0 {
			t.Errorf("%d of %d shared points moved between frames", moved, shared)
		}
	})
}

// The trail has to actually go somewhere: every frame a different stretch of
// the path, ending on the last point integrated.
func TestTheTrailSweepsTheWholeRun(t *testing.T) {
	withRenderFlags(t, 10, 20, func() {
		const total = 1000
		seen := map[[2]int]bool{}
		var lastHi int
		for i := 0; i < renderFrames; i++ {
			lo, hi := frameSpan(total, i, renderFrames)
			if lo < 0 || hi > total || hi-lo < 2 {
				t.Fatalf("frame %d spans %d..%d, which is not inside 0..%d", i, lo, hi, total)
			}
			if i > 0 && hi <= lastHi {
				t.Errorf("frame %d ends at %d, no later than frame %d at %d — the trail is not advancing", i, hi, i-1, lastHi)
			}
			lastHi = hi
			seen[[2]int{lo, hi}] = true
		}
		if len(seen) != renderFrames {
			t.Errorf("%d distinct windows over %d frames; some frames are the same picture", len(seen), renderFrames)
		}
		if lastHi != total {
			t.Errorf("the last frame ends at %d, not at the end of the run (%d)", lastHi, total)
		}
		// Frame 0 is a full trail, not an empty frame building up.
		if lo, hi := frameSpan(total, 0, renderFrames); hi-lo != total/4 {
			t.Errorf("the first frame shows %d points, want a full trail of %d", hi-lo, total/4)
		}
	})
}

// THE DEFAULT MUST NOT DECIMATE. Trajectory thins a run to the point budget by
// keeping every Nth step, and the picture is then drawn as chords across N
// integration steps — visibly polygonal at the turns, and indistinguishable
// from a worse renderer if you do not know it is happening. It WAS happening:
// the default 220 model-seconds of Lorenz at dt=0.005 is 44,000 steps inside
// 20,000 points, one point kept in three.
func TestTheDefaultRunIsDrawnAtFullResolution(t *testing.T) {
	old := struct {
		secs float64
		pts  int
	}{renderSecs, renderPts}
	t.Cleanup(func() { renderSecs, renderPts = old.secs, old.pts })
	renderSecs, renderPts = 0, 5000

	for _, mode := range []string{"lorenz", "rossler", "thomas", "halvorsen"} {
		got := len(trajectoryFor(mode))
		// One point per step means the budget is filled exactly. A point or
		// two either way is the loop's rounding, not decimation.
		if got < renderPts-2 {
			dt, _, _ := dynamics.FlowFor(mode)
			t.Errorf("%s: %d points for a budget of %d at dt=%g — the run is being thinned",
				mode, got, renderPts, dt)
		}
	}
}

// Asking for a longer run than the budget holds still works; it is only the
// silence about it that was wrong.
func TestALongerRunStillDrawsAndIsStillCapped(t *testing.T) {
	old := struct {
		secs float64
		pts  int
	}{renderSecs, renderPts}
	t.Cleanup(func() { renderSecs, renderPts = old.secs, old.pts })
	renderSecs, renderPts = 400, 3000

	got := len(trajectoryFor("lorenz"))
	if got == 0 {
		t.Fatal("a long run drew nothing")
	}
	if got > renderPts {
		t.Errorf("%d points for a budget of %d; MaxPoints is not a maximum", got, renderPts)
	}
}

// ONE FLAG, EVERY WRITER. --gradient reached the SVG and not the PNG, because
// drawOptions built its own palette instead of asking for the shared one; the
// same model exported twice was colored two different ways. Before that, the
// SVG had a hardcoded stroke and ignored --colors as well.
func TestEveryWriterUsesTheSamePalette(t *testing.T) {
	old := struct {
		grad   string
		colors int
	}{renderGrad, renderColors}
	t.Cleanup(func() { renderGrad, renderColors = old.grad, old.colors })

	for _, c := range []struct {
		flag string
		want int
	}{
		{"x", 0}, {"y", 1}, {"z", 2}, {"trail", rasterview.SourceTrail},
	} {
		renderGrad = c.flag
		if got := gradientFor().Source; got != c.want {
			t.Errorf("--gradient %s gives source %d, want %d", c.flag, got, c.want)
		}
		if got := drawOptions().Gradient.Source; got != c.want {
			t.Errorf("--gradient %s: the raster path uses source %d, want %d — it is not reading gradientFor", c.flag, got, c.want)
		}
	}
	renderGrad, renderColors = "z", 3
	if drawOptions().Gradient != gradientFor() {
		t.Error("the raster path's palette is not the shared one")
	}
}

// The trail source has to be the one that run-length encodes: it is the only
// parameter that moves monotonically along the path, which is what makes an
// SVG of it a few hundred runs instead of one per segment.
func TestTheTrailPaletteIsMonotonicAlongThePath(t *testing.T) {
	g := rasterview.DefaultGradient()
	g.Source = rasterview.SourceTrail
	g.Colors = 4
	min, max := [3]float32{0, 0, 0}, [3]float32{1, 1, 1}
	// Same point, different ages: the color must follow the age.
	a := g.ColorAt(0.5, 0.5, 0.5, min, max, 0)
	b := g.ColorAt(0.5, 0.5, 0.5, min, max, 1)
	if a == b {
		t.Error("the trail palette gives one color for the ends of the path; it is not reading the age")
	}
	// A model axis must NOT follow the age.
	g.Source = 2
	if g.ColorAt(0.5, 0.5, 0.5, min, max, 0) != g.ColorAt(0.5, 0.5, 0.5, min, max, 1) {
		t.Error("a model-axis palette changed with the age; the age leaked into the wrong source")
	}
}
