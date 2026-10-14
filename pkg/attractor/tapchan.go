package attractor

// The channel a single-signal reader takes from a stereo pair, shared by the
// page's tap and by AudioFigure.

// tapChan names the signal a consumer reads out of the tap. The first three
// are a fold of one (L, R) pair; they are the Stereo Embedding's basis choices
// under the same names, because they are the same four signals.
type tapChan uint8

const (
	tapMix  tapChan = iota // (L+R)/2 — what is playing, and what Drain used to give
	tapLeft                // one channel as it is
	tapRight
	tapMid  // (L+R)/2 by construction; the same as the mix, and named for the pair
	tapSide // (L−R)/2 — what is NOT common to the two, and silence on a mono source
)

// tapFold reduces one (L, R) pair to the signal a channel names. mid carries
// the ½ and side carries it too, so that switching between them does not resize
// the figure and a full-scale input stays inside ±1 — stereoChanValue's
// argument, and the bound every fixed-scale camera fit in this package relies
// on.
func tapFold(c tapChan, l, r float32) float32 {
	switch c {
	case tapLeft:
		return l
	case tapRight:
		return r
	case tapSide:
		return (l - r) * 0.5
	default: // tapMix, tapMid
		return (l + r) * 0.5
	}
}

// tapChanSel clamps a knob value to a channel. Audio modulation can drive any
// registered parameter, so the value arriving is not necessarily a detent, and
// the range is checked BEFORE the conversion because a float-to-int conversion
// whose value does not fit is implementation-defined in Go (stereoAxisSel's
// argument, and its trap).
func tapChanSel(v float32) tapChan {
	if !(v > 0) { // false for NaN too
		return tapMix
	}
	if last := float32(len(tapChanNames) - 1); v > last {
		v = last
	}
	// Clamped to the name table just above, so the value is 0..4 — gosec sees
	// only int → uint8 and cannot see the clamp.
	return tapChan(int(v + 0.5)) //nolint:gosec // clamped to len(tapChanNames)-1 above
}

// tapChanNames are the positions of any knob that selects one.
var tapChanNames = []string{"mix", "left", "right", "mid", "side"}
