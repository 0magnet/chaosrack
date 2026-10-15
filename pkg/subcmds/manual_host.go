//go:build !js

package subcmds

import (
	"fmt"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/chaosrack/pkg/rackcatalog"
	"github.com/0magnet/chaosrack/pkg/racktui"
)

// bayPage is bay n's page of the manual as the catalog read it off the page,
// with model's bank where it reads otherwise: no browser needed.
func bayPage(_ Dialer, n int, model string) (racktui.ManualPage, error) {
	cat, err := rackcatalog.Load()
	if err != nil {
		return racktui.ManualPage{}, err
	}
	if n < 1 || n > len(cat.Manual) {
		return racktui.ManualPage{}, fmt.Errorf("the rack has bays 1 to %d", len(cat.Manual))
	}
	mods := cat.Manual[n-1]
	if model != "" {
		b, ok := cat.Banks[model]
		if !ok {
			return racktui.ManualPage{}, fmt.Errorf("no model %q (chaosrack models lists them)", model)
		}
		if m, ok := b.Manual[n]; ok {
			mods = m
		}
	}
	return racktui.ManualPage{Title: attractor.BayTitle(n, mods), Mods: mods}, nil
}
