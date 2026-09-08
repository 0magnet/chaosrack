package attractor

import "testing"

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

		// The boundary. A bare mode token with no parameters is not a
		// permalink either: applyStateFrom reads nothing from it, so writing
		// over it would discard a fragment for no gain.
		{"#lorenz", false, "a bare word, whoever meant it"},
	} {
		if got := isAppHash(c.hash); got != c.want {
			t.Errorf("isAppHash(%q) = %v, want %v — %s", c.hash, got, c.want, c.why)
		}
	}
}
