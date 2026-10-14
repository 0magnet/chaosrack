package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/takens"
)

// The Takens mode's measurement of τ, shared by the page (takens_js.go, where
// the section comment says why it is one-shot and never per frame) and a
// native rack: when it is due, the estimate, and the readout's text.

// takensEstMax caps the measurement window. The estimators are O(n²) in the
// FNN search; 4096 samples (~170 ms at 24 kHz) is a run of a few milliseconds
// and already far more data than the histograms need.
const takensEstMax = 4096

// TakensEstMax is how many of the newest samples a measurement reads.
const TakensEstMax = takensEstMax

// TauMeter measures τ once, the first time there is enough signal, and
// remembers what it wrote so it never measures over a τ somebody chose.
type TauMeter struct {
	meas     takens.EmbeddingResult // last measurement, for the readout
	measured bool                   // one has been taken since the last arm

	// autoDone is the one-shot guard. Set the first time the automatic
	// measurement runs, and cleared only by armAutoMeasure — which mode
	// entry and a change of audio source call, and nothing else does.
	autoDone bool

	// autoSet is the τ the automatic measurement last wrote, and it is
	// what lets the measurement tell ITS OWN value from one somebody chose.
	//
	// Without it the automatic measurement is a control that takes the knob
	// away: set τ by hand, switch to Lorenz to compare, come back, and the
	// measurement has thrown the setting away and replaced it. A permalink is
	// the same problem one step worse — a link that carries a τ carries it
	// because whoever made the link meant that τ, and measuring over it
	// discards the one thing the link was for.
	//
	// So the measurement only runs while τ is a value NOBODY CHOSE: the
	// default, or whatever a previous automatic measurement put there. The
	// permalink case falls out of this rather than needing its own rule, since
	// a permalink records a parameter only when it differs from the default.
	autoSet float32

	// chanMeasured is the SRC the standing measurement was taken from, so
	// that turning that knob re-arms the one-shot. It is deliberately NOT a
	// pointer at the knob: the knob is a float a modulator can wobble, and what
	// matters is the detent it lands on.
	chanMeasured tapChan
}

// armAutoMeasure re-arms the one-shot: the next time there is a full window
// it measures again.
//
// A change of source is the case that matters. τ is a property of what is
// playing, and swapping the microphone for a server feed or for the signal
// generator swaps what is playing entirely — the measured τ from the old source
// describes nothing about the new one. Mode entry re-arms for the same reason
// one step removed: the source may well have changed while the mode was away.
func (m *TauMeter) armAutoMeasure() { m.autoDone = false }

// Arm is armAutoMeasure, for a native rack's mode entry.
func (m *TauMeter) Arm() { m.armAutoMeasure() }

// rearmOnChan re-arms when the SRC knob has moved to another channel. A
// different SRC is a different signal, so the τ measured from the last one
// describes nothing about this one — the same argument the source swap makes,
// one level down. Re-arming rather than measuring, so the one-shot still has
// to see a full window before it fires.
func (m *TauMeter) rearmOnChan(chanKnob float32) {
	if ch := tapChanSel(chanKnob); ch != m.chanMeasured {
		m.chanMeasured = ch
		m.armAutoMeasure()
	}
}

// due reports whether the one-shot should measure now, at τ knob tau with
// avail samples to measure from.
//
//   - autoDone is the ONE-SHOT. It is what makes this a measurement rather
//     than a control loop.
//   - the window length is the QUALITY GATE. Measuring the first 512 samples
//     of a source that has just opened is measuring its fade-in, so the
//     estimate waits for a full takensEstMax of audio.
func (m *TauMeter) due(tau float32, avail int) bool {
	if m.autoDone {
		return false
	}
	// Never over a τ somebody chose — see autoSet. The knob is theirs from
	// the moment they touch it, and a measurement that overrides it is not a
	// convenience, it is a control fighting the person using it.
	if tau != takens.TauDef && tau != m.autoSet {
		return false
	}
	return avail >= takensEstMax
}

// estimate runs both estimators on x, a signal at rate sr, and reports the τ
// it found in source samples, or false when there is no first minimum (white
// noise has none).
//
// The knob's reach is the search range: an answer the knob cannot hold would
// be a number to look at and nothing more. The estimator works in SOURCE
// samples, because that is what the series it is handed is made of, while the
// knob counts reference samples — so the reach is converted into the
// estimator's units on the way in.
func (m *TauMeter) estimate(x []float64, sr int) (int, bool) {
	m.meas = takens.EstimateEmbedding(x, takens.TauSamples(takens.TauMax, sr), 8)
	m.measured = true
	return m.meas.Tau, m.meas.Tau >= 1
}

// tauKnob converts a delay in source samples at rate sr into the τ knob's
// reference-rate unit.
func tauKnob(tauSrc, sr int) float32 {
	ref := float32(tauSrc)
	if sr > 0 && sr != takens.RefRate {
		ref = float32(tauSrc) * float32(takens.RefRate) / float32(sr)
	}
	return float32(int(ref + 0.5))
}

// AutoMeasure is the one-shot for a native rack: given τ knob tau, the SRC
// knob chanKnob, and x, the newest TakensEstMax samples of that channel at
// rate sr, it measures if one is due and reports the τ to turn the knob to.
func (m *TauMeter) AutoMeasure(tau, chanKnob float32, x []float32, sr int) (float32, bool) {
	m.rearmOnChan(chanKnob)
	if !m.due(tau, len(x)) {
		return 0, false
	}
	m.autoDone = true
	xs := make([]float64, len(x))
	for i, v := range x {
		xs[i] = float64(v)
	}
	if t, ok := m.estimate(xs, sr); ok {
		m.autoSet = tauKnob(t, sr)
		return m.autoSet, true
	}
	// "no min" leaves τ where it was, and where it was is still not a value
	// anybody chose.
	m.autoSet = tau
	return 0, false
}

// tauMS is the τ knob in milliseconds.
func tauMS(tauRef float32) float32 { return tauRef * 1000 / takens.RefRate }

// measText renders the measurement cell at τ knob tau. Seven characters is
// what its display holds, so one decimal and no space: "τ1.5m3", or "τ1.5m>3"
// when the dimension never settled.
func (m *TauMeter) measText(tau float32) string {
	if m.meas.Tau < 1 {
		return "τ-- m-"
	}
	s := "τ" + led.Format(float64(tauMS(tau)), 1, 1, false)
	if m.meas.OK {
		return s + "m" + strconv.Itoa(m.meas.Dim)
	}
	return s + "m>" + strconv.Itoa(m.meas.Dim) // FNN never settled inside the search
}

// Text is the readout: the measurement at τ knob tau, "no min" when one was
// taken and found none, and dashes before one is.
func (m *TauMeter) Text(tau float32) string {
	if m.measured && m.meas.Tau < 1 {
		return "no min"
	}
	return m.measText(tau)
}

// Measure is the ↻ button: it measures x, the newest TakensEstMax samples of
// the channel at rate sr, whatever τ is set, and reports the τ to turn the
// knob to. What it writes is a τ somebody asked for, so the one-shot will not
// measure over it.
func (m *TauMeter) Measure(x []float32, sr int) (float32, bool) {
	if len(x) < 512 {
		return 0, false
	}
	xs := make([]float64, len(x))
	for i, v := range x {
		xs[i] = float64(v)
	}
	if t, ok := m.estimate(xs, sr); ok {
		return tauKnob(t, sr), true
	}
	return 0, false
}
