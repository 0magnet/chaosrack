package attractor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The join between Go and the stylesheet, which nothing else checks.
//
// A class name is a string in Go and a selector in CSS, and the two are
// connected by nothing at all. Misspell one in Go and the element quietly
// gets no styling; delete the last user of a rule and the rule stays
// forever. Neither shows up in a build, a test or a lint — the panel just
// looks slightly wrong, or carries rules nobody can safely remove because
// there is no way to know whether anything still uses them.
//
// Both directions are budgets rather than absolutes. Some classes are
// legitimately one-sided — rack-go sets its own, a couple are hooks a
// query uses and nothing styles — and the point is not that the number is
// zero but that it cannot drift upward without someone saying so.

var (
	// A class name in a selector. Crude on purpose: it reads .name wherever
	// it appears, so a descendant selector contributes each of its parts.
	cssClassRe = regexp.MustCompile(`\.(-?[_a-zA-Z][_a-zA-Z0-9-]*)`)

	// The class strings Go sets, the three ways the panel sets them.
	goClassRe = regexp.MustCompile(`class(?:Name)?="([^"]*)"` +
		`|"className", "([^"]*)"` +
		`|Call\("classList"\)\.Call\("(?:add|remove|toggle)", "([^"]+)"`)

	// What a class name can look like, used to throw out the fragments a
	// regex literal in the Go source otherwise contributes.
	classNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

func cssClasses(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, block := range strings.Split(readPanelCSS(t), "}") {
		sel, _, ok := strings.Cut(block, "{")
		if !ok {
			continue
		}
		for _, m := range cssClassRe.FindAllStringSubmatch(sel, -1) {
			out[m[1]] = true
		}
	}
	return out
}

func goClasses(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, src := range panelGoSources(t) {
		for _, m := range goClassRe.FindAllStringSubmatch(src, -1) {
			for _, g := range m[1:] {
				for _, c := range strings.Fields(g) {
					if classNameRe.MatchString(c) {
						out[c] = true
					}
				}
			}
		}
	}
	return out
}

// TestEveryStyledClassIsEmitted finds rules for classes nothing produces:
// dead CSS, or a name renamed on one side only.
//
// "Produced" is looser than the regex above, because Go writes class names
// several ways a pattern cannot catch — a named constant (unitClass), a
// prefix joined to a variable ("ks-"+style), an attribute set by hand. So a
// class counts as produced if its name appears anywhere in the package's Go
// source as a whole token. That is generous, and deliberately: a false
// "this is dead" would get a live rule deleted.
func TestEveryStyledClassIsEmitted(t *testing.T) {
	const budget = 2
	src := strings.Join(panelGoSources(t), "\n")
	var orphan []string
	for c := range cssClasses(t) {
		if classAllowed(c) {
			continue
		}
		if regexp.MustCompile(`[^a-zA-Z0-9_-]` + regexp.QuoteMeta(c) + `[^a-zA-Z0-9_-]`).MatchString(src) {
			continue
		}
		orphan = append(orphan, c)
	}
	sort.Strings(orphan)
	if len(orphan) > budget {
		t.Errorf("%d styled classes that no Go source mentions (budget %d).\n"+
			"Either the rule is dead and should go, or the name was changed on one side:\n  %s",
			len(orphan), budget, strings.Join(orphan, "\n  "))
	}
	if len(orphan) < budget-3 {
		t.Errorf("only %d orphaned rules (budget %d) — lower the budget to lock it in", len(orphan), budget)
	}
}

// TestEveryEmittedClassIsStyled is the other direction: a class Go sets that
// no rule matches. Usually a typo, occasionally a hook something queries.
func TestEveryEmittedClassIsStyled(t *testing.T) {
	const budget = 8
	styled := cssClasses(t)
	var unstyled []string
	for c := range goClasses(t) {
		if !styled[c] && !classAllowed(c) {
			unstyled = append(unstyled, c)
		}
	}
	sort.Strings(unstyled)
	if len(unstyled) > budget {
		t.Errorf("%d classes Go sets that no rule matches (budget %d).\n"+
			"A misspelled class is silent: the element simply comes out unstyled:\n  %s",
			len(unstyled), budget, strings.Join(unstyled, "\n  "))
	}
	if len(unstyled) < budget-3 {
		t.Errorf("only %d unstyled classes (budget %d) — lower the budget to lock it in", len(unstyled), budget)
	}
}

// classAllowed names the classes that are legitimately one-sided: rack-go
// manages its own, and the panel sets a couple purely as query hooks.
func classAllowed(c string) bool {
	switch c {
	case "rack-frame", "rack-mod", "rack-mod-hdr", "rack-grid":
		return true
	}
	return false
}

// panelGoSources is every Go file in the package, read as TEXT.
//
// As text because most of the panel is in js/wasm-tagged files a host test
// cannot compile, and because the markup is a string constant either way:
// what is wanted is the class names written down, not the program.
func panelGoSources(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(filepath.Dir(panelCSSPath), "*.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(names) < 50 {
		t.Fatalf("found %d Go files beside the stylesheet — the glob has stopped matching", len(names))
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		// The glob is this package's own directory, fixed above.
		b, err := os.ReadFile(n) //nolint:gosec // a test reading the package it tests
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		out = append(out, string(b))
	}
	return out
}
