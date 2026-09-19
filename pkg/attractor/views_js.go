//go:build js && wasm

package attractor

// A grid of views of one model, and the sweep that makes a grid usable.
//
// It follows drawSplitPasses (split_js.go), which already draws the model
// twice for the Fore knob: a pass is a viewport, a projection and a call to
// generateForMode. Two differences. These passes sit beside each other
// rather than in front and behind, and each gets its OWN projection,
// because a cell has a different aspect from the canvas and reusing the
// canvas matrix draws the figure stretched inside it.
//
// Sixteen cells are sixteen viewports, which is a loop, and sixteen
// parameter sets, which is not a panel anyone could use. The second of
// those is what limits how many cells there can be, so the grid is not
// sixteen instruments configured by hand: it is one setting with one
// parameter varying across the cells. Pick what varies and the grid draws
// that parameter's range — a contact sheet of a setting rather than
// sixteen panels to fill in.
//
// The sweep is the first PARAMETER SOURCE other than a knob. A parameter's
// value for a cell comes from the knob, or from where that cell sits in
// the sweep. The modulation matrix is a third source that already exists
// and Link is a fourth, and they are the same idea: a value with somewhere
// it comes from. They should end up sharing this path.

import (
	"math"
	"strconv"
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// viewSplit reports whether more than one cell is drawn.
//
// It was the Views checkbox and is now derived from the grid dial, so
// there is one place that says how many cells there are. Everything that
// asks "are we split" means "is there more than one", which is the same
// question whether the answer is two cells or sixteen.
func viewSplit() bool { return viewN() > 1 }

// viewGap is the gutter between cells, in pixels. Without one the figures
// touch and a grid reads as a single crowded picture.
const viewGap = 2

// viewRects is where each cell draws, as GL viewport rectangles measured
// from the bottom-left, row-major from the TOP-LEFT cell — so cell 0 is
// where a reader starts and the sweep runs the way text does.
//
// A canvas too small to hold the grid gives ONE rect: GL rejects a zero or
// negative viewport, and a sixteenth of nothing is not a view.
func viewRects() [][4]int {
	w, h := width, height
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	full := [][4]int{{0, 0, w, h}}

	n := viewN()
	if n <= 1 {
		return full
	}
	cols, rows := viewGridShape(n)
	xs, okX := gridEdges(w, cols)
	ys, okY := gridEdges(h, rows)
	if !okX || !okY {
		return full
	}
	out := make([][4]int, 0, n)
	for i := 0; i < n; i++ {
		cx := i % cols
		cy := i / cols
		// GL counts y from the bottom; the cells are numbered from the top.
		gy := rows - 1 - cy
		out = append(out, [4]int{xs[cx][0], ys[gy][0], xs[cx][1], ys[gy][1]})
	}
	return out
}

// gridEdges divides one axis of length total into n cells with viewGap
// between them, as {start, size} pairs.
//
// The division has to be done on the cut positions rather than on the cell
// size, or integer truncation leaves up to n-1 pixels unpainted at the far
// edge — a column the last view never draws into and nothing else clears.
// Taking each boundary as a fraction of the whole spends the remainder
// across the cells instead, so the sizes differ by at most one pixel and
// the last cell always ends exactly on the edge. The bool is false when the
// canvas is too small for the split to leave a drawable cell, which GL
// rejects outright.
func gridEdges(total, n int) ([][2]int, bool) {
	if n < 1 {
		return nil, false
	}
	span := total + viewGap // the gaps come out of the boundaries below
	out := make([][2]int, n)
	for i := 0; i < n; i++ {
		start := i * span / n
		end := (i+1)*span/n - viewGap
		if end-start < 1 {
			return nil, false
		}
		out[i] = [2]int{start, end - start}
	}
	return out, true
}

// setViewport points GL at one rect and gives the shader the projection for
// its aspect.
//
// The projection has to be per view: a half-width viewport is half the
// aspect ratio, and reusing the full-canvas matrix draws a figure stretched
// to twice its width inside it. projMatrix is a package variable that
// texProgram also reads, so it is restored by the caller running the full
// rect last.
func setViewport(r [4]int) {
	gl.Call("viewport", r[0], r[1], r[2], r[3])
	h := r[3]
	if h < 1 {
		h = 1
	}
	projMatrix = mgl32.Perspective(mgl32.DegToRad(45.0), float32(r[2])/float32(h), 1, 1500.0)
	gl.Call("useProgram", shaderProgram)
	gl.Call("uniformMatrix4fv",
		gl.Call("getUniformLocation", shaderProgram, "Pmatrix"), false, mat4ToTyped(&projMatrix))
}

// drawViewPasses draws the mode once per view.
//
// The scissor is what keeps a pass inside its rect: a viewport clips the
// geometry but not a clear or a full-screen effect, and without it the
// second pass's background wipes the first pass's figure.
func drawViewPasses(mode string) {
	rects := viewRects()
	if len(rects) == 1 {
		generateForMode(mode)
		return
	}
	n := len(rects)
	gl.Call("enable", gl.Get("SCISSOR_TEST"))
	for i, r := range rects {
		// Each pass draws ITS cell's instance. Restored below, because
		// everything outside the passes — the readout, the panel, the
		// next frame — means the focused one.
		stereo = instanceFor(i)
		c := colorFor(i)
		gradientSource, gradientColors = c.src, c.cols
		// And then the sweep, which is a SOURCE for one parameter rather
		// than a value of it: applied for this cell and put straight back,
		// so the knob still holds what the operator set.
		restore := applySweep(mode, i, n)
		gl.Call("scissor", r[0], r[1], r[2], r[3])
		setViewport(r)
		generateForMode(mode)
		restore()
	}
	stereo = focusedInst()
	fc := colorFor(focusedColorIdx())
	gradientSource, gradientColors = fc.src, fc.cols
	gl.Call("disable", gl.Get("SCISSOR_TEST"))
	// Back to the whole canvas, so everything drawn after these passes —
	// the Poincaré overlay, the lens, the next frame's clear — sees the
	// state it has always seen.
	setViewport([4]int{0, 0, width, height})
}

// wireViewGridDial hooks up the grid-size dial.
func wireViewGridDial() {
	sel := doc.Call("getElementById", "view-n")
	if !sel.Truthy() {
		return
	}
	apply := func() {
		if n, err := strconv.Atoi(sel.Get("value").String()); err == nil {
			viewCountF = float32(n)
		}
		// Focus can be past the end of a grid that just shrank, and the
		// panel has to follow whichever instance is now in play.
		if viewFocus >= viewN() {
			viewFocus = 0
		}
		// The focus dial has one position per cell, so it is rebuilt
		// with the grid — before refocus, which reads viewFocus.
		buildFocusDial()
		refocus()
		// After the rebuild, or the marking goes onto rows that are
		// about to be replaced.
		syncSweptMarks()
		// The camera was fitted to a full-canvas viewport; a cell of a grid
		// wants a different distance, and the fit is what knows how to pick
		// one.
		autoFitCamera()
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		apply()
		return nil
	}))
	apply()
}

// ── which view the knobs drive, and whether they drive both ─────────────

// viewLink shares one parameter set between the views. On, both halves
// draw the same instance and the panel means both — which is the state
// that behaves exactly as a single view always did, and is why it is the
// default. Off, each half has its own and the panel means the focused one.
//
// A switch rather than a decision: comparing two settings of one instrument
// wants them separate, and comparing two COLORINGS or two camera angles of
// one setting wants them together, and both are things to want.
var viewLink = true

// viewFocus is which view the panel drives while they are unlinked.
var viewFocus int

// instanceFor returns the stereo instance a view draws.
func instanceFor(i int) *stereoInst {
	if viewLink || i < 0 || i >= len(viewInsts) {
		return viewInsts[0]
	}
	return viewInsts[i]
}

// focusedInst is the instance the panel drives: the focused view's when the
// views are split and unlinked, and view A's otherwise. With one view or
// with the two linked there is only one instance in play, and pointing the
// knobs at the other would be pointing them at something not on screen.
func focusedInst() *stereoInst {
	if !viewSplit() || viewLink {
		return viewInsts[0]
	}
	return instanceFor(viewFocus)
}

// refocus points the panel at the right instance and rebuilds the rows.
//
// The rebuild is the whole mechanism. A parameter row binds to a field
// address when it is built, so moving `stereo` afterwards changes what the
// DRAW reads and not what a knob WRITES; only building the rows again
// against the new instance moves both.
func refocus() {
	stereo = focusedInst()
	applyFocusedColor()
	buildParamPanel(selectedMode)
}

// wireViewLinkSwitches hooks up Link and the A/B focus switch.
func wireViewLinkSwitches() {
	if sw := doc.Call("getElementById", "link-sw"); sw.Truthy() {
		sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			viewLink = sw.Get("checked").Bool()
			refocus()
			return nil
		}))
	}
	if sel := doc.Call("getElementById", "focus-n"); sel.Truthy() {
		sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
			if n, err := strconv.Atoi(sel.Get("value").String()); err == nil {
				viewFocus = n
			}
			refocus()
			return nil
		}))
	}
	buildFocusDial()
}

// ── color per view ──────────────────────────────────────────────────────
//
// The color SOURCE and the MAP are package variables, because they apply to
// every mode rather than to any one of them — and that made them the one
// thing Link could not split. Two views of the same figure colored two ways
// is the comparison the split is most useful for (the same moment read as
// correlation and as stereo position, side by side), so they are per view
// too.
//
// Kept beside viewState rather than in it: viewState is the camera, which
// is still shared between the views, and putting a split field next to
// unsplit ones in the same struct would be a struct that is half per-view.
// When the camera splits these fold into it.
type viewColor struct {
	src  int // gradientSource
	cols int // gradientColors
}

// viewColors is each view's coloring. The defaults are the ones the
// gradient selects open at — Z, two-color — so an untouched second view
// looks like the first until something is changed.
var viewColors = newViewColors()

func newViewColors() [viewMax]viewColor {
	var out [viewMax]viewColor
	for i := range out {
		out[i] = viewColor{src: 2, cols: 2}
	}
	return out
}

// colorFor returns the coloring a view draws with, which is the shared one
// while the views are linked.
func colorFor(i int) viewColor {
	if viewLink || i < 0 || i >= len(viewColors) {
		return viewColors[0]
	}
	return viewColors[i]
}

// focusedColorIdx is which entry the gradient selects write to.
func focusedColorIdx() int {
	if !viewSplit() || viewLink {
		return 0
	}
	if viewFocus < 0 || viewFocus >= len(viewColors) {
		return 0
	}
	return viewFocus
}

// noteGradientSource records a change the gradient select just made, so the
// focused view keeps it. Called from the select's own handler, after the
// global it drives has been set.
func noteGradientSource(n int) { viewColors[focusedColorIdx()].src = n }

// noteGradientColors is the same for the map.
func noteGradientColors(n int) { viewColors[focusedColorIdx()].cols = n }

// applyFocusedColor puts the focused view's coloring back into the globals
// and onto the two selects, so the panel reads what the focused view draws.
//
// The selects are set WITHOUT dispatching: their handlers would write
// straight back into the entry being read, which is harmless but circular,
// and updateGradientUI is what the handlers call anyway.
func applyFocusedColor() {
	c := viewColors[focusedColorIdx()]
	gradientSource, gradientColors = c.src, c.cols
	setSelectQuiet("gradient-source", c.src)
	setSelectQuiet("gradient-colors", c.cols)
	updateGradientUI()
}

// setSelectQuiet sets a select's value and refreshes the knob ring built
// over it, without running the select's change handler.
func setSelectQuiet(id string, v int) {
	el := doc.Call("getElementById", id)
	if !el.Truthy() {
		return
	}
	el.Set("value", strconv.Itoa(v))
	// The ring is a set of labels over a hidden select; it reads the value
	// on an input event, which is not the change event the handler wants.
	el.Call("dispatchEvent", js.Global().Get("Event").New("input"))
}

// ── the grid, and the sweep that makes it controllable ──────────────────
//
// Sixteen views are sixteen viewports, which is a loop. Sixteen views are
// also sixteen parameter sets, which is not a panel anyone can use — and
// that, not the rendering, is what limits how many there can be.
//
// So the grid is not N instruments configured by hand. It is ONE setting
// with one parameter varying across the cells: pick what varies and the
// range it covers, and the grid draws that parameter's space. Sixteen
// cells then need two controls rather than sixteen panels, and what you
// get is a contact sheet of a parameter — the same figure sixteen times,
// differing in one dimension, which is the thing worth looking at.
//
// This is the first PARAMETER SOURCE other than a knob. A parameter's
// value for a cell comes from the knob, or from where the cell sits in the
// sweep; the modulation matrix is a third source that already exists, and
// linking is a fourth. They are the same idea and should end up sharing
// this path.

// viewMax is how many cells the grid may hold. Sixteen is four by four,
// which is as many as a figure stays legible in on a normal screen.
const viewMax = 16

// viewCounts are the grid sizes the dial offers: squares, and two, which
// is the side-by-side comparison that is not a square.
var viewCounts = []int{1, 2, 4, 9, 16}

var viewCountNames = []string{
	"1 — a single view",
	"2 — side by side",
	"4 — two by two",
	"9 — three by three",
	"16 — four by four",
}

var viewCountRing = []string{"1", "2", "4", "9", "16"}

// viewCountF is the dial: an index into viewCounts.
var viewCountF float32

// viewN is how many cells are drawn.
func viewN() int {
	i := clampSel(viewCountF, len(viewCounts)-1)
	return viewCounts[i]
}

// viewGridShape is the grid's columns and rows for a cell count.
//
// Two is a row rather than a column: the figures here are wider than they
// are tall, so side by side keeps more of each than one above the other.
func viewGridShape(n int) (cols, rows int) {
	switch {
	case n <= 1:
		return 1, 1
	case n == 2:
		return 2, 1
	}
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	rows = (n + cols - 1) / cols
	return cols, rows
}

// ── the sweep: what varies across the grid ──────────────────────────────

// sweepIDs are what a grid may vary, by control id, and sweepNames and
// sweepRing are how the dial says them. All three are REBUILT PER MODE by
// setSweepTargets: a sweep is over the parameters of the model on screen,
// and a fixed list would offer the stereo embedding's delay while a
// polyhedron is drawn.
//
// The first entry is always "nothing varies", which is the default and
// makes a grid N copies. The last two begin with a hash and are not
// parameters at all but the coloring, which has no id in any mode's table
// because it is not a knob of a model. They are the Warhol case — the same
// figure, a different reading of it in every cell — and they are the
// reason the sweep is not restricted to numbers.
var (
	sweepIDs   = []string{"", "#src", "#map"}
	sweepNames = []string{
		"none — every cell the same",
		"color source — a different reading of the same figure per cell",
		"color map — the same reading in a different palette per cell",
	}
	sweepRing = []string{"—", "csrc", "cmap"}
)

// sweepDialMode is the mode the three lists above were built for, so a
// panel rebuild that is not a mode change leaves the dial alone.
var sweepDialMode string

// sweepNumericOK reports whether a numeric sweep of this mode would be
// TRUE — whether cell i's picture is a function of cell i's value alone.
//
// It is, for the audio EMBEDDINGS, whose trail is a window of the live
// audio rather than an integrated trajectory — isAudioEmbedding is the
// package's existing name for exactly that property — and for the static
// shapes, which rebuild from their parameters on demand.
//
// It is NOT for a flow, a map or a traced curve: those integrate, so one
// pass continues where the last left off and a swept grid of them would
// show one trajectory smeared across sixteen parameter values rather than
// sixteen trajectories. That is a picture of nothing, so the dial does
// not offer it — the two color sweeps stay, because a coloring IS
// recomputed per pass in every mode.
func sweepNumericOK(mode string) bool {
	return isAudioEmbedding(mode) || modeInfo[mode].Class == ClassGeometry
}

// setSweepTargets rebuilds the three lists for a mode and reports whether
// anything changed.
//
// The lists are separate and bound by index, which is the shape of three
// bugs this panel has already had, so they are filled in ONE loop rather
// than written out three times. The ring label is the parameter's own
// short label — the same text the knob under it carries — and the option
// name is that label plus the sentence the help table has for it, so the
// dial explains what it is about to vary rather than naming it twice.
func setSweepTargets(mode string) bool {
	ids := []string{""}
	names := []string{"none — every cell the same"}
	ring := []string{"—"}
	nums := attractorParams[mode]
	if !sweepNumericOK(mode) {
		nums = nil
	}
	for _, pd := range nums {
		// A named setting is a rotary of words: sweeping it would be a
		// contact sheet of unrelated pictures rather than of one range,
		// and paramRange's min and max are indices into the names. The
		// coloring sweeps below are how that case is served.
		if len(paramLabels[pd.ID]) > 0 {
			continue
		}
		if pd.Max <= pd.Min {
			continue
		}
		ids = append(ids, pd.ID)
		ring = append(ring, pd.Label)
		n := pd.Label
		if h := paramHelp[pd.ID]; h != "" {
			n += " — " + h
		} else {
			n += " — across its range"
		}
		names = append(names, n)
	}
	ids = append(ids, "#src", "#map")
	ring = append(ring, "csrc", "cmap")
	names = append(names,
		"color source — a different reading of the same figure per cell",
		"color map — the same reading in a different palette per cell")

	if sweepDialMode == mode && len(ids) == len(sweepIDs) {
		same := true
		for i := range ids {
			if ids[i] != sweepIDs[i] {
				same = false
				break
			}
		}
		if same {
			return false
		}
	}
	sweepIDs, sweepNames, sweepRing = ids, names, ring
	sweepDialMode = mode
	// The dial cannot stay where it was: position 4 of one mode's list is a
	// different parameter in another's, and silently sweeping something the
	// operator did not pick is worse than starting from none.
	sweepParamF = 0
	return true
}

// sweepParamF is the dial: an index into sweepIDs.
var sweepParamF float32

// sweepLo and sweepHi bound the sweep as a FRACTION of the target's own
// range, which is what lets one pair of knobs drive any target: 0 is the
// parameter's minimum and 1 its maximum, whatever those are.
var sweepLo, sweepHi float32 = 0, 1

// sweepTarget is the id the sweep varies, or "" for none.
func sweepTarget() string { return sweepIDs[clampSel(sweepParamF, len(sweepIDs)-1)] }

// sweepColorSrcs is the order the color-source sweep steps through: the
// audio-fed sources, which are the ones worth comparing. Nine of them, and
// nine is a three by three grid.
var sweepColorSrcs = []int{
	gradientSourceAudio, gradientSourceLevel, gradientSourceDB,
	gradientSourceCorr, gradientSourceSide, gradientSourceBalance,
	gradientSourcePosition, gradientSourceFlux, gradientSourcePitch,
}

// sweepColorMaps is the order the map sweep steps through: every genuine
// mapping, two-color first. Nine again, which is not a coincidence — the
// colormaps were chosen to match the spectrogram's own list.
var sweepColorMaps = []int{2, 3, 4, 5, 6, 7, 8, 9, 10}

// sweepFrac is where cell i sits in the sweep, 0..1. A single cell sits at
// the start rather than dividing by zero.
func sweepFrac(i, n int) float32 {
	if n <= 1 {
		return 0
	}
	if i < 0 {
		i = 0
	} else if i >= n {
		i = n - 1
	}
	return float32(i) / float32(n-1)
}

// paramRange finds a parameter's min and max in the mode's own table, so
// the sweep covers what the knob covers and nothing outside it.
func paramRange(mode, id string) (lo, hi float32, ok bool) {
	for _, pd := range attractorParams[mode] {
		if pd.ID == id {
			return pd.Min, pd.Max, true
		}
	}
	return 0, 0, false
}

// applySweep sets cell i's swept value, and returns a function that puts
// back what it changed.
//
// Restored rather than left, because the sweep is a SOURCE for a value and
// not a new value: the knob still holds what the operator set, and turning
// the sweep off has to give that back rather than whatever the last cell
// happened to be.
func applySweep(mode string, i, n int) func() {
	id := sweepTarget()
	if id == "" || n <= 1 {
		return func() {}
	}
	t := sweepLo + (sweepHi-sweepLo)*sweepFrac(i, n)

	switch id {
	case "#src":
		prev := gradientSource
		gradientSource = sweepColorSrcs[int(t*float32(len(sweepColorSrcs)-1)+0.5)]
		return func() { gradientSource = prev }
	case "#map":
		prev := gradientColors
		gradientColors = sweepColorMaps[int(t*float32(len(sweepColorMaps)-1)+0.5)]
		return func() { gradientColors = prev }
	}

	// The instance field first, the mode table's pointer second. They are
	// the same value for every model but the stereo embedding, which is the
	// one with a per-cell instance: there the table's pointer names the
	// FOCUSED instance, and writing through it would sweep one cell's
	// parameter n times instead of each cell's once.
	lo, hi, ok := paramRange(mode, id)
	if !ok {
		return func() {}
	}
	f := stereo.field(id)
	if f == nil {
		f = paramValue(mode, id)
	}
	if f == nil {
		return func() {}
	}
	prev := *f
	*f = lo + (hi-lo)*t
	// A cached mesh is a picture of the PREVIOUS cell's value. Geometry is
	// the one family that keeps its vertices between frames, so a swept
	// grid of it has to pay for a rebuild per cell — which is what the
	// operator asked for by pointing the sweep at a shape.
	if modeInfo[mode].Class == ClassGeometry {
		staticGeomDirty = true
		return func() {
			*f = prev
			staticGeomDirty = true // and back to the knob's own shape
		}
	}
	return func() { *f = prev }
}

// wireSweepDial hooks up the sweep dial.
func wireSweepDial() {
	sel := doc.Call("getElementById", "sweep-p")
	if !sel.Truthy() {
		return
	}
	sel.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		if n, err := strconv.Atoi(sel.Get("value").String()); err == nil {
			sweepParamF = float32(n)
		}
		// A swept parameter is no longer the knob's to set, and a knob that
		// looks live while a sweep overrides it is the panel lying. The
		// rebuild is what carries the swept marking onto the row.
		buildParamPanel(selectedMode)
		syncSweepCells()
		syncSweptMarks()
		return nil
	}))
	syncSweepCells()
	syncSweptMarks()
}

// ── saying so on the knob ───────────────────────────────────────────────

// sweepActive reports whether anything is actually being swept: a target
// AND more than one cell to spread it over. A sweep set on a single view
// changes nothing, and must not be announced as though it had.
func sweepActive() bool { return viewN() > 1 && sweepTarget() != "" }

// sweptCell finds the panel cell for the swept parameter.
//
// By DOM lookup rather than by threading a flag through each builder,
// because the control the sweep names is built in three different places —
// the parameter grid, the Colors module's src dial, its map dial — and a
// rule that has to be re-implemented per builder is a rule that will be
// missing from the fourth one. Every one of them puts the control's id on
// an element inside its cell, so the cell is one closest() away.
func sweptCell() js.Value {
	switch id := sweepTarget(); id {
	case "":
		return js.Undefined()
	case "#src":
		return doc.Call("getElementById", "src-cell")
	case "#map":
		return doc.Call("getElementById", "map-cell")
	default:
		el := doc.Call("getElementById", id)
		if !el.Truthy() {
			return js.Undefined()
		}
		return el.Call("closest", ".punit, .pcell")
	}
}

// syncSweptMarks puts the swept marking on that cell and takes it off
// every other.
//
// A knob whose value the sweep is overriding is a knob that no longer says
// what is on screen, and a panel that does not admit that is a panel
// lying: every dimension added to this instrument has to be visible on the
// control it acts on, or the operator is reading a surface that has quietly
// stopped describing the sound. The marking is the sweep's arrow and a rim
// around the cell, and the cell's own tooltip says what the knob now means
// — the CENTER of nothing, since the ends come from the from/to knobs.
func syncSweptMarks() {
	// Clear first, unconditionally: the target moves, the grid collapses to
	// one cell, the panel rebuilds. All three leave a mark somewhere it no
	// longer belongs.
	for _, sel := range []string{".swept", ".sweptmark"} {
		old := doc.Call("querySelectorAll", sel)
		for i := 0; i < old.Length(); i++ {
			el := old.Index(i)
			if sel == ".swept" {
				el.Get("classList").Call("remove", "swept")
				continue
			}
			if p := el.Get("parentNode"); p.Truthy() {
				p.Call("removeChild", el)
			}
		}
	}
	if !sweepActive() {
		return
	}
	cell := sweptCell()
	if !cell.Truthy() {
		return
	}
	cell.Get("classList").Call("add", "swept")
	m := doc.Call("createElement", "span")
	m.Set("className", "sweptmark")
	m.Set("textContent", "⇢")
	m.Set("title", "Swept — this parameter's value comes from where each cell "+
		"sits in the grid, not from this knob. The from and to knobs beside the "+
		"Sweep dial say which part of its range the cells cover.")
	cell.Call("appendChild", m)
}

// syncSweepCells shows the sweep's range knobs only while a sweep is set.
// Two knobs bounding a sweep that is not running are two knobs that do
// nothing, and the console has enough to read already.
func syncSweepCells() {
	on := sweepTarget() != ""
	for _, id := range []string{"sweep-lo-cell", "sweep-hi-cell"} {
		if el := doc.Call("getElementById", id); el.Truthy() {
			if on {
				el.Get("style").Set("display", "")
			} else {
				el.Get("style").Set("display", "none")
			}
		}
	}
}

// buildSweepDial fills the sweep select from the current target lists and
// puts a fresh labeled rotary around it.
//
// Rebuilt rather than relabeled because the ring is a ring of positioned
// elements sized to the option count: a mode with three parameters and one
// with nine are two different dials, and editing one into the other in
// place is how the label at position n stops meaning option n.
// sweepDialFuncs is the dial's own arena: its ring is rebuilt per mode,
// on a different schedule from the panel's.
var sweepDialFuncs []js.Func

func buildSweepDial() { rebuildInto(&sweepDialFuncs, buildSweepDialInto) }

func buildSweepDialInto() {
	sel := doc.Call("getElementById", "sweep-p")
	holder := doc.Call("getElementById", "sweep-p-stack")
	if !sel.Truthy() || !holder.Truthy() {
		return
	}
	sel.Set("innerHTML", "")
	for i, name := range sweepNames {
		opt := doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", sweepRing[i])
		opt.Set("title", name)
		sel.Call("appendChild", opt)
	}
	sel.Set("value", strconv.Itoa(clampSel(sweepParamF, len(sweepIDs)-1)))
	sel.Get("style").Set("display", "none")

	holder.Set("innerHTML", "")
	stack := soloKnob(sel)
	addSelectorLabels(stack, sweepRing, sel, 45).Set("id", "sweep-p-ring")
	holder.Call("appendChild", stack)
}

// syncSweepDialMode rebuilds the dial when the mode's parameters have
// changed under it. Called from the panel rebuild, which is the one thing
// that happens on every mode change.
func syncSweepDialMode(mode string) {
	if !setSweepTargets(mode) {
		return
	}
	buildSweepDial()
	syncSweepCells()
	syncSweptMarks()
}

// paramValue is a mode parameter's value pointer, for the models that keep
// one copy of it rather than one per view.
func paramValue(mode, id string) *float32 {
	for _, pd := range attractorParams[mode] {
		if pd.ID == id {
			return pd.Value
		}
	}
	return nil
}

// focusDialFuncs is the focus dial's arena, for buildSweepDial's reason:
// its ring is rebuilt whenever the grid changes size.
var focusDialFuncs []js.Func

// focusLabels names the cells: A, B, C … in the order viewRects lays them
// out, which is reading order from the top left.
func focusLabels(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('A' + i))
	}
	return out
}

// buildFocusDial gives the focus select one position per cell of the
// CURRENT grid, and hides it when there is only one cell to focus.
//
// A two-position A/B switch was right when Views drew the model twice. A
// sixteen-cell sheet with a switch that can only reach two of them is a
// panel that cannot drive what it is showing, so the control follows the
// grid — the same rule as the sweep dial, for the same reason.
func buildFocusDial() { rebuildInto(&focusDialFuncs, buildFocusDialInto) }

func buildFocusDialInto() {
	sel := doc.Call("getElementById", "focus-n")
	holder := doc.Call("getElementById", "focus-n-stack")
	if !sel.Truthy() || !holder.Truthy() {
		return
	}
	n := viewN()
	if cell := doc.Call("getElementById", "focus-n-cell"); cell.Truthy() {
		if n > 1 {
			cell.Get("style").Set("display", "")
		} else {
			cell.Get("style").Set("display", "none")
		}
	}
	labels := focusLabels(n)
	sel.Set("innerHTML", "")
	for i, l := range labels {
		opt := doc.Call("createElement", "option")
		opt.Set("value", strconv.Itoa(i))
		opt.Set("textContent", l)
		opt.Set("title", l+" — cell "+strconv.Itoa(i+1)+" of "+strconv.Itoa(n)+
			", counting from the top left")
		sel.Call("appendChild", opt)
	}
	if viewFocus >= n {
		viewFocus = 0
	}
	sel.Set("value", strconv.Itoa(viewFocus))
	sel.Get("style").Set("display", "none")

	holder.Set("innerHTML", "")
	stack := soloKnob(sel)
	addSelectorLabels(stack, labels, sel, 43).Set("id", "focus-n-ring")
	holder.Call("appendChild", stack)
}
