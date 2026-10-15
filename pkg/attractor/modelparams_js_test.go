//go:build js && wasm

package attractor

import (
	"slices"
	"testing"
)

// ModelParams is the page's own tables, model by model: a native rack's Mod
// rows would otherwise name one parameter and drive another.
func TestModelParamsAreThePagesTables(t *testing.T) {
	for mode, ps := range attractorParams {
		if mode == "custom" || mode == "water" {
			continue // Custom's are its equations'; the lens is no model
		}
		var want []ModelParam
		for _, p := range ps {
			want = append(want, ModelParam{p.ID, p.Label})
		}
		if got := ModelParams(mode); !slices.Equal(got, want) {
			t.Errorf("%s: ModelParams has\n%v\nthe page's table is\n%v", mode, got, want)
		}
	}
	for mode := range modelParams {
		if _, ok := attractorParams[mode]; !ok {
			t.Errorf("modelParams names %q, which has no table", mode)
		}
	}
}

// ModViewRows are the page's matrix rows for the view, in its order, with
// the controls and reaches its routes use.
func TestModViewRowsAreThePages(t *testing.T) {
	if len(ModViewRows) != len(modMxViewCols) {
		t.Fatalf("%d view rows, the page has %d", len(ModViewRows), len(modMxViewCols))
	}
	for i, row := range ModViewRows {
		if c := modMxViewCols[i]; row.ID != c.id || row.Label != c.label {
			t.Errorf("row %d is %s %q, the page's is %s %q", i, row.ID, row.Label, c.id, c.label)
		}
		found := false
		for _, vt := range viewModTargets {
			if vt.id == row.ID {
				found = true
				if row.Control != vt.anchor || row.Min != vt.min || row.Max != vt.max {
					t.Errorf("%s moves %s over %v..%v, the page's %s over %v..%v",
						row.ID, row.Control, row.Min, row.Max, vt.anchor, vt.min, vt.max)
				}
			}
		}
		if !found {
			t.Errorf("%s is no route the page has", row.ID)
		}
	}
}
