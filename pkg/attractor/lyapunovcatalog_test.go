package attractor

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/analysis"
)

// Every model that IS a dynamical system must be measurable, and at its
// shipped defaults must be chaotic. This is the mode-defaults guard extended
// to the maps, which arrived with three bad defaults between them.
func TestEveryDynamicalModeMeasuresChaotic(t *testing.T) {
	measured := 0
	for _, k := range CatalogKeys() {
		r := analysis.LyapunovFor(k)
		if r.Verdict == "n/a" {
			continue
		}
		measured++
		if !r.OK {
			t.Errorf("%s: could not be measured (%s)", k, r.Verdict)
			continue
		}
		if r.Verdict != "chaotic" {
			t.Errorf("%s: λ=%.4f reads %q at its defaults", k, r.Lambda, r.Verdict)
		}
	}
	if measured < 30 {
		t.Errorf("only %d modes were measurable; the registries are probably not loaded", measured)
	}
}
