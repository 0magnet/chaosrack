package attractor

import "testing"

func TestTheRegistryRecordsWhatAFrontEndWouldNeed(t *testing.T) {
	saved := controlRegistry
	t.Cleanup(func() { controlRegistry = saved })
	controlRegistry = nil

	registerControl(ControlDesc{
		ID: "zoom", Label: "zoom", Min: -8, Max: 8, Step: 0.1, Def: 0,
		PermaKey: "z", ModTarget: true,
	}, "view")
	got := ControlRegistry()
	if len(got) != 1 {
		t.Fatalf("registered one control, surface has %d", len(got))
	}
	want := ControlInfo{ID: "zoom", Label: "zoom", Min: -8, Max: 8, Step: 0.1, PermaKey: "z", ModTarget: true, Module: "view"}
	if got[0] != want {
		t.Fatalf("registry holds %+v, want %+v", got[0], want)
	}
}

func TestAPanelRebuiltInPlaceDoesNotDoubleTheSurface(t *testing.T) {
	// The rack rebuilds panels — a mode change wipes and re-wires the
	// parameter panel — and a surface that grew every time would report
	// controls that do not exist and get longer for as long as the page is
	// open.
	saved := controlRegistry
	t.Cleanup(func() { controlRegistry = saved })
	controlRegistry = nil

	registerControl(ControlDesc{ID: "zoom", Label: "zoom", Max: 8}, "view")
	registerControl(ControlDesc{ID: "zoom", Label: "zoom", Max: 12}, "view") // rebuilt, wider
	if n := len(ControlRegistry()); n != 1 {
		t.Fatalf("re-registering one id gave %d controls, want 1", n)
	}
	if got := ControlRegistry()[0].Max; got != 12 {
		t.Fatalf("the rebuilt descriptor did not replace the old one: Max=%v, want 12", got)
	}
}

func TestANamelessControlIsNotRegistered(t *testing.T) {
	// A descriptor with no id cannot be addressed by anything reading the
	// surface, so recording it would only make the list wrong.
	saved := controlRegistry
	t.Cleanup(func() { controlRegistry = saved })
	controlRegistry = nil

	registerControl(ControlDesc{Label: "nameless"}, "")
	if n := len(ControlRegistry()); n != 0 {
		t.Fatalf("an id-less control was registered: %d", n)
	}
}

func TestTheRegistryHandsOutACopy(t *testing.T) {
	// A caller that sorts or filters the surface must not be editing the
	// rack's own record of it.
	saved := controlRegistry
	t.Cleanup(func() { controlRegistry = saved })
	controlRegistry = nil

	registerControl(ControlDesc{ID: "a", Label: "a"}, "console")
	got := ControlRegistry()
	got[0].Label = "clobbered"
	if ControlRegistry()[0].Label != "a" {
		t.Fatal("editing the returned slice changed the registry")
	}
}
