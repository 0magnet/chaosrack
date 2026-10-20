package subcmds

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// --walk steps the palette through the animation in equal parts, and a still
// takes its first step.
func TestAWalkStepsThePaletteInEqualParts(t *testing.T) {
	old := renderWalk
	t.Cleanup(func() { renderWalk = old })
	renderWalk = "4/z,2/x,3/y,1/trail"
	want := []struct{ colors, source int }{{4, 2}, {2, 0}, {3, 1}, {1, rasterview.SourceTrail}}
	for i := range 48 {
		g, w := gradientAt(i, 48), want[i/12]
		if g.Colors != w.colors || g.Source != w.source {
			t.Fatalf("frame %d: colors %d source %d, want %d and %d", i, g.Colors, g.Source, w.colors, w.source)
		}
	}
	if g := gradientFor(); g.Colors != 4 || g.Source != 2 {
		t.Errorf("a still takes %d/%d, want the first step", g.Colors, g.Source)
	}
	for _, bad := range []string{"5/z", "4/q", "4", "x/z"} {
		if _, err := parseWalk(bad); err == nil {
			t.Errorf("--walk %q was accepted", bad)
		}
	}
}

// --rock swings the turn out and back about the pose, so the loop has no
// seam: no step between frames, the wrap included, is bigger than the
// largest step inside it.
func TestARockedTurnLoopsWithoutASeam(t *testing.T) {
	withRenderFlags(t, 48, 16, func() {
		old := renderRock
		t.Cleanup(func() { renderRock = old })
		renderRock = true
		renderTurn = []float64{0, math.Pi / 2, 0}
		v := frameViews(48)
		if math.Abs(v[0][1]-renderSpin[1]) > 1e-9 {
			t.Errorf("the loop starts at %v, not at the pose %v", v[0][1], renderSpin[1])
		}
		lo, hi, step := math.Inf(1), math.Inf(-1), 0.0
		for i := range v {
			lo, hi = min(lo, v[i][1]), max(hi, v[i][1])
			step = max(step, math.Abs(v[(i+1)%len(v)][1]-v[i][1]))
		}
		if math.Abs(hi-lo-math.Pi/2) > 0.01 {
			t.Errorf("it swings %v, want π/2", hi-lo)
		}
		if step > 2*math.Pi/2*math.Pi/48 {
			t.Errorf("a step of %v between frames", step)
		}
	})
}

// --sweep=false draws the whole figure in every frame.
func TestWithoutASweepEveryFrameIsTheWholeFigure(t *testing.T) {
	old := renderSweep
	t.Cleanup(func() { renderSweep = old })
	renderSweep = false
	for i := range 10 {
		if lo, hi := frameSpan(1000, i, 10); lo != 0 || hi != 1000 {
			t.Fatalf("frame %d draws %d..%d", i, lo, hi)
		}
	}
}

// --crop frames the lit part: a small mark in a corner of a big drawing
// fills the cropped picture.
func TestACropFillsThePictureWithWhatIsDrawn(t *testing.T) {
	oc, ow, oh, ob := renderCrop, renderW, renderH, renderBG
	t.Cleanup(func() { renderCrop, renderW, renderH, renderBG = oc, ow, oh, ob })
	renderCrop, renderW, renderH, renderBG = true, 50, 50, "#000000"
	m := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 10; y < 20; y++ {
		for x := 10; x < 20; x++ {
			m.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	out := cropOne(m)
	if b := out.Bounds(); b.Dx() != 50 || b.Dy() != 50 {
		t.Fatalf("cropped to %v", b)
	}
	lit := 0
	for y := range 50 {
		for x := range 50 {
			if out.RGBAAt(x, y).R > 128 {
				lit++
			}
		}
	}
	// The 10×10 mark, a tenth of margin aside, is most of the frame.
	if lit < 50*50/2 {
		t.Errorf("only %d of %d pixels are the mark", lit, 50*50)
	}
}

// A moving trail's loop has no seam: the continuation drawn past the end is
// blended into the first frames, so the first frame is mostly what follows
// the last, and the frame the blend ends on is mostly its own.
func TestALoopsSeamIsCrossfaded(t *testing.T) {
	old := renderSweep
	t.Cleanup(func() { renderSweep = old })
	renderSweep = true
	// Each frame a flat gray of its position along the run, so a blend is
	// readable as a number.
	frame := func(i, _ int) *image.RGBA {
		m := image.NewRGBA(image.Rect(0, 0, 1, 1))
		m.SetRGBA(0, 0, color.RGBA{uint8(i), 0, 0, 255}) //nolint:gosec // small
		return m
	}
	f := loopFrames(48, frame)
	k := seamFrames(48)
	if got := int(f[0].Pix[0]); math.Abs(float64(got-48)) > 48.0/float64(k+1)+1 {
		t.Errorf("the first frame reads %d, want close to 48, the frame after the last", got)
	}
	if got := int(f[k].Pix[0]); got != k {
		t.Errorf("frame %d, past the blend, reads %d", k, got)
	}
	renderSweep = false
	if seamFrames(48) != 0 {
		t.Error("a still trail has a seam to blend")
	}
}

// --live draws each frame as the page does: the next full stretch of the run,
// the first from its start and the last to its end.
func TestLiveFramesAreSuccessiveStretches(t *testing.T) {
	ol, ot := renderLive, renderTrail
	t.Cleanup(func() { renderLive, renderTrail = ol, ot })
	renderLive, renderTrail = true, 100
	const n = 10
	prev := -1
	for i := range n {
		lo, hi := frameSpan(100*n, i, n)
		if hi-lo != 100 {
			t.Fatalf("frame %d draws %d points, want the trail's 100", i, hi-lo)
		}
		if lo <= prev {
			t.Fatalf("frame %d starts at %d, not after frame %d's %d", i, lo, i-1, prev)
		}
		prev = lo
	}
	if lo, _ := frameSpan(100*n, 0, n); lo != 0 {
		t.Errorf("the first stretch starts at %d", lo)
	}
	if _, hi := frameSpan(100*n, n-1, n); hi != 100*n {
		t.Errorf("the last stretch ends at %d", hi)
	}
}
