//go:build js && wasm

package attractor

import (
	"strings"
	"testing"
)

// One view is the whole canvas; two split it with a gutter and must tile it
// exactly — a rounding error here is a column of pixels one view clears and
// the other never draws into.
func TestViewRectsTileTheCanvas(t *testing.T) {
	savedW, savedH, savedSplit := width, height, viewCountF
	defer func() { width, height, viewCountF = savedW, savedH, savedSplit }()

	width, height = 1281, 720 // odd, so the halves cannot be equal

	viewCountF = 0 // one cell
	one := viewRects()
	if len(one) != 1 {
		t.Fatalf("unsplit gave %d rects", len(one))
	}
	if one[0] != [4]int{0, 0, width, height} {
		t.Errorf("unsplit rect = %v, want the whole canvas", one[0])
	}

	viewCountF = 1 // two cells
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
	savedW, savedH, savedSplit := width, height, viewCountF
	defer func() { width, height, viewCountF = savedW, savedH, savedSplit }()

	viewCountF = 1 // two cells
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
	savedLink, savedSplit, savedFocus := viewLink, viewCountF, viewFocus
	defer func() {
		viewLink, viewCountF, viewFocus = savedLink, savedSplit, savedFocus
		stereo = focusedInst()
	}()

	viewCountF = 1 // two cells

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
	viewLink, viewCountF = false, 0
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

// The color source and map are per view too, or the split cannot show the
// same figure read two ways — which is the comparison it is most for.
func TestColorIsPerViewWhenUnlinked(t *testing.T) {
	savedLink, savedSplit, savedFocus := viewLink, viewCountF, viewFocus
	savedColors := viewColors
	defer func() {
		viewLink, viewCountF, viewFocus = savedLink, savedSplit, savedFocus
		viewColors = savedColors
	}()

	viewCountF, viewLink = 1, false
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
	savedLink, savedSplit, savedFocus := viewLink, viewCountF, viewFocus
	savedColors := viewColors
	defer func() {
		viewLink, viewCountF, viewFocus = savedLink, savedSplit, savedFocus
		viewColors = savedColors
	}()

	viewCountF, viewLink, viewFocus = 1, false, 1
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
	viewLink, viewCountF = false, 0
	if focusedColorIdx() != 0 {
		t.Error("unsplit, the selects should write to the only view")
	}
	// An out-of-range focus must not index past the array.
	viewCountF, viewLink, viewFocus = 1, false, 7
	if idx := focusedColorIdx(); idx < 0 || idx >= len(viewColors) {
		t.Errorf("focused color index %d is out of range", idx)
	}
}

// The grid tiles without leaving a cell off the edge, at every size the
// dial offers.
func TestGridTilesAtEverySize(t *testing.T) {
	savedW, savedH, savedN := width, height, viewCountF
	defer func() { width, height, viewCountF = savedW, savedH, savedN }()
	width, height = 1281, 721 // odd both ways

	for idx, want := range viewCounts {
		viewCountF = float32(idx)
		rects := viewRects()
		if want == 1 {
			if len(rects) != 1 {
				t.Errorf("size %d gave %d rects", want, len(rects))
			}
			continue
		}
		if len(rects) != want {
			t.Fatalf("size %d gave %d rects", want, len(rects))
		}
		cols, rows := viewGridShape(want)
		if cols*rows < want {
			t.Errorf("size %d: a %dx%d grid cannot hold it", want, cols, rows)
		}
		for i, r := range rects {
			if r[2] < 1 || r[3] < 1 {
				t.Errorf("size %d cell %d is %v, which GL will reject", want, i, r)
			}
			if r[0] < 0 || r[1] < 0 || r[0]+r[2] > width || r[1]+r[3] > height {
				t.Errorf("size %d cell %d is %v, outside the canvas %dx%d",
					want, i, r, width, height)
			}
		}
	}
}

// A sweep spreads a parameter over its own range, ends included: the first
// cell is the minimum and the last the maximum, or a contact sheet is not
// showing the range it claims to.
func TestSweepSpansTheRangeEndToEnd(t *testing.T) {
	if got := sweepFrac(0, 9); got != 0 {
		t.Errorf("first cell sits at %v, want 0", got)
	}
	if got := sweepFrac(8, 9); got != 1 {
		t.Errorf("last cell sits at %v, want 1", got)
	}
	if got := sweepFrac(4, 9); got < 0.49 || got > 0.51 {
		t.Errorf("middle cell sits at %v, want about 0.5", got)
	}
	// One cell has nowhere to be but the start, and must not divide by zero.
	if got := sweepFrac(0, 1); got != 0 {
		t.Errorf("single cell sits at %v", got)
	}
	// Out of range indices clamp rather than running off the ramp.
	if got := sweepFrac(-3, 4); got != 0 {
		t.Errorf("negative index gave %v", got)
	}
	if got := sweepFrac(99, 4); got != 1 {
		t.Errorf("index past the end gave %v", got)
	}
}

// The dial's tables have to line up, as every other rotary's do.
func TestSweepAndGridTablesLineUp(t *testing.T) {
	if len(sweepIDs) != len(sweepNames) || len(sweepIDs) != len(sweepRing) {
		t.Errorf("sweep: %d ids, %d names, %d ring",
			len(sweepIDs), len(sweepNames), len(sweepRing))
	}
	if len(viewCounts) != len(viewCountNames) || len(viewCounts) != len(viewCountRing) {
		t.Errorf("grid: %d counts, %d names, %d ring",
			len(viewCounts), len(viewCountNames), len(viewCountRing))
	}
	// Every numeric sweep target must name a real field, or the sweep
	// silently does nothing.
	inst := newStereoInst()
	for i, id := range sweepIDs {
		if id == "" || id[0] == '#' {
			continue
		}
		if inst.field(id) == nil {
			t.Errorf("sweep target %d (%s) names no field", i, id)
		}
	}
}

// Out-of-range view indices must not panic; the draw loop indexes by
// position in viewRects, and an index past the last instance should
// degrade to view A rather than crash.
func TestInstanceForClamps(t *testing.T) {
	savedLink := viewLink
	defer func() { viewLink = savedLink }()
	viewLink = false
	for _, i := range []int{-1, viewMax, viewMax + 1, 99} {
		if instanceFor(i) != viewInsts[0] {
			t.Errorf("instanceFor(%d) did not fall back to view A", i)
		}
	}
	// In range, every cell has its own instance and no two share one.
	seen := map[*stereoInst]bool{}
	for i := 0; i < viewMax; i++ {
		inst := instanceFor(i)
		if inst == nil {
			t.Fatalf("instanceFor(%d) is nil", i)
		}
		if seen[inst] {
			t.Errorf("cell %d shares an instance with an earlier cell", i)
		}
		seen[inst] = true
	}
}

// A sweep is only running when there is more than one cell to spread it
// over; announcing one on a single view would mark a knob that is still
// setting the value.
func TestSweepActiveNeedsBothATargetAndAGrid(t *testing.T) {
	savedN, savedP := viewCountF, sweepParamF
	defer func() { viewCountF, sweepParamF = savedN, savedP }()

	for _, c := range []struct {
		n, p float32
		want bool
	}{
		{0, 0, false}, // one view, no target
		{0, 1, false}, // one view, a target: nothing to spread it over
		{2, 0, false}, // four views, no target: four copies
		{2, 1, true},  // four views and a target
	} {
		viewCountF, sweepParamF = c.n, c.p
		if got := sweepActive(); got != c.want {
			t.Errorf("grid %v target %v: active = %v, want %v", c.n, c.p, got, c.want)
		}
	}
}

// The dial, its labels and its options are three lists bound by index, and
// every one of the three has been the site of a bug: a dial with more
// options than labels silently loses its last positions, one with more
// labels than options points at nothing. They are built in one loop now,
// so what this guards is that the loop keeps them equal for every mode —
// including the modes that contribute no numeric target at all.
func TestSweepTablesLineUpForEveryMode(t *testing.T) {
	savedIDs, savedNames, savedRing := sweepIDs, sweepNames, sweepRing
	savedMode, savedP := sweepDialMode, sweepParamF
	defer func() {
		sweepIDs, sweepNames, sweepRing = savedIDs, savedNames, savedRing
		sweepDialMode, sweepParamF = savedMode, savedP
	}()

	for mode := range modeInfo {
		sweepDialMode = "" // force the rebuild
		setSweepTargets(mode)
		if len(sweepIDs) != len(sweepNames) || len(sweepIDs) != len(sweepRing) {
			t.Errorf("%s: %d ids, %d names, %d ring labels",
				mode, len(sweepIDs), len(sweepNames), len(sweepRing))
		}
		if len(sweepIDs) < 3 {
			t.Errorf("%s: %d targets, want at least none + the two colorings",
				mode, len(sweepIDs))
		}
		if sweepIDs[0] != "" {
			t.Errorf("%s: the first target is %q, want none", mode, sweepIDs[0])
		}
		last := sweepIDs[len(sweepIDs)-2:]
		if last[0] != "#src" || last[1] != "#map" {
			t.Errorf("%s: the colorings are not the last two targets: %v", mode, last)
		}
		// Every numeric target must name a real parameter with a real
		// range, or the sweep silently does nothing.
		for _, id := range sweepIDs[1 : len(sweepIDs)-2] {
			lo, hi, ok := paramRange(mode, id)
			if !ok {
				t.Errorf("%s: target %s is in no parameter table", mode, id)
			} else if hi <= lo {
				t.Errorf("%s: target %s has an empty range %v..%v", mode, id, lo, hi)
			}
		}
	}
}

// A sweep of a flow would be one trajectory smeared across the cells
// rather than one trajectory per cell, so the dial does not offer it.
// The colorings, which are recomputed per pass, are offered everywhere.
func TestNumericSweepsOnlyWhereTheyAreTrue(t *testing.T) {
	savedIDs, savedNames, savedRing := sweepIDs, sweepNames, sweepRing
	savedMode, savedP := sweepDialMode, sweepParamF
	defer func() {
		sweepIDs, sweepNames, sweepRing = savedIDs, savedNames, savedRing
		sweepDialMode, sweepParamF = savedMode, savedP
	}()

	numeric := func(mode string) int {
		sweepDialMode = ""
		setSweepTargets(mode)
		return len(sweepIDs) - 3 // none, #src, #map
	}
	for mode, info := range modeInfo {
		n := numeric(mode)
		switch info.Class {
		case ClassFlow3D, ClassFlow4D, ClassMap:
			if n != 0 {
				t.Errorf("%s integrates, so its %d numeric sweep targets would "+
					"each show the cell before it", mode, n)
			}
		case ClassParametric:
			// The delay embeddings live here too, and they rebuild from
			// the audio window rather than integrating.
			if n != 0 && !isAudioEmbedding(mode) {
				t.Errorf("%s traces a curve in time, so its %d numeric sweep "+
					"targets would each show the cell before it", mode, n)
			}
		}
		if n < 0 {
			t.Errorf("%s: the colorings are missing", mode)
		}
	}
	// And at least one mode does offer numeric targets, or the gate above
	// is passing by switching the whole feature off.
	if numeric("stereo") < 3 {
		t.Errorf("the stereo embedding offers %d numeric sweep targets", numeric("stereo"))
	}
}

// Switching modes must not leave the dial pointed at position 4 of a list
// it is no longer showing.
func TestModeChangeResetsTheSweep(t *testing.T) {
	savedIDs, savedNames, savedRing := sweepIDs, sweepNames, sweepRing
	savedMode, savedP := sweepDialMode, sweepParamF
	defer func() {
		sweepIDs, sweepNames, sweepRing = savedIDs, savedNames, savedRing
		sweepDialMode, sweepParamF = savedMode, savedP
	}()

	sweepDialMode = ""
	setSweepTargets("stereo")
	sweepParamF = 2
	target := sweepTarget()
	if target == "" {
		t.Fatal("nothing at position 2 of the stereo list")
	}
	// The same mode again changes nothing and must leave the dial alone.
	if setSweepTargets("stereo") {
		t.Error("the same mode rebuilt the dial")
	}
	if sweepTarget() != target {
		t.Errorf("the dial moved to %q on a no-op rebuild", sweepTarget())
	}
	if !setSweepTargets("lorenz") {
		t.Fatal("a mode with different parameters did not rebuild the dial")
	}
	if sweepParamF != 0 {
		t.Errorf("the dial stayed at %v across a mode change", sweepParamF)
	}
	// And the index is always in range, whatever it was before.
	if sweepTarget() != "" {
		t.Errorf("after a mode change the dial reads %q, want none", sweepTarget())
	}
}

// The range knobs the sweep reads have to exist in the markup, or from and
// to are package variables nothing can move.
func TestSweepMarkupCarriesItsControls(t *testing.T) {
	for _, id := range []string{"sweep-p", "sweep-p-stack", "sweep-lo", "sweep-hi",
		"sweep-lo-cell", "sweep-hi-cell", "slider-value-swlo", "slider-value-swhi",
		"rst-swlo", "rst-swhi", "view-n", "view-n-stack"} {
		if !strings.Contains(controlsBody, `id="`+id+`"`) {
			t.Errorf("no %s in the panel markup", id)
		}
	}
	// The grid dial's options ARE static, so they still have to be counted
	// against its table.
	i := strings.Index(controlsBody, `id="view-n"`)
	end := strings.Index(controlsBody[i:], "</select>")
	if got := strings.Count(controlsBody[i:i+end], "<option "); got != len(viewCounts) {
		t.Errorf("the grid dial offers %d options and the table has %d sizes",
			got, len(viewCounts))
	}
}

// sweepTo points the dial at a target by id, which is how a test names one
// now that the positions are per mode.
func sweepTo(t *testing.T, mode, id string) {
	t.Helper()
	sweepDialMode = ""
	setSweepTargets(mode)
	for i, got := range sweepIDs {
		if got == id {
			sweepParamF = float32(i)
			return
		}
	}
	t.Fatalf("%s offers no sweep target %s", mode, id)
}

// The sweep is a SOURCE, not a value: it must put the knob's own setting
// back when the pass is over, or turning the sweep off would leave the
// last cell's value behind.
func TestSweepRestoresTheKnob(t *testing.T) {
	savedIDs, savedNames, savedRing := sweepIDs, sweepNames, sweepRing
	savedMode, savedP := sweepDialMode, sweepParamF
	defer func() {
		sweepIDs, sweepNames, sweepRing = savedIDs, savedNames, savedRing
		sweepDialMode, sweepParamF = savedMode, savedP
	}()

	inst := newStereoInst()
	prev := stereo
	stereo = inst
	defer func() { stereo = prev }()

	inst.tau = 123
	sweepTo(t, "stereo", "stereo-tau")
	restore := applySweep("stereo", 3, 9)
	if inst.tau == 123 {
		t.Error("the sweep did not set the cell's value")
	}
	restore()
	if inst.tau != 123 {
		t.Errorf("the knob's value came back as %v, want 123", inst.tau)
	}

	// Each cell gets a DIFFERENT value, or a sweep is nine copies.
	seen := map[float32]bool{}
	for i := 0; i < 9; i++ {
		r := applySweep("stereo", i, 9)
		seen[inst.tau] = true
		r()
	}
	if len(seen) < 9 {
		t.Errorf("nine cells produced %d distinct values", len(seen))
	}

	// With no sweep selected nothing is touched at all.
	sweepParamF = 0
	inst.tau = 55
	applySweep("stereo", 3, 9)()
	if inst.tau != 55 {
		t.Errorf("a sweep of none changed the value to %v", inst.tau)
	}
}

// The color sweeps step through their lists and restore the globals.
func TestColorSweepStepsAndRestores(t *testing.T) {
	savedIDs, savedNames, savedRing := sweepIDs, sweepNames, sweepRing
	savedMode, savedP := sweepDialMode, sweepParamF
	savedSrc, savedCols := gradientSource, gradientColors
	defer func() {
		sweepIDs, sweepNames, sweepRing = savedIDs, savedNames, savedRing
		sweepDialMode, sweepParamF = savedMode, savedP
		gradientSource, gradientColors = savedSrc, savedCols
	}()

	sweepTo(t, "stereo", "#src")
	gradientSource = 2
	n := len(sweepColorSrcs)

	first := applySweep("stereo", 0, n)
	got := gradientSource
	first()
	if got != sweepColorSrcs[0] {
		t.Errorf("first cell used source %d, want %d", got, sweepColorSrcs[0])
	}
	if gradientSource != 2 {
		t.Errorf("the source was left at %d instead of restored", gradientSource)
	}

	last := applySweep("stereo", n-1, n)
	got = gradientSource
	last()
	if got != sweepColorSrcs[n-1] {
		t.Errorf("last cell used source %d, want %d", got, sweepColorSrcs[n-1])
	}

	// Nine sources and nine maps, so a 3x3 grid shows each exactly once.
	if len(sweepColorSrcs) != 9 || len(sweepColorMaps) != 9 {
		t.Errorf("%d sources and %d maps; a 3x3 grid wants nine of each",
			len(sweepColorSrcs), len(sweepColorMaps))
	}

	// The colorings sweep in EVERY mode, including the ones with no
	// numeric target: a coloring is recomputed per pass wherever it is.
	sweepTo(t, "lorenz", "#map")
	gradientColors = 0
	r := applySweep("lorenz", 8, 9)
	got = gradientColors
	r()
	if got != sweepColorMaps[8] {
		t.Errorf("a flow's map sweep used %d, want %d", got, sweepColorMaps[8])
	}
	if gradientColors != 0 {
		t.Errorf("the map was left at %d instead of restored", gradientColors)
	}
}

// The from and to knobs bound the sweep inside the parameter's range: with
// them turned in, the cells cover a slice in detail instead of the whole
// range coarsely, which is the whole reason they exist.
func TestSweepRangeKnobsBoundIt(t *testing.T) {
	savedIDs, savedNames, savedRing := sweepIDs, sweepNames, sweepRing
	savedMode, savedP := sweepDialMode, sweepParamF
	savedLo, savedHi := sweepLo, sweepHi
	defer func() {
		sweepIDs, sweepNames, sweepRing = savedIDs, savedNames, savedRing
		sweepDialMode, sweepParamF = savedMode, savedP
		sweepLo, sweepHi = savedLo, savedHi
	}()

	inst := newStereoInst()
	prev := stereo
	stereo = inst
	defer func() { stereo = prev }()

	sweepTo(t, "stereo", "stereo-tau")
	lo, hi, ok := paramRange("stereo", "stereo-tau")
	if !ok {
		t.Fatal("no range for stereo-tau")
	}

	read := func(i, n int) float32 {
		r := applySweep("stereo", i, n)
		v := inst.tau
		r()
		return v
	}

	sweepLo, sweepHi = 0, 1
	if got := read(0, 4); got != lo {
		t.Errorf("the first cell is %v, want the parameter's minimum %v", got, lo)
	}
	if got := read(3, 4); got != hi {
		t.Errorf("the last cell is %v, want the parameter's maximum %v", got, hi)
	}

	// Half the range, from the middle up.
	sweepLo, sweepHi = 0.5, 1
	mid := lo + (hi-lo)*0.5
	if got := read(0, 4); got != mid {
		t.Errorf("with from at 0.5 the first cell is %v, want %v", got, mid)
	}

	// Backwards, which is deliberate: neither knob clamps against the other.
	sweepLo, sweepHi = 1, 0
	if read(0, 4) <= read(3, 4) {
		t.Error("to below from did not run the sweep backwards")
	}
}

// The focus control has to reach every cell the grid can draw. It was a
// two-position A/B switch when there were two views; with sixteen, a
// switch that reaches two of them is a panel that cannot drive what it is
// showing.
func TestFocusReachesEveryCell(t *testing.T) {
	savedN, savedFocus := viewCountF, viewFocus
	defer func() {
		viewCountF, viewFocus = savedN, savedFocus
		stereo = focusedInst()
	}()

	for idx, want := range viewCounts {
		viewCountF = float32(idx)
		labels := focusLabels(viewN())
		if len(labels) != want {
			t.Errorf("a grid of %d offers %d focus positions", want, len(labels))
		}
		if want > 0 && labels[0] != "A" {
			t.Errorf("the first cell is called %q", labels[0])
		}
		if want > 1 && labels[1] != "B" {
			t.Errorf("the second cell is called %q", labels[1])
		}
		// Every position selects a different instance, or two of them name
		// the same knobs.
		viewLink = false
		seen := map[*stereoInst]bool{}
		for i := range labels {
			viewFocus = i
			inst := focusedInst()
			if seen[inst] {
				t.Errorf("grid of %d: position %d focuses an instance already focused",
					want, i)
			}
			seen[inst] = true
		}
	}
	// Sixteen is the ceiling, and there are that many instances to reach.
	if len(focusLabels(viewMax)) != len(viewInsts) {
		t.Errorf("%d focus positions for %d instances",
			len(focusLabels(viewMax)), len(viewInsts))
	}
}
