package rackhost

import (
	"strings"
	"testing"
)

// shows is what the model's live display labeled label shows.
func shows(t *testing.T, r *Rack, label string) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.analyzers.ro.item[label]
	if !ok {
		t.Fatalf("the bank has no %q display", label)
	}
	return r.pic.Items[i].Text
}

// One tone on both channels is perfectly correlated, on the Stereo
// Embedding's readout and the XY's.
func TestTheScopesReadTheirCorrelation(t *testing.T) {
	for _, m := range []string{"stereo", "xy"} {
		r, c := toneRack(t, m)
		watch(t, r, c, 0.3)
		if got := shows(t, r, "corr"); got != "r+1.00" {
			t.Errorf("%s reads %q for one tone on both channels, want r+1.00", m, got)
		}
	}
}

// Takens measures τ once and turns the knob to it, but never over a τ
// somebody chose.
func TestTakensMeasuresTauOnce(t *testing.T) {
	r, c := toneRack(t, "takens")
	watch(t, r, c, 0.5)
	if got := r.value("takens-tau"); got == "72" {
		t.Error("τ is still the default after a tone played")
	}
	if got := shows(t, r, "meas"); !strings.HasPrefix(got, "τ") || strings.Contains(got, "--") {
		t.Errorf("the measurement reads %q", got)
	}
	set(t, r, "takens-tau", "40", r.selector, "globe")
	watch(t, r, c, 0.1)
	set(t, r, r.selector, "takens")
	watch(t, r, c, 0.5)
	if got := r.value("takens-tau"); got != "40" {
		t.Errorf("the measurement moved a chosen τ to %s", got)
	}
}

// The recurrence plot's RR, DET and LAM read the matrix.
func TestTheRecurrencePlotReadsItsRQA(t *testing.T) {
	r, c := toneRack(t, "recurrence")
	watch(t, r, c, 1)
	for _, l := range []string{"rr", "det", "lam"} {
		if got := shows(t, r, l); strings.Contains(got, "-") {
			t.Errorf("%s reads %q", l, got)
		}
	}
}

// The waterfall's RT60 display is found, dashed for the live surface.
func TestTheWaterfallShowsItsRT60(t *testing.T) {
	r, c := toneRack(t, "waterfall")
	set(t, r, "wfall-src", "1")
	watch(t, r, c, 0.2)
	if got := shows(t, r, "rt60"); got != "--- s" {
		t.Errorf("the live surface's RT60 reads %q", got)
	}
}

// The Analysis readout measures the model on show: Lorenz is chaotic at about
// +0.9 per unit of time, a globe is no dynamical system at all, and a turned
// constant is measured again once it holds still.
func TestTheLyapunovReadout(t *testing.T) {
	r, c := idleRack(t, "lorenz")
	watch(t, r, c, 0.3)
	val, verdict := shows(t, r, "lyap-led"), shows(t, r, "lyap-verdict")
	if !strings.HasPrefix(val, "+0.9") || !strings.HasSuffix(val, "/t") || verdict != "chaotic" {
		t.Errorf("Lorenz reads %q %q, want about +0.9/t, chaotic", val, verdict)
	}
	// The constant is the integrator's, shared by every rack in the process:
	// put back for the tests after this one.
	rho := r.flow["lorenz-r"].Value
	t.Cleanup(func(v float32) func() { return func() { *rho = v } }(*rho))
	set(t, r, "lorenz-r", "20")
	watch(t, r, c, 0.1)
	if got := shows(t, r, "lyap-led"); got != "measuring…" {
		t.Errorf("a knob just turned reads %q", got)
	}
	watch(t, r, c, 0.5)
	if got := shows(t, r, "lyap-led"); got == val || got == "measuring…" {
		t.Errorf("ρ 20 reads %q, as ρ 28 did", got)
	}
	set(t, r, r.selector, "globe")
	watch(t, r, c, 0.3)
	if got := shows(t, r, "lyap-verdict"); got != "n/a" {
		t.Errorf("a globe's verdict is %q", got)
	}
}
