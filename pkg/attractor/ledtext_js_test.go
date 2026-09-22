//go:build js && wasm

package attractor

import (
	"syscall/js"
	"testing"
)

// countingLED is a readout that records how many times its text was actually
// set — which is the thing the guard exists to reduce. Built out of a plain
// Object like the rest of the fakes here, so it needs no DOM and runs under
// Node.
func countingLED(t *testing.T) (js.Value, func() int) {
	t.Helper()
	el := js.Global().Get("Object").New()
	n := 0
	desc := js.Global().Get("Object").New()
	f := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		n++
		return nil
	})
	t.Cleanup(f.Release)
	desc.Set("set", f)
	desc.Set("configurable", true)
	js.Global().Get("Object").Call("defineProperty", el, "textContent", desc)
	return el, func() int { return n }
}

func TestARepeatedReadingIsNotWritten(t *testing.T) {
	forgetLEDText()
	el, writes := countingLED(t)
	// A rack with nothing playing into it: the same dashes, over and over,
	// five times a second for as long as the page is open.
	for i := 0; i < 50; i++ {
		setLEDText("t-repeat", el, "  --.-")
	}
	if got := writes(); got != 1 {
		t.Fatalf("50 writes of one string reached the DOM %d times, want 1", got)
	}
}

func TestAChangedReadingIsWritten(t *testing.T) {
	forgetLEDText()
	el, writes := countingLED(t)
	for _, s := range []string{"  -1.0", "  -2.0", "  -2.0", "  -3.0", "  -3.0", "  -1.0"} {
		setLEDText("t-change", el, s)
	}
	if got := writes(); got != 4 {
		t.Fatalf("four distinct readings reached the DOM %d times, want 4", got)
	}
}

func TestReadoutsDoNotShareAMemory(t *testing.T) {
	forgetLEDText()
	a, aw := countingLED(t)
	b, bw := countingLED(t)
	setLEDText("t-a", a, "  --.-")
	setLEDText("t-b", b, "  --.-")
	if aw() != 1 || bw() != 1 {
		t.Fatalf("two readouts showing the same string wrote %d and %d, want 1 each", aw(), bw())
	}
}

func TestARebuiltPanelIsWrittenAgain(t *testing.T) {
	// The trap the memory could set: a panel is rebuilt, its LED is a new and
	// empty element, and the remembered string would keep it blank forever.
	forgetLEDText()
	old, _ := countingLED(t)
	setLEDText("t-rebuild", old, "  --.-")
	fresh, freshWrites := countingLED(t)
	forgetLEDText()
	setLEDText("t-rebuild", fresh, "  --.-")
	if got := freshWrites(); got != 1 {
		t.Fatalf("a rebuilt readout was written %d times, want 1 — it would have stayed blank", got)
	}
}

func TestAMissingReadoutIsNotRemembered(t *testing.T) {
	// A module that is not in the rack has no element. Writing to it must not
	// record the string against the key, or the readout would stay blank once
	// the module does appear.
	forgetLEDText()
	setLEDText("t-absent", js.Value{}, "  --.-")
	el, writes := countingLED(t)
	setLEDText("t-absent", el, "  --.-")
	if got := writes(); got != 1 {
		t.Fatalf("readout written %d times after appearing, want 1", got)
	}
}
