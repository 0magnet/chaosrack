package attractor

import (
	"slices"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/colormap"
)

// ModelColors are a model's colors: the SRC and MAP ring positions it comes
// up on, until someone turns a knob in the Layers · Colors module.
//
// The rack's own resting place is the trace colored by Z through the two-color
// ramp, which suits a figure in space: it says which side of the figure is
// which. Two kinds of model are not that.
//
// The embeddings (Takens, stereo, polar) are built from the sound, and Z in a
// delay embedding is the signal two delays back — an axis already drawn. By Z,
// the figure came up split into a red half and a blue half, which says
// nothing. AUDIO colors each point by the spectral centroid of the moment it
// was played, which the geometry does not already show: a change of timbre
// is a change of color along the trace. TURBO because it never reaches black.
// Heat and magma start there, and a quiet or dark passage of the music, low on
// the ramp, vanished into the background — tried on all three, with music
// playing, that is what heat did whenever the level or the frequencies fell.
//
// The spectrogram and FVF are a magnitude painted on a plane, and open on
// heat, the colormap the spectrogram had before its knob became the MAP ring.
type ModelColors struct{ Src, Map string }

// rackColors are where the rings rest for a model with no colors of its own.
var rackColors = ModelColors{Src: "2", Map: "2"} // Z, two-color

// ColorsModule is the Layers · Colors module's name, as the rack knows it by
// its header: the module whose knobs, once turned, are the operator's.
const ColorsModule = "layers · colors"

// ModelColorDefault is the src and map model comes up on.
func ModelColorDefault(model string) ModelColors {
	switch {
	case inModeGroup("Embeddings", model):
		return ModelColors{Src: "4", Map: strconv.Itoa(colormap.First + 3)} // audio, turbo
	case isSpectroSurface(model):
		return ModelColors{Src: rackColors.Src, Map: strconv.Itoa(colormap.First)} // heat
	}
	return rackColors
}

// inModeGroup reports whether model is filed under the mode list's heading.
func inModeGroup(label, model string) bool {
	for _, g := range modeGroups {
		if g.Label == label {
			return slices.Contains(g.Keys, model)
		}
	}
	return false
}

// ColorFollow is the rule for when a model's colors are put on the rings: on
// every change of model, until a knob in the Colors module is turned. After
// that the rings are the operator's, and no change of model moves them.
//
// Not "until the rings are moved off their default", which is what the
// spectrogram's heat used to go by: a ring turned to the very position a
// model would have put it on is still a choice, and the next model would have
// taken it away.
type ColorFollow struct{ chosen bool }

// Choose records that a knob in the Colors module was turned.
func (f *ColorFollow) Choose() { f.chosen = true }

// Forget hands the rings back to the models: Reset All.
func (f *ColorFollow) Forget() { f.chosen = false }

// For is what the rings are to show for model, or false when they are the
// operator's.
func (f *ColorFollow) For(model string) (ModelColors, bool) {
	if f.chosen {
		return ModelColors{}, false
	}
	return ModelColorDefault(model), true
}
