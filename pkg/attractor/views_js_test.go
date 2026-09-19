//go:build js && wasm

package attractor

import "testing"

// One view is the whole canvas; two split it with a gutter and must tile it
// exactly — a rounding error here is a column of pixels one view clears and
// the other never draws into.
func TestViewRectsTileTheCanvas(t *testing.T) {
	savedW, savedH, savedSplit := width, height, viewSplit
	defer func() { width, height, viewSplit = savedW, savedH, savedSplit }()

	width, height = 1281, 720 // odd, so the halves cannot be equal

	viewSplit = false
	one := viewRects()
	if len(one) != 1 {
		t.Fatalf("unsplit gave %d rects", len(one))
	}
	if one[0] != [4]int{0, 0, width, height} {
		t.Errorf("unsplit rect = %v, want the whole canvas", one[0])
	}

	viewSplit = true
	two := viewRects()
	if len(two) != 2 {
		t.Fatalf("split gave %d rects", len(two))
	}
	l, r := two[0], two[1]
	if l[0] != 0 || l[1] != 0 || r[1] != 0 {
		t.Errorf("rects are not flush with the canvas: %v %v", l, r)
	}
	if l[3] != height || r[3] != height {
		t.Errorf("a view is not full height: %v %v", l, r)
	}
	if got := r[0] + r[2]; got != width {
		t.Errorf("the right view ends at %d, want the canvas edge %d", got, width)
	}
	if gap := r[0] - (l[0] + l[2]); gap != viewGap {
		t.Errorf("gutter is %d, want %d", gap, viewGap)
	}
	// Neither view may be wider than the other by more than the rounding
	// the odd width forces.
	if d := l[2] - r[2]; d > 1 || d < -1 {
		t.Errorf("views differ in width by %d: %v %v", d, l, r)
	}
}

// A canvas too narrow to split must still give usable rects rather than a
// zero or negative width, which GL rejects.
func TestViewRectsSurviveATinyCanvas(t *testing.T) {
	savedW, savedH, savedSplit := width, height, viewSplit
	defer func() { width, height, viewSplit = savedW, savedH, savedSplit }()

	viewSplit = true
	for _, w := range []int{0, 1, 2, 3, 4} {
		width, height = w, 100
		for i, r := range viewRects() {
			if r[2] < 1 || r[3] < 1 {
				t.Errorf("width %d: rect %d is %v, which GL will reject", w, i, r)
			}
		}
	}
}
