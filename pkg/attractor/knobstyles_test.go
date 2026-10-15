package attractor

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every KNOB style's colors are the page's: its body's middle stop, its
// border and its pointer, as panel.css paints them.
func TestKnobStylesAreThePagesColors(t *testing.T) {
	css, err := os.ReadFile("panel.css")
	if err != nil {
		t.Fatal(err)
	}
	hex := func(v int) string { return fmt.Sprintf("#%06x", v) }
	// has reports whether s spells color v, in six digits or, where they
	// pair up, three.
	has := func(s, prefix string, v int) bool {
		h := hex(v)
		if strings.Contains(s, prefix+h) {
			return true
		}
		if h[1] == h[2] && h[3] == h[4] && h[5] == h[6] {
			short := "#" + string(h[1]) + string(h[3]) + string(h[5])
			return regexp.MustCompile(regexp.QuoteMeta(prefix+short) + `\b`).MatchString(s)
		}
		return false
	}
	for name, ks := range KnobStyles {
		var knob, ptr string
		if name == "std" {
			knob = regexp.MustCompile(`(?m)^\.knob\{[^}]*\}`).FindString(string(css))
			ptr = regexp.MustCompile(`(?m)^\.knob-ptr\{[^}]*\}`).FindString(string(css))
		} else {
			knob = regexp.MustCompile(`\.ks-` + name + ` \.knob,[^{]*\{[^}]*\}`).FindString(string(css))
			ptr = regexp.MustCompile(`\.ks-` + name + ` \.knob-ptr\{[^}]*\}`).FindString(string(css))
		}
		if knob == "" || ptr == "" {
			t.Errorf("%s: no rules in panel.css", name)
			continue
		}
		_, bg, ok := strings.Cut(knob, "background:")
		if !ok {
			t.Errorf("%s: no background in %s", name, knob)
			continue
		}
		if !has(bg, "", ks.Body) {
			t.Errorf("%s: body %s is not a stop of %s", name, hex(ks.Body), knob)
		}
		if !has(knob, "solid ", ks.Border) {
			t.Errorf("%s: border %s is not %s", name, hex(ks.Border), knob)
		}
		if !has(ptr, "--pcol:", ks.Pointer) {
			t.Errorf("%s: pointer %s is not %s", name, hex(ks.Pointer), ptr)
		}
	}
}
