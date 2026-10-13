// Package rackhost is the rack without a browser: what `chaosrack` runs on a
// host, where there is no page to drive.
//
// The panel is the page's, as the catalog recorded it (pkg/rackcatalog): its
// modules, its controls and the picture the page look draws. The values are
// this Rack's own — set by the panel, kept here, and shown by turning the
// picture's knob pointers — and the model is drawn here too, from the same
// figures and trajectories `chaosrack render` draws, with no GPU.
//
// What the page computes in DOM and Web Audio is not here yet: a control's
// value is held and shown, but only the model, its rotation and its zoom act
// on anything. The modules come over one at a time.
package rackhost

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/geom"
	"github.com/0magnet/chaosrack/pkg/rackcatalog"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racksurface"
	"github.com/0magnet/chaosrack/pkg/racktui"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// Rack is a rack held in this process.
type Rack struct {
	mu  sync.Mutex
	cat *rackcatalog.Catalog

	ctls []racktui.Control // the catalog's, with this rack's values
	byID map[string]int

	pic     *rackpic.Picture
	ptrs    map[string]pointer // each dial's pointer, by control id
	reads   map[string]readout // each control's display, by control id
	changed map[int]int        // item index → the generation it last changed in
	full    int                // the generation the picture was last rebuilt in

	// The model bank. The picture's items are those outside the bank region
	// (the first outside of them, in the catalog's order, never moved) and
	// then the bank as the page draws it for the model chosen; inBank is the
	// controls drawn in the region now, and calib what each was captured at.
	outside    int
	outsideBox map[string][4]float64
	inBank     map[string]bool
	calib      map[string]string
	selector   string

	scene scene

	// flow is each flow constant the rack can set, by id: the value the
	// integrator reads (dynamics.Params). refigure says one has changed
	// since the figure was built.
	flow     map[string]dynamics.Param
	refigure atomic.Bool
}

// New is a rack at the catalog's defaults.
func New() (*Rack, error) {
	cat, err := rackcatalog.Load()
	if err != nil {
		return nil, err
	}
	r := &Rack{
		cat:     cat,
		ctls:    append([]racktui.Control(nil), cat.Controls...),
		byID:    map[string]int{},
		ptrs:    map[string]pointer{},
		reads:   map[string]readout{},
		changed: map[int]int{},
	}
	for i, c := range r.ctls {
		r.byID[c.ID] = i
	}
	r.addFlowParams()
	r.calib = map[string]string{}
	for _, c := range r.ctls {
		r.calib[c.ID] = c.Value
	}
	r.selector = r.modelControl()

	p := *cat.Picture
	p.Gen = 1
	r.pic = &p
	r.outsideBox = map[string][4]float64{}
	if cat.Banks == nil {
		// A catalog without banks: the one picture, whole.
		p.Items = append([]*rackpic.Item(nil), cat.Picture.Items...)
		r.outside = len(p.Items)
		for id, b := range cat.Picture.Ctls {
			r.outsideBox[id] = b
		}
	} else {
		for _, it := range cat.Picture.Items {
			if !rackcatalog.InRegion(it, cat.BankRegion) {
				p.Items = append(p.Items, it)
			}
		}
		r.outside = len(p.Items)
		for id, b := range cat.Picture.Ctls {
			if !inBox(b, cat.BankRegion) {
				r.outsideBox[id] = b
			}
		}
	}
	p.Ctls = r.outsideBox
	r.calibrate(0, r.outside, func(string) bool { return true }, nil)
	r.useBank(r.value(r.selector))
	r.scene.start()
	return r, nil
}

// inBox reports whether a box's middle is in the region rg.
func inBox(b, rg [4]float64) bool {
	x, y := b[0]+b[2]/2, b[1]+b[3]/2
	return x >= rg[0] && x <= rg[0]+rg[2] && y >= rg[1] && y <= rg[1]+rg[3]
}

// useBank draws the model bank as the page draws it for model: its items in
// place of the last model's, its controls' boxes, and their pointers and
// readouts calibrated to it and turned to this rack's values.
func (r *Rack) useBank(model string) {
	b := r.cat.Banks[model]
	if b == nil {
		return
	}
	for id := range r.inBank {
		delete(r.ptrs, id)
		delete(r.reads, id)
	}
	r.pic.Items = append(r.pic.Items[:r.outside:r.outside], b.Items...)
	ctls := make(map[string][4]float64, len(r.outsideBox)+len(b.Ctls))
	for id, box := range r.outsideBox {
		ctls[id] = box
	}
	r.inBank = map[string]bool{}
	for id, box := range b.Ctls {
		ctls[id] = box
		r.inBank[id] = true
	}
	r.pic.Ctls = ctls
	// The bank was captured with each control at its default, and the
	// selector on this model.
	captured := func(c *racktui.Control) string {
		if c.ID == r.selector {
			return model
		}
		return r.calib[c.ID]
	}
	r.calibrate(r.outside, len(r.pic.Items), func(id string) bool { return r.inBank[id] }, captured)
	for id := range r.inBank {
		i, ok := r.byID[id]
		if !ok {
			continue
		}
		c := &r.ctls[i]
		if id != r.selector && c.Value != captured(c) {
			r.turn(c)
		}
	}
	for i := range r.changed {
		if i >= r.outside {
			delete(r.changed, i)
		}
	}
	r.pic.Gen++
	r.full = r.pic.Gen
}

var (
	_ racktui.Source   = (*Rack)(nil)
	_ racktui.Pictured = (*Rack)(nil)
)

// Modules is the catalog's.
func (r *Rack) Modules() ([]racksurface.Item, int, error) {
	return append([]racksurface.Item(nil), r.cat.Modules...), r.cat.SlotsPerRow, nil
}

// Controls is every control with its value now.
func (r *Rack) Controls() ([]racktui.Control, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]racktui.Control(nil), r.ctls...), nil
}

// Set moves one control, as a hand would. A value the control cannot take is
// refused, as the page refuses it: a select takes one of its options, a
// switch 1 or 0, and a dial a number, held to its travel.
func (r *Rack) Set(id, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("the rack has no control %q", id)
	}
	c := &r.ctls[i]
	v, err := settle(c, value)
	if err != nil {
		return fmt.Errorf("%s: %w", id, err)
	}
	if v == c.Value {
		return nil
	}
	c.Value = v
	if c.Module == modelModule {
		r.refigure.Store(true)
	}
	if p, ok := r.flow[id]; ok {
		if f, err := strconv.ParseFloat(v, 32); err == nil {
			*p.Value = float32(f)
			r.refigure.Store(true)
		}
	}
	if id == r.selector {
		r.useBank(v)
		return nil
	}
	r.turn(c)
	return nil
}

// settle is value as control c holds it, or why it cannot be.
func settle(c *racktui.Control, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch {
	case c.IsSwitch:
		switch strings.ToLower(value) {
		case "1", "on", "true", "yes", "y":
			return "1", nil
		case "0", "off", "false", "no", "n":
			return "0", nil
		}
		return "", fmt.Errorf("%q is not on or off", value)
	case c.IsSelect:
		for _, o := range c.Options {
			if o == value {
				return value, nil
			}
		}
		return "", fmt.Errorf("%q is not one of: %s", value, strings.Join(c.Options, " "))
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("%q is not a number", value)
	}
	if c.Max > c.Min {
		f = min(max(f, c.Min), c.Max)
	}
	return strconv.FormatFloat(f, 'f', -1, 64), nil
}

// number is a control's value as a number, 0 when it is not one.
func (r *Rack) number(id string) float64 {
	i, ok := r.byID[id]
	if !ok {
		return 0
	}
	f, err := strconv.ParseFloat(r.ctls[i].Value, 64)
	if err != nil {
		return 0
	}
	return f
}

// value is a control's value, "" when the rack has none.
func (r *Rack) value(id string) string {
	if i, ok := r.byID[id]; ok {
		return r.ctls[i].Value
	}
	return ""
}

// ── the picture ─────────────────────────────────────────────────────────

// pointer is a dial's pointer in the picture, and where the page drew it.
type pointer struct {
	item int     // the picture item
	at   float64 // its angle in the catalog, in degrees
	v0   float64 // the dial's value then
}

// calibrate finds the pointers and readouts of the controls only picks among
// the picture's items lo to hi, against the values captured gives (each
// control's value now, when nil): what the page drew for those values is
// where a new value is drawn from.
func (r *Rack) calibrate(lo, hi int, only func(id string) bool, captured func(c *racktui.Control) string) {
	if captured == nil {
		captured = func(c *racktui.Control) string { return c.Value }
	}
	for i := range r.ctls {
		c := &r.ctls[i]
		if !only(c.ID) {
			continue
		}
		box, ok := r.pic.Ctls[c.ID]
		if !ok {
			continue
		}
		v := captured(c)
		if p, ok := r.findPointer(c, v, box, lo, hi); ok {
			r.ptrs[c.ID] = p
		}
		if ro, ok := r.findReadout(c, v, box, lo, hi); ok {
			r.reads[c.ID] = ro
		}
	}
}

// findPointer is a dial's pointer: of the items in the dial's box that carry
// an angle, the one nearest the box's middle. A box is a whole P-unit, which
// can hold a neighbor's knob or a second pointer as well.
func (r *Rack) findPointer(c *racktui.Control, value string, box [4]float64, lo, hi int) (pointer, bool) {
	if c.IsSwitch || c.IsSelect || c.Max <= c.Min {
		return pointer{}, false
	}
	v0, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return pointer{}, false
	}
	mx, my := box[0]+box[2]/2, box[1]+box[3]/2
	best, dist := -1, math.Inf(1)
	for i := lo; i < hi; i++ {
		it := r.pic.Items[i]
		if !it.Pointer() || !inside(it, box) {
			continue
		}
		if d := math.Hypot(it.X+it.W/2-mx, it.Y+it.H/2-my); d < dist {
			best, dist = i, d
		}
	}
	if best < 0 {
		return pointer{}, false
	}
	return pointer{item: best, at: *r.pic.Items[best].Angle, v0: v0}, true
}

// inside reports whether an item's middle is in box (x, y, w, h).
func inside(it *rackpic.Item, box [4]float64) bool {
	x, y := it.X+it.W/2, it.Y+it.H/2
	return x >= box[0] && x <= box[0]+box[2] && y >= box[1] && y <= box[1]+box[3]
}

// angle is where a dial's pointer stands for value v: where the page drew it
// for the catalog's value, moved by the share of the travel v is from there
// across the knob's 270°. That is the page's turn for its linear dials and
// keeps the default exact for the rest (level knobs in decibels, detented
// inputs), whose curves the catalog does not record.
func (p pointer) angle(c *racktui.Control, v float64) float64 {
	return min(max(p.at+270*(v-p.v0)/(c.Max-c.Min), -135), 135)
}

// rotationKnobs are the View module's X, Y and Z: a rate is what they set,
// but what their pointers show is the model's angle about that axis, as it
// turns. Their pointers follow the scene (Scene), not their values.
var rotationKnobs = [3]string{"rotation-controls-x", "rotation-controls-y", "rotation-controls-z"}

func isRotationKnob(id string) bool {
	for _, k := range rotationKnobs {
		if k == id {
			return true
		}
	}
	return false
}

// turn shows a control's new value in the picture.
func (r *Rack) turn(c *racktui.Control) {
	if ro, ok := r.reads[c.ID]; ok {
		r.write(ro.item, ro.show(c))
	}
	p, ok := r.ptrs[c.ID]
	if !ok || isRotationKnob(c.ID) {
		return
	}
	v, err := strconv.ParseFloat(c.Value, 64)
	if err != nil {
		return
	}
	r.point(p.item, p.angle(c, v))
}

// write sets a text item to s, as the next patch's.
func (r *Rack) write(item int, s string) {
	if r.pic.Items[item].Text == s {
		return
	}
	r.pic.Gen++
	it := *r.pic.Items[item]
	it.Text = s
	r.pic.Items[item] = &it
	r.changed[item] = r.pic.Gen
}

// point turns a pointer item to a, in degrees, as the next patch's.
func (r *Rack) point(item int, a float64) {
	if old := r.pic.Items[item].Angle; old != nil && math.Abs(*old-a) < 0.05 {
		return
	}
	r.pic.Gen++
	it := *r.pic.Items[item]
	it.Angle = &a
	r.pic.Items[item] = &it
	r.changed[item] = r.pic.Gen
}

// showAngles turns the rotation knobs' pointers to the model's angles.
func (r *Rack) showAngles(angle [3]float64) {
	for i, id := range rotationKnobs {
		if p, ok := r.ptrs[id]; ok {
			r.point(p.item, math.Mod(angle[i]*180/math.Pi+360*8, 360))
		}
	}
}

// Picture is the whole panel now.
func (r *Rack) Picture() (*rackpic.Picture, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := *r.pic
	p.Items = append([]*rackpic.Item(nil), r.pic.Items...)
	return &p, nil
}

// PictureChanges is what changed since the picture of generation gen.
func (r *Rack) PictureChanges(gen int) (*rackpic.Patch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if gen < r.full {
		// The picture was rebuilt since: a new model's bank. All of it.
		p := *r.pic
		p.Items = append([]*rackpic.Item(nil), r.pic.Items...)
		return &rackpic.Patch{Gen: r.pic.Gen, Full: &p}, nil
	}
	d := &rackpic.Patch{Gen: r.pic.Gen}
	for i, g := range r.changed {
		if g > gen {
			if d.Items == nil {
				d.Items = map[int]*rackpic.Item{}
			}
			d.Items[i] = r.pic.Items[i]
		}
	}
	return d, nil
}

// Act does what a click or a turn of the wheel at x, y of the picture does to
// the control there: a click throws a switch or steps a selector, and the
// wheel turns a dial by its step or a selector by one detent.
func (r *Rack) Act(_ int, x, y float64, kind string, delta float64) error {
	r.mu.Lock()
	id := r.controlAt(x, y)
	if id == "" {
		r.mu.Unlock()
		return nil
	}
	c := r.ctls[r.byID[id]]
	r.mu.Unlock()
	switch {
	case c.IsSwitch && kind == "click":
		if c.Value == "1" {
			return r.Set(id, "0")
		}
		return r.Set(id, "1")
	case c.IsSelect:
		step := 1
		if kind == "wheel" && delta < 0 {
			step = -1
		}
		return r.Set(id, stepOption(c, step))
	case kind == "wheel" && !c.IsSwitch:
		s := c.Step
		if s <= 0 {
			s = (c.Max - c.Min) / 100
		}
		f, _ := strconv.ParseFloat(c.Value, 64) //nolint:errcheck // a dial's value is a number, and 0 is a fine start if not
		if delta > 0 {
			s = -s
		}
		return r.Set(id, strconv.FormatFloat(f+s, 'f', -1, 64))
	}
	return nil
}

// stepOption is the option step places from c's, wrapping around.
func stepOption(c racktui.Control, step int) string {
	n := len(c.Options)
	if n == 0 {
		return c.Value
	}
	at := 0
	for i, o := range c.Options {
		if o == c.Value {
			at = i
		}
	}
	return c.Options[((at+step)%n+n)%n]
}

// controlAt is the smallest control box holding x, y.
func (r *Rack) controlAt(x, y float64) string {
	best, area := "", math.Inf(1)
	for id, b := range r.pic.Ctls {
		if x >= b[0] && x <= b[0]+b[2] && y >= b[1] && y <= b[1]+b[3] && b[2]*b[3] < area {
			best, area = id, b[2]*b[3]
		}
	}
	return best
}

// Canvases is the panel's canvases asked for; the native rack draws none of
// them yet, and an empty answer leaves them as the catalog shows them.
func (r *Rack) Canvases(int, []rackpic.CanvasWant) (map[int]*rackpic.Image, error) {
	return nil, nil
}

// ── the scene ───────────────────────────────────────────────────────────

// scene is the model behind the panel, drawn here.
type scene struct {
	// mu guards the rest: the panel draws on its loop, and a drag turns the
	// model from another goroutine.
	mu       sync.Mutex
	key      string
	fig      attractor.Figure
	err      error
	angle    [3]float64
	last     time.Time
	turned   bool // a drag has set the angle since the last frame
	drag     [2]float64
	dragging bool
}

func (s *scene) start() {
	s.angle = [3]float64{0.35, 0, 0}
	s.last = time.Now()
}

// autoSpin is how fast the model turns about its upright axis with Auto-rot
// on and every rate at zero, in radians a second: what the page does.
const autoSpin = 0.3

// modelControl is the id of the control that says what the model is: the
// model bay's selector.
func (r *Rack) modelControl() string {
	for _, c := range r.ctls {
		if c.IsSelect && strings.HasPrefix(c.ID, "bay-") && strings.HasSuffix(c.ID, "-sel-all") {
			return c.ID
		}
	}
	return ""
}

// ErrBrowserOnly is a model only the page can draw: it needs WebGL, Web Audio
// or the DOM.
var ErrBrowserOnly = errors.New("draws only in a browser")

// Scene is the model as w x h pixels, each shape times as tall as wide.
func (r *Rack) Scene(w, h int, shape float64) (*rackpic.Image, error) {
	r.mu.Lock()
	key := "globe"
	if id := r.modelControl(); id != "" {
		key = r.value(id)
	}
	on := r.value("power-sw") != "0"
	auto := r.value("auto-rotate") != "0"
	rate := [3]float64{r.number("rotation-controls-x"), r.number("rotation-controls-y"), r.number("rotation-controls-z")}
	zoom := r.number("camera-zoom")
	r.mu.Unlock()

	s := &r.scene
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	dt := now.Sub(s.last).Seconds()
	s.last = now
	if on && !s.turned {
		for i := range s.angle {
			s.angle[i] += rate[i] * dt
		}
		if auto && rate == ([3]float64{}) {
			s.angle[1] += autoSpin * dt
		}
	}
	s.turned = false
	angle := s.angle
	r.mu.Lock()
	r.showAngles(angle)
	r.mu.Unlock()

	img := &rackpic.Image{W: w, H: h, Px: make([]byte, 3*w*h)}
	if key == "" || !on {
		return img, nil
	}
	if key != s.key || r.refigure.Swap(false) {
		s.key = key
		s.fig, s.err = figureFor(key, r.knobs())
	}
	if s.err != nil {
		// Dark, and why: the panel shows the reason on its status line.
		return img, s.err
	}
	o := attractor.DrawOptions{
		Width: w, Height: h,
		View: rasterview.View{
			AngleX: s.angle[0], AngleY: s.angle[1], AngleZ: s.angle[2],
			Scale:       zoomScale(zoom),
			BackDim:     0.5,
			PixelAspect: 1 / shape,
		},
	}
	n := len(s.fig.Points)
	if s.fig.Kind == attractor.FigureLines {
		n = len(s.fig.Edges)
	}
	m := attractor.DrawFigure(s.fig, 0, n, o)
	for i := range w * h {
		img.Px[3*i], img.Px[3*i+1], img.Px[3*i+2] = m.Pix[4*i], m.Pix[4*i+1], m.Pix[4*i+2]
	}
	return img, nil
}

// zoomScale is the fraction of the frame the model fills at the View
// module's Zoom: 0.85 at zero, and twice or half that a third of the way to
// either end. The page zooms its camera; this is the same direction and a
// similar reach, not its exact curve.
func zoomScale(zoom float64) float64 {
	return 0.85 * math.Pow(2, zoom/31.7)
}

// figureFor is a model's geometry, as `chaosrack render` builds it, with the
// model's own knobs where the rack carries them out (knob is a control's
// value, 0 when the rack has none).
func figureFor(key string, knob func(id string) float64) (attractor.Figure, error) {
	if dynamics.HasFlow(key) {
		return attractor.Figure{Kind: attractor.FigurePath, Points: dynamics.Trajectory(key, dynamics.DefaultTrajectory())}, nil
	}
	if key == "globe" {
		// Its circles of latitude and its meridians, as the page's knobs
		// count them: geom.Globe at the page's defaults is StaticFigure's.
		lat, lon := int(knob("globe-lat")), int(knob("globe-lon"))
		if lat > 0 && lon > 0 {
			return linesOf(geom.Globe(lat, lon, 60)), nil
		}
	}
	if f, ok := attractor.StaticFigure(key, 20000); ok {
		return f, nil
	}
	return attractor.Figure{}, fmt.Errorf("%s %w", key, ErrBrowserOnly)
}

// linesOf is a wireframe as a figure.
func linesOf(l geom.Lines) attractor.Figure {
	f := attractor.Figure{Kind: attractor.FigureLines, Edges: l.Indices}
	for i := 0; i+2 < len(l.Vertices); i += 3 {
		f.Points = append(f.Points, [3]float64{float64(l.Vertices[i]), float64(l.Vertices[i+1]), float64(l.Vertices[i+2])})
	}
	return f
}

// knobs is the model bank's values now, for figureFor.
func (r *Rack) knobs() func(id string) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := map[string]float64{}
	for _, c := range r.ctls {
		if c.Module == modelModule {
			if f, err := strconv.ParseFloat(c.Value, 64); err == nil {
				v[c.ID] = f
			}
		}
	}
	return func(id string) float64 { return v[id] }
}

// modelModule is the model bank's module: a change there redraws the model.
const modelModule = "visual · model"

// SceneAct turns the model with a drag and zooms it with the wheel, as the
// page's own canvas does.
func (r *Rack) SceneAct(w, h int, _ float64, x, y float64, kind string, delta float64) error {
	s := &r.scene
	if kind != "wheel" {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	switch kind {
	case "down":
		s.drag = [2]float64{x, y}
		s.dragging = true
	case "move":
		if s.dragging && w > 0 && h > 0 {
			s.angle[1] += (x - s.drag[0]) / float64(w) * math.Pi
			s.angle[0] += (y - s.drag[1]) / float64(h) * math.Pi
			s.drag = [2]float64{x, y}
			s.turned = true
		}
	case "up":
		s.dragging = false
	case "wheel":
		r.mu.Lock()
		z := r.number("camera-zoom")
		r.mu.Unlock()
		step := -2.5
		if delta < 0 {
			step = 2.5
		}
		return r.Set("camera-zoom", strconv.FormatFloat(z+step, 'f', -1, 64))
	}
	return nil
}
