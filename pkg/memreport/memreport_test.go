package memreport

import (
	"strings"
	"testing"
)

// TestWrite prints a reading and checks it says what the fields hold: sizes
// in megabytes, the largest module and canvas first, and only as many as
// asked for.
func TestWrite(t *testing.T) {
	r := Report{
		Compiler:   "gc",
		Uptime:     3725,
		WasmMemory: 44 << 20,
		WasmModule: 32 << 20,
		Go:         GoHeap{HeapAlloc: 18 << 20, HeapInuse: 20 << 20, HeapSys: 40 << 20, Sys: 43 << 20, NumGC: 12},
		Elements:   25000,
		Modules: []Module{
			{Name: "speed", Elements: 300},
			{Name: "visual · model", Elements: 4000},
			{Name: "parameters", Elements: 900, Hidden: true},
		},
		Canvases: []Canvas{{Name: "scope-screen", Width: 563, Height: 384}, {Name: "gocanvas", Width: 1911, Height: 994}},
	}
	var b strings.Builder
	if err := r.Write(&b, 2); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"build", "go, up 1h 2m", "44.0 MB", "32.0 MB", "18.0 MB", "after 12 collections", "3.0 MB for the runtime itself", "25000"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if i, j := strings.Index(out, "visual · model"), strings.Index(out, "parameters"); i < 0 || j < 0 || i > j {
		t.Errorf("modules not largest first:\n%s", out)
	}
	if strings.Contains(out, "speed") {
		t.Errorf("--top 2 listed a third module:\n%s", out)
	}
	if i, j := strings.Index(out, "gocanvas"), strings.Index(out, "scope-screen"); i < 0 || j < 0 || i > j {
		t.Errorf("canvases not largest first:\n%s", out)
	}

	// TinyGo can report less in Sys than in HeapSys; that is no reason to
	// print eighteen exabytes.
	r.Compiler, r.Go.Sys = "tinygo", 1
	b.Reset()
	if err := r.Write(&b, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "tinygo") || !strings.Contains(b.String(), "0.0 MB for the runtime itself") {
		t.Errorf("tinygo reading:\n%s", b.String())
	}
}

// TestWriteBridgeAndBrowser prints what the page counted of its program and
// the browser's account of the tab, and the ?diag lists only when asked for.
func TestWriteBridgeAndBrowser(t *testing.T) {
	r := Report{
		Compiler: "gc",
		Go:       GoHeap{NumGC: 3, PauseTotalNs: 6e6},
		Bridge: &Bridge{
			Calls:   []Count{{"valueGet", 120000}, {"valueCall", 950}},
			Handles: &Handles{Live: 14761, Table: 20000, Kinds: []Count{{"Object", 4007}}},
			Stalls:  Stalls{Kind: "long-animation-frame", N: 4, Ms: 380, Worst: 150},
			Names:   []Count{{"get width", 77}},
		},
		Browser: &Browser{Pid: 42, Footprint: 370 << 20, PeakRSS: 520 << 20, Pages: 2,
			Parts: []Count{{"blink_gc", 164 << 20}, {"v8", 23 << 20}}},
	}
	var b strings.Builder
	if err := r.Write(&b, 0); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"gc pauses", "6 ms", "120.0k", "valueGet 120.0k, valueCall 950", "14761 held", "Object 4007",
		"long frames", "380 ms in all, the worst 150 ms (long-animation-frame)", "browser, pid 42", "370.0 MB", "peak 520.0 MB",
		"shared by 2 pages", "blink_gc", "164.0 MB", "Blink's heap"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "get width") {
		t.Errorf("listed calls by name without ?diag:\n%s", out)
	}
	r.Bridge.Detail = true
	b.Reset()
	if err := r.Write(&b, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "get width") {
		t.Errorf("no calls by name with ?diag:\n%s", b.String())
	}
}

// TestRates turns two readings into rates per minute and per second.
func TestRates(t *testing.T) {
	a := Report{Uptime: 100, Go: GoHeap{TotalAlloc: 10 << 20, NumGC: 4, PauseTotalNs: 2e6},
		Bridge: &Bridge{Calls: []Count{{"valueGet", 1000}}, Stalls: Stalls{N: 1, Ms: 60}}}
	b := Report{Uptime: 130, Go: GoHeap{TotalAlloc: 13 << 20, NumGC: 6, PauseTotalNs: 5e6},
		Bridge: &Bridge{Calls: []Count{{"valueGet", 3000}, {"valueCall", 1000}}, Stalls: Stalls{N: 3, Ms: 160}}}
	r := Rates(a, b)
	if r.Seconds != 30 || r.AllocMBPerMin != 6 || r.GCPerMin != 4 || r.PauseMsPerMin != 6 ||
		r.CallsPerSec != 100 || r.StallsPerMin != 4 || r.StallMsPerMin != 200 {
		t.Errorf("rates %+v", r)
	}
	if r := Rates(b, a); r.AllocMBPerMin != 0 {
		t.Errorf("an earlier reading second gave rates %+v", r)
	}
}
