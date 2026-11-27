package rasterview

import (
	"math"
	"testing"
)

func TestClipSegment(t *testing.T) {
	// Inside: untouched.
	if t0, t1, ok := clipSegment(10, 10, 20, 0, 0, 0, 100, 100); !ok || t0 != 0 || t1 != 1 {
		t.Errorf("inside: %v %v %v", t0, t1, ok)
	}
	// Across from far out: only the 100 pixels on the picture remain.
	t0, t1, ok := clipSegment(-1e9, 50, 2e9, 0, 0, 0, 100, 100)
	if !ok || math.Abs((t1-t0)*2e9-100) > 1e-3 {
		t.Errorf("across: %v %v %v, %v pixels", t0, t1, ok, (t1-t0)*2e9)
	}
	// Wholly outside, and not a number.
	if _, _, ok := clipSegment(-50, -50, 10, 0, 0, 0, 100, 100); ok {
		t.Error("outside drew")
	}
	if _, _, ok := clipSegment(math.NaN(), 0, 1, 1, 0, 0, 100, 100); ok {
		t.Error("NaN drew")
	}
}
