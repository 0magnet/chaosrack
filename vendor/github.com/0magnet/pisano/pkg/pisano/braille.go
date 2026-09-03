package pisano

import "strings"

// Braille is a canvas that packs a 2×4 grid of dots into every character cell,
// using the Unicode braille patterns at U+2800.
//
// The box-drawing canvas in canvas.go is exact but rigid: one lattice step is
// one cell, so a figure can only ever be drawn at its natural size. An open
// turtle path has no natural size — it keeps drifting — so watching one means
// zooming out, and zooming out means sub-character resolution. Eight dots per
// cell is what makes that possible, at the cost of the crisp joins the box
// canvas gives you.
//
// It also lifts the other restriction: dots are addressed individually, so a
// line at any angle can be drawn. That is what lets the circular designs, which
// are chords at arbitrary angles, appear in a terminal at all.
type Braille struct {
	w, h  int // in characters
	dots  []byte
	color []string
}

// brailleBits maps a dot's position within its cell to its bit. The braille
// block numbers its dots down the left column then down the right, with the
// fourth row added later and tacked onto the high bits — hence the jump from
// 0x04 to 0x40 rather than a tidy progression.
var brailleBits = [4][2]byte{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// NewBraille makes a canvas w characters wide and h characters tall, which is
// 2w by 4h dots.
func NewBraille(w, h int) *Braille {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Braille{w: w, h: h, dots: make([]byte, w*h), color: make([]string, w*h)}
}

// Size reports the dot resolution of the canvas.
func (b *Braille) Size() (dotsX, dotsY int) { return b.w * 2, b.h * 4 }

// Set lights the dot at a pixel coordinate. Anything off the canvas is dropped,
// which is what makes the canvas a viewport onto a larger drawing.
func (b *Braille) Set(px, py int, col string) {
	if px < 0 || py < 0 || px >= b.w*2 || py >= b.h*4 {
		return
	}
	i := (py/4)*b.w + px/2
	b.dots[i] |= brailleBits[py%4][px%2]
	if col != "" {
		b.color[i] = col
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Line draws between two pixel coordinates with Bresenham, so the arbitrary
// angles the circular designs need come out evenly stepped.
func (b *Braille) Line(x0, y0, x1, y1 int, col string) {
	dx, sx := abs(x1-x0), 1
	if x0 >= x1 {
		sx = -1
	}
	dy, sy := -abs(y1-y0), 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	for {
		b.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// String renders the canvas. Empty cells emit a space rather than U+2800, the
// blank braille pattern: the two look identical but the space keeps the output
// selectable and lets a terminal background show through unchanged.
func (b *Braille) String(colorize bool) string {
	var sb strings.Builder
	for y := 0; y < b.h; y++ {
		end := -1
		for x := b.w - 1; x >= 0; x-- {
			if b.dots[y*b.w+x] != 0 {
				end = x
				break
			}
		}
		cur := ""
		for x := 0; x <= end; x++ {
			i := y*b.w + x
			if colorize {
				if col := b.color[i]; col != cur {
					if cur != "" {
						sb.WriteString("\x1b[0m")
					}
					sb.WriteString(col)
					cur = col
				}
			}
			if b.dots[i] == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteRune(0x2800 + rune(b.dots[i]))
			}
		}
		if colorize && cur != "" {
			sb.WriteString("\x1b[0m")
		}
		if y < b.h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
