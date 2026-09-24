package attractor

import "github.com/0magnet/chaosrack/pkg/stlmodels"

// STLModels is the built-in solids, with each flow under the name the mode
// picker gives it. cmd/stlgen writes these and the STL mode offers them.
func STLModels() []stlmodels.Model { return stlmodels.All(modeLabel) }

// STLModelByName looks a built-in up by its key.
func STLModelByName(name string) (stlmodels.Model, bool) {
	for _, m := range STLModels() {
		if m.Name == name {
			return m, true
		}
	}
	return stlmodels.Model{}, false
}
