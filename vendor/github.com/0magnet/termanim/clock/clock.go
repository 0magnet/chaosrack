// Package clock is an analog clock drawn in a terminal.
//
// A port of aclock, by Antoni Sawicki — "absolutely useless, except for turning
// your old, expensive mainframe or supercomputer into a wall clock". It has
// been compiled for something like 250 platforms, from AS/400 and Plan 9 to
// MSX-DOS and a Tektronix vector terminal, and this is one more.
//
// Unlike the rest of this repository, which is written from descriptions
// because the obvious ancestors are copyleft, aclock is Apache-2.0 and can
// simply be followed. So this follows it: the same dial of sixty marks, the
// same three hands lettered h, m and a dot, the same title and digital readout
// above and below the center. What is new is color, which the original has
// none of, and reading the clock every frame rather than sleeping a second
// between redraws.
//
//	https://github.com/tenox7/aclock
package clock

import (
	"fmt"
	"math"
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// cellAspect is how much wider a terminal cell is drawn than it is tall — or
// rather, how many columns make the width of one row.
//
// Without it the dial is an ellipse half as tall as it is wide, which is the
// single thing most worth getting right about drawing a circle in a grid. Two
// is what aclock uses and what nearly every terminal font is close to.
const cellAspect = 2

// Clock is the animation. The zero value is not usable; call New.
type Clock struct {
	cols, rows int

	// Now supplies the time. It is a field so a test can hold the clock still;
	// everything else about this animation is a pure function of it.
	Now func() time.Time

	// Sweep makes the second hand move continuously instead of stepping once a
	// second. Off by default, because a stepping second hand is what an analog
	// clock does and what the original did.
	Sweep bool

	// Colors. The original is monochrome — a terminal in 1994 largely was.
	Dial, Mark, Hour, Minute, Second, Text tcell.Color
}

// New returns a clock.
func New() *Clock {
	return &Clock{
		Now:    time.Now,
		Dial:   tcell.NewRGBColor(70, 80, 95),
		Mark:   tcell.NewRGBColor(150, 165, 185),
		Hour:   tcell.NewRGBColor(255, 245, 220),
		Minute: tcell.NewRGBColor(180, 210, 255),
		Second: tcell.NewRGBColor(255, 110, 90),
		Text:   tcell.NewRGBColor(120, 135, 155),
	}
}

// Resize records the terminal size. There is nothing to allocate: a clock is
// drawn from the time, not accumulated from previous frames.
func (c *Clock) Resize(cols, rows int) { c.cols, c.rows = cols, rows }

// radius is the dial's, in rows.
//
// The width is divided by the cell aspect first so that a wide, short terminal
// gives a dial that fits its height rather than one clipped left and right.
func (c *Clock) radius() int {
	smallest := c.rows
	if w := c.cols / cellAspect; w < smallest {
		smallest = w
	}
	return smallest/2 - 1
}

// angle converts a position on the dial — 0 to 60, whichever hand — into
// radians, with 0 at the top and the sixty running clockwise.
//
// The quarter turn is why the fifteen is subtracted: a circle's zero is to the
// right and a clock's is at the top.
func angle(pos float64) float64 { return (pos - 15) * math.Pi / 180 * 6 }

// Frame draws the clock. dt is unused: the time is read rather than advanced,
// so a dropped frame costs a redraw and never accumulates error.
func (c *Clock) Frame(screen tcell.Screen, cols, rows int, _ float64) {
	c.cols, c.rows = cols, rows
	r := c.radius()
	if r < 3 {
		return // no room for a dial worth drawing
	}
	cx, cy := cols/2, rows/2

	now := c.Now()
	if now.IsZero() {
		now = time.Now()
	}

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			screen.Put(x, y, canvas.Blank, tcell.StyleDefault) //nolint:errcheck // one cell cannot fail
		}
	}

	c.drawDial(screen, cx, cy, r)

	sec := float64(now.Second())
	if c.Sweep {
		sec += float64(now.Nanosecond()) / 1e9
	}
	min := float64(now.Minute())
	// The hour hand creeps between the hours rather than jumping at the top of
	// each one. Five dial positions per hour, so a minute is a twelfth of a
	// position — aclock divides by ten instead, which steps the hand in
	// ten-minute jumps and has it a whole position past the hour by the time
	// the hour is up.
	hr := float64(now.Hour()%12)*5 + float64(now.Minute())/12

	c.drawHand(screen, cx, cy, hr, 2*r/3, 'h', c.Hour)
	c.drawHand(screen, cx, cy, min, r-2, 'm', c.Minute)
	c.drawHand(screen, cx, cy, sec, r-1, '.', c.Second)

	c.put(screen, cx-5, cy-3*r/5, ".:ACLOCK:.", c.Text)
	c.put(screen, cx-5, cy+3*r/5,
		fmt.Sprintf("[%02d:%02d:%02d]", now.Hour(), now.Minute(), now.Second()), c.Text)
}

// drawDial marks the sixty minutes, with the twelve hours picked out.
func (c *Clock) drawDial(screen tcell.Screen, cx, cy, r int) {
	for i := 0; i < 60; i++ {
		a := float64(i) * math.Pi / 180 * 6
		x := cx + int(math.Round(math.Cos(a)*float64(r)*cellAspect))
		y := cy + int(math.Round(math.Sin(a)*float64(r)))
		ch, col := '.', c.Dial
		if i%5 == 0 {
			ch, col = 'o', c.Mark
		}
		c.set(screen, x, y, ch, col)
	}
}

// drawHand draws one hand from the center outward.
//
// It starts at one rather than zero so the three hands do not pile onto the
// center cell, where the last one drawn would be the only one visible and the
// clock would appear to have a single hand at its middle.
func (c *Clock) drawHand(screen tcell.Screen, cx, cy int, pos float64, length int, ch rune, col tcell.Color) {
	a := angle(pos)
	for n := 1; n < length; n++ {
		x := cx + int(math.Round(math.Cos(a)*float64(n)*cellAspect))
		y := cy + int(math.Round(math.Sin(a)*float64(n)))
		c.set(screen, x, y, ch, col)
	}
}

func (c *Clock) set(screen tcell.Screen, x, y int, ch rune, col tcell.Color) {
	if x < 0 || y < 0 || x >= c.cols || y >= c.rows {
		return
	}
	canvas.PutRune(screen, x, y, ch, tcell.StyleDefault.Foreground(col))
}

func (c *Clock) put(screen tcell.Screen, x, y int, s string, col tcell.Color) {
	for i, ch := range s {
		c.set(screen, x+i, y, ch, col)
	}
}

// Run draws a clock on the screen until the user quits.
func Run(screen tcell.Screen) error {
	// Ten frames a second. The clock only changes once a second, but the second
	// hand should land on the second rather than up to a second after it, and a
	// resize should be picked up promptly.
	return canvas.RunCells(screen, New(), canvas.Options{FPS: 10})
}
