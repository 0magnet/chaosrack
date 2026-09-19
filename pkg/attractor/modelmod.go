package attractor

// The model's own output as a modulation source — closing the loop.
//
// Everything that could drive a parameter until now came from OUTSIDE the
// model: an audio feature, a cell's position in the grid, a knob. This makes
// the model a source too, so the attractor can modulate its own constants
// and the picture becomes a function of where it has already been.
//
// That is a feedback loop, and it is meant to be. A Lorenz whose rho follows
// its own z is not a Lorenz any more: it is a different system, and one
// nobody has an analytic solution for. It can settle, it can oscillate, and
// at enough depth it will run away — which is not a defect to be clamped out
// but the thing you are looking at. What IS a defect is a runaway that
// happens instantly and off-screen, so the source is smoothed and the value
// is bounded to the parameter's own range by the same clamp every other
// modulation goes through. The depth knob is the one that decides how hard
// the loop is driven, and small is where it is interesting.

// The model sources, by the name stored in a route. Short because they go in
// permalinks beside the audio channel names.
const (
	modSrcModelX = "mx" // the model's current x
	modSrcModelY = "my"
	modSrcModelZ = "mz"
	modSrcModelR = "mr" // its distance from the origin
)

// isModelModSource reports whether a route's source is the model rather than
// the audio. The band EQ means nothing for these — a coordinate has no
// spectrum — so the caller skips it.
func isModelModSource(name string) bool {
	switch name {
	case modSrcModelX, modSrcModelY, modSrcModelZ, modSrcModelR:
		return true
	}
	return false
}

// modelModNorm maps a model coordinate onto the 0..1 that every modulation
// source speaks, using the extent the camera was fitted to as full scale.
//
// The extent and not a fixed number, because the models are not on a common
// scale: a Lorenz runs to about 50, a polyhedron to 1, and a source
// normalized against the wrong one would be either permanently pinned or
// permanently asleep. The camera's fit is the one number that already means
// "as big as this model gets".
//
// Clamped, unlike the raw coordinate: a source is a 0..1 signal by contract
// and the depth knob scales it. An excursion past the fitted extent — which
// happens, the fit is to a warmup — must not become a modulation spike
// larger than the parameter's whole range.
func modelModNorm(v, extent float32) float32 {
	if !(extent > 0) {
		return 0.5 // no fit yet: the middle, which modulates nothing off-center
	}
	t := (v/extent + 1) * 0.5
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// modelModCoef is how fast a model source follows the model, per frame.
//
// Slow on purpose. The audio features are smoothed because the spectrum is
// noisy; this is smoothed because it is a FEEDBACK PATH, and a loop that
// responds within one frame of its own output is an oscillator at the frame
// rate — which is not a thing you can see, only a thing that makes the
// picture flicker. A coefficient this low makes the loop's time constant
// long compared with the integration step, so what you see is the system
// drifting under its own influence rather than buzzing.
const modelModCoef = 0.06

// modelModSmooth advances a one-pole filter one frame toward target.
func modelModSmooth(prev, target float32) float32 {
	return prev + (target-prev)*modelModCoef
}
