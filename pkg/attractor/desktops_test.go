package attractor

import (
	"math"
	"testing"
)

// The desktop selector is built from deskStyleOrder and deskStyleLabel at
// runtime, so a style that is in one and not the other is a detent with no
// name or a name with no detent — and neither shows up until someone turns the
// knob to it.

func TestEveryDesktopStyleHasALabel(t *testing.T) {
	for _, k := range deskStyleOrder {
		if deskStyleLabel[k] == "" {
			t.Errorf("style %q has no label; the selector would show a blank option", k)
		}
	}
}

func TestNoLabelWithoutAStyle(t *testing.T) {
	in := map[string]bool{}
	for _, k := range deskStyleOrder {
		in[k] = true
	}
	for k := range deskStyleLabel {
		if !in[k] {
			t.Errorf("label for %q, which the selector never offers", k)
		}
	}
}

func TestDesktopStylesAreDistinct(t *testing.T) {
	seenKey := map[string]bool{}
	seenLabel := map[string]string{}
	for _, k := range deskStyleOrder {
		if seenKey[k] {
			t.Errorf("style %q appears twice in the order", k)
		}
		seenKey[k] = true
		l := deskStyleLabel[k]
		if prev, dup := seenLabel[l]; dup {
			t.Errorf("styles %q and %q are both called %q", prev, k, l)
		}
		seenLabel[l] = k
	}
}

func TestFlatIsFirst(t *testing.T) {
	// The desk should open as an ordinary window manager. Anything else means
	// switching it on rearranges windows before being asked to.
	if len(deskStyleOrder) == 0 || deskStyleOrder[0] != deskFlat {
		t.Errorf("the first style is %v, want %q", deskStyleOrder, deskFlat)
	}
}

func TestFaceOfCyclesThroughEveryWorkspace(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < deskFaces*3; i++ {
		f := faceOf(i)
		if f < 0 || f >= deskFaces {
			t.Fatalf("faceOf(%d) = %d, outside 0..%d", i, f, deskFaces-1)
		}
		seen[f] = true
	}
	if len(seen) != deskFaces {
		t.Errorf("only %d of %d faces are ever used", len(seen), deskFaces)
	}
	// Round-robin: the fifth window shares a face with the first.
	if faceOf(0) != faceOf(deskFaces) {
		t.Error("windows do not wrap round to the first face")
	}
}

func TestFaceOfHandlesANegativeIndex(t *testing.T) {
	// Go's % keeps the sign of the dividend, so a negative index would give a
	// negative face and a transform of NaN degrees. Nothing passes one today;
	// this is here so that changing the caller cannot start.
	if f := faceOf(-1); f < 0 || f >= deskFaces {
		t.Errorf("faceOf(-1) = %d, outside 0..%d", f, deskFaces-1)
	}
}

func TestClampDeg(t *testing.T) {
	for _, tc := range []struct{ v, lim, want float64 }{
		{0, 75, 0},
		{30, 75, 30},
		{-30, 75, -30},
		{90, 75, 75},
		{-90, 75, -75},
		{75, 75, 75},
		{-75, 75, -75},
	} {
		if got := clampDeg(tc.v, tc.lim); got != tc.want {
			t.Errorf("clampDeg(%v, %v) = %v, want %v", tc.v, tc.lim, got, tc.want)
		}
	}
}

func TestClampDegLeavesNaNAlone(t *testing.T) {
	// NaN compares false against everything, so it falls through both bounds
	// and is returned unchanged. Worth pinning: the caller's defense against
	// NaN is num(), and if that were ever removed the failure would show up
	// here as a transform of "NaNdeg" rather than as a clamp that silently
	// turned NaN into a limit.
	if got := clampDeg(math.NaN(), 75); !math.IsNaN(got) {
		t.Errorf("clampDeg(NaN) = %v, want NaN", got)
	}
}
