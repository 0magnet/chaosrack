package rackhost

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// The rack off the clock, for `chaosrack render` to make a video of: every
// control the panel has, set by its id, and the scene drawn a frame at a
// time at a fixed rate, however long each takes to draw. Its clock moves a
// frame's worth between frames and no further, so what runs on time — a
// trail, a turn, the phosphor's afterglow, Persist, the XY's glow — runs at
// the video's pace rather than the machine's; and its capture is the samples
// it is handed with each frame rather than this machine's audio, so an
// embedding of a recording is drawn from exactly the stretch of it that
// frame stands for.

// Offline is a rack drawn frame by frame at FPS frames a second.
type Offline struct {
	r     *Rack
	fps   int
	frame int
	epoch time.Time
	now   time.Time
	spk   [2][]float32
}

// NewOffline is a rack at the catalog's defaults, off the clock, with Timing's
// TBC on so that a frame stands for its own share of a second rather than a
// sixtieth.
func NewOffline(fps int) (*Offline, error) {
	if fps <= 0 {
		return nil, fmt.Errorf("%d frames a second", fps)
	}
	r, err := New()
	if err != nil {
		return nil, err
	}
	o := &Offline{r: r, fps: fps, epoch: time.Unix(0, 0)}
	o.now = o.epoch
	clock := func() time.Time { return o.now }
	r.sound.mu.Lock()
	r.sound.now = clock
	r.sound.games.now = clock
	r.sound.mu.Unlock()
	r.scene.mu.Lock()
	r.scene.last = o.now
	r.scene.mu.Unlock()
	r.mu.Lock()
	r.fed = true
	err = r.set("tbc-sw", "1")
	r.mu.Unlock()
	return o, err
}

// SampleRate is the rate a frame's samples are counted at.
func (o *Offline) SampleRate() int { return sampleRate }

// Controls are the rack's controls, with their values now.
func (o *Offline) Controls() []racktui.Control {
	o.r.mu.Lock()
	defer o.r.mu.Unlock()
	return slices.Clone(o.r.ctls)
}

// Set turns control id to value, as a hand on the panel does, but refuses a
// number outside the control's range rather than stopping the knob at its
// end, as render's --set always has: a render asked for one value and drawn
// at another reports on a picture nobody asked for. A selector takes the
// name it shows as well as its value ("P31 green" as well as "1").
func (o *Offline) Set(id, value string) error {
	o.r.mu.Lock()
	i, ok := o.r.byID[id]
	var c racktui.Control
	if ok {
		c = o.r.ctls[i]
	}
	o.r.mu.Unlock()
	if !ok {
		return fmt.Errorf("the rack has no control %q", id)
	}
	v := strings.TrimSpace(value)
	switch {
	case c.IsText || c.IsSwitch:
	case c.IsSelect:
		for k, lb := range c.Labels {
			if k < len(c.Options) && !slices.Contains(c.Options, v) && strings.EqualFold(strings.Join(strings.Fields(lb), " "), v) {
				v = c.Options[k]
			}
		}
	case c.Max > c.Min:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil && (f < c.Min || f > c.Max) {
			return fmt.Errorf("%s=%s is outside its range %g .. %g", id, v, c.Min, c.Max)
		}
	}
	if c.IsText {
		v = value
	}
	return o.r.Set(id, v)
}

// Models lists the keys SelectModel takes, in the selector's order with each
// family's members after it.
func (o *Offline) Models() []string {
	r := o.r
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	add := func(keys []string) {
		for _, k := range keys {
			if k != "" && !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	if r.selector != "" {
		add(r.ctls[r.byID[r.selector]].Options)
	}
	for _, c := range r.ctls {
		if isFamily(c.ID) {
			add(c.Options)
		}
	}
	return out
}

// SelectModel puts the model selector on key, or, for a member of a family
// (Sprott B–S), on the family's position with its lettered dial on key.
func (o *Offline) SelectModel(key string) error {
	r := o.r
	r.mu.Lock()
	sel, famID, famPos := r.selector, "", ""
	if sel == "" {
		r.mu.Unlock()
		return fmt.Errorf("the rack has no model selector")
	}
	onSel := slices.Contains(r.ctls[r.byID[sel]].Options, key)
	if !onSel {
		for _, c := range r.ctls {
			if isFamily(c.ID) && len(c.Options) > 0 && slices.Contains(c.Options, key) {
				famID, famPos = c.ID, c.Options[0]
				break
			}
		}
	}
	r.mu.Unlock()
	switch {
	case onSel:
		return o.Set(sel, key)
	case famID != "":
		if err := o.Set(sel, famPos); err != nil {
			return err
		}
		return o.Set(famID, key)
	}
	return fmt.Errorf("the rack has no model %q", key)
}

// SetPose puts the model at the X→Y→Z euler pose a, in radians, as a page
// link's rot does (in degrees there); the rate knobs and auto-rotate turn it
// on from there.
func (o *Offline) SetPose(a [3]float64) {
	o.r.mu.Lock()
	o.r.poseTo = &a
	o.r.mu.Unlock()
}

// SetBackground draws the scene on c rather than on the panel's near-black,
// as a video of the page wants its canvas's black. It is the whole
// process's: a render draws one rack.
func (o *Offline) SetBackground(c color.RGBA) { sceneBack = c }

// Route routes control id as a page link's m. or vm. key does, from spec in
// the link's own form: source~bands~depth[~shape], such as
// st~0,0,0,0,0,0,0,0~0.4~swing. The bands may be fewer than the EQ's.
func (o *Offline) Route(id, spec string) error {
	f := strings.Split(spec, "~")
	if len(f) != 3 && len(f) != 4 {
		return fmt.Errorf("%s: a route is source~bands~depth[~shape], not %q", id, spec)
	}
	var m modRoute
	for _, c := range attractor.ModChannels() {
		if c.Name == f[0] || c.Key == f[0] { // the name a link stores, or the column's label
			m.channel = c.Name
		}
	}
	if m.channel == "" {
		return fmt.Errorf("%s: no source %q", id, f[0])
	}
	for i, b := range strings.Split(f[1], ",") {
		if i >= len(m.bands) {
			break
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(b), 32)
		if err != nil {
			return fmt.Errorf("%s: band %q: %w", id, b, err)
		}
		m.bands[i] = float32(v)
	}
	lvl, err := strconv.ParseFloat(f[2], 32)
	if err != nil {
		return fmt.Errorf("%s: depth %q: %w", id, f[2], err)
	}
	m.level = float32(lvl)
	if len(f) == 4 && f[3] != "push" { // push is the default, which a link leaves out
		if !attractor.IsModShape(f[3]) {
			return fmt.Errorf("%s: no shape %q", id, f[3])
		}
		m.shape = f[3]
	}
	r := o.r
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.modRow(id) < 0 {
		return fmt.Errorf("the Mod matrix has no row %q for this model", id)
	}
	r.mod.routes[id] = m
	return nil
}

// Samples is how many samples frame i stands for: the frames' shares of a
// second, counted so that they add up to the second exactly.
func (o *Offline) Samples(i int) int {
	return (i+1)*sampleRate/o.fps - i*sampleRate/o.fps
}

// Frame draws the next frame, w by h, after feeding the rack this frame's
// stretch of the capture, l and r (Samples long; shorter is padded with
// silence). It returns the picture and what the rack's speakers played over
// the frame: its generators, Model Out and the rest, as the Mixer sends them.
func (o *Offline) Frame(w, h int, l, r []float32) (img *rackpic.Image, spkL, spkR []float32, err error) {
	n := o.Samples(o.frame)
	s := o.r.sound
	s.capt.feed(l, r, n)
	o.spk[0], o.spk[1] = grown(o.spk[0], n), grown(o.spk[1], n)
	o.frame++
	o.now = o.epoch.Add(time.Duration(int64(o.frame) * int64(time.Second) / int64(o.fps)))
	// The speakers' samples, and with them the rack's signal, made up to
	// the frame's end, as the speaker asks for them while it plays.
	o.r.Fill(o.spk[0], o.spk[1])
	img, err = o.r.Scene(w, h, 1)
	return img, o.spk[0], o.spk[1], err
}

// feed puts n samples of l and r into the capture, padded with silence past
// their end, as a capture writing them would.
func (c *capture) feed(l, r []float32, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range n {
		var a, b float32
		if i < len(l) {
			a = l[i]
		}
		if i < len(r) {
			b = r[i]
		}
		c.fifo[0] = append(c.fifo[0], a)
		c.fifo[1] = append(c.fifo[1], b)
	}
	if over := len(c.fifo[0]) - captureDepth; over > 0 {
		// Not taken (no model listens): kept no deeper than a recording.
		c.fifo[0] = append(c.fifo[0][:0], c.fifo[0][over:]...)
		c.fifo[1] = append(c.fifo[1][:0], c.fifo[1][over:]...)
	}
}
