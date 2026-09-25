// Small math + crypto-random helpers used across the package.

package stlview

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	m "math"
	"strconv"
)

func getMaxScalar(vertices []float32) float32 { //nolint:unused // used only by the js build, which the native lint pass cannot see
	var hi float32
	for baseIndex := 0; baseIndex < len(vertices); baseIndex += 3 {
		testScale := scalar(vertices[baseIndex], vertices[baseIndex], vertices[baseIndex])
		if testScale > hi {
			hi = testScale
		}
	}
	return hi
}

func scalar(x float32, y float32, z float32) float32 { //nolint:unused // used only by the js build, which the native lint pass cannot see
	xy := m.Sqrt(float64(x*x + y*y))
	return float32(m.Sqrt(xy*xy + float64(z*z)))
}

func cryptoRandFloat32() float32 { //nolint:unused // used only by the js build, which the native lint pass cannot see
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		panic("crypto/rand read failed: " + err.Error())
	}
	u := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return float32(u) / float32(m.MaxUint32)
}

// cryptoRandIntn returns a uniform value in [0, max) using rejection sampling.
//
// It used to accumulate its draw in a variable declared OUTSIDE the retry
// loop: a rejected draw was not discarded but shifted left eight bits with the
// next byte OR'd in. Having been rejected once for being too large, the value
// could then only grow, so the accept test could never pass again and the loop
// spun — shifting a byte in per turn — until the value overflowed into a
// NEGATIVE number, which passed the "< threshold" test and was returned as a
// negative modulo. Callers use the result as a count or an index, and
// stlview.NewSTL panicked outright on the empty gradient a negative count
// produced. With max=5 the initial rejection chance is 1/256, so loading forty
// models tripped it about one run in six.
func cryptoRandIntn(hi int) (int, error) {
	if hi <= 0 {
		return 0, errors.New("max must be a positive integer")
	}
	m := uint64(hi)
	// Reject the top partial bucket so every value is equally likely.
	limit := ^uint64(0) - (^uint64(0)%m+1)%m
	var b [8]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			return 0, errors.New("error generating random number")
		}
		// Rebuilt from scratch every attempt — that is the whole fix.
		n := binary.BigEndian.Uint64(b[:])
		if n <= limit {
			// n%m < m == uint64(max), and max is a positive int, so the
			// result is in [0, max) and cannot overflow int.
			return int(n % m), nil //nolint:gosec // bounded by m above
		}
	}
}

// f32 formats f to prec decimals, or as few as it needs when prec is -1.
func f32(f float32, prec int) string { //nolint:unused // used only by the js build, which the native lint pass cannot see
	return strconv.FormatFloat(float64(f), 'f', prec, 64)
}

// f64 formats f to two decimals, rounded as a float32.
func f64(f float64) string { //nolint:unused // used only by the js build, which the native lint pass cannot see
	return strconv.FormatFloat(f, 'f', 2, 32)
}
