package attractor

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Guards on the panel stylesheet.
//
// panel.css is 1400 lines that every module's layout comes out of, and it
// had grown two faults that reading does not catch and a diff does not show:
// rules that style nothing, and rules arguing with each other through
// !important. Both were found by measuring, and both will come back unless
// something fails when they do.
//
// `uitool css` reports the same things against a running browser, which is
// the only way to ask "does any element anywhere match this". These are the
// half that needs no browser, so they run on every `go test`.
//
// The counts are RATCHETS, not targets. They are what the file has today;
// the test fails if a change makes one worse, and the number is meant to be
// lowered as the file improves. Raising one is a deliberate act that shows
// up in review.

const panelCSSPath = "panel.css"

func readPanelCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(panelCSSPath)
	if err != nil {
		t.Fatalf("read %s: %v", panelCSSPath, err)
	}
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(b), "")
}

type cssRule struct{ sel, body string }

func panelRules(t *testing.T) []cssRule {
	t.Helper()
	var out []cssRule
	for _, m := range regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`).FindAllStringSubmatch(readPanelCSS(t), -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "" || strings.HasPrefix(sel, "@") ||
			regexp.MustCompile(`^-?[0-9.]+%(\s*,\s*-?[0-9.]+%)*$`).MatchString(sel) {
			continue
		}
		out = append(out, cssRule{sel, m[2]})
	}
	return out
}

// An unbalanced brace turns every rule after it into garbage that the
// browser silently drops, and nothing in Go's build notices. This has
// already happened twice while editing the file by script.
func TestThePanelStylesheetIsWellFormed(t *testing.T) {
	src := readPanelCSS(t)
	if o, c := strings.Count(src, "{"), strings.Count(src, "}"); o != c {
		t.Fatalf("%d open braces and %d close — the stylesheet is truncated or a rule is unclosed", o, c)
	}
	// An empty functional pseudo is a selector error, and a selector error
	// drops the whole rule. `uitool css` reported one of these against
	// itself when it stripped :checked out of :not(:checked).
	if m := regexp.MustCompile(`:(not|has|is|where)\(\s*\)`).FindString(src); m != "" {
		t.Errorf("empty %q — a selector error drops the entire rule silently", m)
	}
}

// !important is how a stylesheet stops being readable: once one rule needs
// it, every rule that has to win against that one needs it too. A third of
// this file carries it, which is the state this ratchet exists to stop
// getting worse.
func TestImportantDoesNotSpread(t *testing.T) {
	const budget = 154 // lower this as the file improves; never raise it casually
	got := strings.Count(readPanelCSS(t), "!important")
	if got > budget {
		t.Errorf("%d !important declarations, budget %d.\n"+
			"Adding one usually means a rule above is claiming something it should not.\n"+
			"Check what you are overriding before raising this number.", got, budget)
	}
	if got < budget-10 {
		t.Errorf("only %d !important left (budget %d) — lower the budget in this test to lock the improvement in", got, budget)
	}
}

// The same property set for the same selector in two places is where a
// change stops being predictable: which one wins depends on source order,
// and neither mentions the other.
func TestAPropertyIsNotSetTwiceForOneSelector(t *testing.T) {
	const budget = 32 // as above: a ratchet, not a target

	props := map[string]map[string]int{}
	for _, r := range panelRules(t) {
		for _, s := range strings.Split(r.sel, ",") {
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			if props[s] == nil {
				props[s] = map[string]int{}
			}
			for _, d := range strings.Split(r.body, ";") {
				if i := strings.Index(d, ":"); i > 0 {
					props[s][strings.TrimSpace(d[:i])]++
				}
			}
		}
	}
	var dup []string
	for s, ps := range props {
		for p, n := range ps {
			if n > 1 {
				dup = append(dup, s+" { "+p+" } x"+strings.Repeat("I", n))
			}
		}
	}
	sort.Strings(dup)
	if len(dup) > budget {
		t.Errorf("%d properties set more than once for one selector, budget %d:\n  %s",
			len(dup), budget, strings.Join(dup[:min(12, len(dup))], "\n  "))
	}
	if len(dup) < budget-5 {
		t.Errorf("only %d duplicates left (budget %d) — lower the budget to lock it in", len(dup), budget)
	}
}

// A flex property on a grid container does nothing, and reads as though it
// does. .vmrow carried flex-flow, and three rules argued about
// justify-content on a container two OTHER rules disagreed about the display
// of — which is why every layout change in that file had to be confirmed in
// a browser rather than read off the source.
//
// Only the properties that are genuinely inert are listed. align-items,
// justify-content, align-content and gap all mean something in grid; flex
// and its shorthands do not.
func TestNoFlexOnlyPropertyOnAGridContainer(t *testing.T) {
	inert := map[string]bool{
		"flex-flow": true, "flex-direction": true, "flex-wrap": true,
	}
	display := map[string]string{} // selector -> last display it is given
	sets := map[string][]string{}  // selector -> inert properties set on it

	for _, r := range panelRules(t) {
		for _, s := range strings.Split(r.sel, ",") {
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			for _, d := range strings.Split(r.body, ";") {
				i := strings.Index(d, ":")
				if i <= 0 {
					continue
				}
				prop := strings.TrimSpace(d[:i])
				val := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d[i+1:]), "!important"))
				switch {
				case prop == "display":
					display[s] = val
				case inert[prop]:
					sets[s] = append(sets[s], prop)
				}
			}
		}
	}

	var bad []string
	for s, ps := range sets {
		if strings.HasPrefix(display[s], "grid") || strings.HasPrefix(display[s], "inline-grid") {
			bad = append(bad, s+" is display:"+display[s]+" but sets "+strings.Join(ps, ", "))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("flex-only properties on a grid container — they do nothing and they mislead:\n  %s",
			strings.Join(bad, "\n  "))
	}
}
