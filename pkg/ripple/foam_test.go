package ripple

import "testing"

// Foam has to stay. The whole point of it over another wave is that it is
// still on the surface seconds after it was made, where a ripple from the
// same moment has long since spread out and faded.
func TestFoamOutlastsTheWaveThatMadeIt(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(100, 1)
	f.Drop(32, 32, 2, 0.5)
	for range 20 {
		fm.Spawn(32, 32)
	}
	for range 100 { // over a second and a half
		f.Step()
		fm.Step(f)
	}
	if len(fm.Bubbles) == 0 {
		t.Fatal("every bubble was gone after 100 steps; foam lasts at least 120")
	}
	for range foamMinLife + foamSpanLife {
		f.Step()
		fm.Step(f)
	}
	if len(fm.Bubbles) != 0 {
		t.Errorf("%d bubbles outlived the longest life", len(fm.Bubbles))
	}
}

// A bubble slides down the slope, which is what gathers foam in the hollows
// between waves.
func TestABubbleSlidesDownhill(t *testing.T) {
	f := New(64, 64)
	for y := range 64 {
		for x := range 64 {
			f.cur[y*64+x] = float32(x) * 0.01 // rising to the right
		}
	}
	fm := NewFoam(1, 2)
	fm.Spawn(32, 32)
	x0 := fm.Bubbles[0].X
	for range 30 {
		fm.Step(f)
	}
	if len(fm.Bubbles) == 0 || fm.Bubbles[0].X >= x0 {
		t.Errorf("the bubble did not move downhill (from x=%.2f): %+v", x0, fm.Bubbles)
	}
}

// The surface carries no more than Max.
func TestFoamKeepsToItsMax(t *testing.T) {
	fm := NewFoam(5, 3)
	for range 50 {
		fm.Spawn(10, 10)
	}
	if len(fm.Bubbles) != 5 {
		t.Errorf("%d bubbles, want the max of 5", len(fm.Bubbles))
	}
}

// A bubble grows in and thins out rather than popping into and out of view.
func TestABubbleFades(t *testing.T) {
	b := Bubble{Life: 100}
	if b.Fade() != 0 {
		t.Errorf("a bubble just made is at %.2f, want 0", b.Fade())
	}
	b.Age = 50
	if b.Fade() != 1 {
		t.Errorf("a bubble mid-life is at %.2f, want 1", b.Fade())
	}
	b.Age = 99
	if f := b.Fade(); f <= 0 || f >= 0.2 {
		t.Errorf("a bubble about to burst is at %.2f, want a little above 0", f)
	}
}
