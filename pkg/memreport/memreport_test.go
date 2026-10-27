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
