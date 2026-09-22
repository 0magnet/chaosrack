package attractor

import "testing"

func fill(s *frameStats, ms ...float32) {
	for _, v := range ms {
		s.add(v)
	}
}

func TestAPerfectSixtyHertzWindowHasNoLateFrames(t *testing.T) {
	var s frameStats
	for i := 0; i < 60; i++ {
		s.add(16.7)
	}
	if got := s.latePct(); got != 0 {
		t.Fatalf("a steady 60 Hz window reported %.1f%% late, want 0", got)
	}
	if got := s.fps(); got < 59.5 || got > 60.5 {
		t.Fatalf("fps %.2f, want ~60", got)
	}
}

func TestAMissedVsyncCountsAsLate(t *testing.T) {
	var s frameStats
	// Three frames in four make it; the fourth misses one interval.
	for i := 0; i < 15; i++ {
		fill(&s, 16.7, 16.7, 16.7, 33.4)
	}
	if got := s.latePct(); got < 24 || got > 26 {
		t.Fatalf("one frame in four missed, reported %.1f%% late, want 25", got)
	}
}

func TestLatenessIsJudgedAgainstTheDisplayNotAConstant(t *testing.T) {
	// The same shape of stutter on a 144 Hz panel must read the same. If the
	// limit were hardcoded to 16.7 ms, none of these would count as late.
	var s frameStats
	for i := 0; i < 15; i++ {
		fill(&s, 6.94, 6.94, 6.94, 13.9)
	}
	if got := s.latePct(); got < 24 || got > 26 {
		t.Fatalf("144 Hz stutter reported %.1f%% late, want 25 — the limit is not tracking the display", got)
	}
}

func TestJitterInsideAnIntervalIsNotLateness(t *testing.T) {
	var s frameStats
	for i := 0; i < 40; i++ {
		fill(&s, 16.0, 17.4)
	}
	if got := s.latePct(); got != 0 {
		t.Fatalf("frames that all made vsync reported %.1f%% late, want 0", got)
	}
}

func TestOneLongFrameIsNotAveragedAway(t *testing.T) {
	// The reason fps comes off the mean and not the median: a rack that
	// hesitates once a second is not a 60 fps rack.
	var s frameStats
	for i := 0; i < 59; i++ {
		s.add(16.7)
	}
	s.add(200)
	if got := s.fps(); got > 52 {
		t.Fatalf("a 200 ms hitch in a second still read %.1f fps — the hitch is being hidden", got)
	}
	if s.max != 200 {
		t.Fatalf("max frame %v, want the hitch", s.max)
	}
}

func TestANonAdvancingClockIsNotAFrame(t *testing.T) {
	var s frameStats
	fill(&s, 0, -4, 16.7)
	if s.n != 1 {
		t.Fatalf("recorded %d frames, want 1 — a clock that did not move is not a frame that took no time", s.n)
	}
	if s.min != 16.7 {
		t.Fatalf("min %v, want 16.7: a zero interval would make every frame look late against it", s.min)
	}
}

func TestAnEmptyWindowReadsZeroRatherThanDividingByIt(t *testing.T) {
	var s frameStats
	if s.avg() != 0 || s.fps() != 0 || s.latePct() != 0 {
		t.Fatalf("empty window gave avg=%v fps=%v late=%v", s.avg(), s.fps(), s.latePct())
	}
	var b sectionBudget
	if m, e, sc := b.perFrame(); m != 0 || e != 0 || sc != 0 {
		t.Fatalf("empty budget gave %v %v %v", m, e, sc)
	}
}

func TestResetClearsTheWindow(t *testing.T) {
	var s frameStats
	fill(&s, 16.7, 100)
	s.reset()
	fill(&s, 8)
	if s.n != 1 || s.max != 8 || s.min != 8 {
		t.Fatalf("after reset n=%d min=%v max=%v, want a window holding only the new frame", s.n, s.min, s.max)
	}
}

func TestTheBudgetIsPerFrameNotPerWindow(t *testing.T) {
	b := sectionBudget{Model: 120, Meters: 60, Scope: 30, Frames: 60}
	m, e, sc := b.perFrame()
	if m != 2 || e != 1 || sc != 0.5 {
		t.Fatalf("per-frame budget %v/%v/%v, want 2/1/0.5", m, e, sc)
	}
}

func TestResettingTheBudgetClearsEverySpan(t *testing.T) {
	// The window has to start empty or the spans accumulate across latches
	// and the readout climbs forever instead of describing the last half
	// second.
	b := sectionBudget{Model: 9, Meters: 9, Scope: 9, Frames: 9}
	b.reset()
	if b != (sectionBudget{}) {
		t.Fatalf("budget after reset is %+v, want zero", b)
	}
}
