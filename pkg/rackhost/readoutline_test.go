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
