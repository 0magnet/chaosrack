package attractor

import "strings"

// isAppHash reports whether a URL fragment is one of ours.
//
// A permalink is a mode token, followed by &key=value pairs for whatever is
// not at its default. With everything at its default it is the bare token,
// #lorenz, which is what the rack writes itself; so a bare word is ours when
// it names a model the rack has. Anything else — #about, #links, a plain
// anchor on a page this rack is only part of — belongs to whoever put it
// there.
//
// A bare model name used to count as somebody else's, and since the rack
// writes exactly that, reloading a link to a model at its defaults left the
// rack never writing the fragment again for the rest of the session.
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
	parts := strings.Split(h, "&")
	return len(parts) >= 2 || knownMode(parts[0])
}

// foldedModes are models that became settings of another, and what they are
// there. A link is a promise that what was shared is what is seen, so a link
// to one of these opens the model it is now, set to be it.
//
// The five Platonic solids are the Polyhedron at their Schläfli symbols; the
// Sphere was the Globe without its parallels, so it is the Globe at lat 0.
var foldedModes = map[string]string{
	"tetrahedron":  "polyhedron&p.p=3&p.q=3",
	"cube":         "polyhedron&p.p=4&p.q=3",
	"octahedron":   "polyhedron&p.p=3&p.q=4",
	"dodecahedron": "polyhedron&p.p=5&p.q=3",
	"icosahedron":  "polyhedron&p.p=3&p.q=5",
	"sphere":       "globe&p.lat=0",
}

// foldedKeys renames a folded model's own parameter keys to the ones they
// became. The Sphere's meridian count is the Globe's lon, and its stacks and
// radius have no counterpart (the Globe's meridians are already smooth; zoom
// is the size).
var foldedKeys = map[string]map[string]string{
	"sphere": {"p.slices": "p.lon", "p.stacks": "", "p.r": ""},
	// The solids' operator was poly-op, which no link could reach: a link's
	// p.KEY is looked up as MODEL-KEY. On the Polyhedron it is polyhedron-op.
	"tetrahedron":  {"p.poly-op": "p.op"},
	"cube":         {"p.poly-op": "p.op"},
	"octahedron":   {"p.poly-op": "p.op"},
	"dodecahedron": {"p.poly-op": "p.op"},
	"icosahedron":  {"p.poly-op": "p.op"},
}

// migrateHash rewrites a link to a folded model as a link to what it is now,
// keeping everything else it carries. Anything else comes back unchanged.
func migrateHash(h string) string {
	body := strings.TrimPrefix(h, "#")
	parts := strings.Split(body, "&")
	to, ok := foldedModes[parts[0]]
	if !ok {
		return h
	}
	rename := foldedKeys[parts[0]]
	out := []string{to}
	for _, p := range parts[1:] {
		k, v, _ := strings.Cut(p, "=")
		if nk, ok := rename[k]; ok {
			if nk == "" {
				continue
			}
			p = nk + "=" + v
		}
		out = append(out, p)
	}
	res := strings.Join(out, "&")
	if strings.HasPrefix(h, "#") {
		res = "#" + res
	}
	return res
}
