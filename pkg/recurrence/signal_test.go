package recurrence

import (
	"math"
	"math/rand"
)

// sineWithNoise is a period-sample sine with Gaussian noise of the given
// amplitude: a pure tone, but not an unrealistically clean one.
func sineWithNoise(n int, period, noise float64, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // a deterministic test signal, not a secret
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Sin(2*math.Pi*float64(i)/period) + noise*rng.NormFloat64()
	}
	return x
}
