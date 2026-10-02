//go:build js && wasm

package attractor

import "testing"

// Every source the Mod matrix offers must be one the value lookup knows, or a
// row of pins does nothing and says nothing about why.
func TestEveryOfferedSourceIsOneTheRouterCanResolve(t *testing.T) {
	for _, c := range modChannels {
		known := isModelModSource(c.name)
		switch c.name {
		case "mono", "L", "R", modSrcSendA, modSrcSendB:
			known = true
		}
		if !known {
			t.Errorf("the matrix offers %q (row %s) and nothing resolves it", c.name, c.key)
		}
		if doc("mod-src="+c.key) == "" {
			t.Errorf("source %q has no entry in the manual, so its column has no tooltip", c.name)
		}
	}
}
