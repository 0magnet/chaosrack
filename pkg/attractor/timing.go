package attractor

// The rack's own frame meter.
//
// chaosrack has always measured its frame time — frameCount, frameMinMs and
// frameMaxMs have sat in stats_js.go since early on — and the only way to read
// any of it was to start the binary with --debug and fetch /debug/stats. That
// is a meter with no panel, in an instrument whose scope module argues, in its
// own comment, that "a scope you have to put the rack into a particular state
// to read is not an instrument in the rack, it is a skin on the main view".
// The same is true of a frame meter, and more so: the thing it measures is
// changed by every knob on every other panel.
//
// What it shows is chosen to answer the question the rack actually raises,
// which is not "how fast is it" but "where did the frame go". A rack that
// renders in 4 ms and spends 9 ms measuring its own audio is not slow, it is
// mis-apportioned, and no single fps number says so.

// lateFactor is how much longer than the display's own interval a frame has
// to be before it counts as late. A frame is handed out on a vsync tick, so a
// frame that misses one arrives at roughly twice the interval; 1.5 sits
// between "on time" and "missed one" and catches the miss without counting
// jitter.
const lateFactor = 1.5

// frameStats accumulates frame intervals over one readout window.
//
// The display's interval is not assumed. A 60 Hz panel and a 144 Hz one have
// nothing in common but that the SHORTEST frame in any window is a frame that
// made it — nothing renders faster than vsync — so the minimum is the interval
// and everything is judged against it. That way "late" means the same thing on
// both, and neither needs configuring.
type frameStats struct {
	n    int
	sum  float32
	min  float32
	max  float32
	over []float32 // kept to count lateness against a minimum only known at the end
}

func (s *frameStats) reset() {
	s.n, s.sum, s.min, s.max = 0, 0, 0, 0
	s.over = s.over[:0]
}

// add records one frame interval, in milliseconds.
func (s *frameStats) add(ms float32) {
	// A zero or negative interval is a clock that did not advance — two
	// callbacks in one tick, or a timestamp that went backwards. It is not a
	// frame that took no time.
	if ms <= 0 {
		return
	}
	if s.n == 0 || ms < s.min {
		s.min = ms
	}
	if ms > s.max {
		s.max = ms
	}
	s.n++
	s.sum += ms
	s.over = append(s.over, ms)
}

func (s *frameStats) avg() float32 {
	if s.n == 0 {
		return 0
	}
	return s.sum / float32(s.n)
}

// fps is the rate the window actually sustained, which is the reciprocal of
// the MEAN interval and not of the median — one 100 ms frame in a second of
// 16.7 ms ones is a real cost and a median hides it entirely.
func (s *frameStats) fps() float32 {
	if a := s.avg(); a > 0 {
		return 1000 / a
	}
	return 0
}

// latePct is the share of frames that missed the display's interval.
func (s *frameStats) latePct() float32 {
	if s.n == 0 || s.min <= 0 {
		return 0
	}
	limit := s.min * lateFactor
	late := 0
	for _, v := range s.over {
		if v > limit {
			late++
		}
	}
	return 100 * float32(late) / float32(s.n)
}

// sectionBudget is where a frame went, in milliseconds summed over a window.
//
// Three spans rather than a full profile, because three is what can be acted
// on: the model is the thing the rack is for, the meters are the thing that
// can be turned down, and the scope is the one panel that draws whatever the
// model is doing.
//
// WHAT THESE DO AND DO NOT INCLUDE, because a meter that implies the model is
// free would be worse than no meter at all. A WebGL call queues a command and
// returns; a canvas stroke records a path and returns. Neither waits for the
// pixels. So MODEL and SCOPE measure the cost of ISSUING a frame, not of
// rasterizing it, and the GPU and compositor work they cause lands in what is
// left over. Expect MODEL to read a fraction of a millisecond while the model
// is plainly costing more than that — the rest of it is real and is in REST.
//
// METERS is different and is the one to trust absolutely: it is arithmetic on
// audio, on this thread, and its number is its cost.
//
// That asymmetry is the reason the total is shown beside the parts rather than
// the parts being shown as shares of it. Three numbers that do not add up to
// the frame are honest; three percentages that add to a hundred would not be.
type sectionBudget struct {
	Model  float32
	Meters float32
	Scope  float32
	Frames int
}

func (b *sectionBudget) reset() { *b = sectionBudget{} }

// perFrame gives the three spans as milliseconds in an average frame.
func (b sectionBudget) perFrame() (model, meters, scope float32) {
	if b.Frames == 0 {
		return 0, 0, 0
	}
	n := float32(b.Frames)
	return b.Model / n, b.Meters / n, b.Scope / n
}
