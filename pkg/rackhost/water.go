package rackhost

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/0magnet/chaosrack/pkg/ripple"
)

// The water lens, natively: BEHIND on water looks at the finished scene
// through a disturbed fluid surface, as the page's does (water_js.go): a
// water tank (pkg/ripple's Tank) stepped each of the page's frames, read by
// refraction — the image bent by the surface's slope; the model is the only
// light, so there is no glint. A drag across the scene draws a wake in it.
// The page's reflection and waterline come with its Fore knob, which a native
// rack does not have: here all of the model is under the water.

// waterLens is the tank and the drag disturbing it.
type waterLens struct {
	field        ripple.Surface
	last         time.Time
	dragging     bool
	lastX, lastY float32
}

// waterOn reports whether BEHIND is on water. Called with r.mu held.
func (r *Rack) waterOn() bool { return r.value("bg-visual") == "water" }

// step runs the tank on by the page's frames since the last, with its
// weather. Called with r.mu held.
func (wa *waterLens) step(now time.Time, env float32, d attractor.WaterSettings) {
	g := attractor.WaterGrid
	if wa.field == nil {
		wa.field = ripple.NewTank(g, g)
	}
	f := wa.field
	f.Tune(d.Speed, d.Damp/1000, d.Spread/100, d.Edge/100)
	frames := 1
	if !wa.last.IsZero() {
		frames = max(1, min(int(now.Sub(wa.last).Seconds()*60+0.5), 6))
	}
	wa.last = now
	for range frames {
		if d.Drive > 0 && env > 0 {
			// The sound pushes the surface where the speakers stand.
			n := max(int(d.Srcs), 1)
			for i := range n {
				x, y := waterSource(i, n)
				f.Drop(x, y, wcells(5), env*d.Drive/100)
			}
		}
		if d.Rain > 0 && rand.Float64() < float64(d.Rain)/60 { //nolint:gosec // decorative
			f.Drop(rand.Float32()*float32(g), rand.Float32()*float32(g), wcells(3), 0.5) //nolint:gosec // decorative
		}
		f.Step()
	}
}

// waterSource is where speaker i of n stands in the tank: round a circle,
// as the page places them, or in the middle alone.
func waterSource(i, n int) (float32, float32) {
	g := float64(attractor.WaterGrid)
	if n <= 1 {
		return float32(g / 2), float32(g / 2)
	}
	th := 2 * math.Pi * float64(i) / float64(n)
	return float32(g/2 + g*0.28*math.Cos(th)), float32(g/2 + g*0.28*math.Sin(th))
}

// refract is frame m seen through the surface: each pixel read from where
// the slope there bends it, as the page's shader reads its frame.
func (wa *waterLens) refract(m *rackpic.Image, d attractor.WaterSettings) {
	if wa.field == nil {
		return
	}
	g := attractor.WaterGrid
	h := func(x, y int) float32 {
		x, y = min(max(x, 0), g-1), min(max(y, 0), g-1)
		return min(max(wa.field.Height(x, y), -1), 1) // past ±1 is past meaning anything
	}
	src := append([]byte(nil), m.Px...)
	amount := d.Amount / attractor.WaterGridFD // the knob counts texels of the grid it was set on
	// The tank is square and the scene is not, so its cells are as wide as
	// the scene is, against its height; the tank steps for that from the
	// next step on, and a slope across is read as the page reads it.
	asp := float32(m.W) / float32(m.H)
	if tk, ok := wa.field.(*ripple.Tank); ok {
		tk.Aspect = asp
	}
	asp2 := wa.field.CellAspect() * wa.field.CellAspect()
	for y := range m.H {
		fy := (y*g + g/2) / m.H
		for x := range m.W {
			fx := (x*g + g/2) / m.W
			// Central differences: the slope, which is what bends light.
			hx := (h(fx+1, fy) - h(fx-1, fy)) / asp2
			hy := h(fx, fy+1) - h(fx, fy-1)
			sx := min(max(x+int(hx*amount*float32(m.W)), 0), m.W-1)
			sy := min(max(y+int(hy*amount*float32(m.H)), 0), m.H-1)
			i, j := 3*(y*m.W+x), 3*(sy*m.W+sx)
			copy(m.Px[i:i+3], src[j:j+3])
		}
	}
}

// touch disturbs the tank at x, y of a w by h scene: a drop where a drag
// begins and a wake along it. Called with r.mu held.
func (wa *waterLens) touch(kind string, x, y float64, w, h int) {
	if wa.field == nil {
		return
	}
	g := float32(attractor.WaterGrid)
	fx, fy := float32(x)/float32(w)*g, float32(y)/float32(h)*g
	switch kind {
	case "down":
		wa.dragging = true
		wa.field.Drop(fx, fy, wcells(4), 0.6)
	case "move":
		if wa.dragging {
			wa.field.Line(wa.lastX, wa.lastY, fx, fy, wcells(3), 0.5)
		}
	case "up":
		wa.dragging = false
	}
	wa.lastX, wa.lastY = fx, fy
}

// waterDrive is the loudness the water's speakers push with, 0..1, when the
// lens's DRIVE is up: the rack's signal, followed as the bifurcation's cursor
// follows it. Called with r.mu held.
func (r *Rack) waterDrive() float32 {
	if r.number("water-drive") <= 0 {
		return 0
	}
	l, rt := r.sound.window(envWindow)
	return r.waterEnv.follow((rms(l)+rms(rt))/2, r.sound.now())
}

// waterSettings is the lens's knobs now (the Water group in the model
// bank). Called with r.mu held.
func (r *Rack) waterSettings() attractor.WaterSettings {
	return attractor.WaterFrom(func(id string) float32 { return float32(r.number(id)) })
}

// wcells is r cells of the grid the lens's disturbances were sized for, in
// the tank's: a splash is the same size on the screen as the page's.
func wcells(r float32) float32 { return r * attractor.WaterGrid / attractor.WaterGridFD }
