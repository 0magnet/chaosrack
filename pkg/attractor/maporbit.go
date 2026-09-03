package attractor

// Orbit state for the discrete maps: where each orbit currently is, how it is
// (re)seeded, and the finiteness guard that says an orbit has escaped.
//
// Untagged, unlike the render loop in mapgen_js.go that consumes it, because
// none of it touches the DOM and all of it is where the failure modes live: a
// transient that is too short draws the path to the attractor instead of the
// attractor (Ikeda in a periodic window looked like a single dot for exactly
// that reason), and an escaped orbit uploads NaNs that blank the whole figure.
// It moved out of the js file when the Custom mode gained its iterate flavor,
// which lets a user type `x' = 2x` and must answer that with a reseed rather
// than a blank screen — behavior worth a native test instead of a browser.

var (
	mapState  [][3]float64 // one entry per orbit
	mapSeeded string       // which mode mapState was seeded for
	// mapOrbitsN is only READ by the render loop in mapgen_js.go, which the
	// host lint pass cannot see — hence the exemption rather than a deletion.
	mapOrbitsN int //nolint:unused // read by generateMap (js/wasm only)
)

// mapTransient is how many iterates to discard after seeding, so the drawn
// cloud is on the attractor rather than on the path to it. Cheap: this is a
// few thousand multiplies, once per reseed.
const mapTransient = 200

// seedMapState (re)seeds every orbit for the given map.
func seedMapState(mode string, m mapSys) {
	n := m.orbits
	if n < 1 {
		n = 1
	}
	mapState = make([][3]float64, n)
	for i := range mapState {
		if m.seed != nil {
			mapState[i] = m.seed(i, n)
		} else {
			mapState[i] = m.ic
		}
		// Run the transient off the attractor's basin. An ensemble map is
		// area-preserving and has no attractor to settle onto, so it gets no
		// transient: every iterate is as valid as any other.
		if m.seed == nil {
			for k := 0; k < mapTransient; k++ {
				p := mapState[i]
				nx, ny, nz := m.step(p[0], p[1], p[2])
				if !finite3(nx, ny, nz) {
					mapState[i] = m.ic
					break
				}
				mapState[i] = [3]float64{nx, ny, nz}
			}
		}
	}
	mapSeeded, mapOrbitsN = mode, n
}

func finite3(a, b, c float64) bool {
	const lim = 1e6
	return a == a && b == b && c == c &&
		a < lim && a > -lim && b < lim && b > -lim && c < lim && c > -lim
}

// ── the typed map ────────────────────────────────────────────────────────────

// customModeKey is the mode key the Custom equation editor draws under. It is
// the same key in both flavors: what changes is which registry the compiled
// system is published to.
const customModeKey = "custom"

// customMapSys holds the typed system when the Custom editor is in iterate
// flavor, and is nil the rest of the time.
//
// Deliberately NOT an entry in mapSystems. That table is the catalog of
// built-in maps: MapKeys drives both the render-loop registration and the test
// that every registered map is reachable from the catalog, and a system the
// user just typed has no catalog entry to be reachable from. Keeping it beside
// the table instead means IsMap, MapStep and therefore the Lyapunov readout's
// per-iterate branch all see a map — which is the question that matters — with
// nothing pretending a typed system is a built-in one.
var customMapSys *mapSys

// customMapIC is where a typed map starts.
//
// NOT the Custom mode's flow seed (0.1, 0.5, −0.6), which was the obvious
// choice and is wrong: measured, that point is outside Henon's basin. The most
// canonical map anyone would type escapes from it within three iterates, the
// escape guard hands back the initial condition, and the screen fills with the
// same few stray dots forever instead of an attractor. Every built-in map
// starts small and near the origin, and a typed one has the same reasons to.
var customMapIC = [3]float64{0.1, 0.1, 0}

// setCustomMap publishes the typed iterate system.
func setCustomMap(step mapStep) {
	customMapSys = &mapSys{step: step, ic: customMapIC, orbits: 1}
	if mapSeeded == customModeKey {
		// The orbit belonged to the previous equations; a half-changed cloud is
		// a picture of neither system.
		mapSeeded = ""
	}
}

// clearCustomMap withdraws it (flow flavor, or a parse error).
func clearCustomMap() {
	customMapSys = nil
	if mapSeeded == customModeKey {
		mapSeeded = ""
	}
}

// mapSysFor is the lookup every map consumer goes through: the built-in table
// first, then the typed system.
func mapSysFor(key string) (mapSys, bool) {
	if m, ok := mapSystems[key]; ok {
		return m, true
	}
	if key == customModeKey && customMapSys != nil {
		return *customMapSys, true
	}
	return mapSys{}, false
}
