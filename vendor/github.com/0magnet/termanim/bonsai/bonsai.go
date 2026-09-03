// Package bonsai grows a small tree, one branch at a time, in the terminal.
//
// Written from the technique rather than from any implementation of it. cbonsai
// is the obvious ancestor and is copyleft; none of its code, tables or artwork
// were consulted, and nothing here derives from them.
//
// The technique is the classic recursive one. A branch is a position, a
// direction and a life. Each step it moves one cell, nudges its direction, and
// may spawn a child branch with a shorter life; when its life runs out it puts
// out a cluster of leaves. What separates a tree from a bush is entirely in the
// probabilities: the trunk climbs steadily and keeps its lean for several rows,
// shoots lean out and each generation inherits three quarters of its parent's
// life, so the structure gets finer with depth and the whole thing tapers
// instead of exploding.
//
// Growth is spread across frames so the tree is watched being drawn. The live
// branches are kept as a stack and the top of it is grown first, which is the
// order a recursive version would draw in: a shoot is followed to its tip
// before the branch it came off resumes.
package bonsai

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// kind distinguishes the parts of the tree, which are drawn in different
// characters and different colours.
type kind int

const (
	trunk kind = iota
	shoot
)

// TrunkGlyphs and ShootGlyphs are the strokes a branch is drawn with, chosen by
// the direction of the step that produced them: a step up-and-left lies along a
// backslash, up-and-right along a slash, and so on.
//
// The trunk is drawn in heavy box drawing and the shoots in light, which is
// what makes a two-character-wide trunk read as thicker than the twigs coming
// off it without needing to actually draw it two cells wide.
var (
	TrunkGlyphs = Glyphs{Vertical: '┃', Horizontal: '━', Left: '╲', Right: '╱'}
	ShootGlyphs = Glyphs{Vertical: '│', Horizontal: '─', Left: '\\', Right: '/'}
)

// Glyphs is one set of branch strokes. Left is the stroke for a step that moves
// up and to the left; Right for up and to the right.
type Glyphs struct {
	Vertical, Horizontal, Left, Right rune
}

// pick chooses the stroke for a step of dx, dy.
func (g Glyphs) pick(dx, dy int) rune {
	switch {
	case dx == 0:
		return g.Vertical
	case dy == 0:
		return g.Horizontal
	case dx < 0:
		return g.Left
	default:
		return g.Right
	}
}

// LeafGlyphs are the characters a leaf cluster is made of. Several of them, so
// a cluster has texture rather than being a solid rectangle of one character.
var LeafGlyphs = []rune("&*%o")

// Bark are the browns the woody parts are drawn in, and Leaves the greens. Two
// browns and four greens: a single flat green reads as a paper cut-out, and the
// variation is most of what makes the canopy look like foliage.
var (
	Bark = []tcell.Color{
		tcell.NewRGBColor(140, 96, 54),
		tcell.NewRGBColor(104, 70, 38),
	}
	Leaves = []tcell.Color{
		tcell.NewRGBColor(60, 170, 60),
		tcell.NewRGBColor(90, 210, 80),
		tcell.NewRGBColor(40, 130, 50),
		tcell.NewRGBColor(140, 230, 110),
	}
)

// branch is one growing tip.
type branch struct {
	x, y   int
	dx, dy int
	life   int // steps remaining before it puts out leaves
	born   int // life it started with, so its age can be judged
	base   int // the row it was spawned on, which bounds how far it may droop
	kind   kind
}

// glyphCell is one painted position.
type glyphCell struct {
	r      rune
	colour tcell.Color
}

// Bonsai is the animation. The zero value is not usable; call New.
type Bonsai struct {
	cols, rows int
	buf        []glyphCell
	// live is a stack of growing tips. Growing the top one first reproduces the
	// drawing order of the recursive version.
	live     []branch
	occupied int
	// hold is how long the finished tree has been standing, in seconds.
	hold float64
	// acc is unspent time. Growth happens in whole steps at a fixed rate, and
	// the remainder is carried rather than dropped, so the tree grows at the
	// same speed whatever the frame rate is.
	acc float64
	// trees counts the trees grown, so a caller — or a test — can tell one from
	// the next.
	trees int
	rng   *rand.Rand

	// StepsPerSecond paces the growth. One step is one branch moving one cell;
	// sixty a second draws a tree in a few seconds without it appearing all at
	// once, and is deliberately independent of the frame rate.
	StepsPerSecond float64
	// TrunkLife is how many steps the trunk climbs for. Zero scales with the
	// window: a little over half its height, which leaves the top of the window
	// for the canopy the shoots carry above the crown.
	TrunkLife int
	// MinBranchLife is the shortest life worth spawning. Below about three
	// steps a shoot is a single stroke and a leaf cluster, which reads as
	// noise stuck to the branch rather than as a twig.
	MinBranchLife int
	// TrunkBranchChance is the chance per step that the trunk throws a shoot,
	// as a reciprocal: 4 means about one shoot every four rows climbed. Much
	// lower and the trunk disappears inside its own foliage; much higher and
	// there is nothing above the stem but a tuft.
	TrunkBranchChance int
	// ShootBranchChance is the same for shoots. Deliberately close to the
	// trunk's: a shoot that splits far more often than the trunk fills its own
	// neighbourhood with twigs and the tree turns into a hedge.
	ShootBranchChance int
	// LeafDensity is how many leaf characters a dead tip puts out.
	LeafDensity int
	// HoldSeconds is how long the finished tree is left standing before the
	// next one starts. In seconds, because that is what it actually is — as a
	// count of frames it silently halved whenever the frame rate doubled.
	HoldSeconds float64
}

// New returns a bonsai. seed of 0 grows a fixed tree, which makes tests
// repeatable.
func New(seed int64) *Bonsai {
	return &Bonsai{
		rng:               rand.New(rand.NewSource(seed)),
		StepsPerSecond:    60,
		MinBranchLife:     3,
		TrunkBranchChance: 4,
		ShootBranchChance: 6,
		LeafDensity:       5,
		HoldSeconds:       4,
	}
}

// Resize allocates the canvas and plants the first tree.
func (b *Bonsai) Resize(cols, rows int) {
	b.cols, b.rows = cols, rows
	b.buf = make([]glyphCell, cols*rows)
	b.plant()
}

// trunkLife is the configured trunk length, or one scaled to the window.
func (b *Bonsai) trunkLife() int {
	if b.TrunkLife > 0 {
		return b.TrunkLife
	}
	n := b.rows * 5 / 9
	if n < 3 {
		n = 3
	}
	return n
}

// plant clears the canvas and starts a new trunk from the bottom centre.
func (b *Bonsai) plant() {
	for i := range b.buf {
		b.buf[i] = glyphCell{}
	}
	b.occupied = 0
	b.hold = 0
	b.trees++
	if b.cols == 0 || b.rows == 0 {
		b.live = nil
		return
	}
	life := b.trunkLife()
	// Centre the trunk, but not exactly: a tree grown on the middle column of
	// the screen every single time looks placed rather than grown.
	x := b.cols/2 + b.rng.Intn(5) - 2
	b.live = append(b.live[:0], branch{
		x: x, y: b.rows - 1,
		dx: 0, dy: -1,
		life: life, born: life,
		base: b.rows - 1,
		kind: trunk,
	})
}

// put paints one cell, keeping the occupied count honest.
func (b *Bonsai) put(x, y int, r rune, c tcell.Color) {
	if x < 0 || y < 0 || x >= b.cols || y >= b.rows {
		return
	}
	i := y*b.cols + x
	if b.buf[i].r == 0 {
		b.occupied++
	}
	b.buf[i] = glyphCell{r: r, colour: c}
}

// leafCluster puts a blob of leaves around a dead tip. Wider than it is tall
// because the canopy of a bonsai spreads sideways, and scattered rather than
// filled so its edge is ragged.
func (b *Bonsai) leafCluster(x, y int) {
	for i := 0; i < b.LeafDensity; i++ {
		lx := x + b.rng.Intn(5) - 2
		ly := y + b.rng.Intn(3) - 1
		r := LeafGlyphs[b.rng.Intn(len(LeafGlyphs))]
		b.put(lx, ly, r, Leaves[b.rng.Intn(len(Leaves))])
	}
	// Always mark the tip itself, so a cluster whose random offsets all landed
	// off screen still leaves something behind.
	b.put(x, y, LeafGlyphs[b.rng.Intn(len(LeafGlyphs))], Leaves[b.rng.Intn(len(Leaves))])
}

// spawn pushes a child of br if one is due.
//
// The child gets three quarters of the life its parent set out with, and that
// one line is what bounds the whole recursion: lives fall geometrically with
// depth, and nothing shorter than MinBranchLife is ever spawned, so the tree is
// finite however the dice fall — at most log(TrunkLife/MinBranchLife) levels
// deep, about six for a normal window.
//
// It is the parent's original life and not its remaining life on purpose. Basing
// it on what is left tapers twice — once by depth and once by how far along the
// parent the shoot appears — and the second taper is savage: the far half of
// every branch ends up bare, and the tree comes out as a trunk with a few
// whiskers instead of a canopy.
func (b *Bonsai) spawn(br *branch) {
	chance := b.ShootBranchChance
	if br.kind == trunk {
		chance = b.TrunkBranchChance
		// Keep the lowest third of the trunk bare. A tree with branches coming
		// out of the ground is a shrub, and a clean length of stem below the
		// crown is most of what makes a bonsai look deliberate.
		if br.born-br.life < br.born/3 {
			return
		}
	}
	life := br.born * 3 / 4
	if life >= br.born {
		life = br.born - 1 // never let a child out-reach its parent
	}
	if life < b.MinBranchLife || b.rng.Intn(chance) != 0 {
		return
	}
	// A shoot leaves sideways: mostly out and up, sometimes flat. Which side is
	// a coin toss, so the tree does not lean the same way every time.
	dx := 1
	if b.rng.Intn(2) == 0 {
		dx = -1
	}
	dy := -1
	if b.rng.Intn(3) == 0 {
		dy = 0
	}
	b.live = append(b.live, branch{
		x: br.x, y: br.y,
		dx: dx, dy: dy,
		life: life, born: life,
		base: br.y,
		kind: shoot,
	})
}

// step grows the tip on top of the stack by one cell.
func (b *Bonsai) step() {
	if len(b.live) == 0 {
		return
	}
	br := &b.live[len(b.live)-1]

	// Out of life, or out of the window: put out leaves and retire.
	if br.life <= 0 || br.x < 0 || br.x >= b.cols || br.y < 0 || br.y >= b.rows {
		b.leafCluster(br.x, br.y)
		b.live = b.live[:len(b.live)-1]
		return
	}

	if br.kind == trunk {
		// The trunk climbs one row per step and re-rolls its lean every third,
		// with straight up three times as likely as either side, and never
		// straight from one lean into the opposite one. Both rules are there to
		// stop the same thing: a trunk that leans every step, or that swaps
		// sides on consecutive rolls, comes out as a zigzag — a lightning bolt
		// rather than a stem.
		br.dy = -1
		if (br.born-br.life)%3 == 0 {
			lean := 0
			switch b.rng.Intn(5) {
			case 0:
				lean = -1
			case 1:
				lean = 1
			}
			if lean == -br.dx {
				lean = 0
			}
			// The bare lower third stands straight up as well: a tree wants a
			// rooted base to grow out of, and a stem that sets off diagonally
			// from the ground looks knocked over.
			if br.born-br.life < br.born/3 {
				lean = 0
			}
			br.dx = lean
		}
	} else {
		// A shoot spends the first half of its life climbing away from the
		// trunk and the second half flattening out and drooping. That arc is
		// the bonsai silhouette; without it the shoots are straight rays.
		if br.life*2 > br.born {
			br.dy = -1
			if b.rng.Intn(3) == 0 {
				br.dy = 0
			}
		} else {
			br.dy = 0
			if b.rng.Intn(4) == 0 {
				br.dy = 1
			}
			// A branch may sag, but only so far. Left to droop step after step
			// it walks back down past the trunk and puts leaves on the ground,
			// which reads as a shrub growing round the base.
			if br.y >= br.base+2 {
				br.dy = 0
			}
		}
		// Keep going the way it set out, with the occasional jog or reversal so
		// the twigs are crooked.
		switch b.rng.Intn(8) {
		case 0:
			br.dx = 0
		case 1:
			br.dx = -br.dx
		}
		if br.dx == 0 && br.dy == 0 {
			br.dy = -1 // never stand still: a step that does not move is a wasted life
		}
	}

	g := ShootGlyphs
	if br.kind == trunk {
		g = TrunkGlyphs
	}
	// Everything woody is brown; the two browns alternate at random so the bark
	// has some grain to it.
	b.put(br.x, br.y, g.pick(br.dx, br.dy), Bark[b.rng.Intn(len(Bark))])

	// The lower half of the trunk gets a second stroke beside it. A tree is
	// thick at the base and thin at the top, and one cell is the narrowest a
	// terminal can draw; widening the bottom to two is the only taper
	// available, and without it the trunk is no heavier than the twigs.
	if br.kind == trunk && br.life*2 > br.born {
		b.put(br.x+1, br.y, g.pick(br.dx, br.dy), Bark[b.rng.Intn(len(Bark))])
	}

	br.x += br.dx
	br.y += br.dy
	br.life--

	// Spawn after moving, so the child starts at the new tip rather than
	// doubling up on the cell just drawn.
	b.spawn(br)
}

// Done reports whether the tree has finished growing.
func (b *Bonsai) Done() bool { return len(b.live) == 0 }

// maxBacklog is the most unspent time the accumulator will hold, in seconds.
// A frame arriving after a long stall must not grow the whole tree at once; the
// backlog beyond this is dropped so the work in any one frame stays bounded.
const maxBacklog = 0.25

// due consumes whole steps' worth of time from the accumulator.
func (b *Bonsai) due(dt float64) int {
	rate := b.StepsPerSecond
	if rate <= 0 {
		rate = 60
	}
	b.acc += dt
	if b.acc > maxBacklog {
		b.acc = maxBacklog
	}
	n := int(b.acc * rate)
	b.acc -= float64(n) / rate
	return n
}

// Frame grows the tree by whatever the elapsed time is worth and repaints. dt
// is seconds since the last frame.
func (b *Bonsai) Frame(screen tcell.Screen, cols, rows int, dt float64) {
	if b.cols == 0 || b.rows == 0 {
		return
	}
	if b.Done() {
		b.hold += dt
		if b.hold > b.HoldSeconds {
			b.plant()
		}
	} else {
		for n := b.due(dt); n > 0 && !b.Done(); n-- {
			b.step()
		}
	}
	b.draw(screen)
}

func (b *Bonsai) draw(screen tcell.Screen) {
	for y := 0; y < b.rows; y++ {
		for x := 0; x < b.cols; x++ {
			c := b.buf[y*b.cols+x]
			if c.r == 0 {
				screen.Put(x, y, canvas.Blank, tcell.StyleDefault) //nolint:errcheck // one cell cannot fail
				continue
			}
			canvas.PutRune(screen, x, y, c.r, tcell.StyleDefault.Foreground(c.colour))
		}
	}
}

// Run grows one bonsai after another until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.RunCells(screen, New(seed), canvas.Options{})
}
