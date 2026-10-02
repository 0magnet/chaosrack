package attractor

import (
	"strings"

	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The mode descriptions — the prose the info overlay shows and the README's
// model reference is generated from. They are the manual's (models.md, each
// as model.<mode>), and this is them by mode. Untagged, next to the mode
// registry in modes.go, so the native tools can read them: cmd/uitool writes
// the README from this map. Modes whose description is composed from data
// (the Sprott catalog) register in the init below.
var attractorDescriptions = func() map[string]string {
	m := map[string]string{}
	for _, k := range manual.Keys() {
		if mode, ok := strings.CutPrefix(k, "model."); ok {
			m[mode] = manual.Text(k)
		}
	}
	return m
}()

func init() {
	// Composed from the catalog data rather than written out nineteen times.
	for _, c := range dynamics.SprottCases {
		attractorDescriptions[c.Key] = c.Name +
			" — one of J. C. Sprott's simple chaotic flows (1994), realized as an" +
			" analog circuit at glensstuff.com. Found by systematic search for the" +
			" algebraically simplest systems that still produce chaos.\n\n" + c.Eq
	}
}
