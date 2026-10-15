package rackhost

import (
	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// Manual is bay n's page of the manual as the catalog read it off the page,
// with this model's bank where it reads otherwise, and the model on show
// while the rack is on: what the page's Info window shows, for the panel's
// Info pane.
func (r *Rack) Manual(bay int) (racktui.ManualPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var mods []manual.Module
	if bay >= 1 && bay <= len(r.cat.Manual) {
		mods = r.cat.Manual[bay-1]
	}
	model := r.model()
	if b := r.cat.Banks[model]; b != nil {
		if m, ok := b.Manual[bay]; ok {
			mods = m
		}
	}
	pg := racktui.ManualPage{Title: attractor.BayTitle(bay, mods), Mods: mods}
	if model != "" && r.value("power-sw") != "0" {
		pg.Model, pg.ModelText = attractor.ModelLabel(model), attractor.ModelDescription(model)
	}
	return pg, nil
}

var _ racktui.Manualed = (*Rack)(nil)
