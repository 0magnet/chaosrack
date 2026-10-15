package attractor

// How an audio feature follows the signal, shared by the page's features
// (audiofeatures_js.go) and a native rack's envelope: a raw measure divided
// by its own recent peak, so it reads the same at any level, then smoothed
// to rise fast and fall slowly. Both are applied once a frame of the page's.

// FeatureNorm scales x by an adaptive peak, peak: it rises at once to a
// louder x and decays slowly, so x/peak is a level-independent 0..1.
func FeatureNorm(peak *float32, x float32) float32 {
	if x > *peak {
		*peak = x
	} else {
		*peak *= 0.9995
	}
	if *peak < 1e-9 {
		return 0
	}
	return min(x / *peak, 1)
}

// FeatureSmooth moves cur toward target: fast on the way up, slow on the way
// down.
func FeatureSmooth(cur, target float32) float32 {
	const attack, release = 0.6, 0.12
	if target > cur {
		return cur + (target-cur)*attack
	}
	return cur + (target-cur)*release
}
