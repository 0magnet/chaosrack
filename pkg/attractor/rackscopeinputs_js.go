//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
)

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

// scopeLetters are the scope controls' letters in their link keys.
var scopeLetters = map[string]string{
	"volts": "v", "time": "t", "vpos": "y", "hpos": "x", "trig": "r",
	"illum": "i", "intens": "n", "focus": "f", "slope": "s", "tmode": "m", "chan": "c",
}

// scopePermaKey is scope control id's link key, s<scope><letter>: s2v is
// Scope 2's VOLTS. The first scope's ids are scope-…, the others' scopeN-….
// Only the inputs had keys, so a link kept what a scope was looking at and
// lost how: its ranges, its trigger, its channel, whether it was on.
func scopePermaKey(id string) string {
	i := strings.IndexByte(id, '-')
	if i < len("scope") || scopeLetters[id[i+1:]] == "" {
		return ""
	}
	n := id[len("scope"):i]
	if n == "" {
		n = "1"
	}
	return "s" + n + scopeLetters[id[i+1:]]
}
