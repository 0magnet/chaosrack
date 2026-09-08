package attractor

import "strings"

// isAppHash reports whether a URL fragment is one of ours.
//
// A permalink is a mode token followed by at least one &key=value pair, which
// is what applyStateFrom requires before it will read anything from a hash.
// Anything else — #about, #links, a plain anchor on a page this rack is only
// part of — belongs to whoever put it there.
//
// It lives here rather than beside the rest of the permalink code because it
// is pure string handling with no js in it, and that file cannot be built or
// tested off js/wasm. The rule it encodes is the one thing in this area worth
// pinning down with a test.
func isAppHash(h string) bool {
	h = strings.TrimPrefix(h, "#")
	if h == "" {
		return false
	}
	return len(strings.Split(h, "&")) >= 2
}
