//go:build !js

package server

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/0magnet/chaosrack/pkg/acoustics"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

func testSignalLR(t *testing.T, name string, secs float64) (l, r []float32) {
	t.Helper()
	sig, err := testSignal(name)
	if err != nil {
		t.Fatal(err)
	}
	n := int(secs * renderSampleRate)
	l, r = make([]float32, n), make([]float32, n)
	audiosrc.NewTestSource(sig, renderSampleRate).Fill(l, r)
	return l, r
}

// A pure tone embedded at a quarter-period delay is a circle: every point is
// the same distance from the axis the tone's delay vector turns about. The
// measured τ has to find that quarter period, or the circle collapses to the
// line a half-period delay draws.
func TestTakensOfAToneIsAClosedCurve(t *testing.T) {
	l, r := testSignalLR(t, "1k", 0.5)
	tau, ok := attractor.MeasureTau(l, r, renderSampleRate)
	if !ok {
		t.Fatal("no τ measured for a pure tone")
	}
	if quarter := float32(renderSampleRate / 1000 / 4); math.Abs(float64(tau-quarter)) > 2 {
		t.Errorf("τ = %g samples, want about a quarter period (%g)", tau, quarter)
	}
	f, ok := attractor.AudioFigure("takens", l, r, len(l), attractor.AudioOptions{SampleRate: renderSampleRate, Tau: tau})
	if !ok {
		t.Fatal("no figure")
	}
	dx, dy, dz := attractor.Extent(f.Points)
	if dx < 0.5 || dy < 0.5 || dz < 0.5 {
		t.Errorf("extent %.3f x %.3f x %.3f: the tone did not open into a curve", dx, dy, dz)
	}
}

// A left-only signal has a silent right channel, so the stereo embedding's
// second axis is zero everywhere.
func TestStereoOfLeftOnlyIsFlatInR(t *testing.T) {
	l, r := testSignalLR(t, "L", 0.5)
	f, ok := attractor.AudioFigure("stereo", l, r, len(l), attractor.AudioOptions{SampleRate: renderSampleRate})
	if !ok {
		t.Fatal("no figure")
	}
	dx, dy, _ := attractor.Extent(f.Points)
	if dx == 0 || dy != 0 {
		t.Errorf("extent L %.3f, R %.3f: want motion in L only", dx, dy)
	}
}

func TestAudioModelsWrite(t *testing.T) {
	old := [4]int{renderW, renderH, renderFrames, renderFPS}
	renderW, renderH, renderFrames, renderFPS = 96, 64, 0, 4
	oldSig, oldSecs, oldOut, oldModel := renderSignal, renderAudioSecs, renderOut, renderModel
	t.Cleanup(func() {
		renderW, renderH, renderFrames, renderFPS = old[0], old[1], old[2], old[3]
		renderSignal, renderAudioSecs, renderOut, renderModel = oldSig, oldSecs, oldOut, oldModel
	})
	dir := t.TempDir()
	renderSignal, renderAudioSecs = "pink", 1.5
	for _, m := range audioInputModels() {
		for _, ext := range []string{".png", ".gif"} {
			renderModel, renderOut = m, filepath.Join(dir, m+ext)
			renderFrames = 0
			if m == "waterfall" && ext == ".gif" {
				renderFrames = 3 // one surface: only a turning camera animates it
			}
			if err := renderAudio(renderCmd); err != nil {
				t.Errorf("%s%s: %v", m, ext, err)
			}
		}
	}
}

// Out-of-polarity noise is the same signal inverted on one channel, so the
// transfer function between them is unity gain at 180 degrees.
func TestTransferOfOutOfPolarity(t *testing.T) {
	l, r := testSignalLR(t, "oop", 2)
	var acc acoustics.TransferAccum
	for at := 0; at+xferFFT <= len(l); at += xferFFT / 2 {
		acc.Add(l[at:at+xferFFT], r[at:at+xferFFT], 0)
	}
	res := acc.Result(renderSampleRate, 6)
	if !res.OK {
		t.Fatal("no result")
	}
	for i, b := range res.Bands {
		if b.Center < 100 || b.Center > 10000 {
			continue
		}
		if math.Abs(res.MagDB[i]) > 0.5 || math.Abs(math.Abs(res.PhaseDeg[i])-180) > 2 {
			t.Errorf("%.0f Hz: %.2f dB, %.1f°; want 0 dB, 180°", b.Center, res.MagDB[i], res.PhaseDeg[i])
		}
	}
}
