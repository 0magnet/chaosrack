// Package plasma is the drifting colored field of the demoscene.
//
// The method predates the terminal by decades: sum a few sine waves of the
// pixel coordinates, offset each by time, and use the total as an index into a
// looping color ramp. Because the sines never align the same way twice the
// field appears to seethe without ever repeating visibly, and because the ramp
// closes on itself there is no seam where the sum wraps.
//
// Written from that description. libcaca's cacademo shows one of these; no
// code is taken from it.
package plasma

import (
	"math"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Plasma is the animation. The zero value is not usable; call New.
type Plasma struct {
	w, h int
	t    float64

	// sinTab is a sine lookup over one full turn, in the same 0..255 units the
	// palette is indexed by. Trigonometry per pixel per frame is the one thing
	// that makes this effect expensive; a table makes it addition.
	sinTab [tabSize]float64

	// Speed scales how fast the field drifts. 1 is a slow seethe.
	Speed float64
	// Palette must loop — its last entry should match its first, or a seam
	// appears wherever the summed value wraps past 255.
	Palette canvas.Palette

	// AudioGain scales how hard sound drives the surge. 1 is the tuned
	// amount, 0 ignores audio even with a source attached.
	AudioGain float64

	// audio is the last sound handed in and env smooths it. See Listen.
	audio canvas.Audio
	env   canvas.Envelope
}

const tabSize = 1024

// audioSurge is how many extra times its resting speed the field drifts at
// full level.
//
// Three, so a loud passage runs at four times the seethe. Anything much less
// is invisible against a field that is already moving, and much more turns the
// wave sum into a strobe: the terms have different rates, so pushing time hard
// enough makes them beat against each other faster than the eye can integrate
// and the field stops looking like a fluid.
const audioSurge = 3

// New returns a plasma.
func New() *Plasma {
	p := &Plasma{Speed: 1, Palette: canvas.Plasma, AudioGain: 1}
	for i := range p.sinTab {
		p.sinTab[i] = math.Sin(float64(i) / tabSize * 2 * math.Pi)
	}
	// Slower at both ends than the flame. Phase is an integral: what is seen
	// is not the level but everywhere the field has been pushed to since, so a
	// sharper attack here only buys a jolt that is over before it can be read,
	// while a long tail keeps the field coasting after the beat the way a
	// fluid with momentum would. 50 ms up, 400 ms down.
	p.env.Attack = 0.050
	p.env.Decay = 0.400
	return p
}

// Listen takes the sound of the coming frame. See canvas.AudioListener.
// Smoothing happens in Frame, where dt is known.
func (p *Plasma) Listen(a canvas.Audio) { p.audio = a }

// sin looks up sin(turns) where turns is in revolutions rather than radians,
// wrapping automatically.
func (p *Plasma) sin(turns float64) float64 {
	i := int(turns*tabSize) % tabSize
	if i < 0 {
		i += tabSize
	}
	return p.sinTab[i]
}

// Resize records the surface size. Nothing is allocated: the field is a pure
// function of position and time, so there is no buffer to keep.
func (p *Plasma) Resize(w, h int) { p.w, p.h = w, h }

// audioLevel advances the envelope by dt and returns the smoothed level,
// clamped, scaled by the gain. Zero when nothing is listening, and exactly
// zero rather than nearly.
func (p *Plasma) audioLevel(dt float64) float64 {
	p.env.Step(p.audio, dt)
	v := p.env.Level() * p.AudioGain
	if v <= 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Frame advances time and draws the field.
func (p *Plasma) Frame(s *canvas.Surface, dt float64) {
	// The mapping is: loudness drives the rate of phase advance.
	//
	// The field is a pure function of position and time, so time is the only
	// thing there is to drive — and driving it is the right thing to drive.
	// Scaling brightness or the palette would make the plasma flash, which any
	// screensaver does; scaling the phase makes the whole field lurch forward
	// and then coast, so the surge is in the motion rather than painted on top
	// of it. Because it is a rate and not a position, the field never jumps: a
	// beat leaves it somewhere further along the same continuous drift.
	//
	// A level of exactly zero multiplies by exactly one, which is why an
	// unattached plasma draws precisely what it always did.
	surge := 1 + audioSurge*p.audioLevel(dt)
	// 0.12 turns per second: the old 0.004 per frame at 30fps.
	p.t += 0.12 * p.Speed * surge * dt
	if p.w == 0 || p.h == 0 {
		return
	}

	// Scale so the pattern has roughly the same number of bands whatever the
	// window size, rather than becoming a single blur when small and a fine
	// stipple when large.
	fw := 6 / float64(p.w)
	fh := 6 / float64(p.h)

	for y := 0; y < p.h; y++ {
		fy := float64(y) * fh
		// The two terms that depend only on y are lifted out of the inner
		// loop; at this pixel count that is most of the work.
		a := p.sin(fy + p.t*1.3)
		b := p.sin(fy*0.5 - p.t*0.7)
		for x := 0; x < p.w; x++ {
			fx := float64(x) * fw
			v := a +
				p.sin(fx+p.t) +
				b +
				// A diagonal term, so the field is not merely the sum of a
				// horizontal and a vertical wave — that reads as a grid.
				p.sin((fx+fy)*0.4+p.t*1.7)
			// v is in -4..4. Fold it into 0..255.
			i := int((v + 4) * 255 / 8)
			if i < 0 {
				i = 0
			} else if i > 255 {
				i = 255
			}
			s.Set(x, y, p.Palette[i])
		}
	}
}

// Run draws plasma on the screen until the user quits.
func Run(screen tcell.Screen) error {
	return canvas.Run(screen, New(), canvas.Options{})
}
