package rackhost

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// path is n points along a line, a trail in order.
func path(n int) attractor.Figure {
	f := attractor.Figure{Kind: attractor.FigurePath}
	for k := range n {
		f.Points = append(f.Points, [3]float64{float64(k), 0, 0})
	}
	return f
}

// An embedding's trail is colored by the audio it was drawn from: a sweep
// from low to high colors the trail from one end of the ramp to the other,
// as the page's table does.
func TestTrailColoredByItsAudio(t *testing.T) {
	r := newRack(t)
	chirp := make([]float32, 8192)
	ph := 0.0
	for k := range chirp {
		f := 200 + 9000*float64(k)/float64(len(chirp))
		ph += 2 * math.Pi * f / sampleRate
		chirp[k] = float32(0.5 * math.Sin(ph))
	}
	r.mu.Lock()
	r.color.w = chirp
	g := rasterview.DefaultGradient()
	f := r.audioColored(path(200), &g, 4, 0)
	r.mu.Unlock()
	if g.Source != rasterview.SourceTrail {
		t.Fatalf("the gradient follows source %d, not the table", g.Source)
	}
	if lo, hi := f.Ages[0], f.Ages[len(f.Ages)-1]; hi-lo < 0.5 {
		t.Errorf("the trail runs %.2f to %.2f along a rising sweep; want most of the ramp", lo, hi)
	}
}

// A model whose trail is not a time axis tints as one, by the sound now; an
// absolute source with nothing measured sits in the middle of its scale.
func TestFlatFillWithoutATrail(t *testing.T) {
	r := newRack(t)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.color.centroid = 0.8
	g := rasterview.DefaultGradient()
	f := r.audioColored(path(10), &g, 4, 0)
	for k, a := range f.Ages {
		if math.Abs(float64(a)-0.8) > 1e-6 {
			t.Fatalf("point %d at %v, want the centroid's 0.8", k, a)
		}
	}
	f = r.audioColored(path(10), &g, 7, 0) // correlation, which reads a pair
	if a := f.Ages[3]; a != 0.5 {
		t.Errorf("correlation with no pair at %v, want 0.5", a)
	}
}

// The coordinate sources are left to the renderer.
func TestCoordinateSourcesAreNotTheTables(t *testing.T) {
	r := newRack(t)
	r.mu.Lock()
	defer r.mu.Unlock()
	g := rasterview.DefaultGradient()
	if f := r.audioColored(path(10), &g, 1, 0); f.Ages != nil || g.Source != 2 {
		t.Error("source Y was colored from the table")
	}
}
