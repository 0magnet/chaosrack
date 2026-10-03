//go:build js && wasm

package attractor

import (
	"regexp"
	"strings"
	"testing"
)

// A button does something exactly when it is labeled and explained: every
// legend has a tooltip, and a button with no legend is inert (assigned)
// rather than quietly doing what its neighbor does.
func TestTrioProgramsLabelWhatTheyDo(t *testing.T) {
	for param, p := range trioPrograms {
		l := p.legends()
		for i, k := range l {
			if k != "" && i >= len(p.help) {
				t.Errorf("%s: button %d (%q) has no tooltip", param, i, k)
			}
		}
		if len(p.help) > len(l) {
			t.Errorf("%s: %d tooltips for %d buttons", param, len(p.help), len(l))
		}
		if p.press == nil || (p.lit == nil && p.lits == nil) {
			t.Errorf("%s: a program presses and lights", param)
		}
	}
}

// settingNames stand for a selector's options one for one, and fit the
// display they are shown on.
func TestSettingNamesMatchTheirSelectors(t *testing.T) {
	for id, names := range settingNames {
		re := regexp.MustCompile(`(?s)<select id="` + regexp.QuoteMeta(id) + `"[^>]*>(.*?)</select>`)
		m := re.FindStringSubmatch(controlsBody)
		if m == nil {
			t.Errorf("%s: no such select in the panel", id)
			continue
		}
		if n := strings.Count(m[1], "<option"); n != len(names) {
			t.Errorf("%s: %d names for %d options", id, len(names), n)
		}
		for _, s := range names {
			if len([]rune(s)) > bankValChars {
				t.Errorf("%s: %q is longer than a display holds (%d)", id, s, bankValChars)
			}
		}
	}
}
