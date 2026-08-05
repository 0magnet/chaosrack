// Small math + crypto-random helpers used across the package.
package stlview

import (
	"crypto/rand"
	"errors"
	m "math"
	"strconv"
)

func getMaxScalar(vertices []float32) float32 {
	var max float32
	for baseIndex := 0; baseIndex < len(vertices); baseIndex += 3 {
		testScale := scalar(vertices[baseIndex], vertices[baseIndex], vertices[baseIndex])
		if testScale > max {
			max = testScale
		}
	}
	return max
}

func scalar(x float32, y float32, z float32) float32 {
	xy := m.Sqrt(float64(x*x + y*y))
	return float32(m.Sqrt(xy*xy + float64(z*z)))
}

func cryptoRandFloat32() float32 {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		panic("crypto/rand read failed: " + err.Error())
	}
	u := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return float32(u) / float32(m.MaxUint32)
}

func cryptoRandIntn(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("max must be a positive integer")
	}
	numBytes := (max + 7) / 8
	maxBytes := 1 << (numBytes * 8)
	randBytes := make([]byte, numBytes)
	randNum := 0
	for {
		_, err := rand.Read(randBytes)
		if err != nil {
			return 0, errors.New("error generating random number")
		}
		for _, b := range randBytes {
			randNum = (randNum << 8) | int(b)
		}
		if randNum < maxBytes-maxBytes%max {
			break
		}
	}
	return randNum % max, nil
}

func f32(f float32, g byte, prec, bitSize int) string {
	return strconv.FormatFloat(float64(f), g, prec, bitSize)
}

func f64(f float64, g byte, prec, bitSize int) string {
	return strconv.FormatFloat(f, g, prec, bitSize)
}
