package attractor

import (
	"math"
	"testing"
)

// allWindows is every window the FFT can apply, with what each is for.
var allWindows = []struct {
	wk   winKind
	name string
}{
	{winHann, "Hann"},
	{winHamming, "Hamming"},
	{winBartlett, "Bartlett"},
	{winRectangular, "rectangular"},
	{winBlackman, "Blackman"},
	{winBlackmanHarris, "Blackman-Harris"},
	{winNuttall, "Nuttall"},
	{winFlatTop, "flat-top"},
}

// sidelobeFloor measures the worst leakage a window leaves more than `skirt`
// bins away from a tone, in dB below the peak. It is the number that decides
// how quiet a thing can be measured beside a loud one, which is the whole of
// what a distortion floor is.
func sidelobeFloor(wk winKind, n, skirt int) float64 {
	// Deliberately off a bin center by a third, which is the worst case for
	// leakage and the case a real signal is almost always in.
	x := make([]float32, n)
	for i := range x {
		x[i] = float32(math.Sin(2 * math.Pi * (float64(n)/8 + 0.333) * float64(i) / float64(n)))
	}
	mags := computeFFTMagsKind(x, wk)
	peak, peakAt := 0.0, 0
	for i, m := range mags {
		if m > peak {
			peak, peakAt = m, i
		}
	}
	worst := 0.0
	for i, m := range mags {
		if i < peakAt-skirt || i > peakAt+skirt {
			if m > worst {
				worst = m
			}
		}
	}
	if worst <= 0 || peak <= 0 {
		return -200
	}
	return 20 * math.Log10(worst/peak)
}

// THE REASON THE NEW WINDOWS EXIST. A distortion floor is set by how far down
// the window's skirts are at the distance the notch ends, and Hann's are not
// far enough: the measured floor through one was 0.013% THD+N, all of it
// window. The low-sidelobe windows have to be dramatically better or there was
// no point adding them.
func TestLowSidelobeWindowsAreActuallyLow(t *testing.T) {
	const n, skirt = 8192, 8
	hann := sidelobeFloor(winHann, n, skirt)
	t.Logf("Hann leaks to %.1f dB past %d bins", hann, skirt)
	// The improvement each is required to buy is stated per window rather than
	// as one threshold, because they are not the same kind of window. Blackman
	// is a middling one — its sidelobes start lower than Hann's but fall away at
	// the same rate, so eight bins out it is only a few dB ahead, which a first
	// version of this test asserted away as a failure when it is simply the
	// truth about Blackman. The 4-term windows are the ones that change what can
	// be measured, and they are the ones the analyzers use.
	for _, c := range []struct {
		wk         winKind
		name       string
		want       float64 // dB, at worst
		betterThan float64 // dB it must beat Hann by
	}{
		{winBlackman, "Blackman", -60, 5},
		{winBlackmanHarris, "Blackman-Harris", -85, 20},
		{winNuttall, "Nuttall", -85, 20},
	} {
		got := sidelobeFloor(c.wk, n, skirt)
		t.Logf("%s leaks to %.1f dB past %d bins", c.name, got, skirt)
		if got > c.want {
			t.Errorf("%s leaks to %.1f dB past %d bins, want below %.0f", c.name, got, skirt, c.want)
		}
		if got > hann-c.betterThan {
			t.Errorf("%s (%.1f dB) beats Hann (%.1f dB) by under %.0f dB; there is no reason for it",
				c.name, got, hann, c.betterThan)
		}
	}
}

// FLAT-TOP IS THE AMPLITUDE WINDOW, and that is the only reason to accept its
// very wide main lobe: a tone anywhere between two bins has to read its true
// amplitude. Hann can be 1.4 dB low at the worst alignment; flat-top should be
// under a tenth of that.
func TestFlatTopReadsAmplitudeWhereverTheToneFalls(t *testing.T) {
	const n = 8192
	amp := func(wk winKind, offset float64) float64 {
		x := make([]float32, n)
		for i := range x {
			x[i] = float32(math.Sin(2 * math.Pi * (100 + offset) * float64(i) / float64(n)))
		}
		mags := computeFFTMagsKind(x, wk)
		peak := 0.0
		for _, m := range mags {
			if m > peak {
				peak = m
			}
		}
		m := windowMetrics(n, wk)
		return 2 * peak / (float64(n) * m.coherentGain)
	}
	spread := func(wk winKind) float64 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, off := range []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9} {
			v := amp(wk, off)
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		return 20 * math.Log10(hi/lo)
	}
	flat, hann := spread(winFlatTop), spread(winHann)
	if flat > 0.15 {
		t.Errorf("flat-top's peak amplitude varies by %.3f dB across a bin; it exists not to", flat)
	}
	if flat > hann {
		t.Errorf("flat-top varies by %.2f dB and Hann by %.2f dB — the wide lobe bought nothing", flat, hann)
	}
}

// Every window's metrics have to be self-consistent, because every amplitude
// and noise-density readout is scaled by them. ENBW is n·Σw²/(Σw)², which for a
// rectangular window is exactly 1 bin by definition — that is the anchor the
// others are read against.
func TestWindowMetricsAreConsistent(t *testing.T) {
	const n = 4096
	for _, w := range allWindows {
		m := windowMetrics(n, w.wk)
		if m.energy <= 0 {
			t.Errorf("%s: energy %v", w.name, m.energy)
		}
		if m.coherentGain <= 0 {
			t.Errorf("%s: coherent gain %v — a window that sums to nothing cannot scale a tone",
				w.name, m.coherentGain)
		}
		if m.enbw < 0.99 {
			t.Errorf("%s: ENBW %.4f bins, and no window collects less noise than a rectangular one",
				w.name, m.enbw)
		}
	}
	if m := windowMetrics(n, winRectangular); math.Abs(m.enbw-1) > 1e-9 {
		t.Errorf("a rectangular window's ENBW is %.6f bins; it is 1 by definition", m.enbw)
	}
	if m := windowMetrics(n, winRectangular); math.Abs(m.coherentGain-1) > 1e-9 {
		t.Errorf("a rectangular window's coherent gain is %.6f; it is 1 by definition", m.coherentGain)
	}
	// Hann's published figures, which is what makes these the right formulas
	// rather than merely consistent ones.
	m := windowMetrics(n, winHann)
	if math.Abs(m.coherentGain-0.5) > 1e-3 {
		t.Errorf("Hann's coherent gain is %.4f, and it is 0.5", m.coherentGain)
	}
	if math.Abs(m.enbw-1.5) > 1e-3 {
		t.Errorf("Hann's ENBW is %.4f bins, and it is 1.5", m.enbw)
	}
	// ...and Blackman-Harris's, the window the distortion analyzer runs on.
	if m := windowMetrics(n, winBlackmanHarris); math.Abs(m.enbw-2.0044) > 5e-3 {
		t.Errorf("Blackman-Harris's ENBW is %.4f bins, and it is 2.0044", m.enbw)
	}
}

// The spectrogram's four have to map to the same four coefficients they always
// did: the local enum was added for the measurement paths and must not have
// moved anything under the display.
func TestSpectrogramWindowsAreUnchanged(t *testing.T) {
	const n = 1024
	den := float64(n - 1)
	want := map[winKind]func(i int) float64{
		winHann:        func(i int) float64 { return 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/den)) },
		winHamming:     func(i int) float64 { return 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/den) },
		winBartlett:    func(i int) float64 { return 1 - math.Abs((float64(i)-den/2)/(den/2)) },
		winRectangular: func(i int) float64 { return 1 },
	}
	for wk, f := range want {
		s := fftScratchFor(n, wk)
		for i := 0; i < n; i++ {
			if math.Abs(s.win[i]-f(i)) > 1e-12 {
				t.Fatalf("window %d coefficient %d is %.15f, was %.15f", wk, i, s.win[i], f(i))
			}
		}
	}
}
