package attractor

import (
	"os"
	"regexp"
	"testing"
)

// A P-unit written in the markup carries its number as a plain range input,
// and knobifyFixed is what hides it and puts the knob in its place. Nothing
// else does, and nothing fails without it: the cell is built, works, and shows
// a browser's slider where every other cell has a knob (the Lattice module's
// ROWS and OPAC did, 2026-10-05).

var (
	punitCellRe = regexp.MustCompile(`<div class="punit"[^>]*>.*?<button class="rst"`)
	punitRange  = regexp.MustCompile(`<input type="range" id="([^"]+)"`)
)

func TestEveryMarkupPUnitRangeIsAKnob(t *testing.T) {
	html, err := os.ReadFile("panelhtml_js.go")
	if err != nil {
		t.Fatal(err)
	}
	main, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, cell := range punitCellRe.FindAll(html, -1) {
		for _, m := range punitRange.FindAllSubmatch(cell, -1) {
			n++
			id := string(m[1])
			if !regexp.MustCompile(`knobifyFixed\("` + regexp.QuoteMeta(id) + `"`).Match(main) {
				t.Errorf("P-unit range %q is never knobifyFixed: it shows as a plain slider", id)
			}
		}
	}
	if n == 0 {
		t.Fatal("found no P-unit ranges in the markup; the pattern no longer matches it")
	}
}
