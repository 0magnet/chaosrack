package rackhost

import (
	"math"
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/meters"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// The Mod module, natively: every audio-modulation route on one pin matrix,
// and the selected route's depth and EQ under it, as the page's
// (modmatrix_js.go, which says why the matrix is laid out as it is). A row
// is a destination — the running model's parameters, then the view's — and
// a column a source: the rack's signal, the Mixer's sends, or the model
// itself. Each frame the routed knobs read what their routes make of the
// sources (attractor/modroute.go), in place of the values they are set to.

// modRoute is one destination's route: the source it follows, the bands its
// EQ weighs, its depth, and its shape (attractor.ModShapes).
type modRoute struct {
	channel string
	bands   [attractor.ModBandCount]float32
	level   float32
	shape   string
}

// on reports whether the route moves anything.
func (m modRoute) on() bool { return m.channel != "" && m.level != 0 }

// modView is the matrix and what its routes make.
type modView struct {
	routes map[string]modRoute
	sel    string // the selected row's destination
	rows   []string
	// nParams is how many rows are the model's parameters; the rest are the
	// view's (attractor.ModViewRows).
	nParams int
	// The matrix's parts in the picture: each row's name display, each pin
	// by row and column, and the EQ's bars.
	legend []int
	pins   [][]int
	eq     [attractor.ModBandCount]int
	lit    map[int]string
	// bands follow the sources' spectra; model the model's own output, as a
	// source, smoothed; hold the whole-step knobs' last values.
	bands attractor.ModBands
	model map[string]float32
	hold  map[string]float32
	// shapers are each route's memory for its shape, by destination.
	shapers map[string]*attractor.ModShaper
	// over is what the routed knobs read this frame, by control.
	over map[string]float64
	// head and extent are where the model's trail is now and how far it
	// reaches, which the model sources follow.
	head   [3]float64
	extent float64
}

// The pins' looks past dark (lamp.go's pinDark): lit by a route, lit by a
// route turned negative, and on a row with nothing to route (the page's
// .mxpin .on, .neg and .mxpin-none).
const (
	pinOn   = 0xffd23c
	pinNeg  = 0x3cd2ff
	pinNone = 0x0e1217
)

// findMod finds the matrix's parts in the picture, by the marks the page
// tags them with. The matrix lies in the bank, so they are found again with
// every bank; the routes stay. Called with r.mu held.
func (r *Rack) findMod() {
	mv := &r.mod
	if mv.routes == nil {
		mv.routes = map[string]modRoute{}
		mv.model, mv.hold = map[string]float32{}, map[string]float32{}
		mv.shapers = map[string]*attractor.ModShaper{}
	}
	mv.lit = map[int]string{}
	mv.legend, mv.pins, mv.eq = nil, nil, [attractor.ModBandCount]int{}
	nSrc := len(attractor.ModChannels())
	for i, it := range r.pic.Items {
		if it == nil {
			continue
		}
		f := strings.Fields(it.Mark)
		if len(f) != 2 {
			continue
		}
		switch {
		case strings.HasPrefix(f[1], "mxleg-"):
			row, err := strconv.Atoi(strings.TrimPrefix(f[1], "mxleg-"))
			if err != nil {
				continue
			}
			for len(mv.legend) <= row {
				mv.legend = append(mv.legend, -1)
			}
			mv.legend[row] = i
		case strings.HasPrefix(f[1], "mxpin-"):
			rs := strings.Split(strings.TrimPrefix(f[1], "mxpin-"), "-")
			if len(rs) != 2 {
				continue
			}
			row, e1 := strconv.Atoi(rs[0])
			src, e2 := strconv.Atoi(rs[1])
			if e1 != nil || e2 != nil || src >= nSrc {
				continue
			}
			for len(mv.pins) <= row {
				mv.pins = append(mv.pins, make([]int, nSrc))
			}
			mv.pins[row][src] = i + 1
		case strings.HasPrefix(f[1], "mxeq-"):
			if b, err := strconv.Atoi(strings.TrimPrefix(f[1], "mxeq-")); err == nil && b < len(mv.eq) {
				mv.eq[b] = i + 1
			}
		}
	}
	mv.nParams = max(len(mv.legend)-len(attractor.ModViewRows), 0)
	mv.rows = make([]string, len(mv.legend))
}

// modRetarget points the matrix at model's parameters: each row's name,
// DEST's list, and a selection still on the module. Called with r.mu held,
// when the model changes.
func (r *Rack) modRetarget(model string) {
	mv := &r.mod
	if len(mv.rows) == 0 {
		return
	}
	var ps []attractor.ModelParam
	if model == "custom" {
		for _, name := range r.custom.Params() {
			ps = append(ps, attractor.ModelParam{ID: "custom-" + name, Label: name})
		}
	} else {
		ps = attractor.ModelParams(model)
	}
	var ids, labels []string
	for i := range mv.rows {
		id, label := "", ""
		switch {
		case i >= mv.nParams && i-mv.nParams < len(attractor.ModViewRows):
			v := attractor.ModViewRows[i-mv.nParams]
			id, label = v.ID, v.Label
		case i < mv.nParams && i < len(ps):
			id, label = ps[i].ID, ps[i].Label
		}
		mv.rows[i] = id
		if mv.legend[i] >= 0 {
			r.write(mv.legend[i], attractor.DisplayText(label))
		}
		if id != "" {
			ids, labels = append(ids, id), append(labels, attractor.DisplayText(label))
		}
	}
	// DEST lists the rows that route something, by name.
	if i, ok := r.byID["modmx-dest"]; ok {
		c := &r.ctls[i]
		c.Options, c.Labels = ids, labels
	}
	if r.modRow(mv.sel) < 0 {
		// The selection moved off the module: the first routed row, or the
		// first row.
		mv.sel = ""
		for _, id := range mv.rows {
			if id != "" && mv.routes[id].on() {
				mv.sel = id
				break
			}
		}
		if mv.sel == "" && len(ids) > 0 {
			mv.sel = ids[0]
		}
	}
	r.modSelect(mv.sel)
	for i := range mv.rows {
		r.modLight(i)
	}
}

// modRow is the row destination id is on, or -1.
func (r *Rack) modRow(id string) int {
	for i, c := range r.mod.rows {
		if id != "" && c == id {
			return i
		}
	}
	return -1
}

// modSelect selects destination id's row: DEST names it, DPTH reads its
// depth, SHAPE its shape, the EQ its bands. Called with r.mu held.
func (r *Rack) modSelect(id string) {
	mv := &r.mod
	if r.modRow(id) < 0 {
		return
	}
	mv.sel = id
	if i, ok := r.byID["modmx-dest"]; ok && r.ctls[i].Value != id {
		r.ctls[i].Value = id
		r.turn(&r.ctls[i])
	}
	if i, ok := r.byID["modmx-depth"]; ok {
		c := &r.ctls[i]
		if v := strconv.FormatFloat(float64(mv.routes[id].level), 'f', -1, 32); c.Value != v {
			c.Value = v
			r.turn(c)
		}
	}
	if i, ok := r.byID["modmx-shape"]; ok {
		r.ctls[i].Value = mv.routes[id].shape
	}
	if i, ok := r.byID["modmx-src"]; ok {
		m := mv.routes[id]
		c := &r.ctls[i]
		c.Value = ""
		if m.on() {
			c.Value = m.channel
		}
	}
	r.placeSweptMarks() // the selection's rim and the EQ's bars are overlays
}

// modLight lights row i's pins as its route stands: lit on its source, the
// brighter the deeper, blue when it pulls the other way. Called with r.mu
// held.
func (r *Rack) modLight(i int) {
	mv := &r.mod
	if i < 0 || i >= len(mv.pins) || i >= len(mv.rows) {
		return
	}
	id := mv.rows[i]
	m := mv.routes[id]
	for j, ch := range attractor.ModChannels() {
		at := mv.pins[i][j] - 1
		if at < 0 {
			continue
		}
		look, fill := "dark", pinDark
		switch {
		case id == "":
			look, fill = "none", pinNone
		case m.on() && m.channel == ch.Name:
			// The page's opacity, 0.45 at no depth to whole at one, over
			// the panel's dark.
			d := min(1, 0.45+0.55*math.Abs(float64(m.level)))
			col := pinOn
			if m.level < 0 {
				col = pinNeg
			}
			fill = mixRGB(pinDark, col, d)
			look = strconv.Itoa(fill)
		}
		if mv.lit[at] == look {
			continue
		}
		mv.lit[at] = look
		r.recolor(at, fill, -1)
	}
}

// modPress does what a click on a matrix part at x, y does: a pin routes its
// column to its row, or takes the route away; a row's name selects it; an
// EQ bar sets its band to where on it the click is. It reports whether
// the click was on one. Called with r.mu held.
func (r *Rack) modPress(mark string, y float64) bool {
	mv := &r.mod
	f := strings.Fields(mark)
	if len(f) != 2 || mv.routes == nil {
		return false
	}
	switch {
	case strings.HasPrefix(f[1], "mxpin-"):
		rs := strings.Split(strings.TrimPrefix(f[1], "mxpin-"), "-")
		row, _ := strconv.Atoi(rs[0]) //nolint:errcheck // findMod read it
		src, _ := strconv.Atoi(rs[1]) //nolint:errcheck // as above
		chs := attractor.ModChannels()
		if row >= len(mv.rows) || src >= len(chs) || mv.rows[row] == "" {
			return true // a row with nothing to route takes the click and does nothing
		}
		id, ch := mv.rows[row], chs[src].Name
		m := mv.routes[id]
		if m.channel == ch && m.level != 0 {
			m.channel = "" // a lit pin takes its route away
		} else {
			m.channel = ch
			if m.level == 0 {
				m.level = modDefDepth
			}
		}
		mv.routes[id] = m
		r.modSelect(id)
		r.modLight(row)
		return true
	case strings.HasPrefix(f[1], "mxleg-"):
		row, err := strconv.Atoi(strings.TrimPrefix(f[1], "mxleg-"))
		if err == nil && row < len(mv.rows) && mv.rows[row] != "" {
			r.modSelect(mv.rows[row])
		}
		return true
	case strings.HasPrefix(f[1], "mxeq-"):
		b, err := strconv.Atoi(strings.TrimPrefix(f[1], "mxeq-"))
		if err != nil || b >= len(mv.eq) || mv.eq[b] == 0 || mv.sel == "" {
			return true
		}
		it := r.pic.Items[mv.eq[b]-1]
		v := 1 - (y-it.Y)/it.H
		if v < 0.06 {
			v = 0 // the bottom of the bar clears the band, as the page snaps it
		}
		m := mv.routes[mv.sel]
		m.bands[b] = float32(min(v, 1))
		mv.routes[mv.sel] = m
		r.placeSweptMarks()
		return true
	}
	return false
}

// modPressAt presses the matrix part at x, y, if one is there. Called with
// r.mu held.
func (r *Rack) modPressAt(x, y float64) bool {
	mv := &r.mod
	at := func(i int) bool {
		it := r.pic.Items[i]
		return it != nil && x >= it.X && x <= it.X+it.W && y >= it.Y && y <= it.Y+it.H
	}
	for _, row := range mv.pins {
		for _, p := range row {
			if p > 0 && at(p-1) {
				return r.modPress(r.pic.Items[p-1].Mark, y)
			}
		}
	}
	for _, l := range mv.legend {
		if l >= 0 && at(l) {
			return r.modPress(r.pic.Items[l].Mark, y)
		}
	}
	for _, b := range mv.eq {
		if b > 0 && at(b-1) {
			return r.modPress(r.pic.Items[b-1].Mark, y)
		}
	}
	return false
}

// modDefDepth is the depth a route is made with when it had none.
const modDefDepth = 0.4

// modSet acts on DEST, SRC, DPTH or SHAPE turned: DEST selects a row, the
// others set the selected route's source, depth or shape. Called with r.mu
// held.
func (r *Rack) modSet(id string) {
	mv := &r.mod
	switch id {
	case "modmx-dest":
		r.modSelect(r.value(id))
	case "modmx-src":
		if mv.sel == "" || mv.routes == nil {
			return
		}
		m := mv.routes[mv.sel]
		m.channel = r.value(id)
		if m.channel != "" && m.level == 0 {
			m.level = modDefDepth
		}
		mv.routes[mv.sel] = m
		r.modSelect(mv.sel)
		r.modLight(r.modRow(mv.sel))
	case "modmx-shape":
		if v := r.value(id); mv.sel != "" && mv.routes != nil && attractor.IsModShape(v) {
			m := mv.routes[mv.sel]
			m.shape = v
			mv.routes[mv.sel] = m
		}
	case "modmx-depth":
		if mv.sel == "" || mv.routes == nil {
			return
		}
		v := r.number(id)
		if math.Abs(v) < 0.005 {
			v = 0
		}
		m := mv.routes[mv.sel]
		m.level = float32(v)
		mv.routes[mv.sel] = m
		r.modLight(r.modRow(mv.sel))
	}
}

// modRouted reports whether any route moves anything. Called with r.mu held.
func (r *Rack) modRouted() bool {
	for _, m := range r.mod.routes {
		if m.on() {
			return true
		}
	}
	return false
}

// modOverlays are the matrix's own marks over the picture: the selected
// row's rim, and the EQ bars filled to the selected route's bands.
func (r *Rack) modOverlays() []*rackpic.Item {
	mv := &r.mod
	row := r.modRow(mv.sel)
	if row < 0 || row >= len(mv.legend) || mv.legend[row] < 0 {
		return nil
	}
	leg := r.pic.Items[mv.legend[row]]
	rim, fill := 0xff3b30, 0x6cf
	out := []*rackpic.Item{{X: leg.X - 3, Y: leg.Y - 3, W: leg.W + 6, H: leg.H + 6, Border: &rim}}
	m := mv.routes[mv.sel]
	for b, at := range mv.eq {
		if at == 0 || m.bands[b] <= 0 {
			continue
		}
		bar := r.pic.Items[at-1]
		h := bar.H * float64(m.bands[b])
		out = append(out, &rackpic.Item{X: bar.X, Y: bar.Y + bar.H - h, W: bar.W, H: h, Fill: &fill})
	}
	return out
}

// modulate works out what every routed knob reads this frame, from the
// sources now, for model on show: the routed knobs then read it (number).
// Called with r.mu held, once a frame.
func (r *Rack) modulate(model string) {
	mv := &r.mod
	mv.over = nil
	if !r.modRouted() {
		return
	}
	r.modListen()
	over := map[string]float64{}
	value := func(id string, m modRoute) float32 {
		var f float32
		if attractor.IsModelModSource(m.channel) {
			f = r.modModel(m.channel)
		} else {
			f = mv.bands.Value(m.channel, m.bands[:])
		}
		s := mv.shapers[id]
		if s == nil {
			s = &attractor.ModShaper{}
			mv.shapers[id] = s
		}
		return s.Signal(m.shape, m.channel, f)
	}
	ctl := func(id string) (base, lo, hi, step float64, ok bool) {
		i, has := r.byID[id]
		if !has {
			return 0, 0, 0, 0, false
		}
		c := r.ctls[i]
		v, err := strconv.ParseFloat(c.Value, 64)
		return v, c.Min, c.Max, c.Step, err == nil && c.Max > c.Min
	}
	for i, id := range mv.rows {
		m := mv.routes[id]
		if id == "" || !m.on() {
			continue
		}
		f := value(id, m)
		if i >= mv.nParams {
			r.modView(over, attractor.ModViewRows[i-mv.nParams], m, f, model)
			continue
		}
		base, lo, hi, step, ok := ctl(id)
		if !ok {
			continue
		}
		held, has := mv.hold[id]
		v := attractor.ModTarget(float32(base), m.level, f, float32(lo), float32(hi), float32(step), held, has)
		mv.hold[id] = v
		over[id] = float64(v)
	}
	mv.over = over
	if len(over) > 0 {
		r.refigure.Store(true) // the model is drawn at its routed knobs
	}
}

// modView puts a view row's route on the control it moves, as the page moves
// the view: not under the audio models and the spectrogram's surfaces,
// which own their own view.
func (r *Rack) modView(over map[string]float64, row attractor.ModViewRow, m modRoute, f float32, model string) {
	if isAnalyzer(model) || attractor.IsAudioModel(model) {
		return
	}
	i, ok := r.byID[row.Control]
	if !ok {
		return
	}
	base, err := strconv.ParseFloat(r.ctls[i].Value, 64)
	if err != nil {
		return
	}
	span := float64(row.Max - row.Min)
	switch row.ID {
	case "view-trail":
		// A fraction of the trail taken away.
		over[row.Control] = base * float64(min(max(1-m.level*f, row.Min), row.Max))
	case "view-pshift":
		over[row.Control] = float64(colormap.WrapShift(float32(base) + m.level*f*float32(span)))
	default:
		over[row.Control] = min(max(base+float64(m.level*f)*span, float64(row.Min)), float64(row.Max))
	}
}

// modListen follows the sources' spectra a frame: the rack's signal, left,
// right and their mix, and the Mixer's two sends. Called with r.mu held.
func (r *Rack) modListen() {
	mv := &r.mod
	l, rt := r.sound.window(colorFFT)
	if len(l) == colorFFT {
		bl := attractor.ComputeBands(meters.ComputeFFTMags(l), sampleRate)
		br := attractor.ComputeBands(meters.ComputeFFTMags(rt), sampleRate)
		mono := make([]float32, len(bl))
		for i := range mono {
			mono[i] = (bl[i] + br[i]) / 2
		}
		mv.bands.Store("L", bl)
		mv.bands.Store("R", br)
		mv.bands.Store("mono", mono)
	}
	a, b := r.sound.modWindow(colorFFT)
	if len(a) == colorFFT {
		chs := attractor.ModChannels()
		mv.bands.Store(chs[3].Name, attractor.ComputeBands(meters.ComputeFFTMags(a), sampleRate))
		mv.bands.Store(chs[4].Name, attractor.ComputeBands(meters.ComputeFFTMags(b), sampleRate))
	}
}

// modModel is model source src now, 0..1, smoothed as the page smooths it
// (modelmod.go): the model's head across its reach, or its distance out.
func (r *Rack) modModel(src string) float32 {
	mv := &r.mod
	h, ext := mv.head, float32(mv.extent)
	var target float32
	switch src {
	case "mx", "my", "mz":
		target = attractor.ModelModNorm(float32(h[map[string]int{"mx": 0, "my": 1, "mz": 2}[src]]), ext)
	default:
		if ext > 0 {
			target = min(float32(math.Sqrt(h[0]*h[0]+h[1]*h[1]+h[2]*h[2]))/ext, 1)
		}
	}
	v := attractor.ModelModSmooth(mv.model[src], target)
	mv.model[src] = v
	return v
}

// modHead records where the model's trail is now and how far the figure
// reaches, for the model sources. Called with r.mu held.
func (r *Rack) modHead(pts, fit [][3]float64) {
	if len(pts) == 0 {
		return
	}
	mv := &r.mod
	mv.head = pts[len(pts)-1]
	if len(fit) == 0 {
		fit = pts
	}
	ext := 0.0
	for _, p := range fit {
		ext = max(ext, math.Abs(p[0]), math.Abs(p[1]), math.Abs(p[2]))
	}
	mv.extent = ext
}

// modWindow is the newest n samples of the Mixer's sends, MOD A and MOD B.
func (s *sound) modWindow(n int) (a, b []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mod.window(n)
}

// modFollow records the model's head for the model sources, when a route
// follows one. Called with r.mu held.
func (r *Rack) modFollow(pts, fit [][3]float64) {
	for _, m := range r.mod.routes {
		if m.on() && attractor.IsModelModSource(m.channel) {
			r.modHead(pts, fit)
			return
		}
	}
}

// addModControls gives the rack the selected route's source as a selector,
// SRC: the pins, which a mouse presses, as a control a keyboard turns too.
// The page has no such control; its pins are the source. And SHAPE, which
// the page has, for a catalog captured before it did.
func (r *Rack) addModControls() {
	var shapes, shapeLabels []string
	for _, m := range attractor.ModShapes {
		shapes, shapeLabels = append(shapes, m.Value), append(shapeLabels, m.Label)
	}
	r.addControl(racktui.Control{
		ControlInfo: controlspec.ControlInfo{ID: "modmx-shape", Label: "shape", IsSelect: true, Module: "mod"},
		Options:     shapes, Labels: shapeLabels,
	})
	opts, labels := []string{""}, []string{"off"}
	for _, ch := range attractor.ModChannels() {
		opts, labels = append(opts, ch.Name), append(labels, ch.Key)
	}
	r.addControl(racktui.Control{
		ControlInfo: controlspec.ControlInfo{ID: "modmx-src", Label: "src", IsSelect: true, Module: "mod"},
		Options:     opts, Labels: labels,
	})
}
