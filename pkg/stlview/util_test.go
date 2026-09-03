package stlview

import "testing"

// cryptoRandIntn used to accumulate rejected draws instead of discarding them,
// so roughly one call in 256 (for max=5) spun until the accumulator overflowed
// and then returned a NEGATIVE number. Callers use it as a count: a negative
// one produced an empty gradient and panicked the STL loader. A loop long
// enough to cover that 1/256 is the test — a handful of calls never saw it,
// which is exactly why it shipped.
func TestCryptoRandIntnStaysInRange(t *testing.T) {
	for _, max := range []int{1, 2, 5, 6, 57, 256, 1000} {
		seen := make(map[int]bool)
		for i := 0; i < 20000; i++ {
			got, err := cryptoRandIntn(max)
			if err != nil {
				t.Fatalf("max=%d: %v", max, err)
			}
			if got < 0 || got >= max {
				t.Fatalf("max=%d: returned %d on call %d, want [0,%d)", max, got, i, max)
			}
			seen[got] = true
		}
		// A generator stuck on one value is in range and still broken.
		if max > 1 && len(seen) < 2 {
			t.Errorf("max=%d: 20000 draws produced only %d distinct value(s)", max, len(seen))
		}
	}
}

// A non-positive bound is an error, not a panic and not a negative result.
func TestCryptoRandIntnRejectsANonPositiveBound(t *testing.T) {
	for _, max := range []int{0, -1} {
		if got, err := cryptoRandIntn(max); err == nil {
			t.Errorf("cryptoRandIntn(%d) = %d, want an error", max, got)
		}
	}
}
