package ripple

import (
	"math"
	"testing"
)

// Foam has to stay. The whole point of it over another wave is that it is
// still on the surface seconds after it was made, where a ripple from the
// same moment has long since spread out and faded.
func TestFoamOutlastsTheWaveThatMadeIt(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(100, 1)
	f.Drop(32, 32, 2, 0.5)
	for range 20 {
		fm.Spawn(32, 32, 0, 0)
	}
	for range 100 { // over a second and a half
		f.Step()
		fm.Step(f)
	}
	if len(fm.Bubbles) == 0 {
		t.Fatal("every bubble was gone after 100 steps; foam lasts at least 120")
	}
	for range foamMinLife + foamSpanLife + foamBurst {
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
	fm.Spawn(32, 32, 0, 0)
	x0 := fm.Bubbles[0].X
	for range 30 {
		fm.Step(f)
	}
	if len(fm.Bubbles) == 0 || fm.Bubbles[0].X >= x0 {
		t.Errorf("the bubble did not move downhill (from x=%.2f): %+v", x0, fm.Bubbles)
	}
}

// A bubble thrown off the waterline travels: it comes from somewhere and
// goes somewhere, rather than sitting where it was made.
func TestAThrownBubbleTravels(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(1, 4)
	fm.Spawn(20, 32, 0.3, 0)
	x0 := fm.Bubbles[0].X
	for range 60 {
		fm.Step(f)
	}
	if d := fm.Bubbles[0].X - x0; d < 2 {
		t.Errorf("a bubble thrown at 0.3 cells a step went %.2f cells in a second; want a few", d)
	}
}

// The surface carries no more than Max.
func TestFoamKeepsToItsMax(t *testing.T) {
	fm := NewFoam(5, 3)
	for range 50 {
		fm.Spawn(10, 10, 0, 0)
	}
	if len(fm.Bubbles) != 5 {
		t.Errorf("%d bubbles, want the max of 5", len(fm.Bubbles))
	}
}

// A bubble inflates, stays whole for its life, and then bursts over a few
// frames — it does not dim away.
func TestABubbleInflatesAndBursts(t *testing.T) {
	b := Bubble{Life: 100}
	if b.Size() != 0 {
		t.Errorf("a bubble just made is at %.2f of its size, want 0", b.Size())
	}
	b.Age = foamGrow
	if b.Size() != 1 || b.Burst() != 0 {
		t.Errorf("a grown bubble: size %.2f burst %.2f, want 1 and 0", b.Size(), b.Burst())
	}
	b.Age = 99
	if b.Burst() != 0 {
		t.Errorf("a bubble a step from bursting is already %.2f burst", b.Burst())
	}
	b.Age = 100
	if p := b.Burst(); p <= 0 || p >= 1 {
		t.Errorf("a bubble bursting is %.2f through it, want between", p)
	}
}

// A burst leaves a ring on the water where the bubble was.
func TestABurstRipplesTheWater(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(1, 5)
	fm.Bubbles = []Bubble{{X: 32, Y: 32, R: 0.8, Age: 49, Life: 50}}
	fm.Step(f)
	if f.Energy() == 0 {
		t.Fatal("the water is flat after a bubble burst on it")
	}
	for range foamBurst {
		fm.Step(f)
	}
	if len(fm.Bubbles) != 0 {
		t.Errorf("the bubble is still there %d steps after it burst", foamBurst)
	}
}

// Bubbles near each other drift together until they touch, and stop there
// rather than piling into one.
func TestBubblesClingAndPack(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(2, 6)
	fm.Bubbles = []Bubble{
		{X: 30, Y: 32, R: 0.5, Age: foamGrow, Life: 1000},
		{X: 32.4, Y: 32, R: 0.5, Age: foamGrow, Life: 1000},
	}
	for range 120 {
		fm.Step(f)
	}
	a, b := fm.Bubbles[0], fm.Bubbles[1]
	d := float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
	if d > 1.3 || d < 0.7 {
		t.Errorf("two bubbles 2.4 cells apart are %.2f apart two seconds later; want touching, about 1", d)
	}
}

// A bubble out of reach of the others is not pulled at all.
func TestAFarBubbleIsLeftAlone(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(2, 7)
	fm.Bubbles = []Bubble{
		{X: 10, Y: 32, R: 0.5, Age: foamGrow, Life: 1000},
		{X: 40, Y: 32, R: 0.5, Age: foamGrow, Life: 1000},
	}
	fm.cling(f)
	if fm.dv[0] != [2]float32{} || fm.dv[1] != [2]float32{} {
		t.Errorf("bubbles 30 cells apart pulled each other: %v", fm.dv)
	}
}

// A crowd of bubbles made together packs into a raft where it was made: the
// pull of many neighbors at once does not fling them off the surface.
func TestACrowdStaysOnTheSurface(t *testing.T) {
	f := New(64, 64)
	fm := NewFoam(200, 8)
	for range 200 {
		fm.Spawn(32, 32, 0, 0)
	}
	for range 100 {
		fm.Step(f)
	}
	if len(fm.Bubbles) != 200 {
		t.Fatalf("%d of 200 bubbles left the surface", 200-len(fm.Bubbles))
	}
	for _, b := range fm.Bubbles {
		if dx, dy := b.X-32, b.Y-32; dx*dx+dy*dy > 20*20 {
			t.Fatalf("a bubble of the crowd went to %.1f, %.1f", b.X, b.Y)
		}
	}
}
