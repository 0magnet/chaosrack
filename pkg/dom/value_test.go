//go:build js && wasm

package dom

import (
	"syscall/js"
	"testing"
)

func valued(v any) js.Value {
	o := js.Global().Get("Object").New()
	o.Set("value", v)
	return o
}

// Zero for everything that is not a number a control can hold, including the
// two ParseFloat accepts without an error.
func TestNumIsZeroForWhatIsNotAFiniteNumber(t *testing.T) {
	for in, want := range map[string]float64{
		"1.5": 1.5, "-3": -3, "1e2": 100, "0": 0,
		"": 0, "abc": 0, "NaN": 0, "Inf": 0, "-Infinity": 0, "1e999": 0,
	} {
		if got := Num(valued(in)); got != want {
			t.Errorf("Num(%q) = %v, want %v", in, got, want)
		}
	}
	if got := Num(js.Null()); got != 0 {
		t.Errorf("Num of a missing element = %v, want 0", got)
	}
	if got := Num(js.Undefined()); got != 0 {
		t.Errorf("Num of an undefined element = %v, want 0", got)
	}
}

func TestIntIsZeroForWhatIsNotAWholeNumber(t *testing.T) {
	for in, want := range map[string]int{"7": 7, "-2": -2, "": 0, "1.5": 0, "x": 0} {
		if got := Int(valued(in)); got != want {
			t.Errorf("Int(%q) = %v, want %v", in, got, want)
		}
	}
	if got := Int(js.Null()); got != 0 {
		t.Errorf("Int of a missing element = %v, want 0", got)
	}
}
