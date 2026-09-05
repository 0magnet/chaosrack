//go:build js && wasm

package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

// THE ONE THAT CANNOT BE SEEN. Unlike the Takens mode, which accumulates its
// own ring, this mode snapshots its window out of the source's — and
// ring.latest indexes modulo the ring length, so a request longer than the ring
// comes back filled with wrapped, already-overwritten samples instead of
// erroring or zeroing. The figure would be built from real audio in the wrong
// order: plausible, live, and completely wrong, with nothing anywhere saying
// so. The window arithmetic is the only thing standing between the WIN knob and
// that, so it is pinned across every rate, budget, duration and τ the app can
// produce.
func TestStereoWindowNeverOutrunsTheSnapshotRing(t *testing.T) {
	for _, sr := range []int{0, 8000, 22050, 24000, 44100, 48000, 96000, 192000} {
		for _, budget := range []int{2000, 20000, 200000} {
			for _, win := range []float32{5, 85, 250, stereoWinMax, 5000} {
				for _, tau := range []int{1, 32, 512} {
					n, stride := stereoWindow(win, sr, budget, tau)
					if n < 2 || stride < 1 {
						t.Errorf("sr=%d budget=%d win=%v tau=%d: n=%d stride=%d",
							sr, budget, win, tau, n, stride)
						continue
					}
					// Exactly what generateStereo asks TimeDomainStereo for.
					if need := (n-1)*stride + tau + 1; need > stereoSpanMax {
						t.Errorf("sr=%d budget=%d win=%v tau=%d: snapshots %d samples from a "+
							"%d-sample ring — the front of the window would be wrapped audio",
							sr, budget, win, tau, need, stereoSpanMax)
					}
					if v := takensVerts(n); v > budget {
						t.Errorf("sr=%d budget=%d win=%v tau=%d: %d vertices overruns the %d-vertex buffer",
							sr, budget, win, tau, v, budget)
					}
				}
			}
		}
	}
}

// The clamp must not be the thing the user is fighting: at the highest rate a
// browser normally reports, and the largest τ, the knob's own maximum has to be
// deliverable in full. If it ever is not, the knob's top end is a lie and
// stereoWinMax is the number to lower — not this test.
func TestStereoWinMaxIsReachableAtEveryNormalRate(t *testing.T) {
	for _, sr := range []int{22050, 24000, 44100, 48000} {
		n, stride := stereoWindow(stereoWinMax, sr, 20000, 512)
		want := float64(stereoWinMax) / 1000 * float64(sr)
		if got := float64(n * stride); got < want*0.85 {
			t.Errorf("sr=%d: the knob's maximum %v ms delivers %.0f samples, want ~%.0f — "+
				"the top of the knob's travel does nothing", sr, float32(stereoWinMax), got, want)
		}
	}
}

// stereoSpanMax exists so the bound is named once. If audiosrc's default ring
// shrinks, this mode's clamp has to shrink with it, and pinning the identity is
// what makes that a compile-time relationship rather than a comment.
func TestStereoSpanMaxTracksTheSourceRing(t *testing.T) {
	if stereoSpanMax != audiosrc.DefaultRingSize {
		t.Errorf("stereoSpanMax is %d but a source retains %d samples",
			stereoSpanMax, audiosrc.DefaultRingSize)
	}
}

// The dial, its position names and its ring labels are three lists that have to
// stay the same length and the same order. paramrings_js_test.go checks the
// names against the ring; this checks both against the PLANS, which is the list
// that actually decides what gets drawn — a name that indexed a plan it does not
// describe would be a detent pointing at the wrong figure.
func TestStereoAxisTablesLineUp(t *testing.T) {
	if len(stereoAxisNames) != len(stereoPlans) {
		t.Errorf("%d position names for %d plans", len(stereoAxisNames), len(stereoPlans))
	}
	if len(stereoAxisRing) != len(stereoPlans) {
		t.Errorf("%d ring labels for %d plans", len(stereoAxisRing), len(stereoPlans))
	}
	if got := paramLabels["stereo-axes"]; len(got) != len(stereoPlans) {
		t.Errorf("paramLabels has %d positions for %d plans — the dial and the drawing disagree",
			len(got), len(stereoPlans))
	}
	// The knob's max is derived from len(stereoPlans); a position past the end
	// of the plans would be a detent that clamps back onto its neighbor.
	for _, p := range attractorParams["stereo"] {
		if p.ID != "stereo-axes" {
			continue
		}
		if int(p.Max) != len(stereoPlans)-1 {
			t.Errorf("the axes knob runs to %v for %d plans", p.Max, len(stereoPlans))
		}
		if int(p.Def) < 0 || int(p.Def) >= len(stereoPlans) {
			t.Errorf("the axes knob defaults to %v, which is not a plan", p.Def)
		}
	}
}

// A THIRD AXIS HAS TO COME FROM SOMEWHERE. Three coordinates that are three
// linear functions of the same two samples all lie in a fixed plane, so the
// "solid" is a sheet and rotating the model shows it edge-on — which is the
// specific mistake (L, R, L−R) makes and the reason it was rejected. Every plan
// must therefore reach outside the instantaneous sample pair, by a delay or by
// the time ramp.
func TestEveryPlanHasAGenuineThirdAxis(t *testing.T) {
	for i, p := range stereoPlans {
		independent := false
		for c := 0; c < 3; c++ {
			if p.delay[c] || p.ch[c] == chTime {
				independent = true
			}
		}
		if !independent {
			t.Errorf("plan %d (%s) is three linear functions of one sample pair; every point "+
				"lies in a plane and the figure is a sheet", i, stereoAxisNames[i])
		}
		// The time ramp is filled from the vertex index, and generateStereo
		// only looks for it — it never mixes it with a delay.
		for c := 0; c < 3; c++ {
			if p.ch[c] == chTime && p.delay[c] {
				t.Errorf("plan %d delays its time axis, which means nothing", i)
			}
		}
		if p.ch[0] == p.ch[1] && !p.delay[0] && !p.delay[1] {
			t.Errorf("plan %d draws the same signal on x and y; the figure is a line by construction", i)
		}
	}
}

// Mid/side is the L/R plane rotated 45°, not a different measurement: M+S must
// give L back and M−S must give R back, exactly. The ½ on each is what makes
// the rotation area-preserving in the sense that matters here — switching basis
// must not resize the figure, or the switch reads as a zoom.
func TestMidSideIsARotationOfLR(t *testing.T) {
	for _, c := range []struct{ l, r float32 }{
		{0, 0}, {1, 1}, {1, -1}, {0.5, 0.25}, {-0.9, 0.3}, {1, 0},
	} {
		m := stereoChanValue(chMid, c.l, c.r)
		s := stereoChanValue(chSide, c.l, c.r)
		if got := m + s; math.Abs(float64(got-c.l)) > 1e-6 {
			t.Errorf("L=%v R=%v: mid+side = %v, want L", c.l, c.r, got)
		}
		if got := m - s; math.Abs(float64(got-c.r)) > 1e-6 {
			t.Errorf("L=%v R=%v: mid−side = %v, want R", c.l, c.r, got)
		}
		// Every coordinate stays inside ±1 for inputs inside ±1, which is what
		// lets takensFitExtent's fixed camera fit apply to this mode unchanged.
		for _, v := range []float32{m, s} {
			if math.Abs(float64(v)) > 1 {
				t.Errorf("L=%v R=%v: a coordinate reached %v; the fixed camera fit assumes ±1",
					c.l, c.r, v)
			}
		}
	}
}

// The mono case, stated as arithmetic rather than as a screenshot. It is
// mathematically correct and it looks broken, which is why the mode reports it
// — and why what it reports has to be true.
func TestMonoCollapsesOntoTheDiagonalAndOntoMid(t *testing.T) {
	const v float32 = 0.7 // the same sample in both channels
	for i, p := range stereoPlans {
		x := stereoChanValue(p.ch[0], v, v)
		y := stereoChanValue(p.ch[1], v, v)
		switch p.ch[0] {
		case chL:
			// L/R basis: x == y, the 45° diagonal.
			if x != y {
				t.Errorf("plan %d: a mono sample gave x=%v y=%v, want the diagonal", i, x, y)
			}
		case chMid:
			// Mid/side basis: everything on mid, side identically zero — so the
			// figure keeps a whole axis to itself instead of streaking across
			// two, which is why the notice points at these positions.
			if x != v {
				t.Errorf("plan %d: mid of a mono sample is %v, want %v (switching basis resized it)", i, x, v)
			}
			if y != 0 {
				t.Errorf("plan %d: side of a mono sample is %v, want 0", i, y)
			}
		default:
			t.Errorf("plan %d starts on %v, which is neither basis", i, p.ch[0])
		}
	}
}

// Audio modulation can drive any registered parameter, and this one indexes a
// table. Anything the modulator produces has to land on a real plan.
func TestStereoAxisSelClampsWhateverModulationDoes(t *testing.T) {
	saved := stereoAxesF
	defer func() { stereoAxesF = saved }()
	for _, v := range []float32{-1000, -1, -0.4, 0, 0.6, 1, 2, 3, 3.4, 99, float32(math.Inf(1))} {
		stereoAxesF = v
		if i := stereoAxisSel(); i < 0 || i >= len(stereoPlans) {
			t.Errorf("axes = %v selected plan %d", v, i)
		}
	}
	// And the detents themselves must round to themselves, not to a neighbor.
	for want := range stereoPlans {
		stereoAxesF = float32(want)
		if got := stereoAxisSel(); got != want {
			t.Errorf("detent %d selected plan %d", want, got)
		}
	}
}

func TestStereoCorrelationReadsTheStereoRelationship(t *testing.T) {
	const n = 2048
	l := make([]float32, n)
	r := make([]float32, n)

	// Identical channels: the mono diagonal.
	for i := range l {
		l[i] = float32(math.Sin(2 * math.Pi * 8 * float64(i) / n))
		r[i] = l[i]
	}
	if c, ok := stereoCorrelation(l, r); !ok || math.Abs(float64(c)-1) > 1e-4 {
		t.Errorf("identical channels: r=%v ok=%v, want +1", c, ok)
	}
	if !stereoIsCollapsed(false, true, 1) {
		t.Error("identical channels are not reported as collapsed")
	}

	// Polarity inversion: the other diagonal, and the thing that disappears
	// when a mix is summed to mono.
	for i := range r {
		r[i] = -l[i]
	}
	if c, ok := stereoCorrelation(l, r); !ok || math.Abs(float64(c)+1) > 1e-4 {
		t.Errorf("inverted channels: r=%v ok=%v, want −1", c, ok)
	}
	// −1 is a real, drawable figure (a line on the other diagonal) but it is
	// not the "no stereo information" case the notice is about.
	if stereoIsCollapsed(false, true, -1) {
		t.Error("a polarity flip was reported as a mono collapse")
	}

	// Quadrature over a whole number of periods: uncorrelated, a round cloud.
	for i := range r {
		r[i] = float32(math.Cos(2 * math.Pi * 8 * float64(i) / n))
	}
	if c, ok := stereoCorrelation(l, r); !ok || math.Abs(float64(c)) > 1e-3 {
		t.Errorf("quadrature channels: r=%v ok=%v, want ~0", c, ok)
	}

	// A SHARED DC OFFSET IS NOT CORRELATION. Two unrelated channels riding the
	// same rail correlate at +1 under Σlr/√(Σl²Σr²); subtracting the means is
	// the whole reason that formula is not used, and a cheap ADC or a window
	// shorter than one cycle of something very low both produce this.
	for i := range l {
		l[i] += 0.5
		r[i] += 0.5
	}
	if c, ok := stereoCorrelation(l, r); !ok || math.Abs(float64(c)) > 1e-3 {
		t.Errorf("quadrature channels on a shared DC offset: r=%v ok=%v, want ~0 — "+
			"the offset was read as correlation", c, ok)
	}

	// Silence, and one dead channel: there is nothing to correlate and saying
	// +1 (or 0, as if measured) would be inventing a measurement.
	for i := range l {
		l[i], r[i] = 0, 0
	}
	if _, ok := stereoCorrelation(l, r); ok {
		t.Error("silence reported a correlation")
	}
	for i := range l {
		l[i] = float32(math.Sin(2 * math.Pi * 8 * float64(i) / n))
	}
	if _, ok := stereoCorrelation(l, r); ok {
		t.Error("a dead right channel reported a correlation")
	}
	if _, ok := stereoCorrelation(l, r[:4]); ok {
		t.Error("mismatched lengths reported a correlation")
	}
}

// The cell is a third of a module wide. The Takens mode's readout is six
// characters ("τ32 m4") and that is the budget; anything longer runs into its
// neighbor, which is how the spectrogram's color dial became unreadable.
func TestStereoReadoutSaysWhatItMeansAndFits(t *testing.T) {
	cases := []struct {
		mono, ok bool
		corr     float32
		want     string
	}{
		{true, false, 0, "mono"},
		{true, true, 1, "mono"}, // a mono source is mono whatever the maths says
		{false, false, 0, "r --"},
		{false, true, 1, "r+1.00"},
		{false, true, -1, "r-1.00"},
		{false, true, 0, "r+0.00"},
		{false, true, 0.4237, "r+0.42"},
		{false, true, -0.871, "r-0.87"},
	}
	for _, c := range cases {
		got := stereoReadout(c.mono, c.ok, c.corr)
		if got != c.want {
			t.Errorf("stereoReadout(%v,%v,%v) = %q, want %q", c.mono, c.ok, c.corr, got, c.want)
		}
		if len([]rune(got)) > 6 {
			t.Errorf("readout %q is %d characters; the cell fits 6", got, len([]rune(got)))
		}
	}
}

// The notice fires on both causes of a straight-line figure and on nothing
// else. Getting this wrong in either direction is bad: silent on a genuinely
// mono source is the whole problem the readout exists for, and firing on a
// wide mix is an app claiming a fault that is not there.
func TestStereoCollapseDetection(t *testing.T) {
	cases := []struct {
		name     string
		mono, ok bool
		corr     float32
		want     bool
	}{
		{"one-channel source", true, false, 0, true},
		{"identical channels", false, true, 1, true},
		{"just inside the threshold", false, true, stereoCollapseR + 1e-5, true},
		{"a very narrow but real image", false, true, 0.99, false},
		{"a wide mix", false, true, 0.2, false},
		{"a polarity flip", false, true, -1, false},
		{"silence", false, false, 0, false},
	}
	for _, c := range cases {
		if got := stereoIsCollapsed(c.mono, c.ok, c.corr); got != c.want {
			t.Errorf("%s: collapsed = %v, want %v", c.name, got, c.want)
		}
	}
}

// The camera is fitted ONCE, to the fixed scale's worst case, exactly as the
// Takens mode's is — which is only correct if no coordinate any plan can
// produce exceeds the gain. Samples are bounded to ±1 by the Source contract;
// mid and side by their ½; the time ramp by its own mapping. Checked here
// rather than assumed, because a plan added later with an unscaled combination
// (L+R with no ½, say) would put peaks off the screen and nothing else would
// notice.
func TestStereoFitBoundHoldsForEveryPlan(t *testing.T) {
	const gain float32 = 10
	corners := []struct{ l, r float32 }{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for i, p := range stereoPlans {
		for _, c := range corners {
			for axis := 0; axis < 3; axis++ {
				var v float32
				if p.ch[axis] == chTime {
					// generateStereo maps the window onto (2w−1)·gain for w in
					// 0..1; the extremes are the ends of the window.
					v = gain
				} else {
					v = stereoChanValue(p.ch[axis], c.l, c.r) * gain
				}
				if math.Abs(float64(v)) > float64(gain)+1e-6 {
					t.Errorf("plan %d axis %d at L=%v R=%v reaches %v, past the gain %v — "+
						"the one-shot camera fit would clip it", i, axis, c.l, c.r, v, gain)
				}
			}
		}
	}
	// And the fit itself covers the corner of that cube, as it does for takens.
	if got, want := takensFitExtent(gain), gain*1.7320508; got < want {
		t.Errorf("fitted to %v but a corner reaches %v", got, want)
	}
}
