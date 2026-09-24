package acoustics

import "math"

// The synthesized tone the analyzer tests are checked against.
//
// The same helper lives in pkg/meters, where the distortion and loudness
// tests use it. Duplicated rather than shared because a test file is not
// importable, and the two analyzers that still live here — the RTA and the
// transfer function — need a signal whose content is known by construction
// rather than by another measurement.
const dtSR = 48000

// distTone builds n samples of a sine at hz with the given amplitude, plus any
// harmonics named as (multiple, amplitude) pairs. Built rather than measured,
// so what is in it is known exactly and an analyzer can be checked against
// arithmetic instead of against another analyzer.
func distTone(n int, hz, amp float64, harm ...[2]float64) []float32 {
	out := make([]float32, n)
	for i := range out {
		t := float64(i) / dtSR
		v := amp * math.Sin(2*math.Pi*hz*t)
		for _, h := range harm {
			v += h[1] * math.Sin(2*math.Pi*hz*h[0]*t)
		}
		out[i] = float32(v)
	}
	return out
}
