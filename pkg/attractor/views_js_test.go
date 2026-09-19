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

// Link is what decides whether the two halves are one instrument or two.
func TestLinkDecidesWhetherTheViewsShareParameters(t *testing.T) {
	savedLink, savedSplit, savedFocus := viewLink, viewSplit, viewFocus
	defer func() {
		viewLink, viewSplit, viewFocus = savedLink, savedSplit, savedFocus
		stereo = focusedInst()
	}()

	viewSplit = true

	viewLink = true
	if instanceFor(0) != instanceFor(1) {
		t.Error("linked views draw different instances")
	}
	if focusedInst() != viewInsts[0] {
		t.Error("linked focus is not view A")
	}

	viewLink = false
	if instanceFor(0) == instanceFor(1) {
		t.Error("unlinked views share an instance")
	}
	if instanceFor(0) != viewInsts[0] || instanceFor(1) != viewInsts[1] {
		t.Error("unlinked views draw the wrong instances")
	}

	// Focus picks which one the panel means, but only when there is a
	// choice: one view, or two linked, leaves exactly one instance on
	// screen and the knobs must point at it.
	viewFocus = 1
	if focusedInst() != viewInsts[1] {
		t.Error("focus B did not select view B's instance")
	}
	viewLink = true
	if focusedInst() != viewInsts[0] {
		t.Error("focus B while linked should still mean the shared instance")
	}
	viewLink, viewSplit = false, false
	if focusedInst() != viewInsts[0] {
		t.Error("focus B with one view should mean the only instance on screen")
	}
}

// Unlinked views really are independent, which is the point of the switch.
func TestUnlinkedViewsKeepSeparateSettings(t *testing.T) {
	a, b := viewInsts[0], viewInsts[1]
	savedA, savedB := a.tau, b.tau
	defer func() { a.tau, b.tau = savedA, savedB }()

	a.tau = 300
	if b.tau == 300 {
		t.Error("setting view A's tau moved view B's")
	}
	b.tau = 50
	if a.tau != 300 {
		t.Error("setting view B's tau moved view A's")
	}
}

// Out-of-range view indices must not panic; the draw loop indexes by
// position in viewRects and a third rect should degrade, not crash.
func TestInstanceForClamps(t *testing.T) {
	savedLink := viewLink
	defer func() { viewLink = savedLink }()
	viewLink = false
	for _, i := range []int{-1, 2, 99} {
		if instanceFor(i) != viewInsts[0] {
			t.Errorf("instanceFor(%d) did not fall back to view A", i)
		}
	}
}

// The color source and map are per view too, or the split cannot show the
// same figure read two ways — which is the comparison it is most for.
func TestColorIsPerViewWhenUnlinked(t *testing.T) {
	savedLink, savedSplit, savedFocus := viewLink, viewSplit, viewFocus
	savedColors := viewColors
	defer func() {
		viewLink, viewSplit, viewFocus = savedLink, savedSplit, savedFocus
		viewColors = savedColors
	}()

	viewSplit, viewLink = true, false
	viewColors[0] = viewColor{src: 7, cols: 8}  // corr / turbo
	viewColors[1] = viewColor{src: 13, cols: 4} // pos / hue sweep

	if colorFor(0) != (viewColor{7, 8}) || colorFor(1) != (viewColor{13, 4}) {
		t.Errorf("unlinked views share a coloring: %v %v", colorFor(0), colorFor(1))
	}

	// Linked, both take view A's, whatever B's entry says.
	viewLink = true
	if colorFor(0) != colorFor(1) || colorFor(1) != (viewColor{7, 8}) {
		t.Errorf("linked views do not share view A's coloring: %v %v", colorFor(0), colorFor(1))
	}
}

// The gradient selects write to whichever entry the panel is showing, and
// that is view A unless the views are split AND apart.
func TestGradientSelectsWriteToTheFocusedView(t *testing.T) {
	savedLink, savedSplit, savedFocus := viewLink, viewSplit, viewFocus
	savedColors := viewColors
	defer func() {
		viewLink, viewSplit, viewFocus = savedLink, savedSplit, savedFocus
		viewColors = savedColors
	}()

	viewSplit, viewLink, viewFocus = true, false, 1
	if got := focusedColorIdx(); got != 1 {
		t.Errorf("focused color index = %d, want 1", got)
	}
	noteGradientSource(9)
	if viewColors[1].src != 9 {
		t.Errorf("the source went to view %d instead of B", 0)
	}
	if viewColors[0].src == 9 {
		t.Error("the source leaked into view A")
	}

	// Linked or unsplit, there is one coloring on screen and it is A's.
	viewLink = true
	if focusedColorIdx() != 0 {
		t.Error("linked, the selects should write to the shared entry")
	}
	viewLink, viewSplit = false, false
	if focusedColorIdx() != 0 {
		t.Error("unsplit, the selects should write to the only view")
	}
	// An out-of-range focus must not index past the array.
	viewSplit, viewLink, viewFocus = true, false, 7
	if idx := focusedColorIdx(); idx < 0 || idx >= len(viewColors) {
		t.Errorf("focused color index %d is out of range", idx)
	}
}
