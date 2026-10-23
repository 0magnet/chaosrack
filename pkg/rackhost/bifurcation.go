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
	// env is the loudness DRIVE on audio follows, and cur the cursor's readout.
	env envFollower
	cur string
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

// followClock is when a follower last advanced: what the followers shared
// with the page (attractor's featurefollow.go) advance by is the time since,
// in 60 Hz frames.
type followClock struct{ last time.Time }

// frames is how many 60 Hz frames have gone by since the last call, one for
// the first.
func (c *followClock) frames(now time.Time) float32 {
	k := float32(1)
	if !c.last.IsZero() {
		k = attractor.FollowFrames(now.Sub(c.last).Seconds())
	}
	c.last = now
	return k
}

// envFollower is the rack's signal's loudness as the page's Audio mod
// envelope follows it (attractor's featurefollow.go): RMS, divided by its
// recent peak, smoothed over the time since the last.
type envFollower struct {
	peak, val float32
	clock     followClock
}

func (e *envFollower) follow(raw float32, now time.Time) float32 {
	k := e.clock.frames(now)
	e.val = attractor.FeatureSmooth(e.val, attractor.FeatureNorm(&e.peak, raw, k), k)
	return e.val
}

// envWindow is how much of the rack's signal one loudness reading is taken
// over: the page's analyzer window.
const envWindow = 2048

// bifCursor is DRIVE on audio: where the rack's loudness puts the swept
// parameter, as a gold rule across the diagram with the diagram's own points
// there lit, and the readout that names it; on sweep, none, and "sweep".
// Called with r.mu held.
func (r *Rack) bifCursor() (attractor.Figure, string) {
	b := &r.analyzers.bif
	ps := dynamics.Params(b.flow)
	idx := attractor.SweepParam(ps, int(r.number("bif-sweep")))
	if r.value("bif-drive") != "1" || idx < 0 {
		return attractor.Figure{}, "sweep"
	}
	p := ps[idx]
	l, rt := r.sound.window(envWindow)
	env := b.env.follow((rms(l)+rms(rt))/2, r.sound.now())
	r.sound.mu.Lock() // the sweep turns the constant under it while it fills
	center := *p.Value
	r.sound.mu.Unlock()
	v := attractor.BifAudioValue(env, float32(r.number("bif-depth")), center, p.Min, p.Max)
	f := attractor.Figure{Kind: attractor.FigurePoints}
	b.sweep.Cursor(v, p.Min, p.Max, func(x, y, _ float32) {
		f.Points = append(f.Points, [3]float64{float64(x), float64(y), 0})
	})
	return f, p.Label + " " + strconv.FormatFloat(float64(v), 'f', 2, 32)
}

// rms is the root mean square of x.
func rms(x []float32) float32 {
	if len(x) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range x {
		s += float64(v) * float64(v)
	}
	return float32(math.Sqrt(s / float64(len(x))))
}
