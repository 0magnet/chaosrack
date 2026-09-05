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
	ClassMap                         // discrete map: x_{n+1} = f(x_n), drawn as points
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
	// turtle is a walk on a lattice, not a flow — nothing is integrated, so it
	// is not in the flow registry and FLOW sonification has no field to run.
	// It is a curve traced in time all the same, which is what gets it the
	// trail, the persist painting and Model Out SCAN.
	"turtle":        {"Turtle Path", ClassParametric, false},
	"graphicartist": {"Graphic Artist", ClassParametric, false},
	"pong":          {"Scope Pong", ClassParametric, false},
	"scopetext":     {"Fourier Text", ClassParametric, false},
	"scopeclock":    {"Scope Clock", ClassParametric, false},
	"bounceball":    {"Bouncing Ball", ClassParametric, false},
	"sprottmorph":   {"Sprott Morph", ClassFlow3D, false},
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
	"henon":         {"Henon", ClassMap, false},
	"ikeda":         {"Ikeda", ClassMap, false},
	"clifford":      {"Clifford", ClassMap, false},
	"dejong":        {"Peter de Jong", ClassMap, false},
	"mira":          {"Gumowski-Mira", ClassMap, false},
	"tinkerbell":    {"Tinkerbell", ClassMap, false},
	"standardmap":   {"Chirikov Standard Map", ClassMap, false},
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
	"stlfile":       {"STL File", ClassGeometry, false},
	"spectrogram":   {"Spectrogram", ClassAudio, false},
	"xy":            {"XY Scope", ClassAudio, false},
	"fvf":           {"FVF Wobbulator", ClassAudio, false},
	// takens draws a TRAIL through the normal 3D pipeline (persist, gradient,
	// beam-dwell, Model Out SCAN), so it's Parametric, not Audio — the audio
	// stream is just its parameter source.
	"takens": {"Takens Embedding", ClassParametric, false},
	// stereo is the same kind of thing as takens and Parametric for the same
	// reason: a trail through the normal 3D pipeline whose coordinates happen
	// to come from the audio. What differs is where the axes come from — two
	// recorded channels rather than one channel's own past.
	"stereo": {"Stereo Embedding", ClassParametric, false},
	// bifurcation renders a progressive 2D scatter through the trail
	// pipeline; Parametric so persist/gradient/points sizing apply.
	"bifurcation": {"Bifurcation", ClassParametric, false},
	// poincare is the same kind of thing for the same reason: a scatter that
	// accumulates through the trail pipeline. Parametric and not Flow3D even
	// though a flow is what it integrates — what it DRAWS is not a trajectory,
	// and filing it as a flow would offer it to every consumer that reaches
	// for flowFor4 (Model Out FLOW, the ring beam, the section overlay itself)
	// as a system to integrate, which it is not.
	"poincare": {"Poincaré Section", ClassParametric, false},
	// recurrence is Audio for the same reason the spectrogram is: it is a
	// texture drawn on a plane, with no trail to persist, gradient or scan.
	"recurrence": {"Recurrence Plot", ClassAudio, false},
	// A terminal is not audio and not a dynamical system; it is a live picture
	// on a plane, which is what ClassGeometry already covers for the modes that
	// are a surface rather than a trajectory.
	"terminal": {"Terminal", ClassGeometry, false},
	// The same kind of thing as the terminal — a live picture on a plane —
	// with one of tuiwasm's demos running on it instead of a shell.
	"termanim": {"Terminal Animation", ClassGeometry, false},
	"hostterm": {"Host Shell", ClassGeometry, false},
	"desk":     {"Desk", ClassGeometry, false},
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
		"hyperrossler"}},
	// Directly after Attractors, because they ARE attractors — a catalog of
	// twenty of them from one 1994 paper, which is why they stay their own
	// category rather than being folded in, but there is no reason for the
	// Scope modes to have sat between the two.
	{"Sprott systems (1994)", []string{"sprottmorph", "sprotta", "sprottb", "sprottc", "sprottd",
		"sprotte", "sprottf", "sprottg", "sprotth", "sprotti", "sprottj", "sprottk",
		"sprottl", "sprottm", "sprottn", "sprotto", "sprottp", "sprottq", "sprottr", "sprotts"}},
	// Discrete maps sit with the other dynamical systems rather than after the
	// geometry: they are the same subject read one iterate at a time instead of
	// one integration step at a time, and burying them past the polyhedra would
	// repeat the mistake that hid the turtle.
	{"Maps", []string{"henon", "ikeda", "clifford", "dejong", "mira",
		"tinkerbell", "standardmap"}},
	{"Scope", []string{"lissajou", "graphicartist", "pong", "scopetext", "scopeclock", "bounceball", "xy", "takens", "stereo"}},
	{"Polyhedra", []string{"tetrahedron", "cube", "octahedron", "dodecahedron",
		"icosahedron", "nestedcube"}},
	{"Geometry", []string{"globe", "sphere", "torus", "magnetosphere"}},
	// The turtle is not geometry. It is an integer sequence read as
	// turn-and-step — arithmetic that happens to draw — with its own camera,
	// tinting, physics and closure classification. Filed next to "sphere" and
	// "torus" it read as one more primitive, which is the wrong thing to tell
	// someone about the most distinctive model in the app.
	{"Sequences", []string{"turtle"}},
	// Nor is the STL mode a shape. It is a model browser: a loader for a file
	// off disk plus a catalog of built-in solids — the rack, the geometry and
	// every attractor swept as a tube — generated in the browser.
	{"Solids", []string{"stlfile", "terminal", "termanim", "hostterm", "desk"}},
	{"Audio", []string{"spectrogram", "xy", "fvf", "takens", "stereo", "recurrence"}},
	{"Analysis", []string{"bifurcation", "poincare", "recurrence"}},
	// Custom is its own category and NOT also an entry in Attractors, where it
	// used to be listed twice. It is a different kind of thing from the rest
	// of that list: every other entry is a system someone published and this
	// one is whichever system you type. As the last of fifteen named
	// attractors it was also the least findable thing in the app, which is a
	// poor place for the feature the README leads with.
	{"Custom", []string{"custom"}},
}

// defaultMode is the <select>'s initially-selected entry.
const defaultMode = "globe" //nolint:unused // built but not wired up yet; kept deliberately

// knownMode reports whether a key names a registered mode (hash validation).
func knownMode(key string) bool { _, ok := modeInfo[key]; return ok } //nolint:unused // built but not wired up yet; kept deliberately

// isAttractorMode: modes that integrate/trace into the trail buffer (and so
// support trail-length, sonification SCAN, persist painting, …).
func isAttractorMode(mode string) bool { //nolint:unused // built but not wired up yet; kept deliberately
	switch modeInfo[mode].Class {
	case ClassFlow3D, ClassFlow4D, ClassParametric, ClassMap:
		return true
	}
	return false
}

// isSkinnable: the spectrogram skin can be painted onto it.
func isSkinnable(mode string) bool { return modeInfo[mode].Skin } //nolint:unused // built but not wired up yet; kept deliberately

// isSpectroSurface: audio modes drawn as the textured spectrogram plane.
func isSpectroSurface(mode string) bool { return mode == "spectrogram" || mode == "fvf" } //nolint:unused // built but not wired up yet; kept deliberately

// isTexturePlane: every mode that is a texture on a quad rather than geometry
// — the spectrogram family and the recurrence plot. They share a camera: face
// on, still, and framed to the quad, because a picture read as a picture is
// unreadable edge-on and worse tumbling.
func isTexturePlane(mode string) bool { //nolint:unused // built but not wired up yet; kept deliberately
	return isSpectroSurface(mode) || mode == "recurrence" || mode == "terminal" ||
		mode == "termanim" || mode == "hostterm" || mode == "desk"
}

// isFlatScope: flat scope-screen modes (games, text banners, demos) that
// must boot face-on and still instead of in a random pose.
func isFlatScope(mode string) bool { //nolint:unused // built but not wired up yet; kept deliberately
	// The maps are 2-D — the third coordinate is only there because the
	// pipeline is 3-D — so they boot face-on for the same reason the scope
	// screens do: edge-on, a plane figure is a line.
	if modeInfo[mode].Class == ClassMap {
		return true
	}
	return mode == "pong" || mode == "scopetext" || mode == "scopeclock" || mode == "bounceball"
}

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
