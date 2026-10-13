package rackhost

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// addFlowParams adds the flows' constants to the rack's controls: σ, ρ and β
// of the Lorenz system and the rest, the values the integrator reads. The
// page builds a model's knobs only when the model is first chosen, so the
// catalog has few of them; the native rack can set every one, and the model
// redraws with it.
func (r *Rack) addFlowParams() {
	r.flow = map[string]dynamics.Param{}
	for _, mode := range dynamics.ParamModes() {
		for _, p := range dynamics.Params(mode) {
			if p.Value == nil {
				continue
			}
			r.flow[p.ID] = p
			if _, have := r.byID[p.ID]; have {
				continue
			}
			r.byID[p.ID] = len(r.ctls)
			r.ctls = append(r.ctls, racktui.Control{
				ControlInfo: controlspec.ControlInfo{
					ID: p.ID, Label: p.Label, Module: modelModule,
					Min: float64(p.Min), Max: float64(p.Max), Step: float64(p.Step), Def: float64(p.Def),
				},
				Value: strconv.FormatFloat(float64(*p.Value), 'g', -1, 32),
			})
		}
	}
}
