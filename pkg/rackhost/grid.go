package rackhost

import (
	"image"
	"image/draw"
	"math"
	"slices"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The Grid, natively: the model drawn in up to sixteen cells, side by side or
// one over another, with what varies across them — one of its parameters,
// the coloring, or a flow's start — as the page draws it (views_js.go, which
// says why each rule is as it is). The cells' arithmetic and what each model
// may sweep are shared (attractor/grid.go).

// gridView is the cells' own state.
type gridView struct {
	// model and ids are what the sweep dials were filled for, and what they
	// offer, by position.
	model string
	ids   []string
	// focus is the cell whose knobs the panel holds: the focused one,
	// unlinked, and the first otherwise. kept is every other cell's, kept
	// while the panel holds another's (cellKnobs).
	focus int
	kept  [attractor.GridMax]map[string]string
	// stereo is each cell's own Stereo Embedding scope, so a cell's trigger
	// remembers what it caught and not what its neighbor did.
	stereo [attractor.GridMax]stereoView
	// starts are a start sweep's trajectories, one a cell, and eps the ε
	// each began from.
	starts []trail
	eps    []float64
	// shapes are the cells' shapes, built from their knobs: a mesh is the
	// same until they move.
	shapes map[string]attractor.Figure
}

// cellKnobs are the knobs a cell has of its own once the cells are unlinked:
// its coloring, as the page keeps one per view, and the Stereo Embedding's,
// whose instances the page keeps one a cell.
var cellKnobs = append([]string{"gradient-source", "gradient-colors"}, attractor.StereoKnobs...)

// gridSet is the Grid's settings for a frame.
type gridSet struct {
	n             int
	overlay, link bool
	across, down  string // what each sweep varies, "" for nothing
	lo, hi        float32
	focus         int
}

// gridNow is the Grid's settings now. Called with r.mu held.
func (r *Rack) gridNow() gridSet {
	g := gridSet{
		n:       attractor.GridCount(float32(r.number("view-n"))),
		overlay: r.value("grid-ovl") == "1",
		link:    r.value("link-sw") != "0",
		lo:      float32(r.number("sweep-lo")),
		hi:      float32(r.number("sweep-hi")),
		focus:   r.grid.focus,
	}
	g.across, g.down = r.sweepTarget("sweep-p"), r.sweepTarget("sweep2-p")
	if g.down == g.across {
		// Rows and columns of the same thing say nothing a strip does not:
		// the down axis yields.
		g.down = ""
	}
	return g
}

// sweepTarget is what sweep dial id is on. Called with r.mu held.
func (r *Rack) sweepTarget(id string) string {
	i, err := strconv.Atoi(r.value(id))
	if err != nil || len(r.grid.ids) == 0 {
		return ""
	}
	return r.grid.ids[min(max(i, 0), len(r.grid.ids)-1)]
}

// starting reports whether the cells are a start sweep of model.
func (g gridSet) starting(model string) bool {
	return g.n > 1 && (g.across == attractor.SweepStart || g.down == attractor.SweepStart) && attractor.StartSweepable(model)
}

// gridTargets fills the sweep dials with what model may sweep, from none,
// when that is not what they offer already: a position of one model's list
// is another parameter in another's. Called with r.mu held.
func (r *Rack) gridTargets(model string) {
	ids, ring := attractor.GridSweepTargets(model, attractor.StartSweepable(model))
	if slices.Equal(ids, r.grid.ids) {
		r.grid.model = model
		return
	}
	r.grid.model, r.grid.ids = model, ids
	names := make([]string, len(ring))
	for i, s := range ring {
		if s == "—" {
			s = "none" // the dial's display names it, as the page's does
		}
		names[i] = s
	}
	r.refill("sweep-p", names, "0")
	r.refill("sweep2-p", names, "0")
}

// gridCells refills the focus dial with one position a cell, and moves the
// panel's knobs to the cell it now drives. Called with r.mu held.
func (r *Rack) gridCells() {
	n := attractor.GridCount(float32(r.number("view-n")))
	labels := make([]string, n)
	for i := range labels {
		labels[i] = string(rune('A' + i))
	}
	at := r.value("focus-n")
	if f, err := strconv.Atoi(at); err != nil || f >= n {
		at = "0" // focus past the end of a grid that shrank
	}
	r.refill("focus-n", labels, at)
	r.refocus()
}

// refill gives select id new positions, named by labels, and puts it at
// value. Called with r.mu held.
func (r *Rack) refill(id string, labels []string, value string) {
	i, ok := r.byID[id]
	if !ok {
		return
	}
	c := &r.ctls[i]
	c.Options = make([]string, len(labels))
	for k := range labels {
		c.Options[k] = strconv.Itoa(k)
	}
	c.Labels = labels
	c.Value = value
	r.turn(c)
}

// refocus moves the panel's own-cell knobs to the cell they now drive,
// keeping the last one's: the focused cell's, unlinked, and the first's
// otherwise. Called with r.mu held.
func (r *Rack) refocus() {
	to := 0
	if r.value("link-sw") == "0" && attractor.GridCount(float32(r.number("view-n"))) > 1 {
		to, _ = strconv.Atoi(r.value("focus-n")) //nolint:errcheck // a position of the dial
	}
	from := r.grid.focus
	if to == from {
		return
	}
	r.grid.focus = to
	keep := make(map[string]string, len(cellKnobs))
	for _, id := range cellKnobs {
		keep[id] = r.value(id)
	}
	r.grid.kept[from] = keep
	for _, id := range cellKnobs {
		_ = r.set(id, r.keptValue(to, id)) //nolint:errcheck // a value the control held
	}
}

// keptValue is knob id's value in cell i while the panel holds another's:
// what it was left at, or where every cell's begins, the default.
func (r *Rack) keptValue(i int, id string) string {
	if v, ok := r.grid.kept[i][id]; ok {
		return v
	}
	if id == "gradient-source" || id == "gradient-colors" {
		return "2" // Z, two-color: where the page's views open
	}
	return r.calib[id]
}

// cellPlan is how one cell is drawn: the values it has in place of the
// knobs, its coloring, and its figure when that is not the model's own.
type cellPlan struct {
	over     map[string]float64
	gradient rasterview.Gradient
	src      int // its gradient source
	fig      *attractor.Figure
}

// planCells works out each cell of the frame. Called with r.mu held, after
// hear, which has fed the models what they draw from.
func (r *Rack) planCells(key string, g gridSet) []cellPlan {
	plans := make([]cellPlan, g.n)
	for i := range plans {
		p := &plans[i]
		p.over = r.cellValues(key, i, g)
		r.cellOver = p.over
		p.gradient = r.look().gradient
		p.src = int(r.number("gradient-source"))
		p.fig = r.cellAudioFigure(key, i, p.over)
		r.cellOver = nil
		across, down := attractor.SweepAxisFracs(i, g.n, g.down != "")
		for _, ax := range [...]struct {
			id string
			t  float32
		}{{g.across, across}, {g.down, down}} {
			t := g.lo + (g.hi-g.lo)*ax.t
			switch ax.id {
			case attractor.SweepSrc:
				p.src = attractor.SweepPick(attractor.SweepColorSources, t)
				setSource(&p.gradient, p.src)
			case attractor.SweepMap:
				p.gradient.Colors = attractor.SweepPick(attractor.SweepColorMaps, t)
			}
		}
		if g.overlay && i > 0 && g.across != attractor.SweepSrc && g.across != attractor.SweepMap &&
			g.down != attractor.SweepSrc && g.down != attractor.SweepMap {
			// One over another, each cell after the first in a hue of its
			// own, so they can be told apart.
			p.gradient = rasterview.Gradient{Colors: 1, Base: attractor.GridOverlayTints[i-1]}
		}
	}
	return plans
}

// cellValues are what cell i has in place of the knobs: its own, unlinked,
// and its place along each sweep of one of the model's parameters. Called
// with r.mu held.
func (r *Rack) cellValues(key string, i int, g gridSet) map[string]float64 {
	over := map[string]float64{}
	if !g.link && i != g.focus {
		for _, id := range cellKnobs {
			if f, err := strconv.ParseFloat(r.keptValue(i, id), 64); err == nil {
				over[id] = f
			}
		}
	}
	across, down := attractor.SweepAxisFracs(i, g.n, g.down != "")
	for _, ax := range [...]struct {
		id string
		t  float32
	}{{g.across, across}, {g.down, down}} {
		if lo, hi, ok := attractor.SweepRange(key, ax.id); ok {
			t := g.lo + (g.hi-g.lo)*ax.t
			over[ax.id] = float64(lo + (hi-lo)*t)
		}
	}
	return over
}

// differs reports whether any of over is a knob of key's own: what makes a
// cell's figure its own rather than the model's.
func differs(key string, over map[string]float64) bool {
	for id := range over {
		if _, _, ok := attractor.SweepRange(key, id); ok {
			return true
		}
		if key == "stereo" && slices.Contains(attractor.StereoKnobs, id) {
			return true
		}
	}
	return false
}

// cellAudioFigure is cell i's figure when it is an audio embedding drawn at
// values of its own, nil otherwise. Called with r.mu held and cellOver set.
func (r *Rack) cellAudioFigure(key string, i int, over map[string]float64) *attractor.Figure {
	if !differs(key, over) {
		return nil
	}
	var f attractor.Figure
	switch key {
	case "stereo":
		st := &r.grid.stereo[i]
		if st.scope == nil {
			st.scope = attractor.NewStereoScope()
		}
		f, _, _ = r.stereoFigureOf(st)
	case "takens", "polar":
		f, _, _ = r.audioFigure(key)
	case "waterfall":
		// The surface is the one measured; a cell draws it at its own
		// LINE, STEP, TOP, RANGE and DEPTH, and gives them back.
		w := r.analyzers.wfall.w
		for id, v := range over {
			w.Set(id, float32(v))
		}
		f = w.Figure()
		r.cellOver = nil
		for id := range over {
			w.Set(id, float32(r.number(id)))
		}
		r.cellOver = over
	default:
		return nil
	}
	return &f
}

// cellShape is cell i's shape at its own knobs, when it has any. Called
// with s.mu held and not r.mu.
func (r *Rack) cellShape(key string, p cellPlan, roll float64) (attractor.Figure, bool) {
	if p.fig != nil || !differs(key, p.over) {
		return attractor.Figure{}, false
	}
	if _, ok := attractor.ShapeFigure(key, func(string) float64 { return 1 }, 0); !ok {
		return attractor.Figure{}, false
	}
	k := r.knobs()
	knob := func(id string) float64 {
		if v, ok := p.over[id]; ok {
			return v
		}
		return k(id)
	}
	sig := key + strconv.FormatFloat(roll, 'g', 6, 64)
	for _, id := range attractor.GridSweepParams[key] {
		sig += "," + strconv.FormatFloat(knob(id.ID), 'g', 6, 64)
	}
	gv := &r.grid
	if f, ok := gv.shapes[sig]; ok {
		return f, true
	}
	f, err := figureFor(key, knob, roll)
	if err != nil {
		return attractor.Figure{}, false
	}
	if gv.shapes == nil || len(gv.shapes) > 4*attractor.GridMax {
		gv.shapes = map[string]attractor.Figure{} // the knobs have moved on
	}
	gv.shapes[sig] = f
	return f, true
}

// startFigures are a start sweep's cells: one trajectory of the flow a cell,
// each begun ε along x from the model's own and run on as the flow is, the
// frame's run shared between them. Called with s.mu held.
func (gv *gridView) startFigures(key string, g gridSet, paused bool, run int, speed float64, dots bool) []attractor.Figure {
	eps := make([]float64, g.n)
	for i := range eps {
		across, down := attractor.SweepAxisFracs(i, g.n, g.down != "")
		frac := across
		if g.across != attractor.SweepStart {
			frac = down
		}
		eps[i] = attractor.StartEps(g.lo + (g.hi-g.lo)*frac)
	}
	if len(gv.starts) != g.n || !slices.Equal(eps, gv.eps) || gv.starts[0].key != key {
		// Started together, so what the cells show apart is the dynamics
		// and not when each began.
		gv.eps, gv.starts = eps, make([]trail, g.n)
		for i := range gv.starts {
			t := &gv.starts[i]
			t.key = key
			t.seed()
			t.x += eps[i]
		}
	}
	out := make([]attractor.Figure, g.n)
	for i := range gv.starts {
		t := &gv.starts[i]
		if !paused || len(t.pts) == 0 {
			t.advanceIn(key, run, speed, trailBudget/g.n)
		}
		out[i] = attractor.Figure{Kind: attractor.FigurePath, Points: append([][3]float64(nil), t.pts...)}
		if dots {
			out[i].Kind = attractor.FigurePoints
		}
	}
	return out
}

// gridGap is the gutter between cells in a frame w wide: the page's two
// pixels, at the share of the frame they take of its own.
func gridGap(w int) int { return max(1, int(math.Round(2*float64(w)/pageWidth))) }

// gridFrame is a w by h frame of g's cells, each drawn by cell at the size
// it is given: side by side in reading order, gaps between them, or each
// over the last on the whole frame.
func gridFrame(g gridSet, w, h int, cell func(i, cw, ch int) *image.RGBA) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(m, m.Bounds(), &image.Uniform{sceneBack}, image.Point{}, draw.Src)
	if g.overlay {
		for i := range g.n {
			if c := cell(i, w, h); c != nil {
				over(m, c)
			}
		}
		return m
	}
	cols, rows := attractor.GridShape(g.n)
	gap := gridGap(w)
	xs, okX := attractor.GridEdges(w, cols, gap)
	ys, okY := attractor.GridEdges(h, rows, gap)
	if !okX || !okY {
		// Too small to divide: the one view, as the page draws it.
		if c := cell(0, w, h); c != nil {
			draw.Draw(m, m.Bounds(), c, image.Point{}, draw.Src)
		}
		return m
	}
	for i := range g.n {
		x, y := xs[i%cols], ys[i/cols] // numbered from the top left, as a reader starts
		if c := cell(i, x[1], y[1]); c != nil {
			draw.Draw(m, image.Rect(x[0], y[0], x[0]+x[1], y[0]+y[1]), c, image.Point{}, draw.Src)
		}
	}
	return m
}

// drawCells draws the frame o describes as g's cells, each by draw1: the
// model's own figure in its own coloring, or the figure and coloring the
// cell has of its own (planCells, startFigures, cellShape). Every cell is
// framed by one camera, fitted to the figure at the knobs' own values, as
// the page's is. Called with s.mu held and not r.mu.
func (r *Rack) drawCells(key string, g gridSet, plans []cellPlan, fig attractor.Figure, o attractor.DrawOptions,
	draw1 func(attractor.Figure, attractor.DrawOptions) *image.RGBA, paused bool, run int, speed float64, dots bool,
) *image.RGBA {
	if o.Fit == nil {
		o.Fit = fig.Points
	}
	var starts []attractor.Figure
	if g.starting(key) {
		starts = r.grid.startFigures(key, g, paused, run, speed, dots)
	}
	// A model that integrates draws its one trace in every cell, linked:
	// overlaid, those are the one trace where it already is.
	replay := g.link && attractor.IsAttractorMode(key) && starts == nil
	return gridFrame(g, o.Width, o.Height, func(i, cw, ch int) *image.RGBA {
		if replay && g.overlay && i > 0 {
			return nil
		}
		p := plans[i]
		co := o
		co.Width, co.Height, co.Gradient = cw, ch, p.gradient
		f := fig
		switch {
		case starts != nil:
			f = starts[i]
		case p.fig != nil:
			f = *p.fig
		default:
			if sf, ok := r.cellShape(key, p, r.scene.roll); ok {
				f = sf
			}
		}
		r.mu.Lock()
		f = r.audioColored(f, &co.Gradient, p.src, i)
		r.mu.Unlock()
		return draw1(f, co)
	})
}

// pictureCells is a flat model's picture — an analyzer's, a terminal's — in
// g's cells, as the page draws the one picture in each; overlaid, they are
// the one picture.
func pictureCells(h heard, v rasterview.View, g gridSet, w, ht int) *image.RGBA {
	if g.n <= 1 {
		return h.picture(v, w, ht)
	}
	return gridFrame(g, w, ht, func(i, cw, ch int) *image.RGBA {
		if g.overlay && i > 0 {
			return nil
		}
		return h.picture(v, cw, ch)
	})
}

// soundColored reports whether a cell may be colored by the sound though the
// panel's coloring is not: a source sweep, or cells with colorings of their
// own.
func (g gridSet) soundColored() bool {
	return g.n > 1 && (g.across == attractor.SweepSrc || g.down == attractor.SweepSrc || !g.link)
}
