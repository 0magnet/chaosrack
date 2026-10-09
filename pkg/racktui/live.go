package racktui

import (
	"sync"
	"time"

	"github.com/gdamore/tcell/v3"
)

// The panel follows the rack, and takes the mouse.
//
// Before this the panel asked the rack what it was only when a key was
// pressed here, so a knob turned on the page sat at its old value in the
// terminal until something was typed. Now a ticker asks every refreshEvery,
// and the panel redraws when an answer differs from what it shows.
//
// The ticker only POSTS; the asking is done on the event loop, which is the
// one goroutine that touches the panel. And once the screen is going it posts
// nothing more: Fini closes the event queue, and a send on it panics (surplus
// met that as a search timer firing after q).

// refreshEvery is how often the panel asks the rack for its values. Fast
// enough that a knob turned on the page is seen turning; slow enough that the
// asking, which over the cable is a round trip, is not the panel's work.
const refreshEvery = 250 * time.Millisecond

// refreshTick is the Data of the interrupt the ticker posts.
type refreshTick struct{}

// poster sends events to a screen until it is closed.
type poster struct {
	sc     tcell.Screen
	mu     sync.RWMutex
	closed bool
}

func (q *poster) post(ev tcell.Event) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return
	}
	select {
	case q.sc.EventQ() <- ev:
	default: // the loop is behind; the next tick says the same thing
	}
}

// close stops all posting; it is called before Fini.
func (q *poster) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
}

// tick posts a refresh every refreshEvery until done is closed.
func (q *poster) tick(done <-chan struct{}) {
	t := time.NewTicker(refreshEvery)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			q.post(tcell.NewEventInterrupt(refreshTick{}))
		}
	}
}

// refresh reads the controls again and reports whether anything shown
// changed. Only the values and option lists are taken over: a control that
// came or went means the rack itself changed (a module switched out, a
// model's bank swapped), and that is a reload, layout and all.
func (p *panel) refresh() bool {
	picChanged := p.refreshPicture()
	fresh, err := p.src.Controls()
	if err != nil {
		if p.err != err.Error() {
			p.err = err.Error()
			return true
		}
		return false
	}
	byID := make(map[string]Control, len(fresh))
	for _, c := range fresh {
		byID[c.ID] = c
	}
	if len(byID) != len(p.all) {
		p.reload()
		return true
	}
	changed := false
	update := func(cs []Control) {
		for i := range cs {
			f, ok := byID[cs[i].ID]
			if !ok {
				continue
			}
			if cs[i].Value != f.Value || !sameStrings(cs[i].Options, f.Options) {
				cs[i].Value, cs[i].Options = f.Value, f.Options
				changed = true
			}
		}
	}
	for _, c := range p.all {
		if _, ok := byID[c.ID]; !ok {
			p.reload()
			return true
		}
	}
	update(p.all)
	if p.filt != "" {
		update(p.ctls)
	}
	for i := range p.mods {
		update(p.mods[i].Ctls)
	}
	if changed && p.err != "" {
		p.err = ""
	}
	return changed || picChanged
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── the mouse ────────────────────────────────────────────────────────────

// mouse handles one mouse event: a click chooses the control under it (and,
// on a P-unit, presses the button it lands on), and the wheel turns the
// control under it, or scrolls the rack where there is none.
func (p *panel) mouse(ev *tcell.EventMouse, w, h int) {
	b := ev.Buttons()
	// Only a press counts. Terminals report a release, and a move with the
	// button held, as events too; acting on each would click twice.
	pressed := b&tcell.Button1 != 0 && p.held&tcell.Button1 == 0
	p.held = b & (tcell.Button1 | tcell.Button2 | tcell.Button3)
	x, y := ev.Position()

	if p.list {
		p.mouseList(b, pressed, y, h)
		return
	}
	vw, vh := maxi(w-1, 1), maxi(h-2, 1)
	if p.scrollBar(b, pressed, x, y, vw, vh) {
		return
	}
	if x >= vw || y >= vh {
		return
	}
	if p.pic != nil {
		p.mousePage(b, pressed, x, y)
		return
	}
	sx, sy := x+p.view.X, y+p.view.Y
	hits := p.hitsAt(sx, sy)

	switch {
	case b&tcell.WheelUp != 0, b&tcell.WheelDown != 0:
		dir := 1
		if b&tcell.WheelDown != 0 {
			dir = -1
		}
		if len(hits) == 0 {
			p.pan(0, -dir)
			return
		}
		p.choose(p.pick(hits))
		p.nudge(dir)
	case b&tcell.WheelLeft != 0:
		p.pan(-1, 0)
	case b&tcell.WheelRight != 0:
		p.pan(1, 0)
	case pressed:
		if len(hits) == 0 {
			return
		}
		// A unit shared by several controls: a second click on it moves to
		// the next one, as the page's legend button steps through them.
		id := p.pick(hits)
		if c, ok := p.at(); ok && c.ID == id && len(hits) > 1 {
			id = hits[(indexOf(hits, id)+1)%len(hits)]
		}
		p.choose(id)
		p.press(sx, sy)
	}
}

// mouseList handles the mouse over the table.
func (p *panel) mouseList(b tcell.ButtonMask, pressed bool, y, h int) {
	switch {
	case b&tcell.WheelUp != 0:
		p.move(-1)
	case b&tcell.WheelDown != 0:
		p.move(1)
	case pressed && y >= 1 && y < h-1:
		if i := p.top + y - 1; i < len(p.ctls) {
			p.cur = i
		}
	}
}

// hitsAt is every control drawn over the surface cell sx, sy: one, or the
// members of a shared P-unit, or none.
func (p *panel) hitsAt(sx, sy int) []string {
	var out []string
	for mi, m := range p.mods {
		for ci, c := range m.Ctls {
			x, y, cw, ch, ok := ctlRect(p.surf, p.mods, ctlAt{Module: mi, Index: ci})
			if ok && sx >= x && sx < x+cw && sy >= y && sy < y+ch {
				out = append(out, c.ID)
			}
		}
	}
	return out
}

// pick is the one of hits to act on: the selected control when it is among
// them, which keeps the wheel on the member of a shared unit already chosen.
func (p *panel) pick(hits []string) string {
	if c, ok := p.at(); ok && indexOf(hits, c.ID) >= 0 {
		return c.ID
	}
	return hits[0]
}

// choose moves the cursor to the control id, clearing a filter that hides it.
func (p *panel) choose(id string) {
	for i, c := range p.ctls {
		if c.ID == id {
			p.cur = i
			p.shown = id // chosen where it is seen: no need to scroll to it
			return
		}
	}
	p.filt = ""
	p.refilter()
	for i, c := range p.ctls {
		if c.ID == id {
			p.cur, p.shown = i, id
			return
		}
	}
}

// press works the P-unit button at surface cell sx, sy, if one is there: the
// resets and the + 0 − trio down the unit's right edge.
func (p *panel) press(sx, sy int) {
	if look != LookPUnit {
		return
	}
	cur := p.cursor()
	x, y, _, _, ok := ctlRect(p.surf, p.mods, cur)
	if !ok || sx-x != puButtons {
		return
	}
	// The top reset is the step trim's, which this panel has no state for.
	switch sy - y {
	case 1, 3:
		p.reset()
	case 2:
		p.nudge(1)
	case 4:
		p.nudge(-1)
	}
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

// scrollBar works the scroll bars: a press on one moves the window there and
// a drag that began on one follows the pointer, as a scroll bar does. It
// reports whether the event was the bars'.
//
// The ends of the track are the ends of the rack, so the first and last row
// of a bar reach them however short the bar is.
func (p *panel) scrollBar(b tcell.ButtonMask, pressed bool, x, y, vw, vh int) bool {
	if b&tcell.Button1 == 0 {
		p.dragging = 0
		return false
	}
	if pressed {
		switch {
		case x == vw && y < vh:
			p.dragging = 'v'
		case y == vh && x < vw:
			p.dragging = 'h'
		default:
			p.dragging = 0
			return false
		}
	}
	s := p.surface()
	switch p.dragging {
	case 'v':
		p.view.Y = along(y, vh, s.Rows-p.view.H)
	case 'h':
		p.view.X = along(x, vw, s.Cols-p.view.W)
	default:
		return false
	}
	p.view = p.view.Clamp(s)
	return true
}

// along is how far into a range of span the pointer at pos of a track n long
// stands, with the track's ends at the range's.
func along(pos, n, span int) int {
	if n <= 1 || span <= 0 {
		return 0
	}
	return min(max(pos, 0), n-1) * span / (n - 1)
}
