package dynamics

// Orbit state for the discrete maps: where each orbit currently is, how it is
// (re)seeded, and the finiteness guard that says an orbit has escaped.
//
// Untagged, unlike the render loop in attractor/mapgen_js.go that drives it,
// because none of it touches the DOM and all of it is where the failure modes
// live: a transient that is too short draws the path to the attractor and not
// the attractor (Ikeda in a periodic window looked like a single dot for
// exactly that reason), and an escaped orbit uploads NaNs that blank the figure.
// It moved out of the js file when the Custom mode gained its iterate flavor,
// which lets a user type `x' = 2x` and must answer that with a reseed rather
// than a blank screen — behavior worth a native test instead of a browser.

// Orbits is where a map's trajectories currently are.
//
// It owns the staleness rule as well as the points. The render loop used to
// hold the points in a package global and ask the question itself —
//
//	if mapSeeded != mode || mapOrbitsN != orbits || len(mapState) != orbits {
//		seedMapState(mode, m)
//	}
//
// — which is three ways for the drawn cloud to belong to a system other than
// the one on screen, written at the call site, where the next caller would
// have had to remember all three. Ensure is that condition with nobody left to
// remember it.
//
// The zero value is ready to use and seeds on first Ensure.
type Orbits struct {
	p      [][3]float64 // one entry per orbit
	seeded string       // which mode p was seeded for
	n      int          // how many orbits p was seeded with
	gen    int          // which revision of a typed system p was seeded from
}

// mapTransient is how many iterates to discard after seeding, so the drawn
// cloud is on the attractor rather than on the path to it. Cheap: this is a
// few thousand multiplies, once per reseed.
const mapTransient = 200

// Ensure makes the orbits be the ones mode and m describe, reseeding if they
// are not already — because the mode changed, because the ensemble size did,
// or because a typed system was edited underneath them.
func (o *Orbits) Ensure(mode string, m MapSys) {
	n := m.Count()
	if o.seeded == mode && o.n == n && len(o.p) == n && o.gen == m.gen {
		return
	}
	o.seed(mode, m, n)
}

// Invalidate forces the next Ensure to reseed. Called when a parameter changes,
// since a map's orbit is only meaningful for the parameters that produced it.
func (o *Orbits) Invalidate() { o.seeded = "" }

// N is how many orbits are live.
func (o *Orbits) N() int { return len(o.p) }

// At is where orbit i has got to, and Set is how the render loop hands it back.
func (o *Orbits) At(i int) [3]float64     { return o.p[i] }
func (o *Orbits) Set(i int, p [3]float64) { o.p[i] = p }

func (o *Orbits) seed(mode string, m MapSys, n int) {
	o.p = make([][3]float64, n)
	for i := range o.p {
		o.p[i] = m.Start(i, n)
		// Run the transient off the attractor's basin. An ensemble map is
		// area-preserving and has no attractor to settle onto, so it gets no
		// transient: every iterate is as valid as any other.
		if m.Seed == nil {
			for k := 0; k < mapTransient; k++ {
				p := o.p[i]
				nx, ny, nz := m.Step(p[0], p[1], p[2])
				if !Bounded(nx, ny, nz) {
					o.p[i] = m.IC
					break
				}
				o.p[i] = [3]float64{nx, ny, nz}
			}
		}
	}
	o.seeded, o.n, o.gen = mode, n, m.gen
}

// Bounded reports whether an iterate is still a number and still on the
// figure. A map pushed out of its bounded regime by a parameter edit escapes
// to infinity, and uploading the NaNs that follow blanks the whole picture.
func Bounded(a, b, c float64) bool {
	const lim = 1e6
	return a == a && b == b && c == c &&
		a < lim && a > -lim && b < lim && b > -lim && c < lim && c > -lim
}

// ── the typed map ────────────────────────────────────────────────────────────

// CustomKey is the mode key the Custom equation editor draws under. It is
// the same key in both flavors: what changes is which registry the compiled
// system is published to.
const CustomKey = "custom"

// customGen counts revisions of the typed system, and rides along on the
// MapSys that MapFor hands out so Orbits.Ensure can see that the system it
// seeded from has been replaced.
//
// It replaces two hand-written invalidations. SetCustomMap and ClearCustomMap
// each used to reach into the orbit state and clear its mode when the typed
// system changed, which is the coupling that makes a package global a global:
// a third way to publish a typed system would have had to know to do it too,
// and a cloud drawn from equations the user has since edited looks exactly
// like a cloud drawn from the current ones. A built-in map never touches the
// counter, so its generation stays 0 and it is never reseeded for this reason.
var customGen int

// customMapSys holds the typed system when the Custom editor is in iterate
// flavor, and is nil the rest of the time.
//
// Deliberately NOT an entry in mapSystems. That table is the catalog of
// built-in maps: MapKeys drives both the render-loop registration and the test
// that every registered map is reachable from the catalog, and a system the
// user just typed has no catalog entry to be reachable from. Keeping it beside
// the table instead means IsMap, MapFor and therefore the Lyapunov readout's
// per-iterate branch all see a map — which is the question that matters — with
// nothing pretending a typed system is a built-in one.
var customMapSys *MapSys

// CustomMapIC is where a typed map starts.
//
// NOT the Custom mode's flow seed (0.1, 0.5, −0.6), which was the obvious
// choice and is wrong: measured, that point is outside Henon's basin. The most
// canonical map anyone would type escapes from it within three iterates, the
// escape guard hands back the initial condition, and the screen fills with the
// same few stray dots forever instead of an attractor. Every built-in map
// starts small and near the origin, and a typed one has the same reasons to.
var CustomMapIC = [3]float64{0.1, 0.1, 0}

// SetCustomMap publishes the typed iterate system. The orbit now belongs to
// the previous equations, and a half-changed cloud is a picture of neither
// system — the generation bump is what says so.
func SetCustomMap(step MapStep) {
	customGen++
	customMapSys = &MapSys{Step: step, IC: CustomMapIC, Orbits: 1, gen: customGen}
}

// ClearCustomMap withdraws it (flow flavor, or a parse error).
func ClearCustomMap() {
	customGen++
	customMapSys = nil
}

// MapFor is the lookup every map consumer goes through: the built-in table
// first, then the typed system.
func MapFor(key string) (MapSys, bool) {
	if m, ok := mapSystems[key]; ok {
		return m, true
	}
	if key == CustomKey && customMapSys != nil {
		return *customMapSys, true
	}
	return MapSys{}, false
}
