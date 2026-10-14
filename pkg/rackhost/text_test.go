package rackhost

import (
	"os"
	"path/filepath"
	"testing"
)

// Typing an equation recompiles Custom: a new constant becomes a knob, and
// the system drawn is the one typed.
func TestTypingAnEquation(t *testing.T) {
	r, c := idleRack(t, "custom")
	if got := r.value("custom-eq-1"); got != "x*(rho - z) - y" {
		t.Fatalf("dy/dt reads %q, want the template's", got)
	}
	watch(t, r, c, 0.1)
	before := shownFigure(t, r, c, 0.1)
	set(t, r, "custom-eq-1", "x*(r - z) - y", "custom-r", "15")
	after := shownFigure(t, r, c, 0.1)
	if before[len(before)/2] == after[len(after)/2] {
		t.Error("the typed system draws as the template did")
	}
}

// A typed banner is what Fourier Text draws.
func TestTypingTheBanner(t *testing.T) {
	r, c := idleRack(t, "scopetext")
	long := len(shownFigure(t, r, c, 0.1))
	set(t, r, "stext-in", "hi")
	if short := len(shownFigure(t, r, c, 0.1)); short >= long {
		t.Errorf("HI is %d points, CHAOSRACK %d", short, long)
	}
}

// STL File shows a built-in the picker names, and a file whose path is
// typed, which its load switch asks the panel for.
func TestSTLFileLoads(t *testing.T) {
	r, c := idleRack(t, "stlfile")
	cube := len(shownFigure(t, r, c, 0.05))
	r.mu.Lock()
	solid := r.ctls[r.byID["stlfile-builtin"]].Options[1]
	r.mu.Unlock()
	set(t, r, "stlfile-builtin", solid)
	if n := len(shownFigure(t, r, c, 0.05)); n == cube {
		t.Errorf("the built-in %s drew the stand-in's %d points", solid, n)
	}
	set(t, r, "model-sw-0", "1") // load
	if id, _, ok := r.Prompt(); !ok || id != stlPath {
		t.Fatalf("load asked for %q (%v)", id, ok)
	}
	path := filepath.Join(t.TempDir(), "tri.stl")
	stl := "solid t\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid t\n"
	if err := os.WriteFile(path, []byte(stl), 0o600); err != nil {
		t.Fatal(err)
	}
	set(t, r, stlPath, path)
	if n := len(shownFigure(t, r, c, 0.05)); n != 3 {
		t.Errorf("one triangle loaded drew %d points", n)
	}
	if r.value("stlfile-builtin") != "" {
		t.Error("the picker stayed on the built-in after a file was loaded")
	}
}
