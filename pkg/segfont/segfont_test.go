package segfont

import "testing"

// Both users place a glyph by offsetting its cell, so a stroke outside the
// 2×3 cell would run into the next character.
func TestEveryStrokeStaysOnTheCell(t *testing.T) {
	for r := range segFont {
		segs, _ := Segments(r)
		for _, s := range segs {
			for i, v := range s {
				hi := 2.0
				if i%2 == 1 {
					hi = 3
				}
				if v < 0 || v > hi {
					t.Errorf("%q has a stroke %v off the 2×3 cell", r, s)
				}
			}
		}
	}
}

// Fourier Text draws a rune it cannot find as '?', and the dimension labels
// are digits: those have to be in the font, and a stranger must say it is not.
func TestTheGlyphsItsUsersNeedAreThere(t *testing.T) {
	for _, r := range "?0123456789-" {
		if segs, ok := Segments(r); !ok || len(segs) == 0 {
			t.Errorf("%q: %d strokes, ok=%v", r, len(segs), ok)
		}
	}
	if segs, ok := Segments(' '); !ok || len(segs) != 0 {
		t.Errorf("a space should be in the font with no strokes, got %d, ok=%v", len(segs), ok)
	}
	if _, ok := Segments('é'); ok {
		t.Error("a rune the font lacks reported ok")
	}
}

// Strokes come in segment order: Fourier Text starts its beam tour at the
// first one, so reordering them would move where every glyph begins.
func TestStrokesComeInSegmentOrder(t *testing.T) {
	segs, _ := Segments('7') // a1 a2 b c
	want := [][4]float64{segEnds[0], segEnds[1], segEnds[2], segEnds[3]}
	if len(segs) != len(want) {
		t.Fatalf("'7' has %d strokes, want %d", len(segs), len(want))
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Errorf("stroke %d = %v, want %v", i, segs[i], want[i])
		}
	}
}
