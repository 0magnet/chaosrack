package toilet

import (
	"github.com/0magnet/img2txt-go/caca"
	"github.com/0magnet/toilet-go/canvas"
)

// filter is one entry of toilet's post-processing table.
type filter struct {
	name string
	fn   func(cv *canvas.Canvas, lines int)
	// desc is the line `-F list` prints. The empty string hides an entry, as
	// it hides "rotate", which is only kept for backwards compatibility.
	desc string
}

// filters is the table, in the order toilet declares it. AddFilter scans it
// backwards, so an entry that is a prefix of a later one loses to it.
var filters = []filter{
	{"crop", filterCrop, "crop unused blanks"},
	{"rainbow", filterRainbow, "add a rainbow colour effect"},
	{"metal", filterMetal, "add a metallic colour effect"},
	{"flip", func(cv *canvas.Canvas, _ int) { cv.Flip() }, "flip horizontally"},
	{"flop", func(cv *canvas.Canvas, _ int) { cv.Flop() }, "flip vertically"},
	{"rotate", func(cv *canvas.Canvas, _ int) { cv.Rotate180() }, ""},
	{"180", func(cv *canvas.Canvas, _ int) { cv.Rotate180() }, "rotate 180 degrees"},
	{"left", func(cv *canvas.Canvas, _ int) { cv.RotateLeft() }, "rotate 90 degrees counterclockwise"},
	{"right", func(cv *canvas.Canvas, _ int) { cv.RotateRight() }, "rotate 90 degrees clockwise"},
	{"border", filterBorder, "surround text with a border"},
}

// FilterList returns the filters `-F list` advertises, as name and description
// pairs. The hidden "rotate" alias is left out.
func FilterList() [][2]string {
	var out [][2]string
	for _, f := range filters {
		if f.desc != "" {
			out = append(out, [2]string{f.name, f.desc})
		}
	}
	return out
}

// filterCrop trims the canvas down to its inked cells.
func filterCrop(cv *canvas.Canvas, _ int) {
	w, h := cv.Width, cv.Height
	xmin, xmax := w, 0
	ymin, ymax := h, 0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if cv.GetChar(x, y) == ' ' {
				continue
			}
			if x < xmin {
				xmin = x
			}
			if x > xmax {
				xmax = x
			}
			if y < ymin {
				ymin = y
			}
			if y > ymax {
				ymax = y
			}
		}
	}

	// An all-blank canvas leaves the bounds crossed and is left alone. A
	// zero-sized one does not cross them, so it grows to a single cell; that
	// is what the original does with an empty line.
	if xmax < xmin || ymax < ymin {
		return
	}

	cv.SetBoundaries(xmin, ymin, xmax-xmin+1, ymax-ymin+1)
}

// rainbowPalette is the six-color cycle of the rainbow filter.
var rainbowPalette = [6]uint8{
	caca.LightMagenta, caca.LightRed, caca.Yellow,
	caca.LightGreen, caca.LightCyan, caca.LightBlue,
}

// filterRainbow colors the inked cells along a diagonal six-color cycle.
func filterRainbow(cv *canvas.Canvas, lines int) {
	for y := 0; y < cv.Height; y++ {
		for x := 0; x < cv.Width; x++ {
			ch := cv.GetChar(x, y)
			if ch == ' ' {
				continue
			}
			cv.SetColorANSI(rainbowPalette[(x/2+y+lines)%6], caca.Transparent)
			cv.PutChar(x, y, ch)
		}
	}
}

// metalPalette is the four-color cycle of the metal filter.
var metalPalette = [4]uint8{
	caca.LightBlue, caca.Blue, caca.LightGray, caca.DarkGray,
}

// filterMetal colors the inked cells in wide diagonal bands.
func filterMetal(cv *canvas.Canvas, lines int) {
	for y := 0; y < cv.Height; y++ {
		for x := 0; x < cv.Width; x++ {
			ch := cv.GetChar(x, y)
			if ch == ' ' {
				continue
			}
			cv.SetColorANSI(metalPalette[((lines+y+x/8)/2)%4], caca.Transparent)
			cv.PutChar(x, y, ch)
		}
	}
}

// filterBorder grows the canvas by one cell all round and draws a box in it.
func filterBorder(cv *canvas.Canvas, _ int) {
	w, h := cv.Width, cv.Height
	cv.SetBoundaries(-1, -1, w+2, h+2)
	cv.DrawCP437Box(0, 0, w+2, h+2)
}
