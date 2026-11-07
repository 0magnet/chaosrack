package ripple

import (
	"math"
	"math/cmplx"
	"testing"
)

// The FFT must agree with the transform written out longhand, square or
// not, and the inverse must give back what went in.
func TestFFTMatchesTheDFT(t *testing.T) {
	for _, sz := range [][2]int{{8, 8}, {16, 4}, {4, 8}} {
		nx, ny := sz[0], sz[1]
		f := newFFT2(nx, ny)
		re, im := make([]float32, nx*ny), make([]float32, nx*ny)
		in := make([]complex128, nx*ny)
		for i := range re {
			re[i], im[i] = float32(math.Sin(float64(i)*1.3)), float32(math.Cos(float64(i)*0.7))
			in[i] = complex(float64(re[i]), float64(im[i]))
		}
		f.forward(re, im)
		for u := range nx {
			for v := range ny {
				var want complex128
				for y := range ny {
					for x := range nx {
						want += in[y*nx+x] * cmplx.Exp(complex(0, -2*math.Pi*(float64(u*x)/float64(nx)+float64(v*y)/float64(ny))))
					}
				}
				// Transposed: the frequency across is the row.
				got := complex(float64(re[u*ny+v]), float64(im[u*ny+v]))
				if cmplx.Abs(got-want) > 1e-3 {
					t.Fatalf("%dx%d: bin (%d,%d) = %v, want %v", nx, ny, u, v, got, want)
				}
			}
		}
		f.inverse(re, im)
		for i := range re {
			if math.Abs(float64(re[i])-real(in[i])) > 1e-5 || math.Abs(float64(im[i])-imag(in[i])) > 1e-5 {
				t.Fatalf("%dx%d: round trip at %d: %v+%vi, want %v", nx, ny, i, re[i], im[i], in[i])
			}
		}
	}
}

// unbounded takes the edges away, so a wave filling the tank is one wave,
// for steps of so many sixtieths.
func unbounded(tk *Tank, steps int) {
	tk.edges(tk.Reflect, steps)
	for i := range tk.edge {
		tk.edge[i] = 1
	}
}

// Each wave must turn at water's own frequency for its length: a standing
// wave cos(kx) comes back as cos(kx)·cos(ωt). This is the whole point of the
// tank — a long wave and a short one, both carried right.
func TestEachWaveTurnsAtItsOwnFrequency(t *testing.T) {
	const n = 128
	for _, m := range []int{2, 8, 32} { // a gravity wave, near the slowest, a capillary ripple
		tk := NewTank(n, n)
		tk.Damping, tk.Spread, tk.Speed = 1, 0, 0.3
		unbounded(tk, 1)
		k := 2 * math.Pi * float64(m) / n
		for y := range n {
			for x := range n {
				tk.h[y*n+x] = float32(math.Cos(k * float64(x)))
			}
		}
		omega, _, _ := dispersion(float32(k / (tankSide / n))) // per cell, in meters
		for range 5 {
			tk.Step()
		}
		// Turned at its own frequency, and as much of it kept as the grid's
		// finest are trimmed by (Tank.top), which is next to none but at the top.
		want := math.Cos(float64(omega)*5*float64(tk.Dt)) * math.Pow(float64(tk.top[m*n]), 5)
		if got := float64(tk.Height(0, 7)); math.Abs(got-want) > 1e-3 {
			t.Errorf("wave %d: height %.5f after 5 steps, want %.5f", m, got, want)
		}
		if got := float64(tk.Height(n/2/m, 7)); math.Abs(got+want) > 1e-3 {
			t.Errorf("wave %d: half a wave on, height %.5f, want %.5f", m, got, -want)
		}
	}
}

// Water's slowest waves are the ones where gravity hands over to surface
// tension: about 23 cm a second, at about 1.7 cm long. Faster both ways
// from there is what makes a splash spread into a train.
func TestTheSlowestWaveIsWatersOwn(t *testing.T) {
	slowest, at := math.Inf(1), 0.0
	for k := float32(10); k < 2000; k += 2 { // radians a meter
		omega, _, _ := dispersion(k)
		if c := float64(omega / k); c < slowest {
			slowest, at = c, 2*math.Pi/float64(k)
		}
	}
	if slowest < 0.21 || slowest > 0.25 {
		t.Errorf("slowest wave %.3f m/s, want about 0.23", slowest)
	}
	if at < 0.015 || at > 0.019 {
		t.Errorf("slowest wave %.4f m long, want about 0.017", at)
	}
}

// A wall must send a wave back and keep it in: what reaches the wall on one
// side must not come out of the other, as it would if the FFT's wrapping
// round showed through. The far side is watched only while no wave could
// have crossed the tank to it (the fastest, long waves in a 2 cm tray, go
// about 190 cells a second), and the wave must still be in the tank long
// after.
func TestTankWallsHoldTheWater(t *testing.T) {
	const n = 128
	tk := NewTank(n, n)
	tk.Damping, tk.Spread, tk.Speed, tk.Reflect = 1, 0.12, 0.5, 1
	tk.Drop(8, n/2, 3, 1)
	start := tk.Energy()
	var far, near float64
	for range 30 { // a quarter second of the tank's time: 48 cells at most
		tk.Step()
		for y := range n {
			for x := n - 24; x < n; x++ {
				far = math.Max(far, math.Abs(float64(tk.Height(x, y))))
			}
			for x := 2; x < 24; x++ {
				near = math.Max(near, math.Abs(float64(tk.Height(x, y))))
			}
		}
	}
	if far > near*0.02 {
		t.Errorf("a wave got through the wall: %.4f at the far side, %.4f at the near", far, near)
	}
	for range 300 {
		tk.Step()
	}
	if e := tk.Energy(); e < start*0.05 {
		t.Errorf("the tank lost its wave: energy %.4g from %.4g", e, start)
	}
}

// Open water must let a wave go, and walls must keep it: the same splash
// after it has reached the edges.
func TestOpenWaterLetsWavesGo(t *testing.T) {
	run := func(reflect float32) float64 {
		tk := NewTank(64, 64)
		tk.Damping, tk.Spread, tk.Speed, tk.Reflect = 1, 0.12, 0.7, reflect
		tk.Drop(32, 32, 3, 1)
		for range 400 {
			tk.Step()
		}
		return tk.Energy()
	}
	walls, open := run(1), run(0)
	if open > walls*0.2 {
		t.Errorf("open water kept %.4g of the splash, walls %.4g", open, walls)
	}
}

// However it is driven, the tank must not blow up: the solution is exact for
// any step, so nothing should make it grow.
func TestTankStaysFinite(t *testing.T) {
	tk := NewTank(64, 64)
	tk.Damping, tk.Spread, tk.Speed, tk.Reflect = 1, 0, 0.7, 1
	for i := range 2000 {
		tk.Drop(float32(i*7%64), float32(i*13%64), 2, 0.3)
		tk.Step()
	}
	for _, v := range tk.Heights() {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 1e4 {
			t.Fatalf("the tank blew up: %v", v)
		}
	}
}

// A wave passing under a floating thing carries it round in a circle and a
// little forward each time: Stokes drift, the reason foam on water travels
// with the waves and not back and forth on the spot.
func TestAWaveCarriesWhatFloats(t *testing.T) {
	const n, m = 128, 8
	tk := NewTank(n, n)
	tk.Damping, tk.Spread, tk.Speed = 1, 0, 0.5
	unbounded(tk, 1)
	// A wave running toward +x: h = a·cos(kx), φ = (ω/A)·a·sin(kx).
	const a = 0.6
	k := float32(2 * math.Pi * m / n)
	omega, A, _ := dispersion(k / (tankSide / n))
	for y := range n {
		for x := range n {
			tk.h[y*n+x] = a * float32(math.Cos(float64(k)*float64(x)))
			tk.phi[y*n+x] = omega / A * a * float32(math.Sin(float64(k)*float64(x)))
		}
	}
	x, y := float32(40), float32(64)
	lo, hi := x, x
	for range 600 {
		tk.Step()
		dx, dy := tk.Drift(x, y)
		x, y = x+dx, y+dy
		lo, hi = min(lo, x), max(hi, x)
	}
	if x <= 40.5 {
		t.Errorf("the wave did not carry it on: x %.2f from 40", x)
	}
	if hi-lo < a {
		t.Errorf("it did not ride the wave round: moved over %.2f cells, the wave is %.1f high", hi-lo, a)
	}
}

// BenchmarkTankStep is a step of the page's tank on a wide screen: the
// frame budget's share.
func BenchmarkTankStep(b *testing.B) {
	tk := NewTank(256, 128)
	tk.Drop(64, 64, 3, 1)
	for b.Loop() {
		tk.Step()
	}
}

// BenchmarkFieldStep is the wave equation's step at the size it ran at.
func BenchmarkFieldStep(b *testing.B) {
	f := New(192, 192)
	f.Drop(96, 96, 3, 1)
	for b.Loop() {
		f.Step()
	}
}

// A splash must spread in a circle on the screen: as far across as up and
// down, and no farther on the diagonals. It did not: on a square grid
// stretched over a wide picture, a small drop was one column wide across,
// and the fine fast ripples that made spread it twice as far that way; and
// the grid's finest ripples, which lead a front, made it a square. So the
// tank on a wide picture is wide too, its cells square.
//
// How round is measured by the spread of the waves' energy: for anything
// round, (E[X⁴]+E[Y⁴])/E[R⁴] is ¾, more for a square, and E[X⁴] is E[Y⁴].
func TestARingIsRound(t *testing.T) {
	const w, h = 256, 128
	for _, steps := range []int{4, 8, 16} {
		tk := NewTank(w, h)
		tk.Speed = 0.2
		tk.Drop(w/2, h/2, 2, 1)
		for range steps {
			tk.Step()
		}
		var x4, y4, r4 float64
		for y := range h {
			for x := range w {
				e := float64(tk.h[y*w+x]) * float64(tk.h[y*w+x])
				X, Y := float64(x-w/2), float64(y-h/2)
				if X*X+Y*Y > 60*60 { // a circle that fits the tank both ways
					continue
				}
				x4 += e * X * X * X * X
				y4 += e * Y * Y * Y * Y
				r4 += e * (X*X + Y*Y) * (X*X + Y*Y)
			}
		}
		if k := (x4 + y4) / r4; k < 0.68 || k > 0.8 {
			t.Errorf("after %d steps the splash is %.3f square; ¾ is round", steps, k)
		}
		if r := x4 / y4; r < 0.85 || r > 1.18 {
			t.Errorf("after %d steps the splash spread %.2f times as far across as up", steps, r)
		}
	}
}

// Every length the tank uses must transform right: the pairs of stages
// leave one stage over when the length is an odd power of two.
func TestFFTEveryLength(t *testing.T) {
	for _, n := range []int{2, 4, 8, 16, 32, 64, 128, 256, 512} {
		f := newFFT1(n)
		re, im := make([]float32, n), make([]float32, n)
		in := make([]complex128, n)
		for i := range re {
			re[i], im[i] = float32(math.Sin(float64(i)*1.7)), float32(math.Cos(float64(i)*0.4))
			in[i] = complex(float64(re[i]), float64(im[i]))
		}
		f.row(re, im)
		for u := range n {
			var want complex128
			for x := range n {
				want += in[x] * cmplx.Exp(complex(0, -2*math.Pi*float64(u*x)/float64(n)))
			}
			if got := complex(float64(re[u]), float64(im[u])); cmplx.Abs(got-want) > 1e-3*float64(n) {
				t.Fatalf("n=%d: bin %d = %v, want %v", n, u, got, want)
			}
		}
	}
}

// A step of two sixtieths, begun on one frame and finished on the next,
// must be the same water as two steps of one: each wave is turned exactly.
// And a drop between the halves must not be lost: it goes in at the next
// Begin.
func TestASplitStepIsTwoSteps(t *testing.T) {
	one, two := NewTank(64, 32), NewTank(64, 32)
	for _, tk := range []*Tank{one, two} {
		tk.Damping, tk.Spread, tk.Speed = 0.99, 0.12, 0.5
		tk.Drop(30, 16, 3, 1)
	}
	unbounded(one, 1)
	unbounded(two, 2)
	for range 10 {
		one.Step()
		one.Step()
		two.Begin(2)
		two.Finish()
	}
	for i := range one.h {
		if d := math.Abs(float64(one.h[i] - two.h[i])); d > 1e-4 {
			t.Fatalf("cell %d: %.6f stepped twice, %.6f once for two", i, one.h[i], two.h[i])
		}
	}
	two.Begin(2)
	two.Drop(10, 10, 2, 1)
	if two.Energy() == 0 || two.Height(10, 10) == 0 {
		t.Fatal("a drop between the halves of a step is not on the water")
	}
	two.Finish()
	two.Begin(2)
	two.Finish()
	if math.Abs(float64(two.Height(10, 10))) < 1e-3 {
		t.Error("a drop between the halves of a step was lost")
	}
}
