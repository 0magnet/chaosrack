//go:build js && wasm

package attractor

import (
	"math"
	"strconv"
	"testing"
)

// The defaults must leave a host that IS the whole page exactly as it was: the
// scroll lock on, the wheel on the canvas, the model centered on the window.
func TestHostPageDefaultsAreTheOldBehavior(t *testing.T) {
	if !LockHostScroll {
		t.Error("LockHostScroll must default true, or every existing host loses its scroll lock")
	}
	if ZoomTargetSelector != "" {
		t.Error("ZoomTargetSelector must default empty so the canvas keeps the wheel")
	}
	if CenterOnSelector != "" {
		t.Error("CenterOnSelector must default empty so the model stays window-centered")
	}
}

// wireHostWheel reports whether it took the wheel, and the caller keeps the old
// canvas binding when it did not. Getting this backwards would either bind twice
// (two zooms per notch) or not at all.
func TestWireHostWheelDeclinesWithoutASelector(t *testing.T) {
	old := ZoomTargetSelector
	defer func() { ZoomTargetSelector = old }()
	ZoomTargetSelector = ""
	if wireHostWheel() {
		t.Error("wireHostWheel claimed the wheel with no selector set")
	}
}

// ftoa is what lands in a CSS transform. It has to floor rather than truncate:
// truncation rounds toward zero, so a -66.5px offset would become -66 above the
// center and +66.5 would become 66 below it, and a backdrop that is meant to be
// symmetric about a point would sit half a pixel off on one side only.
func TestFtoaFloorsTowardNegativeInfinity(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{10, "10"},
		{10.4, "10"},
		{10.9, "10"},
		{-66, "-66"},
		{-66.1, "-67"},
		{-0.5, "-1"},
	} {
		if got := ftoa(tc.in); got != tc.want {
			t.Errorf("ftoa(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ftoa must agree with the formatting the transform string is built from, for
// any offset a layout can produce.
func TestFtoaMatchesFloor(t *testing.T) {
	for _, v := range []float64{-1000.7, -66.5, -1, -0.1, 0, 0.1, 1, 66.5, 1000.7} {
		if got, want := ftoa(v), strconv.Itoa(int(math.Floor(v))); got != want {
			t.Errorf("ftoa(%v) = %q, want %q", v, got, want)
		}
	}
}

// backdropCenterOffset must be a no-op without a selector — it runs on every
// resize and hashchange, and querySelector("") throws.
func TestBackdropCenterOffsetNoSelector(t *testing.T) {
	old := CenterOnSelector
	defer func() { CenterOnSelector = old }()
	CenterOnSelector = ""
	if dx, dy := backdropCenterOffset(); dx != 0 || dy != 0 {
		t.Errorf("backdropCenterOffset() = (%v, %v) with no selector, want (0, 0)", dx, dy)
	}
}
