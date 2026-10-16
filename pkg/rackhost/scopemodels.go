package rackhost

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// The scope models the native rack draws a frame at a time, from the page's
// own generators (pkg/attractor): the Graphic Artist's drifting wireframe,
// the scope clock, and Fourier Text's banner.

// scopeModels are their running state.
type scopeModels struct {
	ga     *attractor.GraphicArtist
	text   *attractor.FourierText
	turtle attractor.TurtleRun
	// cycleAt is when CYCLE next steps the Turtle's modulus.
	cycleAt time.Time
	last    time.Time
}

func newScopeModels() scopeModels {
	return scopeModels{ga: attractor.NewGraphicArtist(), text: attractor.NewFourierText()}
}

// isScopeModel reports whether model key is one of them.
func isScopeModel(key string) bool {
	return key == "graphicartist" || key == "scopeclock" || key == "scopetext" || key == "turtle"
}

// scopeFigure is scope model key now, stepped on to now while running, and
// the frame it is drawn in. Called with r.mu held.
func (r *Rack) scopeFigure(key string, running bool) (attractor.Figure, [][3]float64) {
	sm := &r.analyzers.scope
	now := r.sound.now()
	frames := 0.0
	if running && !sm.last.IsZero() {
		// The page moves them on a frame at 60 a second.
		frames = math.Min(now.Sub(sm.last).Seconds()*60, 60)
	}
	sm.last = now
	sub, scale := speedSteps(r.number("speed-slider"))
	steps := max(int(r.number("trail-slider")), 2)
	switch key {
	case "graphicartist":
		for _, id := range attractor.GraphicArtistKnobs {
			sm.ga.Set(id, float32(r.number(id)))
		}
		sm.ga.Drift(frames, float32(scale))
		f := attractor.Figure{Kind: attractor.FigurePath, Points: make([][3]float64, steps)}
		sm.ga.Trace(steps, func(i int, x, y, z float64) { f.Points[i] = [3]float64{x, y, z} })
		return f, [][3]float64{{-1.5, 0, 0}, {1.5, 0, 0}}
	case "turtle":
		r.turtleCycle(now, running)
		var k [9]float32
		for i, id := range attractor.TurtleKnobs {
			k[i] = float32(r.number(id))
		}
		f := attractor.Figure{Kind: attractor.FigurePath}
		phys := r.turtlePhysics(sub, scale)
		sm.turtle.Step(k, steps, frames, sub, float32(scale), running, phys, func(x, y, z, tint float32) {
			f.Points = append(f.Points, [3]float64{float64(x), float64(y), float64(z)})
			f.Ages = append(f.Ages, tint) // the tinter's color, as the page's trail attribute
		})
		r.aimTurtle()
		if r.value("use-points") == "1" {
			f.Kind = attractor.FigurePoints
		}
		if phys != nil {
			// Framed to the room, so its floor is the picture's bottom edge.
			return f, [][3]float64{{0, -1, 0}, {0, 1, 0}}
		}
		// The walk's own camera holds it still and sized to its box.
		return f, [][3]float64{{-1.2, -1.2, -1.2}, {1.2, 1.2, 1.2}}
	case "scopeclock":
		f := attractor.Figure{Kind: attractor.FigurePath, Points: make([][3]float64, steps)}
		if !attractor.ClockTrace(time.Now(), steps, func(i int, x, y float64) { f.Points[i] = [3]float64{x, y, 0} }) {
			f.Points = nil
		}
		return f, square(f.Points)
	case "scopetext":
		if v := r.value("stext-in"); v != "" {
			sm.text.SetText(strings.ToUpper(v)) // the page's banner is capitals
		}
		sm.text.Set("stext-harm", float32(r.number("stext-harm")))
		sm.text.Sweep(frames, sub, float32(scale))
		f := attractor.Figure{Kind: attractor.FigureLines}
		for _, s := range sm.text.Strokes() {
			// Each stroke on its own, as the beam is blanked between them.
			for k := 0; k+1 < len(s); k += 2 {
				if k > 0 && len(f.Points) < 65535 {
					f.Edges = append(f.Edges, uint16(len(f.Points)-1), uint16(len(f.Points))) //nolint:gosec // held under the index type's reach
				}
				f.Points = append(f.Points, [3]float64{s[k], s[k+1], 0})
			}
		}
		return f, square(f.Points)
	}
	return attractor.Figure{}, nil
}

// square is the frame a plane figure is drawn in: its own reach either way,
// so it holds still while it is drawn.
func square(pts [][3]float64) [][3]float64 {
	e := 0.0
	for _, p := range pts {
		e = math.Max(e, math.Max(math.Abs(p[0]), math.Abs(p[1])))
	}
	if e == 0 {
		e = 1
	}
	return [][3]float64{{-e, -e, 0}, {e, e, 0}}
}

// turtleCycle is CYCLE: every so many seconds the Turtle steps to the next
// modulus, turning the MOD knob as the page's does, so its readout follows.
// Called with r.mu held.
func (r *Rack) turtleCycle(now time.Time, running bool) {
	sm := &r.analyzers.scope
	secs := r.number("turtle-cycle")
	if secs <= 0 || !running {
		sm.cycleAt = time.Time{}
		return
	}
	if sm.cycleAt.IsZero() {
		sm.cycleAt = now.Add(time.Duration(secs * float64(time.Second)))
		return
	}
	if now.Before(sm.cycleAt) {
		return
	}
	sm.cycleAt = now.Add(time.Duration(secs * float64(time.Second)))
	next := int(r.number("turtle-mod")) + 1
	if next > turtleModMax {
		next = 1
	}
	_ = r.set("turtle-mod", strconv.Itoa(next)) //nolint:errcheck // within the knob's travel
}

// turtleModMax is the Turtle's MOD knob's top, where CYCLE wraps.
const turtleModMax = 300

// turtleRoom is half the native Turtle's room, up and down, with weight: the
// frame it is fitted to then (±1 at 0.85 of the half frame), which is what
// makes the picture's bottom edge the floor. Across, it is the page's default
// aspect wider.
const turtleRoom = 1 / 0.85

// turtlePhysics is the Turtle's weight with the Physics module's ON up,
// or nil: the room, the Physics knobs, and the page's step at the Speed
// knob's sub-steps and scale. Called with r.mu held.
func (r *Rack) turtlePhysics(steps int, scale float64) *attractor.TurtlePhysics {
	if r.value("phys-sw") != "1" {
		return nil
	}
	return &attractor.TurtlePhysics{
		HalfW: turtleRoom * 1.6, HalfH: turtleRoom,
		Grav: float32(r.number("turtle-grav")), Fric: float32(r.number("turtle-fric")),
		Bounce: float32(r.number("turtle-bounce")), Spin: float32(r.number("turtle-spin")),
		Dt: 0.016 * float32(steps) * float32(scale),
	}
}
