package attractor

import (
	"strings"
	"testing"
)

// The scope is copied for the others under ids of their own, titled in
// turn, and nothing outside it changes.
func TestWithRackScopesCopiesTheScopeUnderItsOwnIDs(t *testing.T) {
	body := "<p id=\"scope-grat\">before</p>\n" +
		`<div class="sect" id="scope-module"><div class="sect-hdr" title="Scope 1 — a tube">Scope 1</div>` +
		"\n" + `<div class="row"><canvas id="scope-screen"></canvas><button class="rst scope-mini" id="rst-scope-in1"></button>` +
		"\n</div></div>\n<p>after</p>"
	got := withRackScopes(body)
	for _, want := range []string{
		`id="scope-screen"`, `id="scope2-screen"`, `id="scope4-screen"`, `id="rst-scope3-in1"`,
		`title="Scope 3 — a tube">Scope 3</div>`, `id="scope-grat"`, `class="rst scope-mini"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
	if n := strings.Count(got, "<canvas"); n != rackScopeCount {
		t.Errorf("%d tubes, want %d", n, rackScopeCount)
	}
	if !strings.HasSuffix(got, "<p>after</p>") || strings.Count(got, "before") != 1 {
		t.Errorf("the markup around the scope changed:\n%s", got)
	}
	if withRackScopes("<p>no scope</p>") != "<p>no scope</p>" {
		t.Error("a body without the scope was changed")
	}
}
