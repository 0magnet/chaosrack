package ripple

import (
	"math"
	"testing"
)

// A drop must travel. A field that holds a bump where it was put is a
// diffusion, not a wave, and would refract like a static lens.
func TestADropRadiates(t *testing.T) {
	f := New(64, 64)
	f.Drop(32, 32, 3, 1)

	near := math.Abs(float64(f.Height(38, 32)))
	for i := 0; i < 20; i++ {
		f.Step()
	}
	after := math.Abs(float64(f.Height(38, 32)))

	if after <= near {
		t.Errorf("no wave reached 6 cells out: %.5f before, %.5f after", near, after)
	}
}

// Damping below 1 must actually settle the field. A medium that rings forever
// fills with standing waves and nothing behind it stays legible.
func TestDampingSettles(t *testing.T) {
	f := New(48, 48)
	f.Damping = 0.9
	f.Drop(24, 24, 3, 1)

	start := f.Energy()
	for i := 0; i < 400; i++ {
		f.Step()
	}
	end := f.Energy()

	if end >= start*0.01 {
		t.Errorf("energy %.6g did not decay from %.6g", end, start)
	}
}

// The CFL limit is a real cliff: above it the integration does not degrade,
// it goes to NaN. Step clamps, so a knob at its maximum must still be finite.
func TestSpeedIsClampedToStability(t *testing.T) {
	f := New(48, 48)
	f.Speed = 10 // far past sqrt(0.5)
	f.Damping = 1
	f.Drop(24, 24, 2, 1)

	for i := 0; i < 500; i++ {
		f.Step()
	}
	for i, v := range f.Heights() {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("cell %d is %v: the speed clamp did not hold", i, v)
		}
	}
	if e := f.Energy(); e > 1e6 {
		t.Errorf("energy %.6g — bounded, but growing without limit", e)
	}
}

// Walls reflect rather than absorb, so a tank develops interference.
func TestEdgesReflect(t *testing.T) {
	f := New(40, 40)
	f.Damping = 1 // isolate the boundary from the decay
	f.Spread = 0
	f.Drop(20, 20, 2, 1)

	// Long enough for the front to reach a wall and return.
	for i := 0; i < 120; i++ {
		f.Step()
	}
	if f.Energy() < 1e-6 {
		t.Fatal("the field emptied: the boundary absorbed instead of reflecting")
	}
}

// Gradient is what refraction samples, so it must point along the slope. On a
// field raised in the middle, the slope either side points opposite ways.
func TestGradientPointsAlongTheSlope(t *testing.T) {
	f := New(32, 32)
	f.Drop(16, 16, 5, 1)

	gxLeft, _ := f.Gradient(12, 16)
	gxRight, _ := f.Gradient(20, 16)

	if gxLeft <= 0 {
		t.Errorf("left of a bump the x slope should rise toward it, got %.5f", gxLeft)
	}
	if gxRight >= 0 {
		t.Errorf("right of a bump the x slope should fall away, got %.5f", gxRight)
	}
}

// A drag leaves a continuous wake. Dropping only at the current pointer
// position leaves gaps when the pointer moves fast, and the gaps read as a
// dotted line rather than a wake.
func TestDragIsContinuous(t *testing.T) {
	f := New(64, 64)
	f.Line(10, 32, 54, 32, 2, 1)

	for x := 12; x < 52; x += 4 {
		if h := math.Abs(float64(f.Height(x, 32))); h < 1e-4 {
			t.Errorf("gap in the wake at x=%d (height %.6f)", x, h)
		}
	}
}

// Spread is what separates a liquid from a drum head: it should take the
// finest ripples out and leave the coarse ones.
func TestSpreadRemovesTheFinestRipples(t *testing.T) {
	fine := func(spread float32) float64 {
		f := New(64, 64)
		f.Spread = spread
		f.Damping = 1
		// Alternating cells: the shortest wavelength the grid can hold.
		for y := 1; y < 63; y++ {
			for x := 1; x < 63; x++ {
				if (x+y)%2 == 0 {
					f.cur[y*64+x] = 1
				} else {
					f.cur[y*64+x] = -1
				}
			}
		}
		for i := 0; i < 12; i++ {
			f.Step()
		}
		return f.Energy()
	}
	if with, without := fine(0.5), fine(0); with >= without {
		t.Errorf("spread kept the grid-scale ripple: %.4g with, %.4g without", with, without)
	}
}

func TestStillEmptiesTheField(t *testing.T) {
	f := New(32, 32)
	f.Drop(16, 16, 4, 1)
	f.Still()
	if e := f.Energy(); e != 0 {
		t.Errorf("energy %v after Still", e)
	}
}

// A drop off the edge must not panic or wrap to the far side.
func TestDropOutsideTheFieldIsClipped(t *testing.T) {
	f := New(32, 32)
	f.Drop(-5, -5, 4, 1)
	f.Drop(40, 40, 4, 1)
	if h := f.Height(31, 0); h != 0 {
		t.Errorf("a drop off the top-left wrapped to the far corner: %v", h)
	}
}

// Open water must let a wave leave. With the walls absorbing, the tank should
// empty; with them reflecting, the same disturbance stays in the field.
//
// This is the difference the knob exists for, so it is asserted as a
// comparison rather than against a threshold: the absolute energy depends on
// the damping and the drop, but the ordering does not.
func TestReflectControlsWhetherWavesReturn(t *testing.T) {
	run := func(reflect float32) float64 {
		f := New(64, 64)
		f.Reflect = reflect
		f.Damping = 1 // isolate the boundary from the decay
		f.Spread = 0
		f.Drop(32, 32, 3, 1)
		// Long enough for the front to reach a wall and, if it can, return.
		for i := 0; i < 200; i++ {
			f.Step()
		}
		return f.Energy()
	}
	open, wall := run(0), run(1)
	if open >= wall {
		t.Errorf("absorbing walls kept %.6g, reflecting kept %.6g — the wave did not leave", open, wall)
	}
	if open > wall*0.5 {
		t.Errorf("absorbing walls kept %.6g of %.6g: more than half came back", open, wall)
	}
}

// Half-reflecting edges should sit between the two ends rather than doing
// something of their own.
func TestPartialReflectionIsBetweenTheExtremes(t *testing.T) {
	run := func(reflect float32) float64 {
		f := New(64, 64)
		f.Reflect = reflect
		f.Damping = 1
		f.Spread = 0
		f.Drop(32, 32, 3, 1)
		for i := 0; i < 200; i++ {
			f.Step()
		}
		return f.Energy()
	}
	open, half, wall := run(0), run(0.5), run(1)
	if half <= open || half >= wall {
		t.Errorf("half-reflecting %.6g is not between open %.6g and wall %.6g", half, open, wall)
	}
}

// Whatever the edges do, the field must stay finite: an absorbing boundary
// written with the wrong sign is a classic way to make one that amplifies.
func TestAbsorbingEdgesStayFinite(t *testing.T) {
	f := New(48, 48)
	f.Reflect = 0
	f.Damping = 1
	f.Drop(24, 24, 3, 1)
	for i := 0; i < 2000; i++ {
		f.Step()
	}
	for i, v := range f.Heights() {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("cell %d is %v: the absorbing boundary is amplifying", i, v)
		}
	}
}
