package attractor

import "math"

// How an audio feature follows the signal, shared by the page's features
// (audiofeatures_js.go) and a native rack's envelope: a raw measure divided
// by its own recent peak, so it reads the same at any level, then smoothed
// to rise fast and fall slowly.
//
// The coefficients are a 60 Hz frame's, and k is how many of those frames
// the time since the last call stands for (FollowFrames). They were applied
// once a frame of whatever the display ran at, so on a 144 Hz display every
// envelope, a route's average and the model sources ran 2.4 times as fast as
// on a 60 Hz one, and a throttled window slowed them all down.

// followHz is the frame rate the followers' coefficients are written for.
const followHz = 60

// FollowFrames is how many 60 Hz frames dt seconds stand for, held to a
// second's worth: after a stall the followers settle rather than overshoot.
func FollowFrames(dt float64) float32 {
	return float32(min(max(dt*followHz, 0), followHz))
}

// perFrames is how far a follower that goes c of the way a frame goes in k
// frames.
func perFrames(c, k float32) float32 {
	return 1 - float32(math.Pow(float64(1-c), float64(k)))
}

// decayFrames is a per-frame decay d over k frames.
func decayFrames(d, k float32) float32 { return float32(math.Pow(float64(d), float64(k))) }

// FeatureNorm scales x by an adaptive peak, peak: it rises at once to a
// louder x and decays slowly, so x/peak is a level-independent 0..1.
func FeatureNorm(peak *float32, x, k float32) float32 {
	if x > *peak {
		*peak = x
	} else {
		*peak *= decayFrames(0.9995, k)
	}
	if *peak < 1e-9 {
		return 0
	}
	return min(x / *peak, 1)
}

// FeatureSmooth moves cur toward target over k frames: fast on the way up,
// slow on the way down.
func FeatureSmooth(cur, target, k float32) float32 {
	const attack, release = 0.6, 0.12
	if target > cur {
		return cur + (target-cur)*perFrames(attack, k)
	}
	return cur + (target-cur)*perFrames(release, k)
}

// clamp01 holds x to 0..1.
func clamp01(x float32) float32 { return min(max(x, 0), 1) }
