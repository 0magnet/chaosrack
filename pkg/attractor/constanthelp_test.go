package attractor

import (
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// Every integrated system's constant says what it is, and a constant is
// quoted in the lines it enters rather than beside the whole system: the
// fallback is for a constant the equations spell differently, not a habit.
func TestEveryConstantHasHelp(t *testing.T) {
	for _, mode := range dynamics.ParamModes() {
		for _, p := range dynamics.Params(mode) {
			h := systemConstantHelp[p.ID]
			if h == "" {
				t.Errorf("%s (%s): no help", p.ID, p.Label)
				continue
			}
			if strings.Contains(h, "system:\n") {
				t.Errorf("%s (%s) is not found in its equations", p.ID, p.Label)
			}
		}
	}
}

func TestConstantHelpQuotesItsLines(t *testing.T) {
	h := constantHelp("lorenz", "σ")
	if !strings.Contains(h, "dx/dt = σ·(y - x)") || strings.Contains(h, "dy/dt") {
		t.Errorf("lorenz σ: %q", h)
	}
}
