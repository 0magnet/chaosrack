//go:build js && wasm

package attractor

import "strconv"

// What each scope's two channels can be fed from (rackscopes.go has the
// scopes themselves). What each is, is the manual's (scopes.md,
// scope-in=<position>).

// The sources, in the order the input knobs turn through them. The indices
// are the knobs' positions, and so what a link stores.
const (
	scopeInRackL = iota // the rack's signal, as the Mixer makes it
	scopeInRackR
	scopeInCapL // the capture, wherever the Mixer sends it
	scopeInCapR
	scopeInGen1 // through scopeInGen1+3: Gen 1 to 4
	_
	_
	_
	scopeInModelX
	scopeInModelY
	scopeInModelZ
	scopeInCount
)

// scopeInNames are the sources as the tube prints them.
var scopeInNames = [scopeInCount]string{
	"RACK L", "RACK R", "CAP L", "CAP R",
	"GEN 1", "GEN 2", "GEN 3", "GEN 4",
	"MODEL X", "MODEL Y", "MODEL Z",
}

// scopeInHelp is what source in is, as a tooltip says it.
func scopeInHelp(in int) string { return doc("scope-in=" + strconv.Itoa(in)) }

// scopeInDefaults is what each scope's two channels start on: the first two
// on the rack's signal, so IN 1 draws its left on Scope 1 and its right on
// Scope 2, and X-Y on either is a goniometer; the others on the generator
// beside them and a coordinate of the model.
var scopeInDefaults = [rackScopeCount][2]int{
	{scopeInRackL, scopeInRackR},
	{scopeInRackR, scopeInRackL},
	{scopeInGen1 + 2, scopeInModelZ},
	{scopeInGen1 + 3, scopeInModelY},
}

// scopeInKey is the link key of scope n's channel ch (0 or 1): si1a, si1b,
// si2a, and so on.
func scopeInKey(n, ch int) string {
	return "si" + strconv.Itoa(n+1) + "ab"[ch:ch+1]
}
