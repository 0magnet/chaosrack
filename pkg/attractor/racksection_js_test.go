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
			t.Errorf("module %q has no bay; add it to moduleSections (see docs/signal-flow.md)", k)
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
	// Built at runtime (buildPatchbayModule, buildTemplateModule) rather than
	// declared in the markup, so the parser
	// below cannot see them.
	for _, k := range []string{"patchbay", "template"} {
		have[k] = true
	}
	for k := range moduleSections {
		if !have[k] {
			t.Errorf("moduleSections places %q, which the panel does not declare", k)
		}
	}
}

// declaredModuleKeys is every module header in controlsBody, lowercased —
// the same key rack-go derives from the header text.
func declaredModuleKeys(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`<div class="sect-hdr"[^>]*>([^<]*)</div>`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(controlsBody, -1) {
		k := strings.ToLower(strings.TrimSpace(html.UnescapeString(m[1])))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}
