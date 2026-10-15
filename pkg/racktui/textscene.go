package racktui

import (
	"image/color"

	"github.com/gdamore/tcell/v3"
)

// TextScener is a source whose model can be a terminal's screen: a shell,
// or a program that draws in a terminal. A picture of one, half a cell a
// pixel, cannot be read; on a terminal the cells are drawn as the characters
// they are, and the terminal is sized to the scene.
type TextScener interface {
	// TextScene is the model's screen as cells, row by row, cols across, for
	// a scene w by h cells; ok false for a model that is not a terminal.
	TextScene(w, h int) (cells []TextCell, cols, rows int, ok bool)
}

// TextCell is a character and its colors.
type TextCell struct {
	Ch     rune
	Fg, Bg color.RGBA
}

// textScene is the terminal on show, as last asked for.
type textScene struct {
	cells      []TextCell
	cols, rows int
}

// refreshText asks for the model's screen as text, and reports whether it
// is one.
func (p *panel) refreshText() bool {
	ts, ok := p.src.(TextScener)
	if !ok {
		p.text = nil
		return false
	}
	cells, cols, rows, ok := ts.TextScene(p.scrW, maxi(p.scrH-1, 1))
	if !ok {
		p.text = nil
		return false
	}
	p.text = &textScene{cells: cells, cols: cols, rows: rows}
	return true
}

// drawText draws the terminal on show across w x h cells, from the top
// left; what it does not cover is the scene's ground.
func (p *panel) drawText(sc tcell.Screen, w, h int) {
	t := p.text
	r := p.winR
	under := p.winMode == winHalf && r.W > 0
	ground := tcell.StyleDefault.Background(tcell.NewRGBColor(0, 0, 0))
	for y := range h {
		for x := range w {
			if under && x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
				continue
			}
			if x >= t.cols || y >= t.rows || y*t.cols+x >= len(t.cells) {
				sc.SetContent(x, y, ' ', nil, ground)
				continue
			}
			c := t.cells[y*t.cols+x]
			ch := c.Ch
			if ch < ' ' {
				ch = ' '
			}
			st := tcell.StyleDefault.
				Foreground(tcell.NewRGBColor(int32(c.Fg.R), int32(c.Fg.G), int32(c.Fg.B))).
				Background(tcell.NewRGBColor(int32(c.Bg.R), int32(c.Bg.G), int32(c.Bg.B)))
			sc.SetContent(x, y, ch, nil, st)
		}
	}
}
