//go:build js && wasm

package attractor

import "syscall/js"

// The render loop for every discrete map. One loop for all of them, the way
// generateClassic is one loop for every classic flow: each map contributes
// only its parameters and its step function (in mapdata.go).
//
// Points, not a line strip. Successive iterates of a chaotic map are far
// apart, so joining them would draw a hairball across the attractor and hide
// the structure the map exists to show — the filaments of Hénon, the islands
// of the standard map.
//
// State persists across frames like the flows', so the cloud keeps filling in
// rather than restarting every frame. The orbit state itself, its seeding and
// the finiteness guard are in maporbit.go, untagged so they can be tested off
// the browser.

// mapPosed is the mode whose entry pose has been applied.
var mapPosed string

// generateMap iterates the mode's map and uploads the iterates as points.
// It serves the Custom mode's iterate flavor too — a typed map is a map, and
// the transient, the points upload and the escape guard below are exactly what
// it needs.
func generateMap(mode string) {
	m, ok := mapSysFor(mode)
	if !ok {
		return
	}
	orbits := m.orbits
	if orbits < 1 {
		orbits = 1
	}
	if mapSeeded != mode || mapOrbitsN != orbits || len(mapState) != orbits {
		seedMapState(mode, m)
	}

	n := steps
	if n < 2 {
		n = 2
	}
	// Split the point budget across the ensemble; every orbit gets the same
	// share so no orbit is drawn denser than another.
	per := n / orbits
	if per < 1 {
		per = 1
	}
	total := per * orbits

	vertices := vertBuf[:total*4]
	invN := float32(1) / float32(total-1)
	idx := 0
	for o := 0; o < orbits; o++ {
		p := mapState[o]
		for i := 0; i < per; i++ {
			nx, ny, nz := m.step(p[0], p[1], p[2])
			if !finite3(nx, ny, nz) {
				// A parameter edit can push a map out of its bounded regime;
				// restart this orbit rather than uploading NaNs, which would
				// blank the whole figure.
				if m.seed != nil {
					p = m.seed(o, orbits)
				} else {
					p = m.ic
				}
				nx, ny, nz = p[0], p[1], p[2]
			}
			p = [3]float64{nx, ny, nz}
			j := idx * 4
			vertices[j] = float32(nx)
			vertices[j+1] = float32(ny)
			vertices[j+2] = float32(nz)
			vertices[j+3] = float32(idx) * invN
			idx++
		}
		mapState[o] = p
	}
	uploadVerticesOnly(vertices, glTypes.Points, total)
}

// mapDrawMode is the draw mode for a model that may be a map: points for one
// (built-in or typed), and otherwise whatever the user's points/line switch
// says. The uploads below already pass Points, but the PAUSED redraw in
// render.go re-issues drawArrays with the global mode — so pausing a map used
// to join its iterates into a line strip, which is exactly the hairball across
// the attractor that maps are drawn as points to avoid. The typed iterate
// flavor is included by its flavor flag rather than by IsMap, so that a parse
// error (which withdraws the map) still redraws the last cloud as a cloud.
func mapDrawMode(mode string) js.Value {
	if IsMap(mode) || (mode == customModeKey && customIterate) {
		return glTypes.Points
	}
	return attractorDrawMode
}

// mapInvalidate forces a reseed — called when the parameters change, since a
// map's orbit is only meaningful for the parameters that produced it.
func mapInvalidate() { mapSeeded = "" }

// mapLeave forgets the entry pose so re-entering a map re-poses it. Without
// this, leaving a map and coming back would keep whatever angle the previous
// model left behind.
func mapLeave() { mapPosed = "" }

func init() {
	for _, k := range MapKeys() {
		k := k
		registerGenerate(k, func() { generateMap(k) })
	}

	// Knobs. Every map reseeds on a parameter edit (mapInvalidate through the
	// panel's change path): an orbit is only meaningful for the parameters
	// that produced it, and a half-changed cloud is a picture of neither.
	attractorParams["henon"] = []paramDef{
		{"henon-a", "a", &henonA, 1.4, 0.1, 1.45, 0.001},
		{"henon-b", "b", &henonB, 0.3, 0.01, 0.4, 0.001},
	}
	attractorParams["ikeda"] = []paramDef{
		{"ikeda-u", "u", &ikedaU, 0.9, 0.5, 1.0, 0.001},
	}
	attractorParams["clifford"] = []paramDef{
		{"clifford-a", "a", &cliffordA, -1.4, -3, 3, 0.01},
		{"clifford-b", "b", &cliffordB, 1.6, -3, 3, 0.01},
		{"clifford-c", "c", &cliffordC, 1.0, -3, 3, 0.01},
		{"clifford-d", "d", &cliffordD, 0.7, -3, 3, 0.01},
	}
	attractorParams["dejong"] = []paramDef{
		{"dejong-a", "a", &dejongA, 1.641, -3, 3, 0.01},
		{"dejong-b", "b", &dejongB, 1.902, -3, 3, 0.01},
		{"dejong-c", "c", &dejongC, 0.316, -3, 3, 0.01},
		{"dejong-d", "d", &dejongD, 1.525, -3, 3, 0.01},
	}
	attractorParams["mira"] = []paramDef{
		{"mira-mu", "μ", &miraMu, -0.496, -1, 1, 0.001},
		{"mira-a", "a", &miraA, 0.008, 0, 0.1, 0.001},
		{"mira-b", "b", &miraB, 0.05, 0, 1, 0.001},
	}
	attractorParams["tinkerbell"] = []paramDef{
		{"tinkerbell-a", "a", &tinkA, 0.9, -1, 1, 0.001},
		{"tinkerbell-b", "b", &tinkB, -0.6013, -1, 1, 0.001},
		{"tinkerbell-c", "c", &tinkC, 2.0, -3, 3, 0.01},
		{"tinkerbell-d", "d", &tinkD, 0.5, -1, 1, 0.001},
	}
	attractorParams["standardmap"] = []paramDef{
		{"standardmap-k", "K", &stdK, 0.971635, 0, 6, 0.001},
	}
}

// syncMapExtras runs on every panel rebuild, beside the other mode-scoped
// syncs. Leaving a map forgets its entry pose, so coming back re-poses it
// face-on — otherwise a spin picked up from whatever model was visited in
// between would still be running, and a plane figure seen edge-on is a line.
func syncMapExtras(mode string) {
	if !IsMap(mode) {
		mapLeave()
		return
	}
	// Face-on and still on entry. These figures are plane sets — the third
	// coordinate is zero for every point — so a spin carries them edge-on twice
	// a turn, where a plane is a line. The standard map's phase portrait, which
	// is a square, rendered as a 126-pixel-wide vertical smear because the spin
	// from the previous model was still running.
	//
	// Here rather than in the render loop, and guarded by mapPosed rather than
	// by the seeding: this hook fires at a defined point in a mode change, and
	// re-posing the camera because a parameter knob moved would yank the view
	// out from under whoever was turning it.
	if mapPosed != mode {
		mapPosed = mode
		normalizeOrientation()
	}
}
