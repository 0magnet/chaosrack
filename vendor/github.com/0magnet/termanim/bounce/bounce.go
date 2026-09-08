// Package bounce is the bouncing logo screensaver, and the wait for it to hit
// a corner.
//
// # WHY THIS ONE AND NOT THE OTHER CLASSICS
//
// The terminal toys people remember — the party parrot, Nyan Cat, ASCII Star
// Wars — are mostly other people's art under other people's terms. ascii-live
// is GPL-3.0 against this repository's MIT; parrot.live and the nyancat server
// declare no license at all; the Star Wars asciimation and Nyan Cat are both
// somebody's copyrighted character.
//
// A logo bouncing around a screen and changing color when it hits a wall is
// none of those things. It is a behavior, not a drawing: two coordinates, two
// velocities, and a sign flip. Nothing about it is anyone's property, so it
// can be written here from scratch, and everyone recognizes it anyway. That
// is why this is the neighbor the parrot got rather than a cat.
//
// # THE CORNER
//
// The entire cultural weight of this animation rests on the logo hitting an
// exact corner, which almost never happens. That is the joke, and it is also
// a problem for something that has to be worth watching in a browser tab for
// thirty seconds.
//
// So a corner here is a near miss: if the logo bounces off two walls close
// enough together in time, it counts, and the screen flashes. This is a
// deliberate lie about the geometry and an accurate one about the experience,
// because at a terminal's resolution a hit two cells from the corner looks
// exactly like a hit on it. Corners counts them; watch it for a minute and it
// will go up, which honest arithmetic would not manage inside an hour.
package bounce

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Bounce is the animation. The zero value is not usable; call New.
type Bounce struct {
	w, h   float64
	x, y   float64 // logo center, in surface pixels
	dx, dy float64 // pixels per second
	hue    float64
	rng    *rand.Rand

	// lastBounceX and lastBounceY are seconds since each axis last flipped,
	// which is how a corner is recognized: both axes flipping within
	// cornerWindow of each other. Counting frames instead would make the
	// tolerance depend on the frame rate.
	sinceX, sinceY float64

	flash float64 // seconds of flash remaining

	// Corners is how many times the logo has hit one, in the generous sense
	// described in the package comment. It only ever goes up.
	Corners int

	// Speed is the logo's travel in pixels per second, before the aspect of
	// the window is taken into account. Faster is more frantic and hits
	// corners more often.
	Speed float64

	// Size is the logo's half-width as a fraction of the smaller window
	// dimension. Larger fills more of the screen and, because it turns
	// sooner, changes color more often.
	Size float64
}

// The window, in seconds, inside which two wall hits count as one corner.
//
// A tenth of a second is about three frames at thirty: tight enough that a
// hit along the middle of an edge never qualifies, loose enough to catch the
// near misses that look like corners.
const cornerWindow = 0.1

// flashTime is how long the screen stays lit after a corner. Long enough to
// notice, short enough not to hide the logo.
const flashTime = 0.45

// New returns a bouncing logo. The seed sets the starting position and
// heading; seed 0 gives a fixed opening, which is what the tests want.
func New(seed int64) *Bounce {
	return &Bounce{
		rng:   rand.New(rand.NewSource(seed)), //nolint:gosec
		Speed: 26,
		Size:  0.13,
	}
}

// Resize places the logo. It is called before the first frame and on every
// window change, and it keeps the logo's position as a fraction of the window
// so a resize does not teleport it into a wall.
func (b *Bounce) Resize(w, h int) {
	nw, nh := float64(w), float64(h)
	if b.w > 0 && b.h > 0 {
		b.x, b.y = b.x/b.w*nw, b.y/b.h*nh
	} else {
		b.x, b.y = nw*0.5, nh*0.5
		// A heading that is not a neat fraction of the window, so the path
		// does not close into a short loop and retrace itself forever.
		a := b.rng.Float64()*math.Pi/2 + math.Pi/8
		b.dx, b.dy = math.Cos(a), math.Sin(a)
		if b.rng.Intn(2) == 0 {
			b.dx = -b.dx
		}
		if b.rng.Intn(2) == 0 {
			b.dy = -b.dy
		}
		b.hue = b.rng.Float64()
	}
	b.w, b.h = nw, nh
	b.clampInside()
}

// halfSize is the logo's half-extent in pixels. It is wider than tall because
// a logo is, and because the surface's pixels are square.
func (b *Bounce) halfSize() (hw, hh float64) {
	s := math.Min(b.w, b.h) * b.Size
	if s < 2 {
		s = 2
	}
	return s * 1.6, s
}

func (b *Bounce) clampInside() {
	hw, hh := b.halfSize()
	if b.x < hw {
		b.x = hw
	}
	if b.x > b.w-hw {
		b.x = b.w - hw
	}
	if b.y < hh {
		b.y = hh
	}
	if b.y > b.h-hh {
		b.y = b.h - hh
	}
}

// Frame advances and draws one frame.
func (b *Bounce) Frame(s *canvas.Surface, dt float64) {
	if b.w == 0 || b.h == 0 {
		return
	}
	b.sinceX += dt
	b.sinceY += dt
	if b.flash > 0 {
		b.flash -= dt
	}

	hw, hh := b.halfSize()
	speed := b.Speed
	if speed <= 0 {
		speed = 26
	}

	b.x += b.dx * speed * dt
	b.y += b.dy * speed * dt

	hitX, hitY := false, false
	if b.x < hw {
		b.x, b.dx, hitX = hw, math.Abs(b.dx), true
	} else if b.x > b.w-hw {
		b.x, b.dx, hitX = b.w-hw, -math.Abs(b.dx), true
	}
	if b.y < hh {
		b.y, b.dy, hitY = hh, math.Abs(b.dy), true
	} else if b.y > b.h-hh {
		b.y, b.dy, hitY = b.h-hh, -math.Abs(b.dy), true
	}

	if hitX {
		// A corner is this axis flipping while the other one flipped a moment
		// ago — or both in the same frame, which is sinceY still being 0.
		if b.sinceY <= cornerWindow {
			b.corner()
		}
		b.sinceX = 0
	}
	if hitY {
		if b.sinceX <= cornerWindow {
			b.corner()
		}
		b.sinceY = 0
	}
	if hitX || hitY {
		// The color change on contact is the only feedback the animation
		// gives, so it steps by an irrational-ish fraction of the wheel
		// rather than a neat one: a neat step would cycle through a short
		// repeating set of colors and start to look like a pattern.
		b.hue = math.Mod(b.hue+0.147, 1)
	}

	b.draw(s, hw, hh)
}

// corner records a corner hit. Guarded against counting the same one twice
// when both axes flip in a single frame.
func (b *Bounce) corner() {
	if b.flash > flashTime-cornerWindow {
		return
	}
	b.Corners++
	b.flash = flashTime
}

func (b *Bounce) draw(s *canvas.Surface, hw, hh float64) {
	// The flash is a wash over the whole surface that fades out, so the
	// corner registers even if the viewer was looking elsewhere.
	if b.flash > 0 {
		f := b.flash / flashTime
		v := int32(70 * f * f)
		s.Fill(tcell.NewRGBColor(v, v, v/2+v/4))
	} else {
		s.Clear()
	}

	body := hsv(b.hue, 0.85, 1)
	sw, sh := s.Size()
	x0, x1 := clampRange(b.x-hw, b.x+hw, sw)
	y0, y1 := clampRange(b.y-hh, b.y+hh, sh)

	for y := y0; y < y1; y++ {
		ny := (float64(y) + 0.5 - b.y) / hh
		for x := x0; x < x1; x++ {
			nx := (float64(x) + 0.5 - b.x) / hw
			if c, ok := logo(nx, ny, body); ok {
				s.Set(x, y, c)
			}
		}
	}
}

// logo is the mark itself in a local space where the bounding box is -1..1 on
// both axes: a wide disc with a slot cut through the middle. It is drawn
// rather than described so that it scales with the window and owes nothing to
// anyone's trademark.
//
// It was a disc with a flat foot under it first, on the theory that a pure
// ellipse has no up. Rendered, the foot and the squashed disc together read
// unmistakably as a bowler hat, so the foot is gone. A ring is a better mark
// than a hat, and symmetry costs nothing here — the thing is tumbling around
// a screen, not standing on a shelf.
func logo(x, y float64, body tcell.Color) (tcell.Color, bool) {
	// The slot: an empty band across the middle, stopping short of the rim so
	// the mark stays one connected shape. Without it, at the sizes this runs
	// at, the logo is a featureless blob.
	if math.Abs(y) < 0.17 && math.Abs(x) < 0.60 {
		return 0, false
	}
	// The disc, squashed wide the way a logotype is.
	if (x*x)/(0.98*0.98)+(y*y)/(0.86*0.86) < 1 {
		return body, true
	}
	return 0, false
}

func clampRange(lo, hi float64, n int) (int, int) {
	a := int(math.Floor(lo))
	b := int(math.Ceil(hi))
	if a < 0 {
		a = 0
	}
	if b > n {
		b = n
	}
	if b < a {
		b = a
	}
	return a, b
}

// hsv converts a hue in 0..1 with saturation and value in 0..1 to a color.
func hsv(h, s, v float64) tcell.Color {
	h -= math.Floor(h)
	i := math.Floor(h * 6)
	f := h*6 - i
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return tcell.NewRGBColor(int32(r*255), int32(g*255), int32(b*255))
}

// Run bounces a logo on the given screen until the user stops it.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
