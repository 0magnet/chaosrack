package racktui

import (
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"

	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// Putting the surface on a tcell screen.
//
// The renderer writes cellAt values and knows nothing about tcell; this is the
// only file that turns one into the other. That split is what lets the layout
// and the drawing be tested without a terminal, and it is why DrawSurface
// takes a Painter rather than a Screen.

// screenPainter writes the surface onto a screen at an offset.
type screenPainter struct {
	sc     tcell.Screen
	x0, y0 int
	w, h   int
}

func (p screenPainter) Set(x, y int, c cellAt) {
	if x < 0 || y < 0 || x >= p.w || y >= p.h {
		return
	}
	p.sc.SetContent(p.x0+x, p.y0+y, c.Ch, nil, styleOf(c))
}

// styleOf is the cell's color.
//
// A half-block cell carries two 24-bit colors and no style of its own: the
// glyph's foreground paints the top pixel and its background the bottom, which
// is the whole trick. Everything else is a named panel style.
func styleOf(c cellAt) tcell.Style {
	if c.Art {
		return tcell.StyleDefault.
			Foreground(color.NewRGBColor(int32(c.Top[0]), int32(c.Top[1]), int32(c.Top[2]))).
			Background(color.NewRGBColor(int32(c.Bottom[0]), int32(c.Bottom[1]), int32(c.Bottom[2])))
	}
	switch c.Style {
	case styLabel:
		return stPanelLabel
	case styValue:
		return stLED
	case styFrame:
		return stFrame
	case styBayLabel:
		return stBay
	case styCursor:
		return stCursor
	case styDim:
		return stDim
	}
	return stNormal
}

// The panel's colors. Named after what they are ON a panel rather than after
// their hue, so a theme is one edit here.
var (
	stFrame      = tcell.StyleDefault.Foreground(color.NewRGBColor(70, 76, 90))
	stBay        = tcell.StyleDefault.Foreground(color.NewRGBColor(150, 170, 210)).Bold(true)
	stPanelLabel = tcell.StyleDefault.Foreground(color.NewRGBColor(165, 175, 195))
	stLED        = tcell.StyleDefault.Foreground(color.NewRGBColor(255, 70, 60))
)

// ctlRect is where a control sits on the surface, so the view can be moved to
// bring it in. It returns false for a control that is not on the surface at
// all, which is what an orphan control is.
func ctlRect(s racksurface.Surface, mods []moduleCtls, a ctlAt) (x, y, w, h int, ok bool) {
	if a.Module < 0 || a.Module >= len(mods) {
		return 0, 0, 0, 0, false
	}
	var pan racksurface.Panel
	found := false
	for _, pp := range s.Panels {
		if pp.Item == a.Module {
			pan, found = pp, true
			break
		}
	}
	if !found {
		return 0, 0, 0, 0, false
	}
	across := maxi((pan.W-panelPad*2-2)/(knobCols+1), 1)
	block := ctlBlockRows()
	col, row := a.Index%across, a.Index/across
	x = pan.X + 1 + panelPad + col*(knobCols+1)
	y = pan.Y + 1 + panelPad + row*block
	return x, y, knobCols, block, true
}
