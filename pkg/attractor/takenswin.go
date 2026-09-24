package attractor

import "github.com/0magnet/chaosrack/pkg/takens"

// The beam-smoothing knob, and the delay-embedding arithmetic in pkg/takens
// read at its current value.

// takensSmoothF is the beam-smoothing upsample factor, exactly as the xy scope
// uses it: each sample-to-sample step is drawn as this many Catmull-Rom spline
// steps. A LINE_STRIP straight from one delay vector to the next is a chord,
// and chords are what put the visible straight runs and hard corners in the
// figure — they are an artifact of the drawing, not something in the signal.
// The true signal between two samples is a bandlimited curve, so the spline is
// the more faithful reconstruction, not a prettier lie. It also covers the
// decimated case: when a long window forces stride > 1, the curve passes
// through the kept samples instead of cutting across them.
//
// A KNOB, and it was a constant here while the xy scope — doing the identical
// thing, and named in the sentence above as where it came from — has had it on
// a knob all along. It is worth turning: it is the trade between how much
// WINDOW is on screen and how smooth the beam is, because the two come out of
// one vertex budget. Down at 1 the figure is raw chords through four times as
// many samples, which is what to use to see a long window; up at 16 it is a
// glass-smooth beam through a short one.
//
// Shared with the polar embedding, which draws its beam with the same
// arithmetic — the recurrence plot already shares takens-tau the same way.
var takensSmoothF float32 = 4

// takensSmooth is that knob as a step count, clamped (see takens.Smooth).
func takensSmooth() int { return takens.Smooth(takensSmoothF) }

// takensWindow is takens.Window at the knob's smoothing.
func takensWindow(winMS float32, sampleRate, budget int) (n, stride int) {
	return takens.Window(winMS, sampleRate, budget, takensSmooth())
}

// takensVerts is takens.Verts at the knob's smoothing.
func takensVerts(n int) int { return takens.Verts(n, takensSmooth()) }
