package uitool

import (
	"strconv"
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/chester"
)

// Every FORM the knob offers is in the README's gallery, in knob order, and
// its link brings that form up.
func TestChestFormsCoverTheKnob(t *testing.T) {
	forms := modelForms["chestahedron"]
	if len(forms) != chester.NumForms {
		t.Fatalf("%d forms in the gallery, the knob has %d", len(forms), chester.NumForms)
	}
	g := formsGallery("chestahedron")
	for i, f := range forms {
		if f.name != chester.FormNames[i] {
			t.Errorf("gallery form %d is %q, the knob's is %q", i, f.name, chester.FormNames[i])
		}
		link := formLink("chestahedron", f)
		if !strings.Contains(link, "#chestahedron&p.form="+strconv.Itoa(i)+"&") {
			t.Errorf("%s links to %s", f.name, link)
		}
		if !strings.Contains(g, link) {
			t.Errorf("the gallery has no link to %s", f.name)
		}
	}
}
