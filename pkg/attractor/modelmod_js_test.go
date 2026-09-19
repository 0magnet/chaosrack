//go:build js && wasm

package attractor

import "testing"

// Every source offered on the dial must be one the value lookup knows, or a
// position on the selector does nothing and says nothing about why.
func TestEveryOfferedSourceIsOneTheRouterCanResolve(t *testing.T) {
	for _, c := range modChannels {
		if c.name == "" {
			continue // the off position
		}
		known := isModelModSource(c.name)
		switch c.name {
		case "mono", "L", "R":
			known = true
		}
		if !known {
			t.Errorf("the dial offers %q (%s) and nothing resolves it", c.name, c.label)
		}
		if c.desc == "" {
			t.Errorf("source %q has no description, so its dial position has no tooltip", c.name)
		}
	}
}
