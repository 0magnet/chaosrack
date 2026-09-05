// Package aquarium is fish swimming across the terminal.
//
// Unlike the field effects in this repository there is no formula here: it is
// sprites drawn at moving positions, with bubbles rising and seaweed swaying.
// That makes it a glyph animation rather than a pixel one — the fish are made
// of characters and drawing them as half-block pixels would lose them.
//
// Written from the idea rather than from an implementation. asciiquarium is
// the obvious ancestor and is GPL; none of its code, and none of its artwork,
// is used here. These fish are drawn from scratch and are deliberately simpler.
package aquarium

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// sprite is a small picture with a transparent background: a space in the art
// leaves whatever is behind it showing, so fish pass in front of seaweed
// without punching a rectangle through it.
type sprite struct {
	rows []string
	w, h int
}

func newSprite(rows ...string) sprite {
	s := sprite{rows: rows, h: len(rows)}
	for _, r := range rows {
		if n := len([]rune(r)); n > s.w {
			s.w = n
		}
	}
	return s
}

// Fish face right; they are mirrored when swimming left.
//
// The bodies are drawn as closed outlines — parentheses, pipes and box drawing
// in ASCII — rather than the dotted, stippled shapes asciiquarium uses. That
// is a deliberate difference in visual language as much as a legal one: none
// of its artwork is reproduced here.
//
// The larger ones trail a forked caudal fin, swept back from where it meets the
// body. They used to end in a flat run of angle brackets, which reads as an
// arrow rather than a tail — and on one of them the bracket pointed the same way
// as the head, so the fish appeared to be going both ways at once.
//
// They range from one row to nine so there is something to swim in a short
// window as well as a tall one; newFish picks only from those that fit.
var fishRight = []sprite{
	// A minnow, for cramped tanks and for variety among the larger fish.
	newSprite(
		`><>`),
	// A small fish with a forked tail.
	newSprite(
		`  \`,
		`><_>`,
		`  /`),
	// Round-bodied, blunt-nosed.
	newSprite(
		`\       ____`,
		` \_    /    o\`,
		` / \__(       )`,
		`/      \_____/`),
	// A streamlined swimmer with a dorsal fin and a notched tail.
	newSprite(
		`         /\_`,
		`       _/   \_`,
		`\     /     o \`,
		` \___(   ><    )`,
		` /   \       /`,
		`/    \_   __/`,
		`       \_/`),
	// A deep-bodied fish, gills marked, trailing a wide tail.
	newSprite(
		`\         .-.`,
		` \       /   \`,
		`  \___  |   o |`,
		`  /    (  ~   )`,
		` /      | ___ |`,
		`/        \   /`,
		`          '-'`),
	// An angelfish: tall, with long dorsal and ventral fins.
	newSprite(
		`     |\`,
		`     | \`,
		`\    |  \`,
		` \___| o )`,
		` /___    )`,
		`/    |__ )`,
		`     |  /`,
		`     | /`,
		`     |/`),
}

// mirror flips a sprite horizontally, swapping the characters that have a
// handedness. Without the swap a mirrored fish has its fins pointing the wrong
// way and reads as broken rather than reversed.
func mirror(s sprite) sprite {
	flip := map[rune]rune{
		'<': '>', '>': '<',
		'/': '\\', '\\': '/',
		'(': ')', ')': '(',
		'{': '}', '}': '{',
		'[': ']', ']': '[',
	}
	out := make([]string, len(s.rows))
	for i, row := range s.rows {
		r := []rune(row)
		// Pad so mirroring a ragged sprite does not shift its rows apart.
		for len(r) < s.w {
			r = append(r, ' ')
		}
		m := make([]rune, len(r))
		for j, c := range r {
			if f, ok := flip[c]; ok {
				c = f
			}
			m[len(r)-1-j] = c
		}
		out[i] = string(m)
	}
	return newSprite(out...)
}

type fish struct {
	s     sprite
	x, y  float64
	speed float64 // columns per simulation step; negative swims left
	color tcell.Color
}

type bubble struct {
	x, y  float64
	speed float64
}

// Aquarium is the animation. The zero value is not usable; call New.
type Aquarium struct {
	cols, rows int
	fish       []fish
	bubbles    []bubble
	weed       []int // seaweed heights, indexed by column; 0 means none
	phase      int
	rng        *rand.Rand

	// Fish is how many to swim at once.
	Fish int
	// StepRate is simulation steps per second. The tank was tuned at one step
	// per frame at 20 fps, so 20 keeps its old pace while the screen redraws
	// more often.
	StepRate float64

	// acc carries the fraction of a step left over from the last frame.
	acc float64
}

// New returns an aquarium. seed of 0 gives a fixed arrangement.
func New(seed int64) *Aquarium {
	return &Aquarium{rng: rand.New(rand.NewSource(seed)), Fish: 8, StepRate: 20}
}

var fishColors = []tcell.Color{
	tcell.NewRGBColor(255, 160, 0),
	tcell.NewRGBColor(255, 80, 80),
	tcell.NewRGBColor(120, 220, 255),
	tcell.NewRGBColor(255, 220, 90),
	tcell.NewRGBColor(180, 140, 255),
}

// Resize stocks the tank.
func (a *Aquarium) Resize(cols, rows int) {
	a.cols, a.rows = cols, rows
	if a.Fish < 1 {
		a.Fish = 1
	}
	// Stock to the tank. pickRow tries to give each fish rows of its own and
	// falls back to any row when it cannot, so asking for eight fish in a tank
	// with room for three does not fail — it draws them through each other,
	// and two sprites in the same rows are unreadable. Roughly one fish per
	// four rows leaves the avoidance something to work with.
	n := a.Fish
	if room := rows / 4; room < n {
		n = room
	}
	if n < 1 {
		n = 1
	}
	a.fish = make([]fish, 0, n)
	for i := 0; i < n; i++ {
		a.fish = append(a.fish, a.newFish(true))
	}
	a.bubbles = nil
	// Seaweed along the floor, in clumps rather than evenly spaced.
	a.weed = make([]int, cols)
	for x := 0; x < cols; x++ {
		if a.rng.Intn(9) == 0 {
			h := 2 + a.rng.Intn(max(2, rows/3))
			for i := 0; i < 1+a.rng.Intn(3) && x+i < cols; i++ {
				a.weed[x+i] = h - a.rng.Intn(2)
			}
			x += 3
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// pickRow chooses a row for a fish of height h, preferring one whose rows no
// existing fish already occupies.
//
// Two sprites drawn over each other do not blend into a shoal, they blend into
// unreadable punctuation — the tail of one landing inside the body of another.
// A handful of attempts is enough to avoid that almost always, and falling
// back to any row keeps a crowded tank from looping forever.
func (a *Aquarium) pickRow(top, bottom, h int) int {
	for attempt := 0; attempt < 12; attempt++ {
		y := top
		if bottom > top {
			y = top + a.rng.Intn(bottom-top+1)
		}
		if !a.rowsBusy(y, h) {
			return y
		}
	}
	if bottom > top {
		return top + a.rng.Intn(bottom-top+1)
	}
	return top
}

// rowsBusy reports whether any live fish overlaps rows [y, y+h).
func (a *Aquarium) rowsBusy(y, h int) bool {
	for _, f := range a.fish {
		fy := int(f.y)
		if y < fy+f.s.h && fy < y+h {
			return true
		}
	}
	return false
}

// fits returns the fish that will swim in this tank without being clipped,
// leaving a row spare at the bottom for the seaweed. The smallest fish is
// always returned rather than an empty list, so a very short window still has
// something in it.
func (a *Aquarium) fits() []sprite {
	var out []sprite
	for _, s := range fishRight {
		if s.h <= a.rows-1 {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = append(out, fishRight[0])
	}
	return out
}

// newFish makes a fish. When anywhere is true it may start mid-tank, which is
// what stops the aquarium from beginning empty; otherwise it enters from the
// edge it will swim from.
func (a *Aquarium) newFish(anywhere bool) fish {
	// Only consider fish the tank can hold. Without this a nine-row angelfish
	// in a six-row window is clipped to a meaningless fragment, and the row it
	// is given has nowhere to sit.
	fits := a.fits()
	s := fits[a.rng.Intn(len(fits))]
	speed := 0.15 + a.rng.Float64()*0.45
	left := a.rng.Intn(2) == 0
	if left {
		s = mirror(s)
		speed = -speed
	}
	// Keep fish off the very bottom rows, where the seaweed is.
	top := 0
	bottom := a.rows - s.h - 1
	if bottom < top {
		bottom = top
	}
	f := fish{
		s:     s,
		y:     float64(a.pickRow(top, bottom, s.h)),
		speed: speed,
		color: fishColors[a.rng.Intn(len(fishColors))],
	}
	switch {
	case anywhere:
		f.x = a.rng.Float64() * float64(a.cols)
	case left:
		f.x = float64(a.cols)
	default:
		f.x = float64(-s.w)
	}
	return f
}

// Frame advances everything and repaints.
//
// Like matrix, the tank is stepped at a fixed rate taken from elapsed time
// rather than once per frame. Swim speeds are per step and so is the chance a
// fish blows a bubble, so an accumulator keeps all of it meaning what it did
// while the screen redraws as often as it likes. Fish land on whole columns,
// so stepping faster would buy nothing.
func (a *Aquarium) Frame(screen tcell.Screen, cols, rows int, dt float64) {
	if a.cols == 0 || a.rows == 0 {
		return
	}
	rate := a.StepRate
	if rate <= 0 {
		rate = 20
	}
	a.acc += dt * rate
	if a.acc > 4 {
		a.acc = 4 // bound the catch-up after a stall
	}
	for a.acc >= 1 {
		a.step()
		a.acc--
	}
	a.draw(screen)
}

// step advances the tank by one simulation tick.
func (a *Aquarium) step() {
	a.phase++

	for i := range a.fish {
		f := &a.fish[i]
		f.x += f.speed
		// Respawn once fully off the far edge.
		if f.speed > 0 && f.x > float64(a.cols) {
			a.fish[i] = a.newFish(false)
		} else if f.speed < 0 && f.x < float64(-f.s.w) {
			a.fish[i] = a.newFish(false)
		}
		// Occasionally a fish blows a bubble, from its leading edge.
		if a.rng.Intn(140) == 0 {
			bx := f.x
			if f.speed > 0 {
				bx += float64(f.s.w)
			}
			a.bubbles = append(a.bubbles, bubble{x: bx, y: f.y, speed: 0.2 + a.rng.Float64()*0.3})
		}
	}

	kept := a.bubbles[:0]
	for _, b := range a.bubbles {
		b.y -= b.speed
		if b.y > 0 {
			kept = append(kept, b)
		}
	}
	a.bubbles = kept
}

func (a *Aquarium) draw(screen tcell.Screen) {
	for y := 0; y < a.rows; y++ {
		for x := 0; x < a.cols; x++ {
			screen.Put(x, y, canvas.Blank, tcell.StyleDefault) //nolint:errcheck // one cell cannot fail
		}
	}

	// Seaweed first, so fish swim in front of it.
	green := tcell.StyleDefault.Foreground(tcell.NewRGBColor(40, 160, 60))
	for x, h := range a.weed {
		for i := 0; i < h; i++ {
			y := a.rows - 1 - i
			if y < 0 {
				break
			}
			// Sway: each segment leans one way or the other depending on its
			// height and the frame, so the weed ripples instead of flapping
			// as one piece.
			r := '('
			if (a.phase/8+i+x)%2 == 0 {
				r = ')'
			}
			canvas.PutRune(screen, x, y, r, green)
		}
	}

	blue := tcell.StyleDefault.Foreground(tcell.NewRGBColor(140, 200, 255))
	for _, b := range a.bubbles {
		canvas.PutRune(screen, int(b.x), int(b.y), 'o', blue)
	}

	for _, f := range a.fish {
		st := tcell.StyleDefault.Foreground(f.color)
		for dy, row := range f.s.rows {
			y := int(f.y) + dy
			if y < 0 || y >= a.rows {
				continue
			}
			rs := []rune(row)
			// Spaces outside the drawn part of a row are transparent, so fish
			// do not punch rectangles through the scene. Spaces *inside* it
			// are opaque: a fish's body has to hide what is behind it, or
			// seaweed shows through its belly.
			first, last := -1, -1
			for i, r := range rs {
				if r != ' ' {
					if first < 0 {
						first = i
					}
					last = i
				}
			}
			if first < 0 {
				continue // blank row
			}
			for dx := first; dx <= last; dx++ {
				x := int(f.x) + dx
				if x < 0 || x >= a.cols {
					continue
				}
				canvas.PutRune(screen, x, y, rs[dx], st)
			}
		}
	}
}

// Run swims fish across the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.RunCells(screen, New(seed), canvas.Options{})
}
