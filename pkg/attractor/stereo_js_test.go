//go:build js && wasm

package attractor

import (
	"math"
	"strings"
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
					for _, align := range []int{-stereoAlignMax, -7, 0, 7, stereoAlignMax} {
						n, stride := stereoWindow(win, sr, budget, tau, align)
						if n < 2 || stride < 1 {
							t.Errorf("sr=%d budget=%d win=%v tau=%d align=%d: n=%d stride=%d",
								sr, budget, win, tau, align, n, stride)
							continue
						}
						absA := align
						if absA < 0 {
							absA = -absA
						}
						// Exactly what generateStereo asks TimeDomainStereo for.
						if need := (n-1)*stride + tau + absA + 1; need > stereoSpanMax {
							t.Errorf("sr=%d budget=%d win=%v tau=%d align=%d: snapshots %d samples from a "+
								"%d-sample ring — the front of the window would be wrapped audio",
								sr, budget, win, tau, align, need, stereoSpanMax)
						}
						if v := takensVerts(n); v > budget {
							t.Errorf("sr=%d budget=%d win=%v tau=%d align=%d: %d vertices overruns the %d-vertex buffer",
								sr, budget, win, tau, align, v, budget)
						}
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
		n, stride := stereoWindow(stereoWinMax, sr, 20000, 512, stereoAlignMax)
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
	saved := stereo.axesF
	defer func() { stereo.axesF = saved }()
	for _, v := range []float32{-1000, -1, -0.4, 0, 0.6, 1, 2, 3, 3.4, 99, float32(math.Inf(1))} {
		stereo.axesF = v
		if i := stereo.axisSel(); i < 0 || i >= len(stereoPlans) {
			t.Errorf("axes = %v selected plan %d", v, i)
		}
	}
	// And the detents themselves must round to themselves, not to a neighbor.
	for want := range stereoPlans {
		stereo.axesF = float32(want)
		if got := stereo.axisSel(); got != want {
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

// Every knob whose label is an abbreviation has to say what it stands for
// somewhere a hand can find it, or the panel is a row of four-letter words
// only the source explains. "smth" was the one that prompted this.
func TestAbbreviatedKnobsCarryHelp(t *testing.T) {
	for _, id := range []string{
		"takens-tau", "takens-win", "takens-gain", "takens-smooth", "takens-chan",
		"stereo-axes", "stereo-tau", "stereo-win", "stereo-gain",
		"stereo-align", "stereo-width", "stereo-vg", "stereo-span",
		"polar-map", "polar-drive", "polar-tau", "polar-win", "polar-gain", "polar-chan",
	} {
		h := helpFor(id)
		if h == "" {
			t.Errorf("%s has no help sentence", id)
			continue
		}
		if len(h) < 40 {
			t.Errorf("%s help is too terse to explain anything: %q", id, h)
		}
	}
}

// The dial's names and its ring have to stay the same length and order, and
// only the TIME positions may claim to be goniometers — a delayed copy on
// the third axis is a delay embedding, not a two-channel vector display.
func TestOnlyTimePositionsAreCalledGoniometers(t *testing.T) {
	if len(stereoAxisNames) != len(stereoPlans) || len(stereoAxisRing) != len(stereoPlans) {
		t.Fatalf("tables disagree: %d names, %d ring, %d plans",
			len(stereoAxisNames), len(stereoAxisRing), len(stereoPlans))
	}
	for i, p := range stereoPlans {
		isTime := false
		for _, c := range p.ch {
			if c == chTime {
				isTime = true
			}
		}
		named := strings.Contains(stereoAxisNames[i], "goniometer")
		if isTime != named {
			t.Errorf("position %d (%s): time=%v but named %q",
				i, stereoAxisRing[i], isTime, stereoAxisNames[i])
		}
	}
}

// The whole point of the struct: two embeddings with independent controls.
// While the state was eighteen package variables this test could not be
// written, because there was only ever one of each.
func TestTwoStereoInstancesAreIndependent(t *testing.T) {
	a, b := newStereoInst(), newStereoInst()

	if a.tau != b.tau || a.gain != b.gain || a.width != b.width {
		t.Fatalf("fresh instances differ: %+v vs %+v", a, b)
	}

	a.axesF = 1 // L/R goniometer
	a.tau = 200
	a.gain = 40
	a.vgain = 3
	a.span = 4

	if b.axesF != 0 || b.tau != takensTauDef || b.gain != 10 || b.vgain != 1 || b.span != 1 {
		t.Errorf("turning a's knobs moved b: %+v", b)
	}
	if a.axisSel() == b.axisSel() {
		t.Errorf("both instances selected axis %d", a.axisSel())
	}

	// The per-frame measurement state is per instance too, or two views of
	// the same audio would overwrite each other's readout.
	a.corr, a.corrOK, a.collapsed = 0.9, true, 5
	if b.corr != 0 || b.corrOK || b.collapsed != 0 {
		t.Errorf("a's measurement leaked into b: corr=%v ok=%v collapsed=%d",
			b.corr, b.corrOK, b.collapsed)
	}

	// And the camera-fit memo, which is what would make one view refit
	// because the other was turned.
	a.fitGain = 40
	if b.fitGain != 0 {
		t.Errorf("a's camera fit leaked into b: %v", b.fitGain)
	}
}

// The package-level instance is the one the on-screen mode draws, and it
// has to start at the same defaults a fresh one does — otherwise Reset All
// and a new view would disagree about what default means.
func TestPackageInstanceStartsAtDefaults(t *testing.T) {
	fresh := newStereoInst()
	if stereo.tau != fresh.tau || stereo.win != fresh.win ||
		stereo.gain != fresh.gain || stereo.width != fresh.width ||
		stereo.vgain != fresh.vgain || stereo.span != fresh.span {
		t.Errorf("the drawn instance is not at defaults:\n got %+v\nwant %+v", stereo, fresh)
	}
}

// A trigger has to find the most RECENT crossing. Locking to the oldest in
// the margin is the bug the first version shipped with: the window was a
// whole margin stale and jumped whenever the margin's contents rolled,
// which from the front looks exactly like a trigger that does not work.
func TestTriggerFindsTheMostRecentCrossing(t *testing.T) {
	// Larger offset = newer sample, which is the convention the call site
	// needs: the offset is ADDED to the window base.
	margin := 100
	buf := make([]float32, margin+1)
	for off := range buf {
		buf[off] = float32(math.Sin(float64(off) * 0.3))
	}
	got := triggerOffset(buf, margin, 0, 0, true)
	if got == margin {
		t.Fatal("no crossing found in a signal that crosses repeatedly")
	}
	if !(buf[got-1] < 0 && buf[got] >= 0) {
		t.Errorf("offset %d is not a rising zero crossing: %v -> %v", got, buf[got-1], buf[got])
	}
	for off := margin; off > got; off-- {
		if buf[off-1] < 0 && buf[off] >= 0 {
			t.Errorf("offset %d is more recent than the one returned, %d", off, got)
			break
		}
	}
}

func TestTriggerSlopeAndFreeRun(t *testing.T) {
	margin := 100
	// One rising edge, at offset 50 (older below, newer above).
	buf := make([]float32, margin+1)
	for off := range buf {
		if off < 50 {
			buf[off] = -1
		} else {
			buf[off] = 1
		}
	}
	if got := triggerOffset(buf, margin, 0, 0, true); got != 50 {
		t.Errorf("rising edge found at %d, want 50", got)
	}
	// The same signal has no FALLING edge, so it must free-run.
	if got := triggerOffset(buf, margin, 0, 0, false); got != margin {
		t.Errorf("falling search returned %d, want the free-run margin %d", got, margin)
	}
	flat := make([]float32, margin+1)
	if got := triggerOffset(flat, margin, 0, 0, true); got != margin {
		t.Errorf("silence returned %d, want %d", got, margin)
	}
	if got := triggerOffset(buf, 0, 0, 0, true); got != 0 {
		t.Errorf("zero margin returned %d, want 0", got)
	}
}

func TestTriggerLevel(t *testing.T) {
	margin := 100
	buf := make([]float32, margin+1)
	for off := range buf {
		buf[off] = float32(off) / float32(margin) // 0 .. 1, rising with time
	}
	got := triggerOffset(buf, margin, 0.5, 0, true)
	if got < 45 || got > 55 {
		t.Errorf("crossing of 0.5 found at %d, want about 50", got)
	}
}

// Noise reject: a signal that dithers across the level must not produce a
// trigger per dither, or the figure flickers between phases a sample apart.
func TestTriggerHysteresisRejectsDither(t *testing.T) {
	margin := 60
	buf := make([]float32, margin+1)
	// A clean edge well below, then dither around zero near the newest end.
	for off := range buf {
		switch {
		case off < 20:
			buf[off] = -1
		case off < 30:
			buf[off] = 1
		default:
			if off%2 == 0 {
				buf[off] = 0.001
			} else {
				buf[off] = -0.001
			}
		}
	}
	// With no hysteresis the dither wins, being more recent.
	if got := triggerOffset(buf, margin, 0, 0, true); got <= 30 {
		t.Errorf("without hysteresis got %d, expected it to lock to the dither", got)
	}
	// With it, the dither never arms and the real edge is found.
	if got := triggerOffset(buf, margin, 0, 0.05, true); got != 20 {
		t.Errorf("with hysteresis got %d, want the real edge at 20", got)
	}
}

// Coupling narrows what reaches the trigger. A DC offset must not survive
// LF reject, and hiss must not survive HF reject.
func TestTrigCoupling(t *testing.T) {
	sr := 48000
	n := 2000

	dc := make([]float32, n)
	for i := range dc {
		dc[i] = 0.5
	}
	trigCoupling(dc, trigCplLFRej, sr)
	if dc[n-1] > 0.05 || dc[n-1] < -0.05 {
		t.Errorf("LF reject left a DC offset of %v", dc[n-1])
	}

	hiss := make([]float32, n)
	for i := range hiss {
		if i%2 == 0 {
			hiss[i] = 1
		} else {
			hiss[i] = -1
		}
	}
	trigCoupling(hiss, trigCplHFRej, sr)
	var peak float32
	for _, v := range hiss[n/2:] {
		if v > peak {
			peak = v
		}
	}
	if peak > 0.5 {
		t.Errorf("HF reject left nyquist-rate content at %v", peak)
	}

	// DC coupling changes nothing.
	same := []float32{0.1, -0.2, 0.3}
	trigCoupling(same, trigCplDC, sr)
	if same[0] != 0.1 || same[1] != -0.2 || same[2] != 0.3 {
		t.Errorf("DC coupling altered the signal: %v", same)
	}
}

func TestTrigSelectorsClamp(t *testing.T) {
	s := newStereoInst()
	if s.trigSrc() != trigSrcMid || s.trigCpl() != trigCplDC {
		t.Errorf("defaults are not mid/DC: %d %d", s.trigSrc(), s.trigCpl())
	}
	s.tsrc, s.tcpl = 99, 99
	if s.trigSrc() != trigSrcR || s.trigCpl() != trigCplHFRej {
		t.Errorf("out of range did not clamp to the last position")
	}
	s.tsrc, s.tcpl = -5, -5
	if s.trigSrc() != trigSrcMid || s.trigCpl() != trigCplDC {
		t.Errorf("negative did not clamp to the first position")
	}
}
func TestTrigMarginIsZeroWhenOff(t *testing.T) {
	s := newStereoInst()
	if m := s.trigMargin(2000); m != 0 {
		t.Errorf("margin with the trigger off = %d, want 0", m)
	}
	s.trig = stereoTrigRising
	if m := s.trigMargin(2000); m != 2000 {
		t.Errorf("margin = %d, want the window's own 2000", m)
	}
	if m := s.trigMargin(99999); m != stereoTrigMaxMargin {
		t.Errorf("margin = %d, want the cap %d", m, stereoTrigMaxMargin)
	}
}

func TestTrigModeClamps(t *testing.T) {
	s := newStereoInst()
	for _, v := range []float32{-5, 0, 0.4} {
		s.trig = v
		if s.trigMode() != stereoTrigOff {
			t.Errorf("trig %v = %d, want off", v, s.trigMode())
		}
	}
	s.trig = 99
	if s.trigMode() != stereoTrigFalling {
		t.Errorf("trig 99 = %d, want the last position", s.trigMode())
	}
}

// The dial's names and its ring have to line up, as the axes dial's do.
func TestTrigTablesLineUp(t *testing.T) {
	if len(stereoTrigNames) != len(stereoTrigRing) {
		t.Fatalf("%d names, %d ring labels", len(stereoTrigNames), len(stereoTrigRing))
	}
	if len(stereoTrigNames) != stereoTrigFalling+1 {
		t.Errorf("%d names for %d positions", len(stereoTrigNames), stereoTrigFalling+1)
	}
}
