package rackhost

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
	"github.com/0magnet/chaosrack/pkg/scope"
)

// The rack's four scopes, natively: each tube drawn from the same controls
// and the same pkg/scope arithmetic as the page's (rackscope_js.go, which
// says why each part is as it is) — the afterglow INTENSITY sets, the lit
// graticule, the trigger, IN 1, IN 2, SUM or X-Y, and the two inputs, any of
// the rack's signal, the capture, a generator or a coordinate of the model.

// The inputs, in the order the INPUT knobs turn through them, as the page's
// (rackscopeinputs_js.go) — the index is the knob's position.
const (
	scopeInRackL = iota
	scopeInRackR
	scopeInCapL
	scopeInCapR
	scopeInGen1 // through scopeInGen1+3: Gen 1 to 4
	_
	_
	_
	scopeInModelX
	scopeInModelY
	scopeInModelZ
	scopeInCount
)

// scopeInNames are the inputs as the tube prints them.
var scopeInNames = [scopeInCount]string{
	"RACK L", "RACK R", "CAP L", "CAP R",
	"GEN 1", "GEN 2", "GEN 3", "GEN 4",
	"MODEL X", "MODEL Y", "MODEL Z",
}

// The SOURCE positions.
const (
	scopeChanL = iota
	scopeChanR
	scopeChanMid
	scopeChanXY
)

// scopeCount is how many scopes the rack has.
const scopeCount = 4

// scopePrefix is scope n's id prefix, from 0: "scope", then "scope2" on.
func scopePrefix(n int) string {
	if n == 0 {
		return "scope"
	}
	return "scope" + strconv.Itoa(n+1)
}

// scopeUI is a scope's front panel, read off its controls.
type scopeUI struct {
	voltsPerDiv, secPerDiv float64
	vpos, hpos, trig       float64
	rising, auto           bool
	power                  bool
	illum, intens, focus   float64
	grat                   bool
	chanSel                int
	in                     [2]int
	beam                   color.RGBA // the phosphor, the rack's own when one is chosen
}

// scopeTube is a scope's phosphor: the face as it was last drawn, which the
// next frame fades rather than clears.
type scopeTube struct {
	face *image.RGBA
	// model is the scope's own copy of Model Out, so probing the model moves
	// nothing the speakers play.
	model modelVoice
	mo    [3][]float32
}

// scopeUIOf reads scope n's front panel. Called with r.mu held.
func (r *Rack) scopeUIOf(n int) scopeUI {
	p := scopePrefix(n) + "-"
	idx := func(id string, steps []float64, def int) float64 {
		i, err := strconv.Atoi(r.value(p + id))
		if err != nil || i < 0 || i >= len(steps) {
			i = def
		}
		return steps[i]
	}
	illum := r.number(p + "illum")
	ui := scopeUI{
		voltsPerDiv: idx("volts", scope.VoltsDivs, scope.NearestStep(scope.VoltsDivs, 0.5)),
		secPerDiv:   idx("time", scope.Timebases, scope.NearestStep(scope.Timebases, 2e-3)),
		vpos:        r.number(p + "vpos"),
		hpos:        r.number(p + "hpos"),
		trig:        r.number(p + "trig"),
		rising:      r.value(p+"slope") != "1",
		auto:        r.value(p+"tmode") != "1",
		// SCALE ILLUM's OFF detent is the scope's power, as the page's
		// switch-pot reads it: at or below half its negative end.
		power:  illum > -0.1,
		illum:  max(illum, 0),
		intens: r.number(p + "intens"),
		focus:  r.number(p + "focus"),
		grat:   r.value("scope-grat") != "0",
	}
	ui.beam = scopeBeam
	if ph, ok := r.phosphor(); ok {
		ui.beam = color.RGBA{uint8(ph.TR * 255), uint8(ph.TG * 255), uint8(ph.TB * 255), 255} //nolint:gosec // 0..1 each
	}
	if c, err := strconv.Atoi(r.value(p + "chan")); err == nil {
		ui.chanSel = c
	}
	for ch := range 2 {
		in := int(r.number(p+"in"+strconv.Itoa(ch+1)) + 0.5)
		ui.in[ch] = max(0, min(in, scopeInCount-1))
	}
	return ui
}

// scopeFeed fills dst with what input in carries, newest sample last: as much
// of it as there is, the rest silence. Called with the sound's lock held.
func (s *sound) scopeFeed(t *scopeTube, in int, dst []float32) {
	clear(dst)
	right := func(src []float32) { copy(dst[len(dst)-min(len(src), len(dst)):], src[max(len(src)-len(dst), 0):]) }
	switch {
	case in <= scopeInRackR:
		l, r := s.rack.window(len(dst))
		right([2][]float32{l, r}[in-scopeInRackL])
	case in <= scopeInCapR:
		l, r := s.rack.capWindow(len(dst))
		right([2][]float32{l, r}[in-scopeInCapL])
	case in < scopeInGen1+audiosrc.OscCount:
		// A probe on the generator's own output, at its level: its shadow,
		// so the scope moves nothing the rack plays.
		i := in - scopeInGen1
		s.fg.RenderOsc(i, dst)
		for k := range dst {
			dst[k] *= float32(s.level[i])
		}
	default:
		t.model.want, t.model.speed, t.model.level = s.model.want, s.model.speed, s.model.level
		t.model.render(&t.mo, len(dst))
		copy(dst, t.mo[in-scopeInModelX])
	}
}

// scopeBeam is the phosphor, P31 green: the page's tube when no other is
// chosen.
var scopeBeam = color.RGBA{89, 255, 115, 255}

// scopeDark is the face with nothing on it.
var scopeDark = color.RGBA{5, 7, 10, 255}

// scopeFrame is one frame of scope n's tube, w by h. Called with r.mu held.
func (r *Rack) scopeFrame(n, w, h int) *image.RGBA {
	ui := r.scopeUIOf(n)
	t := &r.tubes[n]
	if t.face == nil || t.face.Rect.Dx() != w || t.face.Rect.Dy() != h {
		t.face = image.NewRGBA(image.Rect(0, 0, w, h))
		fillRGBA(t.face, scopeDark, 1)
	}
	m := t.face
	if !ui.power {
		// Off, the lines are still cut into the glass, faintly, with no light
		// behind them.
		fillRGBA(m, scopeDark, 1)
		drawGraticule(m, 0.25)
		return m
	}
	// The afterglow: painted over rather than cleared, as a phosphor fades.
	fillRGBA(m, scopeDark, 0.12+0.5*(1-ui.intens))
	if ui.grat {
		drawGraticule(m, ui.illum)
	}
	r.drawTrace(t, &ui, m)
	drawText(m, int(float64(h)/13*0.6), int(float64(h)/13*0.5),
		"1 "+scopeInNames[ui.in[0]]+"   2 "+scopeInNames[ui.in[1]], ui.beam, 0.35+0.5*ui.intens)
	return m
}

// drawTrace sweeps the scope's inputs across the face.
func (r *Rack) drawTrace(t *scopeTube, ui *scopeUI, m *image.RGBA) {
	span := max(scope.SweepSamples(ui.secPerDiv, sampleRate), 2)
	// One screen behind the window for the trigger to search, as the page's.
	need := min(2*span, busRing)
	span = min(span, need)
	l, rt := make([]float32, need), make([]float32, need)
	s := r.sound
	s.mu.Lock()
	s.scopeFeed(t, ui.in[0], l)
	s.scopeFeed(t, ui.in[1], rt)
	s.mu.Unlock()

	vert := l
	switch ui.chanSel {
	case scopeChanR:
		vert = rt
	case scopeChanMid:
		vert = make([]float32, need)
		for i := range vert {
			vert[i] = (l[i] + rt[i]) * 0.5
		}
	}
	start := need - span // the newest whole window, which is what a free run shows
	if i := scope.TriggerIndex(vert, float32(ui.trig), ui.rising, span); i >= 0 {
		start = i
	} else if !ui.auto {
		return // NORM: no edge, no sweep; the face keeps fading
	}
	start = max(min(start, len(vert)-span), 0)

	w, h := float64(m.Rect.Dx()), float64(m.Rect.Dy())
	px, py := w/float64(scope.DivX), h/float64(scope.DivY)
	cx, cy := w/2, h/2
	blur := scope.Defocus(ui.focus)
	width := 1.0 + 2.2*blur
	alpha := 0.12 + 0.88*ui.intens
	var pts [][2]float64
	if ui.chanSel == scopeChanXY {
		for i := range span {
			pts = append(pts, [2]float64{
				cx + scope.YDiv(l[start+i], ui.voltsPerDiv, ui.hpos)*px,
				cy - scope.YDiv(rt[start+i], ui.voltsPerDiv, ui.vpos)*py,
			})
		}
	} else {
		// More samples than columns: the envelope, which is what the dense
		// trace looks like anyway (pkg/scope/trace.go).
		cols := scope.TraceCols(w)
		seg := vert[start : start+span]
		if span > 2*cols {
			env := make([]float32, 2*cols)
			k := scope.TraceEnvelope(env, seg, cols)
			for c := range k {
				x := cx + (float64(c)/float64(max(k-1, 1))-0.5+ui.hpos/float64(scope.DivX))*w
				lo := cy - scope.YDiv(env[2*c], ui.voltsPerDiv, ui.vpos)*py
				hi := cy - scope.YDiv(env[2*c+1], ui.voltsPerDiv, ui.vpos)*py
				if c%2 == 1 {
					lo, hi = hi, lo
				}
				pts = append(pts, [2]float64{x, lo}, [2]float64{x, hi})
			}
		} else {
			for i, v := range seg {
				x := cx + (float64(i)/float64(max(span-1, 1))-0.5+ui.hpos/float64(scope.DivX))*w
				pts = append(pts, [2]float64{x, cy - scope.YDiv(v, ui.voltsPerDiv, ui.vpos)*py})
			}
		}
	}
	// The halo a real spot has, then the spot: FOCUS tightens both.
	halo := (4 + 10*blur) / 14 * (0.25 + 0.75*ui.intens)
	strokePolyline(m, pts, width*3, ui.beam, 0.18*halo)
	strokePolyline(m, pts, width, ui.beam, alpha)
}

// drawGraticule rules the face, lit lit of the way up.
func drawGraticule(m *image.RGBA, lit float64) {
	w, h := float64(m.Rect.Dx()), float64(m.Rect.Dy())
	px, py := w/float64(scope.DivX), h/float64(scope.DivY)
	cx, cy := w/2, h/2
	// Ticks first, so the heavier lines land on top where they cross.
	for _, weight := range [3]scope.Weight{scope.WeightTick, scope.WeightDiv, scope.WeightAxis} {
		c, a := color.RGBA{120, 155, 185, 255}, 0.40
		switch weight {
		case scope.WeightDiv:
			a = 0.60
		case scope.WeightAxis:
			c, a = color.RGBA{150, 190, 220, 255}, 1.0
		}
		for _, l := range scope.Graticule() {
			if l.W != weight {
				continue
			}
			strokePolyline(m, [][2]float64{
				{cx + float64(l.X0)*px, cy - float64(l.Y0)*py},
				{cx + float64(l.X1)*px, cy - float64(l.Y1)*py},
			}, 1, c, a*lit)
		}
	}
}

// fillRGBA lays c over the whole of m at alpha a.
func fillRGBA(m *image.RGBA, c color.RGBA, a float64) {
	for i := 0; i+3 < len(m.Pix); i += 4 {
		blendPix(m.Pix[i:i+4], c, a)
	}
}

// blendPix lays c over pixel p at alpha a.
func blendPix(p []uint8, c color.RGBA, a float64) {
	k := max(0, min(a, 1))
	p[0] = uint8(float64(p[0])*(1-k) + float64(c.R)*k) //nolint:gosec // a blend of two bytes is a byte
	p[1] = uint8(float64(p[1])*(1-k) + float64(c.G)*k) //nolint:gosec // a blend of two bytes is a byte
	p[2] = uint8(float64(p[2])*(1-k) + float64(c.B)*k) //nolint:gosec // a blend of two bytes is a byte
	p[3] = 255
}

// strokePolyline draws pts joined, width pixels across, in c at alpha a.
// Each pixel is lit once a segment, so a line crossing itself is no brighter
// than a line, as a stroked canvas path is not.
func strokePolyline(m *image.RGBA, pts [][2]float64, width float64, c color.RGBA, a float64) {
	if len(pts) == 0 || a <= 0 {
		return
	}
	b := m.Rect
	r := max(width/2, 0.5)
	seen := make([]bool, len(m.Pix)/4)
	dot := func(x, y float64) {
		for yy := int(math.Floor(y - r + 0.5)); yy <= int(math.Floor(y+r-0.5)); yy++ {
			for xx := int(math.Floor(x - r + 0.5)); xx <= int(math.Floor(x+r-0.5)); xx++ {
				if xx < b.Min.X || yy < b.Min.Y || xx >= b.Max.X || yy >= b.Max.Y {
					continue
				}
				i := m.PixOffset(xx, yy)
				if !seen[i/4] {
					seen[i/4] = true
					blendPix(m.Pix[i:i+4], c, a)
				}
			}
		}
	}
	dot(pts[0][0], pts[0][1])
	for i := 1; i < len(pts); i++ {
		x0, y0, x1, y1 := pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1]
		steps := int(math.Ceil(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))))
		for s := 1; s <= steps; s++ {
			f := float64(s) / float64(steps)
			dot(x0+(x1-x0)*f, y0+(y1-y0)*f)
		}
	}
}
