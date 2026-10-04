package attractor

import (
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/manual"
)

// The Sprott flows' prose is composed from their equations; it reads as a
// sentence (no "−0", no capital mid-sentence) and says what the equations do.
func TestSprottProseReadsTheEquations(t *testing.T) {
	for k, want := range map[string]string{
		"sprottb": "Five terms, two of them nonlinear (yz and xy). ∇·F = −1 everywhere",
		"sprottd": "∇·F = x, which changes sign at x = 0:",
		"sprottk": "volume grows where y > 0.7",
		"sprottf": "by a factor of e every 2 time units",
	} {
		if d := attractorDescriptions[k]; !strings.Contains(d, want) {
			t.Errorf("%s: want %q in\n%s", k, want, d)
		}
	}
	for k, d := range attractorDescriptions {
		if strings.HasPrefix(k, "sprott") && (strings.Contains(d, "−0:") || strings.Contains(d, ", Two")) {
			t.Errorf("%s: %q", k, d)
		}
	}
}

func TestComposedDescriptionsRenderAsTheManualDoes(t *testing.T) {
	h := manual.Render(descriptionMarkdown(attractorDescriptions["sprottb"]))
	if strings.Count(h, "<p>") != 2 || !strings.Contains(h, "<pre><code class=\"language-text\">dx/dt = yz\ndy/dt") {
		t.Errorf("Sprott B rendered as\n%s", h)
	}
}
