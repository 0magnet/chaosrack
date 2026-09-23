package racktui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

// The panel: the bays above, the controls below, a cursor on one of them.
//
// Laid out the way the rack is rather than the way a settings list is — the
// drawing at the top is the same drawing uitool rack prints, so what you are
// turning a knob on is on screen above the knob.

type panel struct {
	src  Source
	rack string
	ctls []Control
	all  []Control // before the filter
	cur  int
	top  int    // first control drawn, for scrolling
	filt string // substring filter
	list bool   // show the table instead of the rack
	msg  string // the last thing that happened
	err  string
}

var (
	stNormal = tcell.StyleDefault
	stDim    = tcell.StyleDefault.Foreground(color.Gray)
	stHead   = tcell.StyleDefault.Bold(true)
	stCursor = tcell.StyleDefault.Reverse(true)
	stValue  = tcell.StyleDefault.Foreground(color.Green)
	stErr    = tcell.StyleDefault.Foreground(color.Red)
)

// RunOn draws the panel on a screen the caller already has.
func RunOn(sc tcell.Screen, src Source) error {
	if err := sc.Init(); err != nil {
		return err
	}
	defer sc.Fini()
	sc.SetStyle(stNormal)

	p := &panel{src: src}
	p.reload()
	// v3 hands events over a channel and reports key RELEASES as well as
	// presses — without the Pressed check every keystroke would move the
	// cursor twice, which reads as a panel that has lost its detents.
	for {
		p.draw(sc)
		switch ev := (<-sc.EventQ()).(type) {
		case *tcell.EventResize:
			sc.Sync()
		case *tcell.EventKey:
			if !ev.Pressed() {
				continue
			}
			if p.key(ev) {
				return nil
			}
		}
	}
}

// reload asks the rack what it is now.
func (p *panel) reload() {
	r, err := p.src.Rack()
	if err != nil {
		p.err = err.Error()
		return
	}
	c, err := p.src.Controls()
	if err != nil {
		p.err = err.Error()
		return
	}
	p.rack, p.all, p.err = r, c, ""
	p.refilter()
}

func (p *panel) refilter() {
	if p.filt == "" {
		p.ctls = p.all
	} else {
		p.ctls = nil
		f := strings.ToLower(p.filt)
		for _, c := range p.all {
			if strings.Contains(strings.ToLower(c.ID), f) || strings.Contains(strings.ToLower(c.Label), f) {
				p.ctls = append(p.ctls, c)
			}
		}
	}
	if p.cur >= len(p.ctls) {
		p.cur = len(p.ctls) - 1
	}
	if p.cur < 0 {
		p.cur = 0
	}
}

// key handles one keystroke and reports whether to quit.
func (p *panel) key(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		return true
	case tcell.KeyUp:
		p.move(-1)
	case tcell.KeyDown:
		p.move(1)
	case tcell.KeyPgUp:
		p.move(-10)
	case tcell.KeyPgDn:
		p.move(10)
	case tcell.KeyHome:
		p.cur = 0
	case tcell.KeyEnd:
		p.cur = len(p.ctls) - 1
	case tcell.KeyLeft:
		p.nudge(-1)
	case tcell.KeyRight:
		p.nudge(1)
	case tcell.KeyTab:
		p.list = !p.list
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if p.filt != "" {
			p.filt = p.filt[:len(p.filt)-1]
			p.refilter()
		}
	case tcell.KeyRune:
		r := firstRune(ev.Str())
		switch r {
		case 'q':
			if p.filt == "" {
				return true
			}
			p.typeFilter(r)
		case '\t':
			p.list = !p.list
		case '/':
			p.filt = ""
			p.refilter()
		case 'r':
			if p.filt == "" {
				p.reload()
				p.msg = "reloaded"
				return false
			}
			p.typeFilter(r)
		case '0':
			if p.filt == "" {
				p.reset()
				return false
			}
			p.typeFilter(r)
		default:
			p.typeFilter(r)
		}
	}
	return false
}

func (p *panel) typeFilter(r rune) {
	p.filt += string(r)
	p.refilter()
}

func (p *panel) move(n int) {
	if len(p.ctls) == 0 {
		return
	}
	p.cur += n
	if p.cur < 0 {
		p.cur = 0
	}
	if p.cur >= len(p.ctls) {
		p.cur = len(p.ctls) - 1
	}
}

// reset puts the control under the cursor back to its default, which the
// registry knows without asking the rack.
func (p *panel) reset() {
	c, ok := p.at()
	if !ok {
		return
	}
	v := c.SelectDef
	if !c.IsSelect {
		v = trimFloat(c.Def)
	}
	p.apply(c, v)
}

// nudge moves the control under the cursor one detent or one step.
func (p *panel) nudge(dir int) {
	c, ok := p.at()
	if !ok {
		return
	}
	if c.IsSelect {
		p.apply(c, nextOption(c, dir))
		return
	}
	step := c.Step
	if step == 0 {
		step = 1
	}
	v, err := strconv.ParseFloat(c.Value, 64)
	if err != nil {
		v = c.Def
	}
	v += float64(dir) * step
	if c.Max != c.Min {
		if v > c.Max {
			v = c.Max
		}
		if v < c.Min {
			v = c.Min
		}
	}
	p.apply(c, trimFloat(v))
}

// nextOption is the detent dir away from the one showing, stopping at the
// ends: a rotary switch does not wrap and neither does this.
func nextOption(c Control, dir int) string {
	if len(c.Options) == 0 {
		return c.Value
	}
	at := 0
	for i, o := range c.Options {
		if o == c.Value {
			at = i
			break
		}
	}
	at += dir
	if at < 0 {
		at = 0
	}
	if at >= len(c.Options) {
		at = len(c.Options) - 1
	}
	return c.Options[at]
}

func (p *panel) at() (Control, bool) {
	if p.cur < 0 || p.cur >= len(p.ctls) {
		return Control{}, false
	}
	return p.ctls[p.cur], true
}

// apply sends the change and then asks the rack what actually happened: a
// control can clamp, quantize to a detent, or refuse, and showing what was
// asked for rather than what was taken is how a panel comes to lie.
func (p *panel) apply(c Control, v string) {
	if err := p.src.Set(c.ID, v); err != nil {
		p.err = err.Error()
		return
	}
	p.err = ""
	p.msg = c.ID + " = " + v
	p.reload()
}

func trimFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// ── drawing ──────────────────────────────────────────────────────────────

func (p *panel) draw(sc tcell.Screen) {
	sc.Clear()
	w, h := sc.Size()
	y := 0

	// The bays, as the rack draws them.
	for _, line := range strings.Split(strings.TrimRight(p.rack, "\n"), "\n") {
		if y >= h/2 {
			break
		}
		puts(sc, 0, y, line, stDim)
		y++
	}
	y++

	if !p.list {
		p.drawRack(sc, y, w, h)
	} else {
		p.drawList(sc, y, w, h)
	}

	// The status line: what just happened, or how to work it.
	status := "↑↓ move   ←→ turn   0 reset   tab rack/list   r reload   / clear filter   q quit"
	st := stDim
	switch {
	case p.err != "":
		status, st = "error: "+p.err, stErr
	case p.filt != "":
		status = "filter: " + p.filt + "   (" + strconv.Itoa(len(p.ctls)) + " of " + strconv.Itoa(len(p.all)) + ")   backspace to edit, / to clear"
	case p.msg != "":
		status = p.msg
	}
	puts(sc, 0, h-1, clip(status, w), st)
	sc.Show()
}

func rangeOf(c Control) string {
	if c.IsSelect {
		if len(c.Options) > 0 {
			return strings.Join(c.Options, " ")
		}
		return "select"
	}
	return fmt.Sprintf("%g .. %g / %g", c.Min, c.Max, c.Step)
}

func puts(sc tcell.Screen, x, y int, s string, st tcell.Style) {
	for _, r := range s {
		sc.SetContent(x, y, r, nil, st)
		x++
	}
}

func clip(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	return string(r[:w])
}

// firstRune is the key a KeyRune event carries. v3 reports a string because a
// keystroke can be a composed sequence; a panel of single-key commands wants
// the first of it and nothing else.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// drawRack draws the controls as the instrument: panels, dials and lamps.
func (p *panel) drawRack(sc tcell.Screen, y, w, h int) {
	panels, where := groupByModule(p.ctls)
	cp, ci := -1, -1
	if p.cur >= 0 && p.cur < len(where) {
		cp, ci = where[p.cur][0], where[p.cur][1]
	}
	lines, spots := layoutRack(panels, w)

	// Scroll so the control under the cursor stays on screen.
	var curY0, curY1 = -1, -1
	for _, s := range spots {
		if s.Panel == cp && s.Index == ci {
			curY0, curY1 = s.Y0, s.Y1
			break
		}
	}
	rows := h - y - 1
	if rows < 1 {
		rows = 1
	}
	if curY0 >= 0 {
		if curY0 < p.top {
			p.top = curY0
		}
		if curY1 >= p.top+rows {
			p.top = curY1 - rows + 1
		}
	}
	if p.top < 0 {
		p.top = 0
	}
	for i := p.top; i < len(lines) && i < p.top+rows; i++ {
		puts(sc, 0, y+i-p.top, clip(lines[i], w), stDim)
	}
	// The cursor, drawn over the panel it is in.
	if curY0 >= 0 {
		for _, s := range spots {
			if s.Panel != cp || s.Index != ci {
				continue
			}
			for yy := s.Y0; yy <= s.Y1; yy++ {
				if yy < p.top || yy-p.top >= rows {
					continue
				}
				line := []rune(lines[yy])
				for x := s.X; x < s.X+panelWidth && x < len(line) && x < w; x++ {
					sc.SetContent(x, y+yy-p.top, line[x], nil, stCursor)
				}
			}
		}
	}
}

// drawList draws the controls as a table, which is the view for finding one
// by name among seventy rather than for turning it.
func (p *panel) drawList(sc tcell.Screen, y, w, h int) {
	puts(sc, 0, y, fmt.Sprintf("%-24s %-14s %-12s %s", "CONTROL", "LABEL", "VALUE", "RANGE"), stHead)
	y++
	rows := h - y - 1
	if rows < 1 {
		rows = 1
	}
	if p.cur < p.top {
		p.top = p.cur
	}
	if p.cur >= p.top+rows {
		p.top = p.cur - rows + 1
	}
	for i := p.top; i < len(p.ctls) && i < p.top+rows; i++ {
		c := p.ctls[i]
		st := stNormal
		if i == p.cur {
			st = stCursor
		}
		puts(sc, 0, y, fmt.Sprintf("%-24s %-14s", clip(c.ID, 24), clip(c.Label, 14)), st)
		vs := stValue
		if i == p.cur {
			vs = st
		}
		puts(sc, 40, y, fmt.Sprintf("%-12s", clip(c.Value, 12)), vs)
		puts(sc, 53, y, clip(rangeOf(c), max(0, w-54)), stDim)
		y++
	}
}
