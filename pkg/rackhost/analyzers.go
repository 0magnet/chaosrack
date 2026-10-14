package rackhost

import (
	"image"
	"image/draw"
	"math"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The analyzers, drawn from the rack's signal as the page draws them from its
// tap: the measurement and the lines are the page's own (pkg/attractor), and
// the lines are laid flat over the scene, the page's clip space stretched to
// its edges as the page stretches it over its canvas.

// analyzers are the native rack's analyzers and when they last looked.
type analyzers struct {
	rta  *attractor.RTA
	xfer *attractor.Transfer
	rec  *attractor.Recurrence
	// recPic is the recurrence plot's matrix as a picture, kept up while the
	// matrix is not rebuilt.
	recPic *image.RGBA
	spect  spectroView
	wfall  waterfallView
	morph  morphView
	tau    attractor.TauMeter
	ro     readoutLine
	// autoMap is whether the MAP ring holds the heat the spectrogram put there
	// (followMode).
	autoMap bool
	// lastFlow is the flow the panel drew last, whose trajectory the
	// recurrence plot's traj source plots, as the page's lastFlowMode.
	lastFlow string
	last     time.Time
}

func newAnalyzers() analyzers {
	return analyzers{
		rta:      attractor.NewRTA(),
		xfer:     attractor.NewTransfer(),
		rec:      attractor.NewRecurrence(),
		spect:    spectroView{ctl: attractor.NewSpectroControls()},
		wfall:    waterfallView{w: attractor.NewWaterfall()},
		morph:    morphView{run: attractor.NewSprottMorph(rand.Float64)},
		lastFlow: "lorenz",
	}
}

// isAnalyzer reports whether the native rack draws model key as a flat
// analyzer.
func isAnalyzer(key string) bool {
	return key == "rta" || key == "xfer" || key == "recurrence" || key == "spectrogram" || key == "fvf"
}

// listens reports whether model key is drawn from the rack's signal.
func listens(key string) bool {
	return attractor.IsAudioModel(key) || isAnalyzer(key) || key == "waterfall"
}

// figureEachFrame reports whether model key is a figure made anew each
// frame and framed to its full scale: an audio model and the waterfall's
// surface, from the rack's signal, and the Sprott Morph, from its run.
func figureEachFrame(key string) bool {
	return attractor.IsAudioModel(key) || key == "waterfall" || key == "sprottmorph"
}

// plotPass is lines drawn at one width.
type plotPass struct {
	v     []attractor.PlotVertex
	width int
}

// analyzerPlot is analyzer key's lines now, measured from the newest of the
// rack's signal at the panel's knobs, for a frame w wide. Called with r.mu
// held.
func (r *Rack) analyzerPlot(key string, w int) []plotPass {
	a := &r.analyzers
	now := r.sound.now()
	// The page's ballistics are per frame at 60 a second; the panel's frames
	// come slower, so each look advances by the frames it stood in for.
	frames := 1.0
	if !a.last.IsZero() {
		frames = math.Min(now.Sub(a.last).Seconds()*60, 60)
	}
	a.last = now
	nowMs := float64(now.UnixNano()) / 1e6
	switch key {
	case "rta":
		m := a.rta
		for _, id := range attractor.RTAKnobs {
			m.Set(id, float32(r.number(id)))
		}
		if l, rt := r.sound.window(attractor.RTAFFT); len(l) == attractor.RTAFFT && m.Due(nowMs) {
			m.Analyze(m.Fold(l, rt), sampleRate)
		}
		m.Advance(frames)
		bars, marks := m.Lines(r.analyzerColors())
		// The page widens its bars to about 2.5 of its thousand-odd pixels
		// and draws its marks one pixel high.
		return []plotPass{
			{v: clone(bars), width: max(1, int(math.Round(float64(w)/400)))},
			{v: clone(marks), width: 1},
		}
	case "xfer":
		m := a.xfer
		for _, id := range attractor.TransferKnobs {
			m.Set(id, float32(r.number(id)))
		}
		if l, rt := r.sound.window(attractor.XferFFT); len(l) == attractor.XferFFT && m.Due(nowMs) {
			m.Analyze(l, rt, sampleRate)
		}
		// The page draws its curves about three of its pixels thick.
		return []plotPass{{v: clone(m.Lines(r.analyzerColors())), width: max(1, int(math.Round(float64(w)/400)))}}
	}
	return nil
}

// analyzerColors is how the Colors module colors an analyzer: its colormap,
// when it names one, and the scope's green otherwise. Called with r.mu held.
func (r *Rack) analyzerColors() attractor.AnalyzerColors {
	m, ok := colormap.Index(int(r.number("gradient-colors")))
	return attractor.AnalyzerColors{Map: m, Mapped: ok, Flat: attractor.ScopeGreen}
}

func clone(v []attractor.PlotVertex) []attractor.PlotVertex {
	return append([]attractor.PlotVertex(nil), v...)
}

// drawPlot draws passes over a w by h frame of the scene's background.
func drawPlot(passes []plotPass, w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(m, m.Bounds(), &image.Uniform{sceneBack}, image.Point{}, draw.Src)
	for _, p := range passes {
		for i := 0; i+1 < len(p.v); i += 2 {
			plotLine(m, p.v[i], p.v[i+1], p.width)
		}
	}
	return m
}

// plotLine draws a line between two clip-space ends, its color running from
// one end's to the other's, width pixels across.
func plotLine(m *image.RGBA, a, b attractor.PlotVertex, width int) {
	w, h := float64(m.Rect.Dx()), float64(m.Rect.Dy())
	// Clip space's -1..1 spans the frame; a pixel's center is at +0.5.
	ax, ay := (float64(a.X)+1)/2*w-0.5, (1-float64(a.Y))/2*h-0.5
	bx, by := (float64(b.X)+1)/2*w-0.5, (1-float64(b.Y))/2*h-0.5
	steps := int(math.Ceil(math.Max(math.Abs(bx-ax), math.Abs(by-ay))))
	lo := -(width - 1) / 2
	for s := 0; s <= steps; s++ {
		t := 0.0
		if steps > 0 {
			t = float64(s) / float64(steps)
		}
		x, y := int(math.Round(ax+(bx-ax)*t)), int(math.Round(ay+(by-ay)*t))
		var c [3]uint8
		for k := range c {
			c[k] = uint8(math.Round(255 * math.Max(0, math.Min(1, float64(a.C[k]+(b.C[k]-a.C[k])*float32(t))))))
		}
		for dy := lo; dy < lo+width; dy++ {
			for dx := lo; dx < lo+width; dx++ {
				if !(image.Point{x + dx, y + dy}).In(m.Rect) {
					continue
				}
				o := m.PixOffset(x+dx, y+dy)
				m.Pix[o], m.Pix[o+1], m.Pix[o+2], m.Pix[o+3] = c[0], c[1], c[2], 255
			}
		}
	}
}

// heard is what the rack's signal draws this frame: an audio model's figure,
// the lines and frame it is read against and the XY's glow, or an analyzer's
// lines.
type heard struct {
	fig, grat attractor.Figure
	fit       [][3]float64
	glow      float64
	plot      []plotPass
	quad      *rasterview.Quad
	// fill lays the quad flat over the whole frame instead, as the page's Fill
	// switch lays the spectrogram's plane over its canvas.
	fill bool
}

// hear measures what model key draws from the rack's signal, in a frame w
// wide, while the rack is on. Called with r.mu held.
func (r *Rack) hear(key string, on bool, w int) (h heard) {
	defer r.showReadouts(key, float64(r.sound.now().UnixNano())/1e6)
	if dynamics.HasFlow(key) {
		r.analyzers.lastFlow = key
	}
	switch {
	case attractor.IsAudioModel(key):
		h.fig, h.grat, h.fit = r.audioFigure(key)
		if key == "xy" {
			h.glow = r.number("xy-persist")
		}
	case key == "sprottmorph":
		h.fig, h.fit = r.morphFigure(on && r.value("pause-sw") != "1")
	case key == "waterfall":
		h.fig, h.fit = r.waterfallFigure(float64(r.sound.now().UnixNano()) / 1e6)
	case (key == "spectrogram" || key == "fvf") && on:
		h.quad = r.spectrogram(float64(r.sound.now().UnixNano())/1e6, key == "fvf")
		h.fill = r.value("spect-fill") == "1"
	case key == "recurrence" && on:
		h.quad = r.recurrencePlot(float64(r.sound.now().UnixNano()) / 1e6)
	case isAnalyzer(key) && on:
		h.plot = r.analyzerPlot(key, w)
	}
	return h
}

// texHalf is the half height of the page's textured planes: the
// recurrence plot's square is this either side of the middle.
const texHalf = 1.5

// recurrencePlot is the recurrence plot now, at the panel's knobs: of the
// rack's signal, or of the last flow's trajectory. Called with r.mu held.
func (r *Rack) recurrencePlot(nowMs float64) *rasterview.Quad {
	a := &r.analyzers
	m := a.rec
	for _, id := range attractor.RecurrenceKnobs {
		m.Set(id, float32(r.number(id)))
	}
	var fresh bool
	if m.Trajectory() {
		var vals []float32
		for _, p := range dynamics.Params(a.lastFlow) {
			if p.Value != nil {
				vals = append(vals, *p.Value)
			}
		}
		fresh = m.FromTrajectory(a.lastFlow, vals, nowMs)
	} else {
		tau := float32(r.number("takens-tau"))
		l, rt := r.sound.window(m.AudioNeed(sampleRate, tau))
		x := make([]float32, len(l))
		for i := range x {
			x[i] = (l[i] + rt[i]) * 0.5 // the page's tap: the mix
		}
		fresh = m.FromAudio(x, len(x), sampleRate, tau)
	}
	n := attractor.RecurrenceSide
	if a.recPic == nil {
		a.recPic = image.NewRGBA(image.Rect(0, 0, n, n))
		fresh = true
	}
	if fresh {
		a.ro.recDirty = true
	}
	if fresh {
		// Row j of the matrix is the j-th oldest moment, and the page puts
		// the first row at the bottom: time runs up and to the right.
		mat := m.Matrix()
		for j := range n {
			for i := range n {
				v := mat[j*n+i]
				o := a.recPic.PixOffset(i, n-1-j)
				a.recPic.Pix[o], a.recPic.Pix[o+1], a.recPic.Pix[o+2], a.recPic.Pix[o+3] = v, v, v, 255
			}
		}
	}
	return &rasterview.Quad{
		Pic: a.recPic,
		BL:  [3]float64{-texHalf, -texHalf, 0},
		BR:  [3]float64{texHalf, -texHalf, 0},
		TL:  [3]float64{-texHalf, texHalf, 0},
	}
}

// drawQuad draws q over a frame of the scene's background, in view v: posed
// and turned with the scene, framed by its own corners, as the page's
// textured models are.
func drawQuad(q *rasterview.Quad, v rasterview.View, w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(m, m.Bounds(), &image.Uniform{sceneBack}, image.Point{}, draw.Src)
	reach := 0.0
	for _, c := range [][3]float64{q.BL, q.BR, q.TL, add(q.BR, q.TL, q.BL)} {
		reach = math.Max(reach, math.Sqrt(c[0]*c[0]+c[1]*c[1]+c[2]*c[2]))
	}
	v.RenderQuad(m, *q, reach)
	return m
}

// picture is an analyzer's frame, w by h: its picture posed in view v, or
// its lines laid flat.
func (h heard) picture(v rasterview.View, w, ht int) *image.RGBA {
	if h.quad != nil && h.fill {
		return drawFlat(h.quad, w, ht)
	}
	if h.quad != nil {
		return drawQuad(h.quad, v, w, ht)
	}
	return drawPlot(h.plot, w, ht)
}

// add is a + b - c: the fourth corner of the parallelogram on c, a and b.
func add(a, b, c [3]float64) [3]float64 {
	return [3]float64{a[0] + b[0] - c[0], a[1] + b[1] - c[1], a[2] + b[2] - c[2]}
}

// drawFlat lays q's picture over the whole w by h frame, face-on.
func drawFlat(q *rasterview.Quad, w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	pw, ph := q.Pic.Rect.Dx(), q.Pic.Rect.Dy()
	for y := range h {
		sy := q.Pic.Rect.Min.Y + y*ph/h
		for x := range w {
			u := (float64(x)+0.5)/float64(w) + q.Shift
			u -= math.Floor(u)
			m.SetRGBA(x, y, q.Pic.RGBAAt(q.Pic.Rect.Min.X+min(int(u*float64(pw)), pw-1), sy))
		}
	}
	return m
}

// spectroTexW is the spectrogram's history, in columns: the page's texture
// width, audioprism's own 2048.
const spectroTexW = 2048

// spectroView is the native spectrogram: its knobs, its column pipeline, and
// the scrolling picture the columns are written into, as the page writes
// them into its texture.
type spectroView struct {
	ctl  *attractor.SpectroControls
	feed attractor.SpectroFeed
	pic  *image.RGBA             // spectroTexW columns, one row per bin
	col  int                     // where the next column goes; the oldest is here too
	read int                     // the bus sample the feed has read up to
	fvf  *attractor.FVFProcessor // FVF's stream through the wobbulator
}

// spectrogram is the spectrogram now: the rack's signal since the last look
// turned into columns at the panel's knobs, on the page's 5 by 3 plane,
// scrolled so the newest column is at its right edge. Called with r.mu held.
func (r *Rack) spectrogram(nowMs float64, wobble bool) *rasterview.Quad {
	sp := &r.analyzers.spect
	for _, id := range attractor.SpectroKnobs {
		sp.ctl.Set(id, float32(r.number(id)))
	}
	resized := sp.ctl.Apply()
	rows := attractor.SpectroRows()
	if resized || sp.pic == nil || sp.pic.Rect.Dy() != rows {
		// The history goes with a new transform size: its columns are
		// spectra at the old resolution, as the page rebuilds its texture.
		sp.pic = image.NewRGBA(image.Rect(0, 0, spectroTexW, rows))
		for i := 3; i < len(sp.pic.Pix); i += 4 {
			sp.pic.Pix[i] = 255
		}
		sp.col = 0
		sp.feed.Reset(attractor.SpectroSize())
	}
	l, rt, next := r.sound.since(sp.read)
	sp.read = next
	x := sp.ctl.Fold(l, rt)
	if over := len(x) - sp.feed.Room(); over > 0 {
		x = x[over:] // the newest, as the page's bounded drain keeps
	}
	if wobble {
		// FVF: the spectrogram of the input through the wobbulator, its own
		// stream of it, as the page runs the spectrogram's drain through one.
		if sp.fvf == nil {
			sp.fvf = r.sound.wob.Processor(sampleRate)
		}
		for i, v := range x {
			x[i] = sp.fvf.Process(v)
		}
	}
	sp.feed.Add(x)
	sp.feed.Step(rows, attractor.SpectroPixel(r.mapNow()))
	sp.feed.Flush(nowMs, sampleRate, func(col []byte) {
		// The column's first row is 0 Hz, at the bottom.
		for y := 0; y < rows && 4*y+3 < len(col); y++ {
			o := sp.pic.PixOffset(sp.col, rows-1-y)
			copy(sp.pic.Pix[o:o+3], col[4*y:4*y+3])
		}
		sp.col = (sp.col + 1) % spectroTexW
	})
	return &rasterview.Quad{
		Pic:   sp.pic,
		BL:    [3]float64{-texHalfW, -texHalf, 0},
		BR:    [3]float64{texHalfW, -texHalf, 0},
		TL:    [3]float64{-texHalfW, texHalf, 0},
		Shift: float64(sp.col) / spectroTexW,
	}
}

// texHalfW is the half width of the page's spectrogram plane.
const texHalfW = 2.5

// mapNow is the MAP ring as the panel has it set, with the Palette's swatches.
// Called with r.mu held.
func (r *Rack) mapNow() colormap.Map {
	base, mid, top, _ := r.swatches()
	return colormap.Map{
		Colors: int(r.number("gradient-colors")),
		Base:   base, Mid: mid, Top: top,
		Freq: float32(r.number("rainbow-freq")),
	}
}

// followMode moves the MAP ring onto heat when the spectrogram or FVF comes
// up and hands it back when they go, as the page does: never over a choice.
// The ring is moved only from its default, and moved back only if it still
// holds what this put there. Called with r.mu held.
func (r *Rack) followMode(model string) {
	const ringDefault = "2"
	heat := strconv.Itoa(colormap.First)
	a := &r.analyzers
	cur := r.value("gradient-colors")
	if model == "spectrogram" || model == "fvf" {
		if cur == ringDefault && r.set("gradient-colors", heat) == nil {
			a.autoMap = true
		}
		return
	}
	if a.autoMap {
		a.autoMap = false
		if cur == heat {
			_ = r.set("gradient-colors", ringDefault) //nolint:errcheck // the ring takes its own default
		}
	}
}

// waterfallView is the native waterfall: its surface, and the signal it is
// built from, as the page keeps them from its tap.
type waterfallView struct {
	w    *attractor.Waterfall
	read int       // the bus sample read up to
	live []float32 // the live surface's window: the newest LiveFFT of its channel
	// ref and meas are the decay surface's capture, the newest
	// WaterfallCaptureSec of the reference and the measurement.
	ref, meas []float32
}

// waterfallFigure is the waterfall's surface now, at the panel's knobs, and
// the sphere it is framed to: the page fits the camera to the surface's
// half-span. Called with r.mu held.
func (r *Rack) waterfallFigure(nowMs float64) (attractor.Figure, [][3]float64) {
	wv := &r.analyzers.wfall
	w := wv.w
	for _, id := range attractor.WaterfallKnobs {
		w.Set(id, float32(r.number(id)))
	}
	if w.Switched(func(id string, p *float32, v float32) {
		// Turned on the panel too, so its knob shows what the surface uses.
		*p = v
		_ = r.set(id, strconv.FormatFloat(float64(v), 'f', -1, 32)) //nolint:errcheck // the knob's own default
	}) {
		wv.live, wv.ref, wv.meas = wv.live[:0], wv.ref[:0], wv.meas[:0]
	}
	l, rt, next := r.sound.since(wv.read)
	wv.read = next
	if w.Live() {
		n := w.LiveFFT()
		if len(wv.live) > n {
			// FFT turned: the surface is spectra of a window that is gone.
			wv.live = wv.live[:0]
			w.DropSurface()
		}
		wv.live = newest(append(wv.live, w.Fold(l, rt)...), n)
		if len(wv.live) == n {
			w.PushLive(wv.live, sampleRate, nowMs)
		}
	} else {
		if w.Swapped() {
			l, rt = rt, l
		}
		want := sampleRate * attractor.WaterfallCaptureSec
		wv.ref = newest(append(wv.ref, l...), want)
		wv.meas = newest(append(wv.meas, rt...), want)
		// No sweep of the rack's own to trigger on: it measures on the
		// page's free-running clock, as the page does with an external sweep.
		if len(wv.ref) == want && w.MeasureDue(0, nowMs) {
			w.Measure(wv.ref, wv.meas, sampleRate)
		}
	}
	s := attractor.WaterfallSpan
	return w.Figure(), [][3]float64{{-s, 0, 0}, {s, 0, 0}}
}

// newest is the last n of x, moved to the front of its own array so it does
// not grow without end.
func newest(x []float32, n int) []float32 {
	if over := len(x) - n; over > 0 {
		x = append(x[:0], x[over:]...)
	}
	return x
}

// morphView is the native Sprott Morph: its run, its reach when entered, and
// when it last stepped.
type morphView struct {
	run   *attractor.SprottMorph
	steps int // the trail's length it was entered at
	reach float64
	last  time.Time
	frac  float64
}

// morphFigure steps the Sprott Morph on to now, while running, and is its
// trail and the sphere it was framed to when entered, as the page fits its
// camera to the warmed trail. Called with r.mu held.
func (r *Rack) morphFigure(running bool) (attractor.Figure, [][3]float64) {
	mv := &r.analyzers.morph
	for _, id := range attractor.SprottMorphKnobs {
		mv.run.Set(id, float32(r.number(id)))
	}
	steps := max(int(r.number("trail-slider")), 2)
	now := r.sound.now()
	if !mv.run.Active() || steps != mv.steps {
		mv.reach, mv.steps = mv.run.Enter(steps), steps
		mv.last, mv.frac = now, 0
	}
	if running {
		// The page steps its sub-steps a frame at 60 a second; the panel's
		// frames come slower, so each look steps the frames it stood in for.
		sub, scale := speedSteps(r.number("speed-slider"))
		frames := min(now.Sub(mv.last).Seconds()*60, 60) + mv.frac
		n := int(frames)
		mv.frac = frames - float64(n)
		if n > 0 {
			mv.run.Advance(n*sub, float32(scale), steps)
		}
	}
	mv.last = now
	f := attractor.Figure{Kind: attractor.FigurePath, Points: make([][3]float64, steps)}
	mv.run.Trail(steps, func(k int, x, y, z float64) { f.Points[k] = [3]float64{x, y, z} })
	return f, [][3]float64{{-mv.reach, 0, 0}, {mv.reach, 0, 0}}
}
