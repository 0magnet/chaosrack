package rackhost

import (
	"math"
	"strconv"
	"time"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The bifurcation explorer, natively: the page's sweep (attractor.BifSweep)
// of a parameter of the last flow shown, grown a few columns a frame.

// bifView is the native explorer: its sweep, the flow it is of, and when it
// last grew.
type bifView struct {
	sweep attractor.BifSweep
	flow  string
	last  time.Time
	frac  float64
}

// bifFigure grows the diagram on to now, while running, and is its points
// and the plane it is framed to. Called with r.mu held.
func (r *Rack) bifFigure(running bool) (attractor.Figure, [][3]float64) {
	b := &r.analyzers.bif
	flow := r.analyzers.lastFlow
	ps := dynamics.Params(flow)
	if flow != b.flow {
		b.flow = flow
		r.sweepOptions(ps)
	}
	idx := attractor.SweepParam(ps, int(r.number("bif-sweep")))
	now := r.sound.now()
	cols := 0
	if running {
		// The page grows three columns a frame at 60 a second.
		frames := 1.0
		if !b.last.IsZero() {
			frames = math.Min(now.Sub(b.last).Seconds()*60, 20) + b.frac
		}
		n := int(frames)
		b.frac = frames - float64(n)
		cols = 3 * n
	}
	b.last = now
	if idx >= 0 && cols > 0 {
		p := ps[idx]
		// The sweep turns the constant while it integrates, and Model Out
		// reads it on the speaker's goroutine.
		r.sound.mu.Lock()
		b.sweep.Fill(flow, idx, p.Value, p.Min, p.Max, cols)
		r.sound.mu.Unlock()
	}
	var f attractor.Figure
	f.Kind = attractor.FigurePoints
	f.Points = make([][3]float64, 0, 1024)
	b.sweep.Points(math.MaxInt32, func(_ int, x, y, _ float32) {
		f.Points = append(f.Points, [3]float64{float64(x), float64(y), 0})
	})
	return f, [][3]float64{{-20, -12, 0}, {20, 12, 0}}
}

// sweepOptions makes SWEEP's positions the flow's parameters, as the page
// refills its options when the flow changes. Called with r.mu held.
func (r *Rack) sweepOptions(ps []dynamics.Param) {
	i, ok := r.byID["bif-sweep"]
	if !ok || len(ps) == 0 {
		return
	}
	c := &r.ctls[i]
	c.Options, c.Labels = nil, nil
	for k, p := range ps {
		c.Options = append(c.Options, strconv.Itoa(k))
		c.Labels = append(c.Labels, p.Label)
	}
	if v, err := strconv.Atoi(c.Value); err != nil || v < 0 || v >= len(ps) {
		c.Value = strconv.Itoa(attractor.SweepParam(ps, -1))
	}
	r.turn(c)
}
