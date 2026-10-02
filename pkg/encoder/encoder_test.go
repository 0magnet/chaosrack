package encoder

import (
	"math"
	"testing"
)

const detent = 2 * math.Pi / 24

// Turned slowly, a detent is one step either way: the click is halfway to the
// next rest position, and turning back over it clicks back.
func TestASlowDetentIsOneStep(t *testing.T) {
	var e Encoder
	if got := e.Turn(detent*0.4, 0); got != 0 {
		t.Fatalf("less than half a detent stepped %d", got)
	}
	if got := e.Turn(detent*0.4, 500); got != 1 {
		t.Fatalf("the rest of it stepped %d, want 1", got)
	}
	if got := e.Turn(-detent, 1000); got != -1 {
		t.Errorf("a detent back stepped %d, want -1", got)
	}
}

// Spun fast, the steps grow; the same rotation turned slowly does not.
func TestAFastSpinAccelerates(t *testing.T) {
	var fast, slow Encoder
	fastSum, slowSum := 0, 0
	for i := range 24 { // a full turn
		fastSum += fast.Turn(detent, float64(i)*5)   // in 120 ms
		slowSum += slow.Turn(detent, float64(i)*500) // in 12 s
	}
	if slowSum != 24 {
		t.Errorf("a slow turn made %d steps, want 24", slowSum)
	}
	if fastSum <= 24*3 {
		t.Errorf("a fast turn made %d steps; it should accelerate well past the slow one's 24", fastSum)
	}
}

func TestAccelIsBounded(t *testing.T) {
	if Accel(0) != 1 || Accel(8) != 1 {
		t.Error("slow turning is not one step a detent")
	}
	if Accel(1e6) != 10 {
		t.Errorf("Accel(huge) = %d, want 10", Accel(1e6))
	}
}
