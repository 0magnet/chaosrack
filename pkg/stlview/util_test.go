package stlview

import "testing"

// cryptoRandIntn used to accumulate rejected draws instead of discarding them,
// so roughly one call in 256 (for max=5) spun until the accumulator overflowed
// and then returned a NEGATIVE number. Callers use it as a count: a negative
// one produced an empty gradient and panicked the STL loader. A loop long
// enough to cover that 1/256 is the test — a handful of calls never saw it,
// which is exactly why it shipped.
func TestCryptoRandIntnStaysInRange(t *testing.T) {
	for _, bound := range []int{1, 2, 5, 6, 57, 256, 1000} {
		seen := make(map[int]bool)
		for i := range 20000 {
			got, err := cryptoRandIntn(bound)
			if err != nil {
				t.Fatalf("bound=%d: %v", bound, err)
			}
			if got < 0 || got >= bound {
				t.Fatalf("bound=%d: returned %d on call %d, want [0,%d)", bound, got, i, bound)
			}
			seen[got] = true
		}
		// A generator stuck on one value is in range and still broken.
		if bound > 1 && len(seen) < 2 {
			t.Errorf("bound=%d: 20000 draws produced only %d distinct value(s)", bound, len(seen))
		}
	}
}

// A non-positive bound is an error, not a panic and not a negative result.
func TestCryptoRandIntnRejectsANonPositiveBound(t *testing.T) {
	for _, bound := range []int{0, -1} {
		if got, err := cryptoRandIntn(bound); err == nil {
			t.Errorf("cryptoRandIntn(%d) = %d, want an error", bound, got)
		}
	}
}
