//go:build js && wasm

package attractor

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// Every flow's INTG position (dynamics/integrator.go): a labeled position
// after its constants, all of them turning the one integrator. Each is a
// parameter like any other, with its own hidden slider, so the slider input
// writes the value and starts the trajectory again as a constant does; what
// is left here is keeping them all, and Custom's own INTG cell, at the same
// setting.

func init() {
	for _, k := range dynamics.Keys() {
		paramLabels[k+dynamics.IntegratorSuffix] = dynamics.IntegratorNames
	}
}

// paramDocID is the manual's id for a parameter: its own, but for every
// flow's INTG position, which share one entry (p.intg).
func paramDocID(id string) string {
	if strings.HasSuffix(id, dynamics.IntegratorSuffix) {
		return "intg"
	}
	return id
}

// syncingIntegrator guards syncIntegratorCells against its own events: each
// cell it moves answers with an input event of its own.
var syncingIntegrator bool

// syncIntegratorCells puts every built INTG cell, and Custom's, at the
// integrator in force. A cell not built yet is built from the value, which is
// already right.
func syncIntegratorCells() {
	if syncingIntegrator || !dom.Doc.Truthy() {
		return
	}
	syncingIntegrator = true
	defer func() { syncingIntegrator = false }()
	want := "0"
	if dynamics.UsingEuler() {
		want = "1"
	}
	ss := dom.Doc.Call("querySelectorAll", `input[type=range][id$="`+dynamics.IntegratorSuffix+`"]`)
	for i := range ss.Length() {
		s := ss.Index(i)
		if s.Get("value").String() != want {
			s.Set("value", want)
			dom.Fire(s, "input")
		}
	}
	custom.showIntegrator()
}

// isIntegratorParam reports whether id is a flow's INTG position.
func isIntegratorParam(id string) bool { return strings.HasSuffix(id, dynamics.IntegratorSuffix) }

// withoutIntegrator is ps less a flow's INTG position: what a bifurcation
// sweeps and a modulation routes to, which a switch between two integrators
// is not. A Grid's views may still differ by it, and show the two side by
// side.
func withoutIntegrator(ps []paramDef) []paramDef {
	if n := len(ps); n > 0 && isIntegratorParam(ps[n-1].ID) {
		return ps[:n-1]
	}
	return ps
}
