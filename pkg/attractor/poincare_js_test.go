//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// The section end to end: a real system, integrated the way the app integrates
// it, sectioned by the real plane the real knobs build.
//
// poincare_test.go pins the arithmetic against curves with closed-form
// crossings. What it cannot see is the wiring — whether the plane the knobs
// build is the plane the crossings are tested against, whether the recorded
// point is the interpolated one or the endpoint it was interpolated from,
// whether the direction reaching poincareCross is the direction the knob is
// on. Each of those is a silent failure: the picture still looks like a
// section, it is just a section of something else.
//
// None of this touches GL or the DOM, which is the whole reason the drawing
// was kept out of sectSeed and sectAdvance.

// sectTestSetup puts the section knobs in a known state and restores them, so
// these tests cannot leak into each other or into the rest of the js suite.
func sectTestSetup(t *testing.T, axis int, pos float32, dir int) (flowSys4, float64) {
	t.Helper()
	oldAxis, oldPos, oldDir, oldSig := sectAxisF, sectPosF, sectDirF, sectSig
	t.Cleanup(func() {
		sectAxisF, sectPosF, sectDirF, sectSig = oldAxis, oldPos, oldDir, oldSig
		sectLog.reset(0)
	})
	sectAxisF, sectPosF, sectDirF = float32(axis), pos, float32(dir)

	sys, ok := flowFor4("lorenz")
	if !ok {
		t.Fatal("lorenz has no registered flow, so there is nothing to section")
	}
	dt := sys.dt()
	if dt <= 0 {
		t.Fatalf("lorenz dt is %v", dt)
	}
	sectSeed("lorenz", sys, dt)
	return sys, dt
}

// THE WIRING CHECK. Every accumulated crossing must lie ON the plane.
//
// This is what says the recorded point is the interpolated crossing and not
// one of the samples it was interpolated between. A snapped section fails this
// by a wide margin — the endpoints are up to a step of arc off the plane,
// which on Lorenz at dt=0.005 is a signed distance of order 0.1 — so the
// tolerance below separates the two by four orders of magnitude. It is not
// tighter than 1e-4 because the hits are stored as float32 for the vertex
// buffer, and a coordinate of order 25 has about 1e-6 of resolution there.
func TestEveryAccumulatedCrossingLandsOnThePlane(t *testing.T) {
	sys, dt := sectTestSetup(t, sectAxisZ, 0, crossRising)
	sectAdvance("lorenz", sys, dt, 60000)

	if n := sectLog.len(); n < 50 {
		t.Fatalf("only %d crossings in 60000 steps; the section is not being fed", n)
	}
	worst := 0.0
	for i := 0; i < sectLog.len(); i++ {
		h := sectLog.at(i)
		d := math.Abs(sectPlane.signed([3]float64{float64(h.P[0]), float64(h.P[1]), float64(h.P[2])}))
		if d > worst {
			worst = d
		}
	}
	t.Logf("%d crossings, worst distance from the plane %.3e", sectLog.len(), worst)
	if worst > 1e-4 {
		t.Errorf("a recorded crossing is %.3e from the plane; an interpolated crossing is on it, "+
			"and a snapped sample would be about 1e-1 away — this is what a section drawn from "+
			"the wrong point looks like", worst)
	}
}

// The direction knob reaches the crossing test. At a rising crossing the flow's
// component along the plane's normal must be positive, and at a falling one
// negative — that is what "rising" means, and it is checkable at each recorded
// point without knowing anything else about the system.
func TestTheDirectionKnobDecidesWhichCrossingsAreKept(t *testing.T) {
	for _, c := range []struct {
		dir  int
		name string
		want float64 // the sign dz/dt must have
	}{
		{crossRising, "up", +1},
		{crossFalling, "down", -1},
	} {
		sys, dt := sectTestSetup(t, sectAxisZ, 0, c.dir)
		sectAdvance("lorenz", sys, dt, 60000)
		if n := sectLog.len(); n < 50 {
			t.Fatalf("%s: only %d crossings", c.name, n)
		}
		bad := 0
		for i := 0; i < sectLog.len(); i++ {
			h := sectLog.at(i)
			_, _, dz, _ := sys.f(float64(h.P[0]), float64(h.P[1]), float64(h.P[2]), 0)
			if dz*c.want <= 0 {
				bad++
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d of %d crossings run the wrong way through the plane; the section "+
				"is two superimposed sheets and the return map is not a function",
				c.name, bad, sectLog.len())
		}
	}
}

// The pos knob moves the plane through the attractor, and it is a fraction of
// the attractor's OWN reach along the axis — so the same knob position means
// the same relative height on any system. Checking the crossings rather than
// the plane's d: it is the crossings that are the feature.
func TestThePosKnobMovesTheSectionThroughTheAttractor(t *testing.T) {
	meanZ := func(pos float32) float64 {
		sys, dt := sectTestSetup(t, sectAxisZ, pos, crossRising)
		sectAdvance("lorenz", sys, dt, 60000)
		if sectLog.len() < 20 {
			t.Fatalf("pos %v: only %d crossings", pos, sectLog.len())
		}
		sum := 0.0
		for i := 0; i < sectLog.len(); i++ {
			sum += float64(sectLog.at(i).P[2])
		}
		return sum / float64(sectLog.len())
	}
	low, mid, high := meanZ(-0.5), meanZ(0), meanZ(0.5)
	t.Logf("mean crossing height: pos-0.5 %.3f  pos0 %.3f  pos+0.5 %.3f", low, mid, high)
	if !(low < mid && mid < high) {
		t.Errorf("the pos knob does not move the section: %.3f / %.3f / %.3f for -0.5 / 0 / +0.5", low, mid, high)
	}
}

// The axis knob picks which coordinate the plane is normal to, and the 2-D
// coordinates the section is read in follow from it. For an x-plane the
// section is read in (y, z) — poincareBasis says so and this checks the whole
// chain agrees, since S and T are what the flat view and the return map plot.
func TestTheAxisKnobPicksThePlaneAndItsCoordinates(t *testing.T) {
	sys, dt := sectTestSetup(t, sectAxisX, 0, crossRising)
	sectAdvance("lorenz", sys, dt, 60000)
	if sectLog.len() < 20 {
		t.Fatalf("only %d crossings through an x plane", sectLog.len())
	}
	for i := 0; i < sectLog.len(); i++ {
		h := sectLog.at(i)
		if math.Abs(float64(h.S-h.P[1])) > 1e-4 || math.Abs(float64(h.T-h.P[2])) > 1e-4 {
			t.Fatalf("crossing %d reads as (%v, %v) in the plane but sits at (%v, %v, %v); an "+
				"x-plane section is read in (y, z)", i, h.S, h.T, h.P[0], h.P[1], h.P[2])
		}
	}
}

// A signature that does not include everything the section depends on is a
// section that keeps stale points after a knob moves — two planes' crossings
// in one scatter, which looks like a thicker section rather than like a bug.
func TestTheSignatureCoversEveryControlThatChangesTheSection(t *testing.T) {
	oldAxis, oldPos, oldDir := sectAxisF, sectPosF, sectDirF
	t.Cleanup(func() { sectAxisF, sectPosF, sectDirF = oldAxis, oldPos, oldDir })

	sectAxisF, sectPosF, sectDirF = sectAxisZ, 0, crossRising
	base := sectSignature("lorenz")
	for name, move := range map[string]func(){
		"axis": func() { sectAxisF = sectAxisX },
		"pos":  func() { sectPosF = 0.25 },
		"dir":  func() { sectDirF = crossFalling },
	} {
		sectAxisF, sectPosF, sectDirF = sectAxisZ, 0, crossRising
		move()
		if sectSignature("lorenz") == base {
			t.Errorf("moving %s leaves the signature unchanged, so the crossings from the old "+
				"setting stay in the scatter", name)
		}
	}
	sectAxisF, sectPosF, sectDirF = sectAxisZ, 0, crossRising
	if sectSignature("rossler") == base {
		t.Error("two different source systems share a signature")
	}
}
