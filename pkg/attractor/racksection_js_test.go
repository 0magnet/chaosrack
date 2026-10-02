//go:build js && wasm

package attractor

import (
	"html"
	"regexp"
	"strings"
	"testing"
)

// Every module the panel declares has a bay.
//
// The grouping is a table rather than a class on each element, which is the
// right call — it is a statement about the instrument and belongs beside
// the signal-flow document — but it means a new module can be added and the
// table not updated, and nothing would say so. It would simply appear in
// UTILITY at the bottom of the rack, which is a plausible-looking wrong
// answer. This is the thing that says so.
func TestEveryDeclaredModuleHasABay(t *testing.T) {
	keys := declaredModuleKeys(t)
	if len(keys) < 20 {
		t.Fatalf("only found %d modules in the markup — the parser has stopped matching", len(keys))
	}
	for _, k := range keys {
		if _, ok := moduleSections[k]; !ok {
			t.Errorf("module %q has no bay; add it to moduleSections (see manual/routing.md)", k)
		}
	}
}

// And every bay names modules that exist. A key that matches nothing is a
// module that was renamed or removed, and the entry left behind is a
// grouping decision about a thing that is not there.
func TestEveryBayNamesAModuleThatExists(t *testing.T) {
	have := map[string]bool{}
	for _, k := range declaredModuleKeys(t) {
		have[k] = true
	}
	// Built at runtime rather than declared in the markup, so the parser
	// below cannot see them: the two model selectors (buildCategoryModules),
	// the Mod module (buildModMatrix), Custom's Equation editor
	// (buildCustomPanel), the Synth bay's modules (synthModules), and the
	// Mixer (mixModule).
	for _, k := range []string{"models", "model", "mod", "equation", "mixer"} {
		have[k] = true
	}
	for _, m := range synthModules {
		have[strings.ToLower(m.title)] = true
	}
	for k := range moduleSections {
		if !have[k] {
			t.Errorf("moduleSections places %q, which the panel does not declare", k)
		}
	}
}

// declaredModuleKeys is every module header in the panel (controlsBody, its
// scope copied for the others), lowercased —
// the same key rack-go derives from the header text.
func declaredModuleKeys(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`<div class="sect-hdr"[^>]*>([^<]*)</div>`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(withRackScopes(controlsBody), -1) {
		k := strings.ToLower(strings.TrimSpace(html.UnescapeString(m[1])))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// A module with a screen in it leads a bay, and bayScreens is how the
// packer knows which those are outside the model rows.
//
// The list is two entries and could have been a query — "does this module
// contain a canvas" — except that the packer runs on slot counts before
// anything has been laid out, and a module quietly acquiring a canvas
// should move it in the rack only on purpose. So: a list, and this, which
// fails if the markup and the list stop agreeing.
func TestEveryModuleWithAScreenLeadsABay(t *testing.T) {
	// Every module starts at a .sect (the scope was once an instrument panel
	// outside them, and its tube was not the Console's screen; it is two
	// modules now, and the first carries its own header and canvas).
	start := regexp.MustCompile(`<div class="sect[" ]`)
	hdr := regexp.MustCompile(`<div class="sect-hdr"[^>]*>([^<]*)</div>`)
	markup := withRackScopes(controlsBody)
	at := start.FindAllStringIndex(markup, -1)
	if len(at) < 20 {
		t.Fatalf("only found %d modules in the markup — the parser has stopped matching", len(at))
	}
	var withScreen []string
	for i, m := range at {
		end := len(markup)
		if i+1 < len(at) {
			end = at[i+1][0]
		}
		body := markup[m[0]:end]
		h := hdr.FindStringSubmatch(body)
		if h == nil || !strings.Contains(body, "<canvas") {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(html.UnescapeString(h[1])))
		withScreen = append(withScreen, k)
		if _, ok := bayScreens[k]; !ok {
			t.Errorf("module %q has a screen in it but is not in bayScreens: add it, leading a bay or not", k)
		}
	}
	for k := range bayScreens {
		found := false
		for _, s := range withScreen {
			if s == k {
				found = true
			}
		}
		if !found {
			t.Errorf("bayScreens names %q, which has no screen in it", k)
		}
	}
}
