package rackhost

import (
	"strconv"
	"strings"

	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/rhythm"
)

// The Matrix in the picture: its pads lit as the pattern is and the playhead
// passes, its rhythm tabs down or up, and its beat lamps; and a press on any
// of them, which no control's box describes. The catalog marks each part
// (rackpic.Item.Mark), and its colors are panel.css's.

// The pads' colors (panel.css, .tm-cell): a lit pad's is its gradient's
// body, as the picture reads a gradient.
const (
	padOff   = 0x10151a
	padEdge  = 0x232b33
	drumEdge = 0x2c2a22
	phOff    = 0x182028 // a column under the playhead
	phEdge   = 0x2e3944
	padOn    = 0x0b3a20
	phOn     = 0x5fd894
	beatLit  = 0xff3b30
)

// laneOn is each drum lane's lit color: bass drum, snare, hi-hat, cymbal.
var laneOn = [rhythm.VoiceCount]int{0xa8480b, 0xa88a0b, 0x0b7fa8, 0x5b2ba8}

// lanes is how many rows of pads there are, pitches and drum lanes.
const lanes = rhythm.GridRows + rhythm.VoiceCount

// matrixPic is where the Matrix's parts are in the picture and what they
// show.
type matrixPic struct {
	// pad is each pad's item, by the column the catalog drew it in and its
	// row, plus one: 0 is none. The lanes were drawn four to the beat; a
	// triplet preset draws three of each beat's four, narrower (layLanes).
	pad  [rhythm.GridSteps][lanes]int
	base map[int]rackpic.Item // the parts as the catalog drew them
	tabs map[string]int
	// tabDown and tabUp are how a tab looks pressed and not.
	tabDown, tabUp *rackpic.Item
	beats          []int
	marked         []int // every marked item, for presses

	pat  [rhythm.GridSteps][rhythm.GridRows]bool
	drum [rhythm.GridSteps][rhythm.VoiceCount]bool

	drawn  bool // whether it has been drawn once
	shown  matrixView
	preset string
}

// findMatrix finds the Matrix's parts among the items outside the bank.
func (r *Rack) findMatrix() {
	mp := &r.mx
	mp.base = map[int]rackpic.Item{}
	mp.tabs = map[string]int{}
	down := r.calib["rhythm-preset"]
	if down == "" {
		down = rhythm.DefaultPreset
	}
	for i, it := range r.pic.Items[:r.outside] {
		if it == nil || it.Mark == "" {
			continue
		}
		mp.marked = append(mp.marked, i)
		mp.base[i] = *it
		f := strings.Fields(it.Mark)
		switch f[0] {
		case "pad":
			c, row, ok := padOf(f)
			if ok && c < rhythm.GridSteps && row < lanes {
				mp.pad[c][row] = i + 1
			}
		case "tab":
			if len(f) < 2 {
				continue
			}
			mp.tabs[f[1]] = i
			cp := *it
			if f[1] == down {
				mp.tabDown = &cp
			} else if mp.tabUp == nil {
				mp.tabUp = &cp
			}
		case "beat":
			mp.beats = append(mp.beats, i)
		}
	}
}

// padOf reads "pad C R".
func padOf(f []string) (c, row int, ok bool) {
	if len(f) != 3 {
		return 0, 0, false
	}
	c, err1 := strconv.Atoi(f[1])
	row, err2 := strconv.Atoi(f[2])
	return c, row, err1 == nil && err2 == nil && c >= 0 && row >= 0
}

// laneItem is the item of lane step d of drum lane v, for perBeat steps to
// the beat, plus one: 0 is none.
func (mp *matrixPic) laneItem(d, v, perBeat int) int {
	b, k := d/perBeat, d%perBeat
	c := b*rhythm.GridPerBeat + k
	if b >= rhythm.GridBeats || c >= rhythm.GridSteps {
		return 0
	}
	return mp.pad[c][rhythm.GridRows+v]
}

// layLanes draws the lanes perBeat to the beat: each beat's pads spread
// across the room four took, the ones it does not need taken away, as the
// page builds the lanes again for a triplet beat.
func (r *Rack) layLanes(perBeat int) {
	mp := &r.mx
	for v := range rhythm.VoiceCount {
		row := rhythm.GridRows + v
		for b := range rhythm.GridBeats {
			first, last := mp.pad[b*rhythm.GridPerBeat][row], mp.pad[b*rhythm.GridPerBeat+rhythm.GridPerBeat-1][row]
			second := mp.pad[b*rhythm.GridPerBeat+1][row]
			if first == 0 || last == 0 || second == 0 {
				continue
			}
			f, l, s := mp.base[first-1], mp.base[last-1], mp.base[second-1]
			gap := s.X - (f.X + f.W)
			w := (l.X + l.W - f.X - float64(perBeat-1)*gap) / float64(perBeat)
			for k := range rhythm.GridPerBeat {
				i := mp.pad[b*rhythm.GridPerBeat+k][row] - 1
				if i < 0 {
					continue
				}
				if k >= perBeat {
					r.setItem(i, nil)
					continue
				}
				it := mp.base[i]
				if perBeat != rhythm.GridPerBeat {
					it.X, it.W = f.X+float64(k)*(w+gap), w
				}
				r.setItem(i, &it)
			}
		}
	}
}

// setItem puts it in the picture at item i, as the next patch's.
func (r *Rack) setItem(i int, it *rackpic.Item) {
	old := r.pic.Items[i]
	if old == nil && it == nil || old != nil && it != nil && sameLook(old, it) {
		return
	}
	r.pic.Gen++
	r.pic.Items[i] = it
	r.changed[i] = r.pic.Gen
}

// sameLook reports whether two items draw the same.
func sameLook(a, b *rackpic.Item) bool {
	return a.X == b.X && a.W == b.W && a.Y == b.Y && a.H == b.H &&
		eqInt(a.Fill, b.Fill) && eqInt(a.Border, b.Border) && eqInt(a.Ink, b.Ink)
}

func eqInt(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// paint colors pad item i (plus one), if it is drawn.
func (r *Rack) paint(i, fill int, border *int) {
	if i == 0 || r.pic.Items[i-1] == nil {
		return
	}
	it := *r.pic.Items[i-1]
	it.Fill = &fill
	if r.mx.base[i-1].Border != nil {
		// Only where the catalog drew the edge: at a small scale a pad's
		// 1 px border is less than a pixel, and the picture has none.
		it.Border = border
	}
	r.setItem(i-1, &it)
}

// padLook is how a pad of row is drawn: lit or not, under the playhead or
// not, and past the end of the loop (dimmed, as the page dims those columns).
func padLook(row int, on, ph, past bool) (int, *int) {
	fill, border := padOff, padEdge
	drum := row >= rhythm.GridRows
	switch {
	case on && drum:
		fill, border = laneOn[row-rhythm.GridRows], -1
		if ph {
			fill = brighten(fill, 1.6)
		}
	case on && ph:
		fill, border = phOn, padOn
	case on:
		fill, border = padOn, -1
	case ph:
		fill, border = phOff, phEdge
	case drum:
		border = drumEdge
	}
	if past {
		// The column's opacity, .35, over the module.
		fill = mixRGB(fill, pinDark, 0.65)
		if border >= 0 {
			border = mixRGB(border, pinDark, 0.65)
		}
	}
	if border < 0 {
		return fill, nil
	}
	return fill, &border
}

// brighten is CSS brightness(k) of a color.
func brighten(c int, k float64) int {
	out := 0
	for sh := 16; sh >= 0; sh -= 8 {
		out |= min(255, int(float64(c>>sh&0xff)*k)) << sh
	}
	return out
}

// showMatrix brings the Matrix's picture up to the sound: its pads, its
// playhead, its beat lamps and its tabs. Called with r.mu held.
func (r *Rack) showMatrix() {
	mp := &r.mx
	if len(mp.marked) == 0 {
		return
	}
	s := r.sound
	s.mu.Lock()
	v := s.mat.view()
	edited := !mp.drawn || v.edits != mp.shown.edits
	if edited {
		mp.pat, mp.drum = s.mat.pat, s.mat.drum
	}
	s.mu.Unlock()

	old := mp.shown
	if !mp.drawn || v.perBeat != old.perBeat {
		r.layLanes(v.perBeat)
	}
	if edited || v.steps != old.steps || v.perBeat != old.perBeat {
		for c := range rhythm.GridSteps {
			r.paintColumn(c, v)
		}
		for d := range rhythm.GridBeats * v.perBeat {
			r.paintLane(d, v)
		}
	} else {
		if old.col != v.col {
			r.paintColumn(old.col, v)
			r.paintColumn(v.col, v)
		}
		if old.lane != v.lane {
			r.paintLane(old.lane, v)
			r.paintLane(v.lane, v)
		}
	}
	if !mp.drawn || v.beat != old.beat || v.beats != old.beats {
		for k, i := range mp.beats {
			if k >= v.beats {
				r.setItem(i, nil)
				continue
			}
			it := mp.base[i]
			if k == v.beat {
				lit := beatLit
				it.Fill = &lit
			}
			r.setItem(i, &it)
		}
	}
	if p := r.value("rhythm-preset"); !mp.drawn || p != mp.preset {
		mp.preset = p
		for name, i := range mp.tabs {
			look := mp.tabUp
			if name == p {
				look = mp.tabDown
			}
			if look == nil {
				continue
			}
			it := mp.base[i]
			it.Fill, it.Border, it.Ink = look.Fill, look.Border, look.Ink
			r.setItem(i, &it)
		}
	}
	mp.shown, mp.drawn = v, true
}

// paintColumn draws grid column c's pads.
func (r *Rack) paintColumn(c int, v matrixView) {
	if c < 0 || c >= rhythm.GridSteps {
		return
	}
	for row := range rhythm.GridRows {
		fill, border := padLook(row, r.mx.pat[c][row], c == v.col, c >= v.steps)
		r.paint(r.mx.pad[c][row], fill, border)
	}
}

// paintLane draws lane step d's pads.
func (r *Rack) paintLane(d int, v matrixView) {
	if d < 0 {
		return
	}
	loop := max(1, v.steps/rhythm.GridPerBeat) * v.perBeat
	for ln := range rhythm.VoiceCount {
		fill, border := padLook(rhythm.GridRows+ln, r.mx.drum[d][ln], d == v.lane, d >= loop)
		r.paint(r.mx.laneItem(d, ln, v.perBeat), fill, border)
	}
}

// markAt is the mark of the smallest marked part drawn at x, y, or "".
func (r *Rack) markAt(x, y float64) string {
	best, area := "", -1.0
	for _, i := range append(r.mx.marked[:len(r.mx.marked):len(r.mx.marked)], r.analyzers.ro.btns...) {
		it := r.pic.Items[i]
		if it == nil || x < it.X || x > it.X+it.W || y < it.Y || y > it.Y+it.H {
			continue
		}
		if a := it.W * it.H; area < 0 || a < area {
			best, area = it.Mark, a
		}
	}
	return best
}

// press does what a click on a marked part does: a pad lights or goes out, a
// tab loads its rhythm and starts the loop, Clear puts every pad out. A pad
// or a tab is a hand on the Matrix, which starts its sound (matrix). It
// reports whether the part does anything. Called with r.mu held.
func (r *Rack) press(mark string) bool {
	f := strings.Fields(mark)
	s := r.sound
	switch f[0] {
	case "pad":
		c, row, ok := padOf(f)
		if !ok {
			return false
		}
		s.mu.Lock()
		if row >= rhythm.GridRows {
			// The catalog's column is the lanes' four to the beat.
			pb := s.mat.perBeat
			b, k := c/rhythm.GridPerBeat, c%rhythm.GridPerBeat
			c = b*pb + k
			if k >= pb {
				c = -1
			}
		}
		s.mat.press(c, row)
		s.mat.armed = true
		s.mu.Unlock()
		return true
	case "tab":
		if len(f) < 2 {
			return false
		}
		if r.value("rhythm-preset") == f[1] {
			// The tab already down loads its rhythm again, as the page's does.
			s.mu.Lock()
			r.matrixSet("rhythm-preset")
			s.mu.Unlock()
		} else if err := r.set("rhythm-preset", f[1]); err != nil {
			return false
		}
		if err := r.set("tm-run", "1"); err != nil {
			return false
		}
		s.mu.Lock()
		s.mat.armed = true
		s.mu.Unlock()
		return true
	case "clear":
		s.mu.Lock()
		s.mat.clear()
		s.mu.Unlock()
		return true
	case "btn":
		return len(f) > 1 && r.remeasure(f[1])
	}
	return false
}
