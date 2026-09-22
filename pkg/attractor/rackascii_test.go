package attractor

import (
	"strings"
	"testing"
)

func demoRack() ([]rackModule, []packItem) {
	spec := []struct {
		key     string
		slots   int
		section string
	}{
		{"console", 6, secConsole}, {"presets", 4, secConsole}, {"template", 3, secConsole},
		{"test", 5, secInput},
		{"loudness", 6, secAnalyze}, {"distortion", 6, secAnalyze},
		{"wow & flutter", 6, secAnalyze}, {"counter", 3, secAnalyze}, {"timing", 5, secAnalyze},
		{"patchbay", 8, secMod}, {"envelope", 4, secMod},
	}
	var mods []rackModule
	var items []packItem
	for _, s := range spec {
		mods = append(mods, rackModule{Key: s.key, Slots: s.slots})
		items = append(items, packItem{Slots: s.slots, Section: s.section})
	}
	return mods, items
}

func TestTheDrawingIsTheLayoutAndNotADescriptionOfIt(t *testing.T) {
	mods, items := demoRack()
	const capacity = 20
	units := packBySection(items, capacity, nil)
	got := drawRack(mods, items, units, capacity, nil)

	// One line per bay, plus the two rules and the tally.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != len(units)+3 {
		t.Fatalf("drew %d lines for %d bays:\n%s", len(lines), len(units), got)
	}
	// Every module that has slots must appear.
	for _, m := range mods {
		short := m.Key
		if len(short) > 3 {
			short = short[:3]
		}
		if m.Slots > 0 && !strings.Contains(got, short) {
			t.Errorf("module %q is not in the drawing:\n%s", m.Key, got)
		}
	}
	t.Logf("\n%s", got)
}

func TestTheDrawingShowsTheChassisMonitor(t *testing.T) {
	mods, items := demoRack()
	const capacity = 20
	monitors := map[string]int{secConsole: 4, secAnalyze: 4, secInput: 4, secMod: 4}
	units := packBySection(items, capacity, monitors)
	got := drawRack(mods, items, units, capacity, monitors)
	if !strings.Contains(got, "▚") {
		t.Fatalf("no monitor drawn:\n%s", got)
	}
	// Every bay must open with one, which is the whole invariant.
	for _, line := range strings.Split(got, "\n") {
		if !strings.HasPrefix(line, "│") || !strings.Contains(line, "bay ") {
			continue
		}
		if !strings.HasPrefix(line, "│▚") {
			t.Errorf("a bay does not begin with its monitor:\n%s", line)
		}
	}
	t.Logf("\n%s", got)
}

func TestBlankPanelIsCountedHonestly(t *testing.T) {
	mods := []rackModule{{Key: "a", Slots: 4}}
	items := []packItem{{Slots: 4, Section: secConsole}}
	got := drawRack(mods, items, packBySection(items, 20, nil), 20, nil)
	if !strings.Contains(got, "16 blank (80%)") {
		t.Fatalf("blank panel not reported as 16 of 20:\n%s", got)
	}
}
