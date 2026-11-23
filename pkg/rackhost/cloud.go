package rackhost

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The swarm and the density view as the page draws them (swarm_js.go,
// density_js.go): Display's POINTS S on a 3-D flow, TRAIL's count of
// particles carried through it at once, and POINTS D on a map, how often the
// orbit lands in each part of the plane. The engines are the page's own,
// dynamics.Swarm and dynamics.Density; what differs is the drawing, which
// here is points through rasterview, whose back-of-figure dimming is the
// depth cue.

// cloudState is the swarm and the density counts the scene keeps between
// frames, and the switches as the last frame saw them, so a switch-on starts
// each afresh as the page's does.
type cloudState struct {
	swarm   dynamics.Swarm
	swarmOn bool
	dens    dynamics.Density
	densKey string
	densOn  bool
	orbits  dynamics.Orbits
	levels  []byte
}

// swarmStepsPerFrame is the page's: four steps of the model's dt a frame at
// SPEED 1, so a ball spreads over the attractor in seconds.
const swarmStepsPerFrame = 4

// densityItersPerFrame is the page's: ten iterates a frame per TRAIL point.
const densityItersPerFrame = 10

// swarmFigure advances the swarm on key and returns it as points, or false
// for a model the swarm cannot carry. Called with s.mu held.
func (c *cloudState) swarmFigure(key string, on, paused bool, run int, speed, frameDt float64) ([][3]float64, bool) {
	if !on {
		c.swarmOn = false
		return nil, false
	}
	dt, f, ok := dynamics.FlowFor(key)
	if !ok || dt <= 0 {
		return nil, false
	}
	if !c.swarmOn {
		c.swarmOn = true
		c.swarm.Reseed()
	}
	ic := dynamics.InitCondFor(key)
	c.swarm.Ensure(key, max(run, 2), dt, f, [3]float64{float64(ic[0]), float64(ic[1]), float64(ic[2])})
	if !paused {
		sub, scale := speedSteps(speed)
		// The page takes its steps a frame at sixty frames a second; a frame
		// here stands for frameDt seconds, so its share of them.
		want := int(math.Round(swarmStepsPerFrame * float64(sub) * frameDt * 60))
		steps := max(min(want, trailBudget/len(c.swarm.P)), 1)
		c.swarm.Step(f, dt*scale, steps, dynamics.UsingEuler())
	}
	return c.swarm.P, true
}

// densityFigure counts this frame's iterates of the map key and returns the
// occupied cells as points colored by their level through g, with the box's
// corners to frame it by, or false for a model that is not a map. Called with
// s.mu held.
func (c *cloudState) densityFigure(key string, on, paused bool, run int, g rasterview.Gradient) (attractor.Figure, [][3]float64, bool) {
	if !on {
		c.densOn = false
		return attractor.Figure{}, nil, false
	}
	m, ok := dynamics.MapFor(key)
	if !ok {
		return attractor.Figure{}, nil, false
	}
	if !c.densOn || c.densKey != key {
		c.densOn, c.densKey = true, key
		lo, hi := dynamics.DensityBox(m, 100000)
		c.dens.Reset(lo, hi)
		c.orbits.Invalidate()
	}
	if !paused {
		c.orbits.Ensure(key, m)
		orbits := m.Count()
		per := max(max(run, 2)*densityItersPerFrame/orbits, 1)
		for o := range orbits {
			p := c.orbits.At(o)
			for range per {
				x, y, z := m.Step(p[0], p[1], p[2])
				if !dynamics.Bounded(x, y, z) {
					p = m.Start(o, orbits)
					continue
				}
				p = [3]float64{x, y, z}
				c.dens.Add(x, y)
			}
			c.orbits.Set(o, p)
		}
	}
	d := &c.dens
	if len(c.levels) != d.W*d.H {
		c.levels = make([]byte, d.W*d.H)
	}
	d.LevelBytes(c.levels)
	// A level's color: the gradient along its own length, as the page's
	// density image takes the trace's colors, dimmed by the level.
	g.Source = 3
	var lut [256][3]float32
	for lv := 1; lv < 256; lv++ {
		t := float32(lv) / 255
		col := g.ColorAt(0, 0, 0, [3]float32{}, [3]float32{1, 1, 1}, t)
		lut[lv] = [3]float32{col[0] * t, col[1] * t, col[2] * t}
	}
	cw := (d.Hi[0] - d.Lo[0]) / float64(d.W)
	ch := (d.Hi[1] - d.Lo[1]) / float64(d.H)
	fig := attractor.Figure{Kind: attractor.FigurePoints}
	for k, lv := range c.levels {
		if lv == 0 {
			continue
		}
		i, j := k%d.W, k/d.W
		fig.Points = append(fig.Points, [3]float64{d.Lo[0] + (float64(i)+0.5)*cw, d.Lo[1] + (float64(j)+0.5)*ch, 0})
		fig.Colors = append(fig.Colors, lut[lv])
	}
	fit := [][3]float64{{d.Lo[0], d.Lo[1], 0}, {d.Hi[0], d.Lo[1], 0}, {d.Lo[0], d.Hi[1], 0}, {d.Hi[0], d.Hi[1], 0}}
	return fig, fit, true
}
