package ripple

import (
	"math"
	"math/bits"
)

// fft1 is a one-dimensional FFT of a power-of-two length, in place on
// separate real and imaginary rows.
type fft1 struct {
	n        int
	rev      []int32
	twr, twi []float32 // the twiddles, stage by stage: the stage of half-width h at h-1
}

func newFFT1(n int) *fft1 {
	f := &fft1{n: n, rev: make([]int32, n), twr: make([]float32, n), twi: make([]float32, n)}
	lg := bits.TrailingZeros(uint(n))
	for i := range n {
		f.rev[i] = int32(bits.Reverse(uint(i)) >> (bits.UintSize - lg)) //nolint:gosec // i < n, so the reversed index is too
	}
	for h := 1; h < n; h <<= 1 {
		for k := range h {
			a := -math.Pi * float64(k) / float64(h)
			f.twr[h-1+k], f.twi[h-1+k] = float32(math.Cos(a)), float32(math.Sin(a))
		}
	}
	return f
}

// row transforms one row in place: decimation in time, two radix-2 stages
// to a pass over the row. The pair of stages is the same arithmetic as two
// passes, but each point is loaded and stored half as often, which in wasm,
// where every slice access is checked, was a third of the time.
func (f *fft1) row(re, im []float32) {
	n := f.n
	for i := range n {
		if j := int(f.rev[i]); j > i {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	h := 1
	if bits.TrailingZeros(uint(n))%2 == 1 { // an odd number of stages: one alone first, its twiddle 1
		for a := 0; a+1 < n; a += 2 {
			xr, xi := re[a+1], im[a+1]
			re[a+1], im[a+1] = re[a]-xr, im[a]-xi
			re[a] += xr
			im[a] += xi
		}
		h = 2
	}
	for ; h < n; h <<= 2 {
		// The stage of half-width h, and the one of 2h after it, whose
		// twiddle for the odd quarter is its even quarter's times -i.
		t1r := f.twr[h-1 : 2*h-1]
		t1i := f.twi[h-1 : 2*h-1][:len(t1r)]
		t2r := f.twr[2*h-1 : 3*h-1][:len(t1r)]
		t2i := f.twi[2*h-1 : 3*h-1][:len(t1r)]
		for st := 0; st < n; st += 4 * h {
			r0, i0 := re[st : st+h][:len(t1r)], im[st : st+h][:len(t1r)]
			r1, i1 := re[st+h : st+2*h][:len(t1r)], im[st+h : st+2*h][:len(t1r)]
			r2, i2 := re[st+2*h : st+3*h][:len(t1r)], im[st+2*h : st+3*h][:len(t1r)]
			r3, i3 := re[st+3*h : st+4*h][:len(t1r)], im[st+3*h : st+4*h][:len(t1r)]
			for k := range t1r {
				w1r, w1i := t1r[k], t1i[k]
				xr := r1[k]*w1r - i1[k]*w1i
				xi := r1[k]*w1i + i1[k]*w1r
				b0r, b0i := r0[k]+xr, i0[k]+xi
				b1r, b1i := r0[k]-xr, i0[k]-xi
				yr := r3[k]*w1r - i3[k]*w1i
				yi := r3[k]*w1i + i3[k]*w1r
				b2r, b2i := r2[k]+yr, i2[k]+yi
				b3r, b3i := r2[k]-yr, i2[k]-yi
				w2r, w2i := t2r[k], t2i[k]
				ur := b2r*w2r - b2i*w2i
				ui := b2r*w2i + b2i*w2r
				vr := b3r*w2i + b3i*w2r // b3·w2·(-i)
				vi := -(b3r*w2r - b3i*w2i)
				r0[k], i0[k] = b0r+ur, b0i+ui
				r2[k], i2[k] = b0r-ur, b0i-ui
				r1[k], i1[k] = b1r+vr, b1i+vi
				r3[k], i3[k] = b1r-vr, b1i-vi
			}
		}
	}
}

// fft2 is a two-dimensional FFT of a grid nx across by ny down, each a
// power of two, on separate real and imaginary planes.
//
// It transforms the rows, transposes, and transforms the rows again, rather
// than running down the columns: a column is a row's length apart at every
// step, and on a 128-wide grid walking it cost four times the rows. The
// forward transform leaves its result transposed — nx rows of ny, the row
// being the frequency across — and the inverse expects it so, which costs a
// tank nothing: everything it does in between is per wave.
type fft2 struct {
	nx, ny int
	x, y   *fft1
	tr, ti []float32 // the transpose's other side
}

func newFFT2(nx, ny int) *fft2 {
	f := &fft2{nx: nx, ny: ny, x: newFFT1(nx), tr: make([]float32, nx*ny), ti: make([]float32, nx*ny)}
	f.y = f.x
	if ny != nx {
		f.y = newFFT1(ny)
	}
	return f
}

// rows transforms each of the planes' rows of length f's.
func rows(f *fft1, re, im []float32) {
	n := f.n
	for o := 0; o+n <= len(re); o += n {
		f.row(re[o:o+n], im[o:o+n])
	}
}

// transpose turns planes of r rows of c into c rows of r, in blocks small
// enough to stay in cache.
func (f *fft2) transpose(re, im []float32, r, c int) {
	b := min(16, r, c)
	for by := 0; by < r; by += b {
		for bx := 0; bx < c; bx += b {
			for y := by; y < by+b; y++ {
				for x := bx; x < bx+b; x++ {
					f.tr[x*r+y] = re[y*c+x]
					f.ti[x*r+y] = im[y*c+x]
				}
			}
		}
	}
	copy(re, f.tr)
	copy(im, f.ti)
}

// forward transforms re, im in place, leaving the spectrum transposed.
func (f *fft2) forward(re, im []float32) {
	rows(f.x, re, im)
	f.transpose(re, im, f.ny, f.nx)
	rows(f.y, re, im)
}

// inverse undoes forward: a transposed spectrum back to the grid, scaled.
// It is the same transform of the conjugate, conjugated, taken the other
// way round, so the transpose puts the rows back where they were.
func (f *fft2) inverse(re, im []float32) {
	for i := range im {
		im[i] = -im[i]
	}
	rows(f.y, re, im)
	f.transpose(re, im, f.nx, f.ny)
	rows(f.x, re, im)
	s := 1 / float32(f.nx*f.ny)
	for i := range re {
		re[i] *= s
		im[i] *= -s
	}
}
