package dsputils

import (
	"math"
)

const (
	closeFactor = 1e-8
)

// PrettyClose returns true if the slices a and b are very close, else false.
func PrettyClose(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !Float64Equal(c, b[i]) {
			return false
		}
	}
	return true
}

// PrettyCloseC returns true if the slices a and b are very close, else false.
func PrettyCloseC(a, b []complex128) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !ComplexEqual(c, b[i]) {
			return false
		}
	}
	return true
}

// PrettyClose2 returns true if the matrixes a and b are very close, else false.
func PrettyClose2(a, b [][]complex128) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !PrettyCloseC(c, b[i]) {
			return false
		}
	}
	return true
}

// PrettyClose2F returns true if the matrixes a and b are very close, else false.
func PrettyClose2F(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !PrettyClose(c, b[i]) {
			return false
		}
	}
	return true
}

// ComplexEqual returns true if a and b are very close, else false.
func ComplexEqual(a, b complex128) bool {
	r_a := real(a)
	r_b := real(b)
	i_a := imag(a)
	i_b := imag(b)

	return Float64Equal(r_a, r_b) && Float64Equal(i_a, i_b)
}

// Float64Equal returns true if a and b are very close, else false.
func Float64Equal(a, b float64) bool {
	return math.Abs(a-b) <= closeFactor || math.Abs(1-a/b) <= closeFactor
}
