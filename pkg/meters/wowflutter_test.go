package meters

import (
	"math"
	"testing"
)

const wfSR = 48000

// wfTone builds a carrier frequency-modulated by one sine: `devPct` percent
// peak deviation at `modHz`. That is exactly what a wobbling turntable does to
// a test tone, so the measurement has a known answer by construction.
//
// The phase is the INTEGRAL of the instantaneous frequency, which is where an
// FM generator is usually got wrong: writing sin(2π(f+Δsin(2πf_m t))t) modulates
// the frequency-times-time rather than the frequency, and produces a sweep
// rather than a wobble.
func wfTone(secs float64, carrier, devPct, modHz float64) []float32 {
	n := int(secs * wfSR)
	out := make([]float32, n)
	dev := carrier * devPct / 100
	phase := 0.0
	for i := range out {
		t := float64(i) / wfSR
		f := carrier
		if modHz > 0 {
			f += dev * math.Sin(2*math.Pi*modHz*t)
		}
		phase += 2 * math.Pi * f / wfSR
		out[i] = float32(math.Sin(phase))
	}
	return out
}

// A STEADY TONE HAS NO WOW AND NO FLUTTER. Whatever this reports on one is the
// instrument's own floor, and every figure it gives for a real deck is that
// plus the deck.
func TestASteadyToneHasNoWowOrFlutter(t *testing.T) {
	r := AnalyzeWowFlutter(wfTone(8, WfCarrier, 0, 0), wfSR, WfCarrier)
	if !r.OK {
		t.Fatal("no measurement")
	}
	t.Logf("floor: speed %+.4f%%, wow %.4f%%, flutter %.4f%%, weighted %.4f%%",
		r.SpeedPct, r.WowPct, r.FlutterPct, r.WeightedPct)
	if r.WowPct > 0.01 || r.FlutterPct > 0.01 || r.WeightedPct > 0.01 {
		t.Errorf("a steady tone measured wow %.4f%%, flutter %.4f%%, weighted %.4f%%",
			r.WowPct, r.FlutterPct, r.WeightedPct)
	}
	if math.Abs(r.SpeedPct) > 0.01 {
		t.Errorf("a tone at exactly the nominal frequency measured a speed error of %+.4f%%", r.SpeedPct)
	}
}

// SPEED ERROR IS A DIFFERENT FAULT FROM WOW, and the measurement has to tell
// them apart: a constant error is a pitch shift and a varying one is a warble.
// A turntable running fast is a carrier that is high and perfectly steady.
func TestSpeedErrorIsMeasuredAndIsNotWow(t *testing.T) {
	for _, pct := range []float64{-2, -0.5, 0.33, 1, 2} {
		f := WfCarrier * (1 + pct/100)
		r := AnalyzeWowFlutter(wfTone(8, f, 0, 0), wfSR, WfCarrier)
		if !r.OK {
			t.Fatalf("%.2f%%: no measurement", pct)
		}
		if math.Abs(r.SpeedPct-pct) > 0.02 {
			t.Errorf("a deck running %+.2f%% fast measured %+.4f%%", pct, r.SpeedPct)
		}
		if r.WowPct > 0.02 || r.FlutterPct > 0.02 {
			t.Errorf("%+.2f%% speed error leaked into wow %.4f%% / flutter %.4f%%",
				pct, r.WowPct, r.FlutterPct)
		}
	}
}

// THE HEADLINE TEST. A known deviation at a known rate has to come back as that
// deviation. The reading is an RMS and the signal is a sine, so the RMS of a
// peak deviation D is D/√2 — the measurement is arithmetic, not a judgement.
func TestAKnownWobbleIsMeasured(t *testing.T) {
	for _, c := range []struct{ devPct, modHz float64 }{
		{0.5, 2}, {0.2, 3}, {1.0, 4}, {0.1, 1},
	} {
		r := AnalyzeWowFlutter(wfTone(12, WfCarrier, c.devPct, c.modHz), wfSR, WfCarrier)
		if !r.OK {
			t.Fatalf("%.2f%% at %.0f Hz: no measurement", c.devPct, c.modHz)
		}
		want := c.devPct / math.Sqrt2
		if rel := math.Abs(r.WowPct-want) / want; rel > 0.15 {
			t.Errorf("%.2f%% peak at %.0f Hz should read %.4f%% RMS; measured %.4f%%",
				c.devPct, c.modHz, want, r.WowPct)
		}
	}
}

// ...and the same deviation up in the flutter band lands in flutter instead,
// which is what makes the two separate readings rather than one number twice.
func TestFastModulationIsFlutterAndNotWow(t *testing.T) {
	r := AnalyzeWowFlutter(wfTone(12, WfCarrier, 0.4, 30), wfSR, WfCarrier)
	if !r.OK {
		t.Fatal("no measurement")
	}
	want := 0.4 / math.Sqrt2
	if rel := math.Abs(r.FlutterPct-want) / want; rel > 0.2 {
		t.Errorf("0.4%% at 30 Hz should read %.4f%% flutter; measured %.4f%%", want, r.FlutterPct)
	}
	if r.FlutterPct < r.WowPct*3 {
		t.Errorf("30 Hz modulation read wow %.4f%% against flutter %.4f%%; it belongs in flutter",
			r.WowPct, r.FlutterPct)
	}
}

// ...and the reverse, so neither band is simply picking up everything.
func TestSlowModulationIsWowAndNotFlutter(t *testing.T) {
	r := AnalyzeWowFlutter(wfTone(16, WfCarrier, 0.4, 1.5), wfSR, WfCarrier)
	if !r.OK {
		t.Fatal("no measurement")
	}
	if r.WowPct < r.FlutterPct*3 {
		t.Errorf("1.5 Hz modulation read wow %.4f%% against flutter %.4f%%; it belongs in wow",
			r.WowPct, r.FlutterPct)
	}
}

// THE WEIGHTING IS A PSYCHOACOUSTIC CURVE peaked at 4 Hz, where the ear is most
// sensitive to pitch movement. The same physical deviation has to read HIGHER
// there than well above or below it, or the weighted figure is not weighted and
// two decks' quoted numbers are not comparable.
func TestTheWeightingPeaksAtFourHertz(t *testing.T) {
	const dev = 0.5
	at := func(hz float64) float64 {
		return AnalyzeWowFlutter(wfTone(16, WfCarrier, dev, hz), wfSR, WfCarrier).WeightedPct
	}
	four := at(4)
	for _, hz := range []float64{0.6, 40} {
		if other := at(hz); four < other*1.5 {
			t.Errorf("the same %.1f%% deviation reads %.4f%% at 4 Hz and %.4f%% at %.1f Hz; "+
				"the weighting is meant to peak at 4", dev, four, other, hz)
		}
	}
}

// The carrier is found from the signal rather than assumed, because a deck
// running fast puts the tone somewhere else — and mixing against the nominal
// would leave a beat the demodulator would report as enormous flutter.
func TestTheCarrierIsFoundWhereverItIs(t *testing.T) {
	for _, pct := range []float64{-5, 0, 5} {
		f := WfCarrier * (1 + pct/100)
		r := AnalyzeWowFlutter(wfTone(8, f, 0, 0), wfSR, WfCarrier)
		if !r.OK {
			t.Fatalf("%+.0f%%: no measurement", pct)
		}
		if rel := math.Abs(r.Carrier-f) / f; rel > 1e-3 {
			t.Errorf("a carrier at %.2f Hz was found at %.2f", f, r.Carrier)
		}
		if r.FlutterPct > 0.05 {
			t.Errorf("a steady carrier %+.0f%% off nominal reported %.4f%% flutter — the "+
				"demodulator is beating against the wrong frequency", pct, r.FlutterPct)
		}
	}
}

// A nominal of zero means "the tone's true frequency is not known": wow and
// flutter are still measurable, a speed error is not, and reporting one anyway
// would be inventing a reference.
func TestWithoutANominalThereIsNoSpeedError(t *testing.T) {
	r := AnalyzeWowFlutter(wfTone(12, 1000, 0.5, 3), wfSR, 0)
	if !r.OK {
		t.Fatal("no measurement")
	}
	if r.SpeedPct != 0 {
		t.Errorf("with no nominal the speed error came back as %+.4f%%", r.SpeedPct)
	}
	want := 0.5 / math.Sqrt2
	if rel := math.Abs(r.WowPct-want) / want; rel > 0.2 {
		t.Errorf("wow on a 1 kHz carrier measured %.4f%%, want %.4f%%", r.WowPct, want)
	}
}

// Too little audio cannot hold a cycle of the slowest wow, and saying so beats
// reporting a number made of a fraction of one.
func TestTooShortIsRefusedForWowAndFlutter(t *testing.T) {
	if r := AnalyzeWowFlutter(wfTone(0.2, WfCarrier, 0, 0), wfSR, WfCarrier); r.OK {
		t.Errorf("a fifth of a second produced wow %.4f%%", r.WowPct)
	}
	if r := AnalyzeWowFlutter(nil, wfSR, WfCarrier); r.OK {
		t.Error("an empty buffer produced a measurement")
	}
	if r := AnalyzeWowFlutter(wfTone(4, WfCarrier, 0, 0), 0, WfCarrier); r.OK {
		t.Error("a zero sample rate produced a measurement")
	}
}

// Silence has no carrier to measure, and the search must not lock onto the
// noise floor and report the wobble of nothing.
func TestSilenceHasNoCarrier(t *testing.T) {
	if r := AnalyzeWowFlutter(make([]float32, wfSR*4), wfSR, WfCarrier); r.OK && r.WowPct > 0.1 {
		t.Errorf("silence measured %.4f%% wow at a carrier of %.1f Hz", r.WowPct, r.Carrier)
	}
}
