// Package lightning is a branching electrical discharge.
//
// A stroke is built by random midpoint displacement. Take the two ends of the
// channel, put a point halfway between them and shove it sideways by a random
// amount; do the same to each half with half the shove, and again, and again.
// Six rounds of that gives a path that is jagged at every scale, which is what
// a spark actually is — the discharge follows whichever local patch of air
// breaks down first, and that decision is made again at every scale on the way
// down. Drawing a smooth arc and adding wobble gives a wire with wobble;
// subdividing gives lightning.
//
// The stroke is revealed downward rather than appearing whole, because that is
// what happens: the stepped leader gropes its way toward the ground in dim
// jumps over some tens of milliseconds, throwing off branches that go nowhere,
// and only when it arrives does the return stroke fire back up the channel and
// produce nearly all of the light. So this descends dimly, flashes hard along
// the main channel the instant it lands, and then decays through an afterglow
// while the pause runs out and the next leader starts building. The branches
// are lit a fraction after the leader passes their root, which is what makes
// them read as coming off the channel rather than being drawn with it.
//
// The whole picture is one intensity field. Every segment adds light to it and
// the field decays exponentially each frame, which is why the flash leaves a
// fading ghost of its own shape rather than switching off: an afterglow is a
// decay, not a second animation. It is also what makes the branches fade out
// from the tips first, since they were dimmer to begin with.
//
// Pairs with rain, which is the same weather from the other end.
//
// Written from a description of the physics and of the midpoint displacement
// technique, not from any implementation.
package lightning

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Bolt is the default ramp: nothing, through the deep blue-violet of ionized
// air, to a white core.
//
// The dark end has to be nearly black rather than actually black, because
// almost the whole screen is afterglow almost all of the time and a ramp that
// bottoms out in pure black throws away the dim half of every stroke's decay,
// which is most of what there is to look at between flashes.
var Bolt = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 4, G: 4, B: 16},
	canvas.Stop{At: 0.35, R: 60, G: 40, B: 170},
	canvas.Stop{At: 0.70, R: 150, G: 170, B: 255},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 255},
)

// seg is one straight piece of a stroke, held until the descent reaches it.
type seg struct {
	x0, y0, x1, y1 float64
	// order is where in the descent this segment lights, 0 at the top of the
	// channel and 1 at the ground. A branch takes its parent's order plus a
	// little, so it appears just after the leader has gone past its root.
	order float64
	// amp is how much light it adds when it does. The main channel is drawn at
	// full strength and branches progressively weaker, which is the only thing
	// telling the eye which path the discharge actually took.
	amp float64
	// main marks the channel from the cloud to the ground. Only these carry
	// the return stroke.
	main bool
}

// Lightning is the animation. The zero value is not usable; call New.
type Lightning struct {
	w, h   int
	fw, fh float64

	// glow is the light in the air, one value per pixel. Everything is drawn
	// by adding into it and it decays on its own, so the afterglow costs
	// nothing beyond the decay that was going to happen anyway.
	glow []float32

	segs []seg
	// px and py are the scratch the midpoint displacement works in. Held here
	// so building a stroke inside Frame allocates nothing.
	px, py []float64

	// progress is how far the leader has descended, 0 to 1; last is where it
	// was at the previous frame, so each segment is stamped exactly once as
	// the descent passes it.
	progress, last float64
	waiting        bool
	wait           float64

	// ambient is the flat flash added to the whole surface at the moment of
	// the return stroke — the light a real one throws on everything around it.
	// It decays much faster than the channel does.
	ambient float64

	strikes int

	rng *rand.Rand

	// DescendTime is how long the leader takes to reach the ground, in
	// seconds. Short enough that the descent reads as one gesture rather than
	// a line being drawn.
	DescendTime float64

	// MinPause and MaxPause bound the dark gap after a stroke, in seconds. A
	// fixed gap turns the animation into a metronome, which is the one thing
	// lightning never looks like.
	MinPause, MaxPause float64

	// Afterglow is the time constant of the decay, in seconds: the channel
	// falls to about a third of its brightness in this long. Longer leaves
	// ghost strokes hanging in the air, shorter makes each flash a blink with
	// nothing after it.
	Afterglow float64

	// AmbientLife is the same for the flat flash on the whole surface, which
	// is deliberately much shorter — a lingering full-screen wash reads as a
	// fault in the terminal rather than as weather.
	AmbientLife float64

	// Levels is how many times the channel is subdivided. Each level doubles
	// the segment count and halves the displacement, so this is how fine the
	// jaggedness goes; past about eight the extra detail is smaller than a
	// pixel and only costs work.
	Levels int

	// Roughness is the first displacement, as a fraction of the surface width.
	// It sets how far the stroke wanders from a straight line overall.
	Roughness float64

	// Branches is how many side branches are thrown off, and BranchSegments
	// how many pieces each is drawn in. Branches are what make it a discharge
	// rather than a crack; too many and the channel is lost in them.
	Branches, BranchSegments int

	// FlashGain is how much brighter the return stroke is than the leader that
	// preceded it. In nature it is most of the light in the event, and making
	// it a factor of several is what gives the strike its snap.
	FlashGain float64

	// AmbientFlash is how much light the return stroke throws on the rest of
	// the surface, as a palette fraction.
	AmbientFlash float64

	// Halo is how much of a segment's light spills into the neighboring
	// pixels. A one-pixel channel on a half-block surface is a hairline and
	// reads as a scratch; the spill is what gives it a glow around it.
	Halo float64

	// Cutoff is the intensity below which a pixel is left as the terminal's
	// own background. The decay never actually reaches zero, so without this
	// the whole surface stays faintly lit forever and the strokes stop
	// standing out of the dark.
	Cutoff float64

	// Palette colors the field, dark at nothing and white at the core.
	Palette canvas.Palette
}

// New returns a lightning animation. seed of 0 gives a fixed sequence of
// strokes, which makes tests repeatable.
func New(seed int64) *Lightning {
	return &Lightning{
		rng:            rand.New(rand.NewSource(seed)), //nolint:gosec
		DescendTime:    0.32,
		MinPause:       0.45,
		MaxPause:       1.9,
		Afterglow:      0.30,
		AmbientLife:    0.11,
		Levels:         6,
		Roughness:      0.16,
		Branches:       9,
		BranchSegments: 7,
		FlashGain:      3.2,
		AmbientFlash:   0.11,
		Halo:           0.4,
		Cutoff:         0.02,
		Palette:        Bolt,
	}
}

// maxLevels bounds the subdivision so a caller cannot ask Resize for an
// arbitrarily large scratch buffer.
const maxLevels = 10

// Resize allocates the glow field and the scratch a stroke is built in, and
// starts the first leader. Called before the first frame and on every resize.
func (l *Lightning) Resize(w, h int) {
	l.w, l.h = w, h
	l.fw, l.fh = float64(w), float64(h)
	l.glow = make([]float32, w*h)

	if l.Levels < 1 {
		l.Levels = 1
	}
	if l.Levels > maxLevels {
		l.Levels = maxLevels
	}
	n := 1<<l.Levels + 1
	l.px = make([]float64, n)
	l.py = make([]float64, n)

	if l.Branches < 0 {
		l.Branches = 0
	}
	if l.BranchSegments < 1 {
		l.BranchSegments = 1
	}
	// The channel is one segment per interval between points, and every branch
	// is BranchSegments more. Sized once, appended into forever.
	l.segs = make([]seg, 0, (n-1)+l.Branches*l.BranchSegments)

	l.progress, l.last = 0, 0
	l.ambient = 0
	l.waiting = false
	l.strikes = 0
	if w > 0 && h > 0 {
		l.build()
	}
}

// Frame advances the strike and draws the field.
func (l *Lightning) Frame(s *canvas.Surface, dt float64) {
	if l.w == 0 || l.h == 0 {
		return
	}
	l.advance(dt)

	// Exponential decay, so the same elapsed time always costs the same
	// fraction of the light however it was divided into frames. A subtraction
	// per frame would fade twice as fast at twice the frame rate.
	keep := float32(math.Exp(-dt / l.Afterglow))
	l.ambient *= math.Exp(-dt / l.AmbientLife)

	amb := float32(l.ambient)
	cut := float32(l.Cutoff)
	// Decaying and drawing in the same sweep, because both are a pass over
	// every pixel and there is no reason to make two of them.
	for y := 0; y < l.h; y++ {
		row := y * l.w
		for x := 0; x < l.w; x++ {
			v := l.glow[row+x] * keep
			l.glow[row+x] = v
			v += amb
			if v < cut {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			n := int(v * 255)
			if n > 255 {
				n = 255
			}
			s.Set(x, y, l.Palette[n])
		}
	}
}

// advance runs the descent, the flash and the pause across dt seconds,
// carrying any time left over from one phase into the next.
//
// Carrying matters more than it looks: without it each phase change would
// round up to the next frame, and a stroke would take a couple of frames
// longer at thirty a second than at sixty. The whole sequence would then run
// at a rate that depended on the terminal.
func (l *Lightning) advance(dt float64) {
	// Two phase changes in one frame is already an unreasonable frame; the
	// bound is only here so a pathological DescendTime cannot spin.
	for i := 0; dt > 0 && i < 8; i++ {
		if l.waiting {
			if l.wait > dt {
				l.wait -= dt
				return
			}
			dt -= l.wait
			l.waiting = false
			l.build()
			continue
		}
		adv := dt / l.DescendTime
		if l.progress+adv < 1 {
			l.progress += adv
			l.reveal()
			return
		}
		dt -= (1 - l.progress) * l.DescendTime
		l.progress = 1
		l.reveal()
		l.flash()
		l.waiting = true
		l.wait = l.MinPause + l.rng.Float64()*(l.MaxPause-l.MinPause)
	}
}

// reveal stamps every segment the leader has passed since the previous frame.
func (l *Lightning) reveal() {
	for i := range l.segs {
		s := &l.segs[i]
		if s.order > l.last && s.order <= l.progress {
			l.addLine(s.x0, s.y0, s.x1, s.y1, s.amp)
		}
	}
	l.last = l.progress
}

// flash fires the return stroke back up the channel and lights the air.
func (l *Lightning) flash() {
	for i := range l.segs {
		s := &l.segs[i]
		g := l.FlashGain
		if !s.main {
			// Branches carry a share of the return stroke rather than none —
			// they do light up in a real strike — but much less of it, which
			// is what keeps the main channel legible through the flash.
			g *= 0.35
		}
		l.addLine(s.x0, s.y0, s.x1, s.y1, s.amp*g)
	}
	l.ambient += l.AmbientFlash
	l.strikes++
}

// build lays out the next stroke. Nothing is drawn here; the segments wait for
// the descent to reach them.
func (l *Lightning) build() {
	l.segs = l.segs[:0]
	l.progress, l.last = 0, 0

	n := len(l.px)
	// The channel runs the full height. Starting it off screen at the top and
	// ending it at the bottom edge means the eye never sees either end, which
	// is what puts the cloud above the window and the ground below it.
	x0 := (0.15 + l.rng.Float64()*0.7) * l.fw
	drift := (l.rng.Float64()*2 - 1) * l.fw * 0.35
	l.px[0], l.py[0] = x0, 0
	l.px[n-1], l.py[n-1] = x0+drift, l.fh-1
	for i := 1; i < n-1; i++ {
		// y is a straight interpolation and only x is displaced. Displacing
		// both would let the path double back on itself, and a discharge that
		// climbs is not what anyone has ever seen out of a window.
		l.py[i] = l.fh * float64(i) / float64(n-1)
	}

	disp := l.Roughness * l.fw
	for step := n - 1; step > 1; step /= 2 {
		half := step / 2
		for i := half; i < n; i += step {
			l.px[i] = (l.px[i-half]+l.px[i+half])/2 + (l.rng.Float64()*2-1)*disp
		}
		// Halving the displacement with the interval is what makes the result
		// self-similar: the same relative jaggedness at every scale.
		disp *= 0.55
	}

	for i := 0; i+1 < n; i++ {
		l.push(seg{
			x0: l.px[i], y0: l.py[i], x1: l.px[i+1], y1: l.py[i+1],
			order: float64(i+1) / float64(n-1),
			amp:   0.45,
			main:  true,
		})
	}

	for b := 0; b < l.Branches; b++ {
		// Rooted in the upper part of the channel, because a branch off the
		// last few pixels has no room to go anywhere and reads as a stub.
		i := 1 + l.rng.Intn(n*3/4)
		l.branch(i, float64(i)/float64(n-1))
	}
}

// branch grows one side branch from point i of the channel.
func (l *Lightning) branch(i int, at float64) {
	x, y := l.px[i], l.py[i]
	// Off to one side but still descending. A branch that leaves horizontally
	// looks stuck on; the discharge is going down and so is everything it
	// throws off.
	side := 1.0
	if l.rng.Intn(2) == 0 {
		side = -1
	}
	ang := math.Pi/2 - side*(0.35+l.rng.Float64()*0.7)
	// Length is a share of what is left below the root, so a branch never runs
	// past the ground the main channel is heading for.
	span := (l.fh - y) * (0.15 + l.rng.Float64()*0.4)
	if span < 2 {
		return
	}
	stepLen := span / float64(l.BranchSegments)
	amp := 0.3
	for k := 0; k < l.BranchSegments; k++ {
		// The branch wanders as it goes, for the same reason the channel does.
		ang += (l.rng.Float64()*2 - 1) * 0.32
		nx := x + math.Cos(ang)*stepLen
		ny := y + math.Sin(ang)*stepLen
		// A hair after the leader passes the root, and a hair more per piece,
		// so a branch grows outward instead of appearing all at once.
		order := at + float64(k+1)*0.008
		if order > 0.999 {
			order = 0.999
		}
		l.push(seg{x0: x, y0: y, x1: nx, y1: ny, order: order, amp: amp})
		x, y = nx, ny
		// Branches taper to nothing, which is how they end without a visible
		// cut.
		amp *= 0.78
	}
}

// push appends a segment if there is room. There always is for the defaults;
// the check is for a caller that raised Branches after Resize sized the slice.
func (l *Lightning) push(s seg) {
	if len(l.segs) < cap(l.segs) {
		l.segs = append(l.segs, s)
	}
}

// addLine adds light along a segment, one sample per pixel of its longer axis.
func (l *Lightning) addLine(x0, y0, x1, y1, amp float64) {
	dx, dy := x1-x0, y1-y0
	n := int(math.Max(math.Abs(dx), math.Abs(dy)))
	if n < 1 {
		n = 1
	}
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		l.addPoint(x0+dx*t, y0+dy*t, amp)
	}
}

// addPoint adds light at one pixel and spills some into its neighbors.
func (l *Lightning) addPoint(x, y float64, amp float64) {
	xi, yi := int(math.Round(x)), int(math.Round(y))
	l.add(xi, yi, amp)
	h := amp * l.Halo
	l.add(xi-1, yi, h)
	l.add(xi+1, yi, h)
	l.add(xi, yi-1, h)
	l.add(xi, yi+1, h)
}

func (l *Lightning) add(x, y int, amp float64) {
	if x < 0 || y < 0 || x >= l.w || y >= l.h {
		return
	}
	l.glow[y*l.w+x] += float32(amp)
}

// Run strikes the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
