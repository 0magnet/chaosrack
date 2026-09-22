package attractor

// frameScale converts a per-frame increment into a per-unit-time one.
//
// The X/Y/Z rate sliders and the auto-rotate switch advance the pose by a
// fixed amount EVERY FRAME. That is only a rate if every frame lasts the
// same length of time, and frames do not: the rack's own per-frame work
// varies (the meters analyze on their own clocks, so one frame in thirty
// carries a whole wow-and-flutter pass), the browser drops frames under
// load, and a 144 Hz display hands out 2.4x as many frames as a 60 Hz one.
// A fixed step per frame turns every one of those into a change of SPEED —
// the model visibly hesitates on a late frame and spins faster on a faster
// monitor.
//
// So the step is scaled by how long the last frame actually took, measured
// against the 60 Hz frame the rates were dialed in against. At 60 Hz the
// scale is 1 and nothing changes; a frame that took twice as long advances
// twice as far, which is what holds the angular velocity constant.
//
// There was an attempt at this already — the step read
// `rotation/20 + tdiff*0.00000001` — but at a 16.7 ms frame that second
// term contributes 1.7e-7 radians against the 5e-3 of the first. Five
// orders of magnitude out is the same as absent, and it had the wrong
// shape besides: a time term ADDED to the step makes a late frame rotate
// slightly further, where what is needed is the whole step MULTIPLIED.
func frameScale(tdiffMs float32) float32 {
	const nominalMs = 1000.0 / 60.0
	// The first frame after a start, a resume, or a tab coming back from
	// the background reports everything since the last one — seconds, not
	// milliseconds. Honoring that would teleport the model a half turn.
	// Past maxCatchUp the animation has already visibly stopped, so losing
	// angle is the better failure: the model resumes from where it was
	// rather than from somewhere else.
	const maxCatchUp = 6.0 // 100 ms; below ~10 fps there is nothing to keep smooth
	if tdiffMs <= 0 {
		return 1
	}
	s := tdiffMs / nominalMs
	if s > maxCatchUp {
		return maxCatchUp
	}
	return s
}
