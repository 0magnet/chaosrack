package ripple

import (
	"math"
	"sort"
	"testing"
)

// still is a calm tank 64 cells a side whose clock runs in real time,
// stepped once so its steps have a length.
func still() *Tank {
	tk := NewTank(64, 64)
	tk.Speed = 1
	tk.Step()
	return tk
}

// Foam has to stay. The whole point of it over another wave is that it is
// still on the surface seconds after it was made, where a ripple from the
// same moment has long since spread out and faded; and it has to go in the
// end.
func TestFoamOutlastsTheWaveThatMadeIt(t *testing.T) {
	tk := still()
	fm := NewFoam(100, 1)
	tk.Drop(32, 32, 2, 0.5)
	for range 20 {
		fm.Spawn(32, 32)
	}
	for range 90 { // a second and a half
		tk.Step()
		fm.Step(tk)
	}
	if len(fm.Bubbles) == 0 {
		t.Fatal("every bubble was gone after a second and a half")
	}
	for range int(60 * foamMaxLife) {
		tk.Step()
		fm.Step(tk)
	}
	if len(fm.Bubbles) != 0 {
		t.Errorf("%d bubbles outlived the longest life", len(fm.Bubbles))
	}
}

// Most bubbles are tiny and a few are large, and none is past the largest.
func TestMostBubblesAreSmall(t *testing.T) {
	fm := NewFoam(1000, 2)
	for range 1000 {
		fm.Spawn(10, 10)
	}
	r := make([]float64, len(fm.Bubbles))
	for i, b := range fm.Bubbles {
		if b.R < foamMinR || b.R > foamMaxR {
			t.Fatalf("a bubble of radius %.2f, outside %.1f..%.1f", b.R, foamMinR, foamMaxR)
		}
		r[i] = float64(b.R)
	}
	sort.Float64s(r)
	if med := r[len(r)/2]; med > 2*foamMinR {
		t.Errorf("the middle bubble's radius is %.2f; most should be near the smallest, %.1f", med, foamMinR)
	}
	if r[len(r)-1] < 4*foamMinR {
		t.Errorf("the largest of a thousand is only %.2f", r[len(r)-1])
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

// A bubble comes up through the surface rather than appearing.
func TestABubbleRises(t *testing.T) {
	b := Bubble{Life: 5}
	if b.Risen() != 0 {
		t.Errorf("a bubble just made has risen %.2f, want 0", b.Risen())
	}
	b.Age = foamRise / 2
	if r := b.Risen(); r <= 0 || r >= 1 {
		t.Errorf("a bubble halfway up has risen %.2f", r)
	}
	b.Age = foamRise
	if b.Risen() != 1 {
		t.Errorf("a bubble up has risen %.2f, want 1", b.Risen())
	}
}

// A burst is over at once, and leaves a ring on the water where the bubble
// was.
func TestABurstRipplesTheWater(t *testing.T) {
	tk := still()
	fm := NewFoam(1, 5)
	fm.Bubbles = []Bubble{{X: 32, Y: 32, R: 0.8, Age: 0.999, Life: 1}}
	fm.Step(tk)
	if tk.Energy() == 0 {
		t.Fatal("the water is flat after a bubble burst on it")
	}
	if len(fm.Bubbles) != 0 {
		t.Errorf("the bubble is still there after it burst")
	}
}

// Bubbles near each other drift together until they touch, and stop there
// rather than piling into one.
func TestBubblesClingAndPack(t *testing.T) {
	tk := still()
	fm := NewFoam(2, 6)
	fm.Bubbles = []Bubble{
		{X: 30, Y: 32, R: 0.5, Age: 1, Life: 1000},
		{X: 32.4, Y: 32, R: 0.5, Age: 1, Life: 1000},
	}
	for range 120 {
		fm.Step(tk)
	}
	a, b := fm.Bubbles[0], fm.Bubbles[1]
	d := float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
	if d > 1.3 || d < 0.7 {
		t.Errorf("two bubbles 2.4 cells apart are %.2f apart two seconds later; want touching, about 1", d)
	}
}

// A bubble out of reach of the others is not pulled at all.
func TestAFarBubbleIsLeftAlone(t *testing.T) {
	fm := NewFoam(2, 7)
	fm.Bubbles = []Bubble{
		{X: 10, Y: 32, R: 0.5, Age: 1, Life: 1000},
		{X: 40, Y: 32, R: 0.5, Age: 1, Life: 1000},
	}
	fm.cling(64, 64, 1)
	if fm.Bubbles[0].pull != [2]float32{} || fm.Bubbles[1].pull != [2]float32{} {
		t.Errorf("bubbles 30 cells apart pulled each other: %v %v", fm.Bubbles[0].pull, fm.Bubbles[1].pull)
	}
}

// A crowd of bubbles made together packs into a raft where it was made: the
// pull of many neighbors at once does not fling them off the surface.
func TestACrowdStaysOnTheSurface(t *testing.T) {
	tk := still()
	fm := NewFoam(200, 8)
	for range 200 {
		fm.Spawn(32, 32)
		fm.Bubbles[len(fm.Bubbles)-1].Life = 1000 // none bursts: any lost was flung
	}
	for range 100 {
		fm.Step(tk)
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

// The water moves what floats on it: a splash beside a bubble rocks it.
func TestASplashRocksABubble(t *testing.T) {
	tk := still()
	fm := NewFoam(1, 9)
	fm.Bubbles = []Bubble{{X: 36, Y: 32, R: 0.3, Age: 1, Life: 1000}}
	tk.Drop(32, 32, 3, 1)
	var most float32
	for range 30 {
		tk.Step()
		fm.Step(tk)
		most = max(most, float32(math.Abs(float64(fm.Bubbles[0].X-36))))
	}
	if most < 0.1 {
		t.Errorf("a splash four cells away moved the bubble only %.3f cells", most)
	}
}

// BenchmarkFoamStep is a step of a surface carrying the page's most
// bubbles, strung along a waterline as the page's are.
func BenchmarkFoamStep(b *testing.B) {
	tk := NewTank(256, 128)
	tk.Step()
	fm := NewFoam(1500, 10)
	for i := range 1500 {
		fm.Spawn(40+float32(i%600)*0.3, 64+float32(i%7))
		fm.Bubbles[len(fm.Bubbles)-1].Life = 1e9
	}
	for b.Loop() {
		fm.Step(tk)
	}
}
