package attractor

// The mode registry — THE single source of mode identity. Before this table,
// adding a mode meant touching ~14 registration sites (the <select> HTML, the
// generateForMode switch, isAttractorMode / isSkinnable / isAudioMode, the
// hash-mode validation list, the params/IC/description maps, and the uitool
// tour lists). Now: one ModeInfo row + one modeGroups placement (+ a
// registerGenerate call beside the mode's generator), and everything else
// derives. Untagged so the native tests and cmd/uitool can read it too.

// ModeClass is the mode's broad behavior family.
type ModeClass int

const (
	ClassFlow3D     ModeClass = iota // 3-D ODE flow with a persistent trail
	ClassFlow4D                      // 4-D flow (hidden state) — equation-engine or bespoke
	ClassParametric                  // parametric curve traced in time (lissajou, graphic artist)
	ClassGeometry                    // static geometry (polyhedra, spheres, field lines)
	ClassAudio                       // audio display modes (spectrogram, xy scope, fvf)
)

// ModeInfo carries the derivable per-mode facts. Skin is explicit because it
// doesn't follow class exactly (magnetosphere is Geometry but not skinnable).
type ModeInfo struct {
	Label string
	Class ModeClass
	Skin  bool // spectrogram skin can be painted onto it
}

var modeInfo = map[string]ModeInfo{
	"rossler":       {"Rossler", ClassFlow3D, false},
	"lorenz":        {"Lorenz", ClassFlow3D, false},
	"chua":          {"Chua", ClassFlow3D, false},
	"aizawa":        {"Aizawa", ClassFlow3D, false},
	"sprott":        {"Sprott", ClassFlow3D, false},
	"thomas":        {"Thomas", ClassFlow3D, false},
	"halvorsen":     {"Halvorsen", ClassFlow3D, false},
	"chen":          {"Chen", ClassFlow3D, false},
	"dadras":        {"Dadras", ClassFlow3D, false},
	"rabinovich":    {"Rabinovich-Fabrikant", ClassFlow3D, false},
	"burkeshaw":     {"Burke-Shaw", ClassFlow3D, false},
	"lu":            {"Lü", ClassFlow3D, false},
	"newtonleipnik": {"Newton-Leipnik", ClassFlow3D, false},
	"hyperrossler":  {"Hyper-Rössler (4D)", ClassFlow4D, false},
	"custom":        {"Custom equation", ClassFlow4D, false},
	"lissajou":      {"Lissajous", ClassParametric, false},
	"graphicartist": {"Graphic Artist", ClassParametric, false},
	"pong":          {"Scope Pong", ClassParametric, false},
	"sprotta":       {"Sprott A", ClassFlow3D, false},
	"sprottb":       {"Sprott B", ClassFlow3D, false},
	"sprottc":       {"Sprott C", ClassFlow3D, false},
	"sprottd":       {"Sprott D", ClassFlow3D, false},
	"sprotte":       {"Sprott E", ClassFlow3D, false},
	"sprottf":       {"Sprott F", ClassFlow3D, false},
	"sprottg":       {"Sprott G", ClassFlow3D, false},
	"sprotth":       {"Sprott H", ClassFlow3D, false},
	"sprotti":       {"Sprott I", ClassFlow3D, false},
	"sprottj":       {"Sprott J", ClassFlow3D, false},
	"sprottk":       {"Sprott K", ClassFlow3D, false},
	"sprottl":       {"Sprott L", ClassFlow3D, false},
	"sprottm":       {"Sprott M", ClassFlow3D, false},
	"sprottn":       {"Sprott N", ClassFlow3D, false},
	"sprotto":       {"Sprott O", ClassFlow3D, false},
	"sprottp":       {"Sprott P", ClassFlow3D, false},
	"sprottq":       {"Sprott Q", ClassFlow3D, false},
	"sprottr":       {"Sprott R", ClassFlow3D, false},
	"sprotts":       {"Sprott S", ClassFlow3D, false},
	"tetrahedron":   {"Tetrahedron", ClassGeometry, true},
	"cube":          {"Cube", ClassGeometry, true},
	"octahedron":    {"Octahedron", ClassGeometry, true},
	"dodecahedron":  {"Dodecahedron", ClassGeometry, true},
	"icosahedron":   {"Icosahedron", ClassGeometry, true},
	"nestedcube":    {"Nested Cube", ClassGeometry, true},
	"globe":         {"Globe", ClassGeometry, true},
	"sphere":        {"Sphere", ClassGeometry, true},
	"torus":         {"Torus", ClassGeometry, true},
	"magnetosphere": {"Magnetosphere", ClassGeometry, false},
	"spectrogram":   {"Spectrogram", ClassAudio, false},
	"xy":            {"XY Scope", ClassAudio, false},
	"fvf":           {"FVF Wobbulator", ClassAudio, false},
	// takens draws a TRAIL through the normal 3D pipeline (persist, gradient,
	// beam-dwell, Model Out SCAN), so it's Parametric, not Audio — the audio
	// stream is just its parameter source.
	"takens": {"Takens Embedding", ClassParametric, false},
	// bifurcation renders a progressive 2D scatter through the trail
	// pipeline; Parametric so persist/gradient/points sizing apply.
	"bifurcation": {"Bifurcation", ClassParametric, false},
}

// modeGroups is the mode <select>'s layout — ordered optgroups of ordered
// keys. A key may appear in more than one group (xy is in Scope and Audio;
// custom is in Attractors and Custom); the nested category/model selector
// knobs derive from this same structure at runtime.
var modeGroups = []struct {
	Label string
	Keys  []string
}{
	{"Attractors", []string{"rossler", "lorenz", "chua", "aizawa", "sprott", "thomas",
		"halvorsen", "chen", "dadras", "rabinovich", "burkeshaw", "lu", "newtonleipnik",
		"hyperrossler", "custom"}},
	{"Scope", []string{"lissajou", "graphicartist", "pong", "xy", "takens"}},
	{"Sprott systems (1994)", []string{"sprotta", "sprottb", "sprottc", "sprottd",
		"sprotte", "sprottf", "sprottg", "sprotth", "sprotti", "sprottj", "sprottk",
		"sprottl", "sprottm", "sprottn", "sprotto", "sprottp", "sprottq", "sprottr", "sprotts"}},
	{"Polyhedra", []string{"tetrahedron", "cube", "octahedron", "dodecahedron",
		"icosahedron", "nestedcube"}},
	{"Geometry", []string{"globe", "sphere", "torus", "magnetosphere"}},
	{"Audio", []string{"spectrogram", "xy", "fvf", "takens"}},
	{"Analysis", []string{"bifurcation"}},
	{"Custom", []string{"custom"}},
}

// defaultMode is the <select>'s initially-selected entry.
const defaultMode = "globe"

// knownMode reports whether a key names a registered mode (hash validation).
func knownMode(key string) bool { _, ok := modeInfo[key]; return ok }

// isAttractorMode: modes that integrate/trace into the trail buffer (and so
// support trail-length, sonification SCAN, persist painting, …).
func isAttractorMode(mode string) bool {
	switch modeInfo[mode].Class {
	case ClassFlow3D, ClassFlow4D, ClassParametric:
		return true
	}
	return false
}

// isSkinnable: the spectrogram skin can be painted onto it.
func isSkinnable(mode string) bool { return modeInfo[mode].Skin }

// isSpectroSurface: audio modes drawn as the textured spectrogram plane.
func isSpectroSurface(mode string) bool { return mode == "spectrogram" || mode == "fvf" }

// ModeKeys returns every registered key whose class is in the given set, in
// stable modeGroups order without duplicates — exported for cmd/uitool's
// tour/gallery lists.
func ModeKeys(classes ...ModeClass) []string {
	want := map[ModeClass]bool{}
	for _, c := range classes {
		want[c] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range modeGroups {
		for _, k := range g.Keys {
			if !seen[k] && want[modeInfo[k].Class] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}
