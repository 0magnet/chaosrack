// Package tunnel flies the viewer down a textured tube.
//
// The trick is that a straight tube seen from inside has a closed-form
// mapping from the screen back to its surface. For a pixel at angle a and
// distance r from the center, the point of the tube it shows is at the same
// angle a and at a distance along the tube proportional to one over r: the
// far end of the tube crowds into the vanishing point, so small r means large
// depth. Index a repeating pattern by that pair and the flat screen reads as
// a tube. Add a constant to the depth each frame and the viewer flies down it;
// add one to the angle and the tube rolls.
//
// Both coordinates depend only on where the pixel is, never on time, so they
// are computed once per resize into tables. Per frame each pixel then costs
// two byte additions and a lookup, where the direct version would cost an
// atan2 and a division — the difference between an effect that runs anywhere
// and one that does not.
//
// Written from that description. No code is taken from any existing
// implementation.
package tunnel

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Both coordinates are kept as bytes, so a full turn of the tube and one
// period of its texture are both 256 units and both wrap for free when the
// offsets are added. cellBits splits that period into the squares of the
// checker: 5 bits gives 8 sectors around the tube and a ring every 32 units
// of depth, which is coarse enough to read at terminal resolution.
const cellBits = 5

// baseDepth and baseSpin are the offsets added per second at Speed and Spin of
// 1. A ring every 32 units means one passes the viewer roughly every second and
// a third, and the tube rolls once every twenty-four seconds — fast enough to
// be alive, slow enough not to be a strobe. Per second rather than per frame,
// so the tube travels at the same rate whatever the frame rate is and a dropped
// frame is skipped over instead of slowing the whole effect down.
const (
	baseDepth = 24
	baseSpin  = 10.5
)

// Tunnel is the animation. The zero value is not usable; call New.
type Tunnel struct {
	w, h int

	// Per-pixel tables, row-major, rebuilt on resize. angle is the bearing of
	// the pixel from the center and depth is how far down the tube it shows.
	// shade is separate because it must not wrap: it is how bright the tube
	// is at that distance, and a wrap there would put a hard black ring where
	// the light should simply be fading out.
	angle []uint8
	depth []uint8
	shade []uint8

	// tDepth and tSpin are the accumulated offsets, kept as floats so that a
	// fraction of a texture unit in a frame still adds up over time.
	tDepth, tSpin float64

	// Speed multiplies how fast the viewer travels down the tube. Negative
	// flies backwards.
	Speed float64
	// Spin multiplies how fast the tube rolls about the axis of travel.
	Spin float64
	// Palette colors the tube by distance, dark at the vanishing point and
	// bright at the mouth.
	Palette canvas.Palette
	// Contrast is how much darker the dark squares of the checker are, from 0
	// (no pattern at all) to 1 (black). Without a pattern on the wall there is
	// nothing to move, and the tunnel is just a glowing ring.
	Contrast float64
}

// New returns a tunnel. The seed is accepted so that every animation in this
// repository is constructed the same way; nothing here is random.
func New(seed int64) *Tunnel {
	_ = seed
	return &Tunnel{
		Speed:    1,
		Spin:     1,
		Palette:  canvas.Fire,
		Contrast: 0.5,
	}
}

// Resize builds the per-pixel tables. This is the whole cost of the effect.
func (t *Tunnel) Resize(w, h int) {
	t.w, t.h = w, h
	if w <= 0 || h <= 0 {
		t.angle, t.depth, t.shade = nil, nil, nil
		return
	}
	t.angle = make([]uint8, w*h)
	t.depth = make([]uint8, w*h)
	t.shade = make([]uint8, w*h)

	cx, cy := float64(w)/2, float64(h)/2

	// The tube's radius is tied to the shorter side, so the same view of it
	// appears whatever the window's shape or size. Rings fall at radius/n for
	// each whole number n, crowding towards the vanishing point; putting the
	// first one just outside the corner leaves half a dozen of them across
	// the picture, which is enough to read as travel without turning the
	// middle of the screen into a moiré. Scaling by the texture period puts
	// that ring on a square boundary rather than somewhere arbitrary.
	radius := math.Min(cx, cy) * 2 * (1 << cellBits)

	// The brightest thing on screen is the corner, the part of the tube
	// closest to the viewer. Measuring against it means the full range of the
	// palette is used in every window shape.
	maxR := math.Hypot(cx, cy)

	for y := 0; y < h; y++ {
		dy := float64(y) - cy
		for x := 0; x < w; x++ {
			dx := float64(x) - cx
			r := math.Hypot(dx, dy)
			// The exact center is the vanishing point, where the depth is
			// infinite. Clamping just under a pixel keeps the division finite
			// without moving anything that can be seen.
			if r < 0.5 {
				r = 0.5
			}

			i := y*w + x

			// atan2 spans -pi..pi; shifted and scaled it spans one byte, and
			// the seam at the back of the tube falls where the byte wraps,
			// which is exactly where the pattern wraps too.
			a := (math.Atan2(dy, dx) + math.Pi) / (2 * math.Pi) * 256
			t.angle[i] = uint8(int(a) & 255)

			// Depth grows without bound towards the center; only its low bits
			// survive into the byte, and those are the ones the pattern uses.
			t.depth[i] = uint8(int(radius/r) & 255)

			v := int(r / maxR * 255)
			if v > 255 {
				v = 255
			}
			t.shade[i] = uint8(v)
		}
	}
}

// Frame advances the offsets by dt seconds and draws the tube.
func (t *Tunnel) Frame(s *canvas.Surface, dt float64) {
	if t.w == 0 || t.h == 0 || t.angle == nil {
		return
	}
	t.tDepth += baseDepth * t.Speed * dt
	t.tSpin += baseSpin * t.Spin * dt
	// Keeping the accumulators inside one period stops them from drifting into
	// the range where a float64 can no longer resolve a single unit, which is
	// what makes a long-running effect quietly freeze.
	t.tDepth = math.Mod(t.tDepth, 256)
	t.tSpin = math.Mod(t.tSpin, 256)

	dOff := uint8(int(t.tDepth) & 255)
	aOff := uint8(int(t.tSpin) & 255)

	// How bright the dark squares are, as a fraction in 256ths. Fixed point so
	// that dimming a pixel is a multiply and a shift rather than a trip
	// through the floating point unit on every pixel of every frame.
	dark := int((1 - t.Contrast) * 256)
	if dark < 0 {
		dark = 0
	} else if dark > 256 {
		dark = 256
	}

	for y := 0; y < t.h; y++ {
		row := y * t.w
		for x := 0; x < t.w; x++ {
			i := row + x
			a := t.angle[i] + aOff
			// Adding the offset to the depth, rather than subtracting it,
			// sends the rings outward past the viewer, which is forward
			// travel. The other sign pulls them into the vanishing point and
			// reads as falling backwards.
			d := t.depth[i] + dOff
			v := int(t.shade[i])
			// One bit of each coordinate, exclusive-ored: the squares
			// alternate both around the tube and along it, so neither rings
			// nor stripes dominate and the surface has a grain to move.
			if (a^d)&(1<<cellBits) != 0 {
				v = v * dark >> 8
			}
			s.Set(x, y, t.Palette[v])
		}
	}
}

// Run draws a tunnel on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
