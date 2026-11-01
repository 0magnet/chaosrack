package rackhost

import (
	"strings"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// customIntID is Custom's INTG position, whose settings run euler, rk4: the
// other way round from every other flow's, which run rk4, euler.
const customIntID = "custom-int-select"

// mirrorIntegrator turns every flow's INTG position to match id, which has
// just been set to v. They all turn the one integrator, and the scene writes
// each position into it in turn (flowValues): left apart, whichever was
// written last would win. Called with r.mu held.
func (r *Rack) mirrorIntegrator(id, v string) {
	var euler bool
	switch {
	case id == customIntID:
		euler = v == "0"
	case strings.HasSuffix(id, dynamics.IntegratorSuffix):
		euler = v == "1"
	default:
		return
	}
	for other, i := range r.byID {
		switch {
		case other == id:
		case other == customIntID:
			r.ctls[i].Value = map[bool]string{true: "0", false: "1"}[euler]
		case strings.HasSuffix(other, dynamics.IntegratorSuffix):
			r.ctls[i].Value = map[bool]string{true: "1", false: "0"}[euler]
		}
	}
	if euler {
		dynamics.SetIntegrator(dynamics.EulerIntegrator)
	} else {
		dynamics.SetIntegrator(dynamics.RK4Integrator)
	}
}
