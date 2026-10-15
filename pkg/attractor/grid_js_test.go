//go:build js && wasm

package attractor

import (
	"slices"
	"testing"
)

// GridSweepParams is what the page's own tables give: every numeric
// parameter of a model a numeric sweep is true of, in its table's order,
// with the knob's label and range. A native rack reads the shared table, so
// a parameter added to a model without it would be one the page sweeps and
// a native rack does not.
func TestGridSweepParamsAreThePagesParameters(t *testing.T) {
	for mode := range modeInfo {
		var want []GridParam
		if sweepNumericOK(mode) {
			for _, pd := range attractorParams[mode] {
				if len(paramLabels[pd.ID]) > 0 || pd.Max <= pd.Min {
					continue
				}
				want = append(want, GridParam{pd.ID, pd.Label, pd.Min, pd.Max})
			}
		}
		if got := GridSweepParams[mode]; !slices.Equal(got, want) {
			t.Errorf("%s: GridSweepParams has\n%v\nthe page's table gives\n%v", mode, got, want)
		}
	}
	for mode := range GridSweepParams {
		if _, ok := modeInfo[mode]; !ok {
			t.Errorf("GridSweepParams names %q, which is no model", mode)
		}
	}
}

// The source sweep steps through the audio-fed sources by their names.
func TestSweepColorSourcesAreTheAudioSources(t *testing.T) {
	want := []int{
		gradientSourceAudio, gradientSourceLevel, gradientSourceDB,
		gradientSourceCorr, gradientSourceSide, gradientSourceBalance,
		gradientSourcePosition, gradientSourceFlux, gradientSourcePitch,
	}
	if !slices.Equal(SweepColorSources, want) {
		t.Errorf("SweepColorSources = %v, want %v", SweepColorSources, want)
	}
}
