//go:build !js

package server

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// Every model render lists has to draw something, in every output it
// writes. This is --check plus the writers the check does not exercise.
func TestEveryDrawableModelDraws(t *testing.T) {
	old := renderPts
	renderPts = 2000
	t.Cleanup(func() { renderPts = old })
	for _, k := range drawableKeys() {
		f, err := figureFor(k)
		if err != nil {
			t.Fatalf("%s: listed by models but not drawable: %v", k, err)
		}
		dx, dy, dz := attractor.Extent(f.Points)
		if math.Max(dx, math.Max(dy, dz)) == 0 {
			t.Errorf("%s: no extent", k)
		}
	}
}

func TestFigureWriters(t *testing.T) {
	old := [3]int{renderW, renderH, renderFrames}
	renderW, renderH, renderFrames = 64, 64, 3
	t.Cleanup(func() { renderW, renderH, renderFrames = old[0], old[1], old[2] })
	dir := t.TempDir()
	for _, k := range []string{"henon", "cube", "globe", "lissajou"} {
		f, err := figureFor(k)
		if err != nil {
			t.Fatal(err)
		}
		for _, ext := range []string{".png", ".svg"} {
			if err := writeFigure(filepath.Join(dir, k+ext), f); err != nil {
				t.Errorf("%s%s: %v", k, ext, err)
			}
		}
		for _, ext := range []string{".gif", ".svg"} {
			if err := writeFigureAnimation(filepath.Join(dir, k+"-anim"+ext), f); err != nil {
				t.Errorf("%s animated %s: %v", k, ext, err)
			}
		}
	}
}

func TestMapRevealGrows(t *testing.T) {
	f, err := figureFor("henon")
	if err != nil {
		t.Fatal(err)
	}
	if a, b := figureReveal(f, 0, 4), figureReveal(f, 3, 4); a >= b || b != len(f.Points) {
		t.Errorf("reveal %d then %d of %d", a, b, len(f.Points))
	}
	w, err := figureFor("cube")
	if err != nil {
		t.Fatal(err)
	}
	if figureReveal(w, 0, 4) != len(w.Points) {
		t.Error("a wireframe should be whole in every frame")
	}
}
