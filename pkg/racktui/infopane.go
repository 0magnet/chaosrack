package racktui

import (
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"

	"github.com/0magnet/chaosrack/manual"
)

// The Info pane (i): the page's Info window, in the terminal. While a model
// is on show it is the model's description, as the page's is; under it, and
// alone with the rack off, the manual's page for the bay the cursor is in —
// each module on it, what it is, each of its controls by address with its
// entry — scrolled to the control under the cursor. The page shows that page
// only with the rack off, beside tooltips that say what each control is; a
// terminal has no tooltips, so the pane keeps it whatever is running.
//
// Beside it, the first sentence of the control's entry goes on the status
// line as the cursor moves, as a hint that there is more.

// infoPane is the pane's state.
type infoPane struct {
	on bool
	// top is the first line shown; at is the address it was last scrolled
	// to, so a cursor that moves takes the pane with it and one that does
	// not leaves the reader where they scrolled.
	top int
	at  string
	// page is the last page fetched, of bay, when; a fetch may cross a wire,
	// so the page is kept a while rather than asked for every frame.
	page  ManualPage
	bay   int
	when  time.Time
	moved bool // the cursor has moved: the status line hints
	// rows is how many lines it last showed: a page, to PgUp and PgDn.
	rows int
}

// infoRefresh is how long a fetched page stands: long enough not to ask a
// page across a wire every frame, short enough to follow a model change.
const infoRefresh = time.Second

// The pane's looks.
var (
	stInfo      = tcell.StyleDefault.Background(color.NewRGBColor(16, 20, 26))
	stInfoTitle = stInfo.Bold(true).Foreground(color.NewRGBColor(255, 210, 74))
	stInfoHead  = stInfo.Bold(true).Foreground(color.NewRGBColor(138, 204, 255))
	stInfoAddr  = stInfo.Foreground(color.NewRGBColor(125, 148, 166))
	stInfoHere  = stInfo.Reverse(true)
)

// cursorLoc is the address of the control under the cursor, and its bay.
func (p *panel) cursorLoc() (loc string, bay int) {
	if p.cur < 0 || p.cur >= len(p.ctls) {
		return "", 0
	}
	loc = p.ctls[p.cur].Loc
	b, err := strconv.Atoi(strings.SplitN(loc, ".", 2)[0])
	if err != nil {
		return loc, 0
	}
	return loc, b
}

// manualPage is the page of the cursor's bay, fetched when it is another
// bay's or has stood long enough; ok is false for a Source with no manual.
func (p *panel) manualPage() (ManualPage, bool) {
	m, ok := p.src.(Manualed)
	if !ok {
		return ManualPage{}, false
	}
	_, bay := p.cursorLoc()
	if bay == 0 {
		bay = max(p.info.bay, 1)
	}
	if bay != p.info.bay || time.Since(p.info.when) > infoRefresh {
		pg, err := m.Manual(bay)
		if err != nil {
			return p.info.page, true
		}
		p.info.page, p.info.bay, p.info.when = pg, bay, time.Now()
	}
	return p.info.page, true
}

// hint is the first sentence of what the manual says of the control at loc,
// or "".
func (p *panel) hint(loc string) string {
	if loc == "" {
		return ""
	}
	pg, ok := p.manualPage()
	if !ok {
		return ""
	}
	for _, m := range pg.Mods {
		for _, c := range m.Cells {
			if c.Loc == loc {
				return manual.FirstSentence(manual.EntryText(c.Tip, c.Keys))
			}
		}
	}
	return ""
}

// infoLines are the pane's lines, wrapped to width.
func (p *panel) infoLines(width int) []manual.Line {
	pg, ok := p.manualPage()
	if !ok {
		return manual.Wrap([]manual.Line{{Text: "Info", Kind: manual.LineTitle}, {}, {Text: "This rack has no manual to show."}}, width)
	}
	var lines []manual.Line
	if pg.Model != "" {
		lines = append(manual.TextLines(pg.Model, pg.ModelText), manual.Line{})
	}
	lines = append(lines, manual.BayLines(pg.Title, pg.Mods)...)
	return manual.Wrap(lines, width)
}

// infoWidth is how wide the pane is on a screen w wide: a third of it,
// enough to read, never the whole.
func infoWidth(w int) int { return min(max(w/3, 44), w-12) }

// drawInfo draws the pane over the right of the screen, above the status
// line.
func (p *panel) drawInfo(sc tcell.Screen, w, h int) {
	pw := infoWidth(w)
	if pw < 20 || h < 4 {
		return
	}
	x0, rows := w-pw, h-2
	p.info.rows = rows
	lines := p.infoLines(pw - 3)
	loc, _ := p.cursorLoc()
	// Scrolled to the cursor's control when the cursor has moved to it.
	if loc != p.info.at {
		p.info.at = loc
		for i, l := range lines {
			if l.Loc == loc && loc != "" {
				p.info.top = max(i-2, 0)
				break
			}
		}
	}
	p.info.top = min(max(p.info.top, 0), max(len(lines)-rows, 0))
	for y := range h - 1 {
		sc.SetContent(x0, y, '│', nil, stInfoAddr)
		for x := x0 + 1; x < w; x++ {
			sc.SetContent(x, y, ' ', nil, stInfo)
		}
	}
	title := " Info   i close   PgUp/PgDn scroll "
	puts(sc, x0+1, 0, clip(title, pw-1), stInfoAddr)
	for r := range rows {
		i := p.info.top + r
		if i >= len(lines) {
			break
		}
		l := lines[i]
		st := stInfo
		switch l.Kind {
		case manual.LineTitle:
			st = stInfoTitle
		case manual.LineHeading:
			st = stInfoHead
		case manual.LineAddress:
			st = stInfoAddr
			if l.Text == loc {
				st = stInfoHere
			}
		}
		puts(sc, x0+2+l.Indent, r+1, clip(l.Text, pw-2-l.Indent), st)
	}
}

// infoKey handles the pane's keys, and reports whether it took ev: i opens
// and closes it (a letter that starts a filter, mid-filter), and while it is
// open PgUp and PgDn scroll it.
func (p *panel) infoKey(ev *tcell.EventKey) bool {
	if _, ok := p.src.(Manualed); ok && ev.Key() == tcell.KeyRune && ev.Str() == "i" &&
		ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 && p.commands() {
		p.info.on, p.info.at = !p.info.on, ""
		return true
	}
	if !p.info.on {
		return false
	}
	page := max(p.info.rows-2, 1)
	switch ev.Key() {
	case tcell.KeyPgUp:
		p.info.top -= page
	case tcell.KeyPgDn:
		p.info.top += page
	default:
		return false
	}
	return true
}

// cursorHint is the status line's hint for the control under the cursor:
// its name, and the first sentence of its entry.
func (p *panel) cursorHint() string {
	loc, _ := p.cursorLoc()
	h := p.hint(loc)
	if h == "" {
		return ""
	}
	name := p.ctls[p.cur].Label
	if name == "" {
		name = p.ctls[p.cur].ID
	}
	return name + " (" + loc + ")   " + h
}
