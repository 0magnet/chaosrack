package attractor

import (
	"strings"
	"testing"
)

// The rack is not always the whole page. On magnetosphere.net it is the
// backdrop of a store whose sections are plain anchors, and it used to
// overwrite the fragment with its own state a moment after boot — so every
// shared link to #about landed on the globe, and the section never appeared
// because the CSS that reveals it keys on :target.
//
// This is the rule that decides whether the address bar is ours to write.
func TestIsAppHashTellsAPermalinkFromAnAnchor(t *testing.T) {
	for _, c := range []struct {
		hash string
		want bool
		why  string
	}{
		// Ours: a mode token and at least one key=value pair.
		{"#lorenz&ry=0.1", true, "a plain permalink"},
		{"lorenz&ry=0.1", true, "the same without the leading #"},
		{"#globe&ry=0.1&rx=2", true, "several parameters"},
		{"#terminal&x=1", true, "a mode that is not a model"},

		// Not ours: somebody else's anchor.
		{"#about", false, "a store section"},
		{"#links", false, "a store section"},
		{"#policy", false, "a store section"},
		{"#home", false, "a store section"},
		{"#cat-resistor", false, "a category anchor, hyphen and all"},
		{"", false, "no fragment at all"},
		{"#", false, "an empty fragment"},

		// The boundary. A bare model name is the rack's own link to a model at
		// its defaults, which it writes itself; a bare word that is no model is
		// somebody else's.
		{"#lorenz", true, "a model at its defaults, as the rack writes it"},
		{"#waterfall", true, "the same for an audio model"},
		{"#lorenzo", false, "a word that is no model"},
	} {
		if got := isAppHash(c.hash); got != c.want {
			t.Errorf("isAppHash(%q) = %v, want %v — %s", c.hash, got, c.want, c.why)
		}
	}
}

// A link to a model that was folded into another opens that model, set to be
// it, and keeps everything else it carried.
func TestALinkToAFoldedModelOpensWhatItIsNow(t *testing.T) {
	for in, want := range map[string]string{
		"#cube":                     "#polyhedron&p.p=4&p.q=3",
		"#icosahedron&rot=1,2,3":    "#polyhedron&p.p=3&p.q=5&rot=1,2,3",
		"#dodecahedron&p.poly-op=2": "#polyhedron&p.p=5&p.q=3&p.op=2",
		"#sphere&p.slices=24&p.stacks=9&p.r=2&ar=0": "#globe&p.lat=0&p.lon=24&ar=0",
		"#lorenz&p.s=10": "#lorenz&p.s=10",
		"#about":         "#about",
		"":               "",
	} {
		if got := migrateHash(in); got != want {
			t.Errorf("migrateHash(%q) = %q, want %q", in, got, want)
		}
	}
	// Every folded model lands on a model that exists.
	for from, to := range foldedModes {
		if m, _, _ := strings.Cut(to, "&"); !knownMode(m) {
			t.Errorf("%s folds into %q, which is not a model", from, m)
		}
		if knownMode(from) {
			t.Errorf("%s is folded but still a model of its own", from)
		}
	}
}
