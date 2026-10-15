// Package rackhost is the rack without a browser: what `chaosrack` runs on a
// host, where there is no page to drive.
//
// The panel is the page's, as the catalog recorded it (pkg/rackcatalog): its
// modules, its controls and the picture the page look draws. The values are
// this Rack's own — set by the panel, kept here, and shown by turning the
// picture's knob pointers — and the model is drawn here too, from the same
// figures and trajectories `chaosrack render` draws, with no GPU.
//
// The sound is made here too and played on the host's speakers (sound.go):
// the signal generators, Model Out, the Matrix with its drum lanes, the keys
// with the Synth bay, and the scope games' blips (games.go), through the
// Mixer's speaker pins. The Mixer's RACK rows mix them, and the capture of
// the machine's audio, into the rack's signal, which the audio models draw
// (bus.go). FVF is the page's still.
package rackhost

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
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
	lamps   map[string]int     // each Mixer pin's lamp, by control id
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

	sound *sound
	mx    matrixPic // the Matrix's parts in the picture
	keys  keysPic   // the keys in the picture, and the one the pointer holds
	// audioSource is what the capture records: "monitor" (what is playing),
	// "default" (the input), or a source's ID.
	audioSource string
	// stereo is the Stereo Embedding's scope, which keeps its trigger's
	// memory from frame to frame (bus.go).
	stereo stereoView
	// grid is the Grid's cells (grid.go), and cellOver the values the cell
	// being drawn has in place of the knobs'.
	grid     gridView
	cellOver map[string]float64
	// color is what the audio gradient sources color from (audiocolor.go).
	color colorFeed
	// analyzers are the flat analyzers, which keep their meters and averages
	// from frame to frame (analyzers.go).
	analyzers analyzers
	// legendItem is each programmable switch's legend display (modelsw.go).
	legendItem map[string]int
	// custom is Custom's system, compiled and registered (custom.go).
	custom *attractor.CustomSystem
	// stl is the STL File model's wireframe (stl.go).
	stl stlView
	// lattice is the Lattice's stack of terminals (lattice.go).
	lattice *attractor.LatticeStack
	// latFig is the model chosen as ROWS's L hands it to the Lattice
	// (latticefig.go).
	latFig latticeFig
	// anim is the terminal animation running (termanim.go).
	anim animView
	// shell is the Host Terminal's shell (hostterm.go).
	shell shellView
	// websh is the Terminal's shell (websh_linux.go).
	websh shellView
	// desk is the Desk's windows (desk.go).
	desk deskView
	// meters are the measurement modules (meters.go).
	meters meterBank
	// rec is the Record module's take, and recSaved where the last one was
	// written (record.go).
	rec      recorder
	recSaved string
	// jamNext is when Jam next hops, 0 for at once (jam.go).
	jamNext float64
	// water is BEHIND's water lens, and waterEnv the loudness that drives
	// it (water.go).
	water    waterLens
	waterEnv envFollower
	// turtleAim is what the Turtle's VIEW last turned the scene for, and
	// poseTo the pose it asks for, taken by the next Scene (turtleview.go).
	turtleAim turtleAim
	poseTo    *[3]float64
	// canvasKinds is what each of the panel's screens is, and tubes the
	// scopes' phosphors (canvases.go, scopes.go).
	canvasKinds map[int]panelCanvas
	tubes       [scopeCount]scopeTube
	// prompt is the text control the panel is to ask for (text.go).
	prompt string
	// viewAngle is the scene's pose as last drawn, for what turns with the view
	// (the Lattice's programs).
	viewAngle [3]float64
}

// New is a rack at the catalog's defaults.
func New() (*Rack, error) {
	cat, err := rackcatalog.Load()
	if err != nil {
		return nil, err
	}
	r := &Rack{
		cat:         cat,
		ctls:        append([]racktui.Control(nil), cat.Controls...),
		byID:        map[string]int{},
		ptrs:        map[string]pointer{},
		reads:       map[string]readout{},
		lamps:       map[string]int{},
		changed:     map[int]int{},
		audioSource: "monitor",
		stereo:      stereoView{scope: attractor.NewStereoScope()},
		analyzers:   newAnalyzers(),
		custom:      attractor.NewCustomSystem(),
		lattice:     attractor.NewLatticeStack(),
		desk:        newDesk(),
		meters:      newMeterBank(),
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
		// Only the items outside the bank: the copy of the picture still has
		// the catalog's, which would stay under every later bank.
		p.Items = nil
		for _, it := range cat.Picture.Items {
			if !rackcatalog.InBank(it, cat.BankRegion) {
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
	r.custom.Compile()
	r.seedCustomKnobs()
	r.addTextControls()
	r.findLegends()
	r.useBank(r.model())
	r.scene.start()
	r.sound = newSound()
	r.soundStart()
	r.showModelSwitches(r.model())
	r.gridTargets(r.model())
	r.gridCells()
	r.findMatrix()
	r.findKeys()
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
	r.findReadouts()
	r.findMeters()
	r.colorLEDs()
	r.pic.Gen++
	r.full = r.pic.Gen
}

var (
	_ racktui.Source   = (*Rack)(nil)
	_ racktui.Pictured = (*Rack)(nil)
	_ racktui.Paced    = (*Rack)(nil)
	_ racktui.GameKeys = (*Rack)(nil)
	_ racktui.Typist   = (*Rack)(nil)
	_ racktui.Prompter = (*Rack)(nil)
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
	r.sound.hand()
	r.mu.Lock()
	err := r.set(id, value)
	r.mu.Unlock()
	// The speaker is told what to play with no lock held: a speaker that
	// starts asks for samples (Fill) before it returns.
	r.sound.tell()
	return err
}

// set is Set with r.mu held.
func (r *Rack) set(id, value string) error {
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
	for _, s := range r.soundSet(id) {
		// A generator turned while pinned nowhere is pinned, as a hand on the
		// Mixer would, so its lamp lights too; a preset moves the steps.
		if err := r.set(s[0], s[1]); err != nil {
			return err
		}
	}
	if c.Module == modelModule {
		r.refigure.Store(true)
	}
	if id == "led-color" {
		r.colorLEDs()
	}
	switch id {
	case "view-n":
		r.gridCells()
	case "focus-n", "link-sw":
		r.refocus()
	}
	if c.IsText {
		if err := r.textSet(id, v); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	if _, ok := r.flow[id]; ok {
		// The integrator reads the constant under the scene's lock, so the
		// scene writes it there, before its next step (flowValues).
		r.refigure.Store(true)
	}
	if id == r.selector || isFamily(id) {
		m := r.model()
		r.followMode(m)
		r.gridTargets(m)
		// Takens measures τ once on entry, as the page re-arms it on every
		// mode change: the signal may have changed while it was away.
		r.analyzers.tau.Arm()
		if m != "sprottmorph" {
			r.analyzers.morph.run.Leave() // coming back enters it afresh, as the page does
		}
		r.useBank(m)
		r.showModelSwitches(m)
		return nil
	}
	r.turn(c)
	return nil
}

// settle is value as control c holds it, or why it cannot be.
func settle(c *racktui.Control, value string) (string, error) {
	if c.IsText {
		return value, nil // as it was typed
	}
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

// number is a control's value as a number, 0 when it is not one: or the
// value a grid cell being drawn has in its place (grid.go).
func (r *Rack) number(id string) float64 {
	if v, ok := r.cellOver[id]; ok {
		return v
	}
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
		if !isPin(c.ID) {
			continue
		}
		if l, ok := r.findLamp(box, lo, hi); ok {
			r.lamps[c.ID] = l
		}
	}
}

// findPointer is a dial's pointer: of the items in the dial's box that carry
// an angle, the one nearest the box's middle. A box is a whole P-unit, which
// can hold a neighbor's knob or a second pointer as well — and a palette
// knob's hue ring and level knob share one middle, so of pointers within a
// pixel of the nearest, the one the size of the dial's box is its own.
func (r *Rack) findPointer(c *racktui.Control, value string, box [4]float64, lo, hi int) (pointer, bool) {
	if c.IsSwitch || c.IsSelect || c.Max <= c.Min {
		return pointer{}, false
	}
	v0, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return pointer{}, false
	}
	mx, my := box[0]+box[2]/2, box[1]+box[3]/2
	best, dist, fit := -1, math.Inf(1), math.Inf(1)
	for i := lo; i < hi; i++ {
		it := r.pic.Items[i]
		if !it.Pointer() || !inside(it, box) {
			continue
		}
		d := math.Hypot(it.X+it.W/2-mx, it.Y+it.H/2-my)
		f := math.Abs(it.W - box[2])
		if d < dist-1 || math.Abs(d-dist) <= 1 && f < fit {
			best, dist, fit = i, d, f
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
	if isFullTurn(c.ID) {
		return math.Mod(math.Mod(p.at+360*(v-p.v0)/(c.Max-c.Min), 360)+360, 360)
	}
	return min(max(p.at+270*(v-p.v0)/(c.Max-c.Min), -135), 135)
}

// isFullTurn reports whether dial id turns all the way round over its
// travel: a palette knob's hue ring, whose pointer is the hue itself.
func isFullTurn(id string) bool {
	return strings.HasPrefix(id, "ck-") && strings.HasSuffix(id, "-hue")
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
	if l, ok := r.lamps[c.ID]; ok {
		r.light(l, c)
	}
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
	r.viewAngle = angle
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
	r.playGames()
	r.showMatrix()
	r.showKeys()
	r.runMeters(float64(r.sound.now().UnixNano()) / 1e6)
	p := *r.pic
	p.Items = append([]*rackpic.Item(nil), r.pic.Items...)
	return &p, nil
}

// PictureChanges is what changed since the picture of generation gen.
func (r *Rack) PictureChanges(gen int) (*rackpic.Patch, error) {
	// After the lock is let go: a key let go stops ringing with no hand on
	// the rack, and the speaker is told so here, as the panel looks.
	defer r.sound.settle()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playGames()
	r.needCapture()
	r.sound.busLook()
	r.showMatrix()
	r.showKeys()
	nowMs := float64(r.sound.now().UnixNano()) / 1e6
	r.runMeters(nowMs)
	// Before the picture is handed over: a hop puts up a new bank, and the
	// full picture goes with it.
	r.jamTick(nowMs)
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
// wheel turns a dial by its step or a selector by one detent. On the
// keys, a click holds a key down, a drag plays the keys it crosses, and a
// release lets go.
func (r *Rack) Act(_ int, x, y float64, kind string, delta float64) error {
	r.sound.hand()
	r.mu.Lock()
	if r.keysAct(x, y, kind) {
		r.mu.Unlock()
		r.sound.tell()
		return nil
	}
	if kind != "click" && kind != "wheel" {
		// A drag or a release that was not on the keys.
		r.mu.Unlock()
		return nil
	}
	if kind == "click" {
		// The Matrix's pads, tabs and Clear, which are not controls.
		if m := r.markAt(x, y); m != "" && r.press(m) {
			r.mu.Unlock()
			r.sound.tell()
			return nil
		}
	}
	id := r.controlAt(x, y)
	if id == "" {
		r.mu.Unlock()
		return nil
	}
	c := r.ctls[r.byID[id]]
	r.mu.Unlock()
	switch {
	case isPin(id) && kind == "click":
		// A Mixer pin, clicked, is pinned at full gain or unpinned, as the
		// page's is; the wheel trims its gain.
		if c.Value != "0" {
			return r.Set(id, "0")
		}
		return r.Set(id, "1")
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

// RefreshEvery is how often a panel should ask this rack for its picture: it
// answers in microseconds, and the Matrix's playhead moves every sixteenth.
func (r *Rack) RefreshEvery() time.Duration { return 50 * time.Millisecond }

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
	// trail is the flow running live, and last the frame drawn, which
	// Display's Persist draws over and Pause draws again.
	trail trail
	shown *image.RGBA
	// crt is the phosphor tube's afterglow (crt.go).
	crt crtState
	// roll is the torus tube's phase, which its ROLL knob turns.
	roll float64
	// liss is the Lissajous beam, which runs on frame by frame (lissajous.go).
	liss attractor.LissajousBeam
}

func (s *scene) start() {
	s.angle = [3]float64{0.35, 0, 0}
	s.last = time.Now()
}

// sceneBack is what the model is drawn on: the panel's own near-black.
var sceneBack = color.RGBA{10, 13, 20, 255}

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

// Scene is the model as w x h pixels, each shape times as tall as wide: on
// the chosen phosphor's tube, when one is (crt.go).
func (r *Rack) Scene(w, h int, shape float64) (*rackpic.Image, error) {
	r.mu.Lock()
	rec := r.rec.on
	r.mu.Unlock()
	dw, dh, dshape := w, h, shape
	if rec {
		// A take is drawn at its own size, square pixels, and the panel's
		// scene shrunk from it: the model is drawn once a frame either way.
		dw, dh = recSize(w, h, shape)
		dshape = 1
	}
	m, err := r.drawScene(dw, dh, dshape)
	r.mu.Lock()
	p, on := r.phosphor()
	if r.waterOn() && m != nil {
		// The finished frame, seen through the water (water.go).
		r.water.step(time.Now(), r.waterDrive())
		r.water.refract(m)
	}
	r.mu.Unlock()
	if m != nil && on {
		r.scene.mu.Lock()
		r.scene.crt.crt(m, p, time.Now())
		r.scene.mu.Unlock()
	} else {
		r.scene.mu.Lock()
		r.scene.crt = crtState{} // a tube switched off keeps no afterglow
		r.scene.mu.Unlock()
	}
	if rec && m != nil {
		r.mu.Lock()
		r.recordFrame(m)
		r.mu.Unlock()
		m = shrink(m, w, h)
	}
	return m, err
}

// drawScene is the model as w x h pixels, each shape times as tall as wide.
func (r *Rack) drawScene(w, h int, shape float64) (*rackpic.Image, error) {
	r.mu.Lock()
	key := "globe"
	if r.selector != "" {
		key = r.model()
	}
	on := r.value("power-sw") != "0"
	auto := r.value("auto-rotate") != "0"
	tbc := r.value("tbc-sw") != "0"
	rate := [3]float64{r.number("rotation-controls-x"), r.number("rotation-controls-y"), r.number("rotation-controls-z")}
	zoom := r.number("camera-zoom")
	look := r.look()
	paused := r.value("pause-sw") == "1"
	persist := r.value("persist-trail") == "1"
	inLat := r.value("lat-on") == "1" && key != "lattice"
	run, speed := int(r.number("trail-slider")), r.number("speed-slider")
	refigure := r.refigure.Swap(false)
	rollRate := 0.0
	if key == "torus" {
		rollRate = r.number("torus-roll")
	}
	r.playGames()
	r.needCapture()
	r.sound.busLook()
	audio := figureEachFrame(key)
	heard := r.hear(key, on, w)
	// The Grid's cells, each worked out from what the models were just
	// fed (grid.go).
	g := r.gridNow()
	var plans []cellPlan
	if g.n > 1 {
		plans = r.planCells(key, g)
	}
	src := r.gradientSource(g)
	bd := r.backdrop(key, on, w, h)
	poseTo := r.poseTo
	r.poseTo = nil
	var consts map[*float32]float32
	if refigure {
		consts = r.flowValues()
	}
	r.mu.Unlock()

	s := &r.scene
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(consts) > 0 {
		// Model Out reads them too, on the speaker's goroutine.
		r.sound.mu.Lock()
		for p, v := range consts {
			*p = v
		}
		r.sound.mu.Unlock()
	}
	dt := s.frameTime(time.Now(), tbc)
	if rollRate != 0 {
		// The page turns the tube rate/20 a frame, at sixty frames a second.
		s.roll += 3 * rollRate * dt
		refigure = true
	}
	// A picture on a plane comes up face-on and holds still, as the page sets
	// its textured planes up: a hand or the rate knobs still turn it.
	s.pose(key, heard.quad != nil || len(heard.quads) > 0 || attractor.IsFlatScope(key), on && !paused, auto, rate, dt)
	if poseTo != nil {
		s.angle = *poseTo
	}
	angle := s.angle
	r.mu.Lock()
	r.showAngles(angle)
	r.mu.Unlock()

	img := &rackpic.Image{W: w, H: h, Px: make([]byte, 3*w*h)}
	if key == "" || !on {
		s.shown = nil
		return img, nil
	}
	if isAnalyzer(key) || isTerminalModel(key) {
		// A flat analyzer, or a terminal animation's screen, drawn over the whole scene as the page draws it
		// over its canvas (analyzers.go).
		s.key, s.err, s.shown = key, nil, nil
		v := rasterview.View{
			AngleX: s.angle[0], AngleY: s.angle[1], AngleZ: s.angle[2],
			Scale:       zoomScale(zoom),
			PixelAspect: 1 / shape,
			PanX:        look.pan[0],
			PanY:        look.pan[1],
		}
		toPixels(img, look.bg, pictureCells(heard, v, g, w, h))
		return img, nil
	}
	game := isGame(key)
	var gameFit [][3]float64
	switch {
	case game:
		// A scope game, drawn as it plays (games.go).
		r.sound.mu.Lock()
		s.fig, gameFit = r.sound.games.figure()
		r.sound.mu.Unlock()
		s.key, s.err = key, nil
	case audio:
		// An audio model, drawn from the rack's signal (bus.go).
		s.fig, gameFit = heard.fig, heard.fit
		s.key, s.err = key, nil
	case key != s.key || refigure:
		s.key = key
		switch key {
		case "custom":
			r.mu.Lock()
			s.fig, s.err = r.customFigure()
			r.mu.Unlock()
		case "stlfile":
			r.mu.Lock()
			s.fig, s.err = r.stlFigure()
			r.mu.Unlock()
		default:
			s.fig, s.err = figureFor(key, r.knobs(), s.roll)
		}
	}
	if s.err != nil {
		// Dark, and why: the panel shows the reason on its status line.
		s.shown = nil
		return img, s.err
	}
	o := attractor.DrawOptions{
		Width: w, Height: h,
		View: rasterview.View{
			AngleX: s.angle[0], AngleY: s.angle[1], AngleZ: s.angle[2],
			Scale:       zoomScale(zoom),
			BackDim:     0.5,
			PixelAspect: 1 / shape,
			PanX:        look.pan[0],
			PanY:        look.pan[1],
			LineWidth:   lineWidth(look.line, w),
		},
		Gradient:   look.gradient,
		Background: sceneBack,
	}
	fig := s.fig
	if game || audio {
		// Framed to its court or its full scale, not to what it draws now.
		o.Fit = gameFit
	}
	if game || key == "xy" {
		// Face-on, as the page sets a game or a scope's screen up.
		o.View.AngleX, o.View.AngleY, o.View.AngleZ = 0, 0, 0
	}
	if !game && !audio {
		fig, o.Fit = s.running(fig, o.Fit, key, r.knobs, paused, run, speed, look.dots)
	}
	if inLat {
		// ROWS's L: the model drawn by the Lattice instead (latticefig.go).
		r.mu.Lock()
		fig, o.Fit = r.latticeOf(fig)
		r.mu.Unlock()
	}
	draw1 := func(fig attractor.Figure, o attractor.DrawOptions) *image.RGBA {
		n := len(fig.Points)
		if fig.Kind == attractor.FigureLines {
			n = len(fig.Edges)
		}
		m := attractor.DrawFigure(fig, 0, n, o)
		if len(heard.overlay.Points) > 0 {
			// LINE S: the section's crossings in gold where they are, over
			// the trail, framed as the flow is.
			ov := o
			ov.Gradient = rasterview.Gradient{Colors: 1, Base: sectGold}
			ov.View.BackDim = 0
			if ov.Fit == nil {
				ov.Fit = fig.Points
			}
			over(m, attractor.DrawFigure(heard.overlay, 0, len(heard.overlay.Points), ov))
		}
		if len(heard.grat.Edges) > 0 {
			// The Stereo Embedding's graticule, flat gray behind the trace.
			og := o
			og.Gradient = rasterview.Gradient{Colors: 1, Base: attractor.GraticuleColor}
			og.View.BackDim = 0 // flat, as the page draws it
			under(m, attractor.DrawFigure(heard.grat, 0, len(heard.grat.Edges), og))
		}
		return m
	}
	var m *image.RGBA
	if g.n <= 1 {
		r.mu.Lock()
		fig = r.audioColored(fig, &o.Gradient, src, 0)
		r.mu.Unlock()
		m = draw1(fig, o)
	} else {
		m = r.drawCells(key, g, plans, fig, o, draw1, paused, run, speed, look.dots)
	}
	if k := glowKeep(heard.glow, dt); k > 0 && s.shown != nil && s.shown.Bounds() == m.Bounds() {
		// The XY's GLOW: the last frame fades like a phosphor and the trace
		// is drawn over it, as the page multiplies its frame down rather
		// than clearing it.
		fade(m, s.shown, k)
	}
	if persist && s.shown != nil && s.shown.Bounds() == m.Bounds() {
		// Display's Persist: the old trails stay, and the new one is drawn
		// over them, as the page leaves its color buffer uncleared.
		for i := 0; i < len(m.Pix); i += 4 {
			if isBack(m.Pix[i : i+3]) {
				copy(m.Pix[i:i+3], s.shown.Pix[i:i+3])
			}
		}
	}
	s.shown = m
	toPixels(img, look.bg, withBackdrop(m, bd))
	return img, nil
}

// toPixels copies m into img, which is as big, with the Palette's bg tint
// added to what shows only the background: the page clears its canvas to
// that color at no opacity, so it tints what shows through and leaves what
// is drawn alone.
func toPixels(img *rackpic.Image, bg [3]float32, m *image.RGBA) {
	var tint [3]uint8
	for c := range tint {
		tint[c] = uint8(min(max(bg[c], 0), 1) * 255)
	}
	for i := range img.W * img.H {
		p := m.Pix[4*i : 4*i+3]
		if tint != ([3]uint8{}) && isBack(p) {
			for c := range 3 {
				img.Px[3*i+c] = p[c] + min(tint[c], 255-p[c]) // saturating
			}
			continue
		}
		img.Px[3*i], img.Px[3*i+1], img.Px[3*i+2] = p[0], p[1], p[2]
	}
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
// value, 0 when the rack has none), and roll the torus tube's phase.
func figureFor(key string, knob func(id string) float64, roll float64) (attractor.Figure, error) {
	if dynamics.HasFlow(key) {
		return attractor.Figure{Kind: attractor.FigurePath, Points: dynamics.Trajectory(key, dynamics.DefaultTrajectory())}, nil
	}
	if knob != nil {
		// A shape as its knobs make it: the globe's rings and twist, the
		// torus's radii and roll, the solid {p,q} and what is done to it.
		if f, ok := attractor.ShapeFigure(key, knob, roll); ok {
			return f, nil
		}
	}
	if f, ok := attractor.StaticFigure(key, 20000); ok {
		return f, nil
	}
	return attractor.Figure{}, fmt.Errorf("%s %w", key, ErrBrowserOnly)
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
	if kind != "wheel" && w > 0 && h > 0 {
		// On water, a hand on the scene disturbs it too (water.go).
		r.mu.Lock()
		if r.waterOn() {
			r.water.touch(kind, x, y, w, h)
		}
		r.mu.Unlock()
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

// glowKeep is how much of the last frame the XY's GLOW knob keeps after dt
// seconds: the page keeps up to 0.98 of it a frame at 60 frames a second.
func glowKeep(glow, dt float64) float64 {
	if !(glow > 0) || !(dt > 0) {
		return 0
	}
	return math.Pow(min(glow, 0.98), min(dt, 1)*60)
}

// under puts src's pixels where dst shows only the background.
func under(dst, src *image.RGBA) {
	for i := 0; i+3 < len(dst.Pix) && i+3 < len(src.Pix); i += 4 {
		if isBack(dst.Pix[i:i+3]) && !isBack(src.Pix[i:i+3]) {
			copy(dst.Pix[i:i+3], src.Pix[i:i+3])
		}
	}
}

// fade adds what was shown, k of the way up from the background, onto dst.
func fade(dst, shown *image.RGBA, k float64) {
	back := [3]uint8{sceneBack.R, sceneBack.G, sceneBack.B}
	for i := 0; i+3 < len(dst.Pix); i += 4 {
		for c := range 3 {
			v := float64(dst.Pix[i+c]) + k*(float64(shown.Pix[i+c])-float64(back[c]))
			dst.Pix[i+c] = uint8(min(max(v, 0), 255))
		}
	}
}

// isBack reports whether a pixel is the scene's background.
func isBack(p []uint8) bool {
	return p[0] == sceneBack.R && p[1] == sceneBack.G && p[2] == sceneBack.B
}

// model is the model the rack plays: the MODEL knob's, or, on a family's
// position (Sprott A–S), the member its lettered dial is on, as the page's
// family position plays the member it stands for. Called with r.mu held.
func (r *Rack) model() string {
	m := r.value(r.selector)
	for _, c := range r.ctls {
		if isFamily(c.ID) && len(c.Options) > 0 && c.Options[0] == m {
			return c.Value
		}
	}
	return m
}

// isFamily reports whether id is a family's lettered dial.
func isFamily(id string) bool { return strings.HasPrefix(id, "fam-") }

// pose turns the model on by dt: by the rate knobs while it runs, and by
// auto-rotate when they are still, unless it is flat, which comes up
// face-on when it is chosen and holds still. Paused, it holds still and a
// hand still turns it, as the page's does. Called with s.mu held.
func (s *scene) pose(key string, flat, running, auto bool, rate [3]float64, dt float64) {
	if flat && key != s.key {
		s.angle = [3]float64{}
	}
	if running && !s.turned {
		for i := range s.angle {
			s.angle[i] += rate[i] * dt
		}
		if auto && rate == ([3]float64{}) && !flat {
			s.angle[1] += autoSpin * dt
		}
	}
	s.turned = false
}

// liveTrail is a flow's figure as it runs on, a fresh stretch of its trail
// each frame, framed by its whole trajectory so the camera holds still while
// it moves; with Display's Dots, its points unjoined. Called with s.mu held.
func (s *scene) liveTrail(fig attractor.Figure, fit [][3]float64, key string, paused bool, run int, speed float64, dots bool) (attractor.Figure, [][3]float64) {
	if (paused && s.trail.key == key && len(s.trail.pts) > 0) || (!paused && s.trail.advance(key, run, speed)) {
		fit = fig.Points
		fig.Points = s.trail.pts
	}
	if dots {
		fig.Kind = attractor.FigurePoints
	}
	return fig, fit
}

// frameTime is how long the frame at now stands for, in seconds: the time
// since the last, with Timing's TBC on; off, a frame is a frame, however long
// it took — a sixtieth of a second's turn, as the page turned before the
// correction. Called with s.mu held.
func (s *scene) frameTime(now time.Time, tbc bool) float64 {
	dt := now.Sub(s.last).Seconds()
	s.last = now
	if !tbc {
		return 1.0 / 60
	}
	return dt
}

// over puts src's drawn pixels over dst's.
func over(dst, src *image.RGBA) {
	for i := 0; i+3 < len(dst.Pix) && i+3 < len(src.Pix); i += 4 {
		if !isBack(src.Pix[i : i+3]) {
			copy(dst.Pix[i:i+3], src.Pix[i:i+3])
		}
	}
}
