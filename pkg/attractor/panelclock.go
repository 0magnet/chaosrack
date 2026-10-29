package attractor

// The panel's clock: the frames on which readouts that change on their own
// are written.
//
// Every write to the panel costs the frame it lands in a style pass, a layout
// and a repaint of the whole rack, which is one composited layer; it is the
// repaint that is expensive, and two writes in one frame cost barely more
// than one. The readouts that follow a running rack — the rotation knobs, the
// Timing module, the clock, a scrolling model name — each kept a clock of its
// own, so between them they dirtied the panel on seven or eight frames a
// second where four would show the same thing. On this clock they land on the
// same frames.
//
// A control being turned is not on it: its handler writes at once, as it
// always has. This is for what moves while nobody touches anything.

// panelPeriodMs is how often the panel's readouts are written.
const panelPeriodMs = 250

// panelClock decides, once a frame, whether this frame is a tick.
type panelClock struct {
	nextMs float64
	due    bool     // this frame is a tick
	n      int      // ticks so far
	every  []func() // run on each tick, in the order added
}

// advance moves the clock to this frame's rAF timestamp. Ticks keep to a
// grid rather than restarting from whichever frame was late, so a 60 Hz rack
// ticks four times a second and not three and a half; a rack slower than the
// grid (a throttled window) ticks on every frame it has, and a clock that went
// backwards (a test, a restarted timeline) ticks at once and starts again.
func (c *panelClock) advance(nowMs float64) {
	c.due = nowMs >= c.nextMs || c.nextMs-nowMs > panelPeriodMs
	if !c.due {
		return
	}
	c.n++
	c.nextMs += panelPeriodMs
	if c.nextMs <= nowMs || c.nextMs-nowMs > panelPeriodMs {
		c.nextMs = nowMs + panelPeriodMs
	}
	for _, f := range c.every {
		f()
	}
}

// onTick runs f on every tick from now on.
func (c *panelClock) onTick(f func()) {
	c.every = append(c.every, f)
}
