package racktui

import (
	"strings"
	"testing"

	"github.com/0magnet/chaosrack/pkg/controlspec"
)

func ctl(mod, id, label string, min, max, step, def float64, val string) Control {
	return Control{ControlInfo: controlspec.ControlInfo{
		ID: id, Label: label, Module: mod, Min: min, Max: max, Step: step, Def: def,
	}, Value: val}
}

func sel(mod, id, label string, opts []string, val string) Control {
	return Control{ControlInfo: controlspec.ControlInfo{
		ID: id, Label: label, Module: mod, IsSelect: true,
	}, Options: opts, Value: val}
}

func TestAKnobPointsWhereTheValueIs(t *testing.T) {
	lo := strings.Join(drawKnob(0), "\n")
	mid := strings.Join(drawKnob(0.5), "\n")
	hi := strings.Join(drawKnob(1), "\n")
	if lo == mid || mid == hi || lo == hi {
		t.Fatalf("a knob drew the same at three positions:\n%s\n%s\n%s", lo, mid, hi)
	}
	// Every dial is the same size, or the panels below it move as it turns.
	for _, f := range []float64{0, 0.3, 0.5, 0.9, 1} {
		k := drawKnob(f)
		if len(k) != 3 {
			t.Fatalf("knob at %v is %d rows", f, len(k))
		}
		for _, r := range k {
			if n := len([]rune(r)); n != 5 {
				t.Fatalf("knob at %v has a %d-cell row %q", f, n, r)
			}
		}
	}
}

func TestAKnobClampsRatherThanWrapping(t *testing.T) {
	// A value outside its range is a bug somewhere else; a pointer that wraps
	// to the minimum because of it would read as the opposite of the truth.
	if strings.Join(drawKnob(2), "") != strings.Join(drawKnob(1), "") {
		t.Fatal("a knob past full scale did not stop at full scale")
	}
	if strings.Join(drawKnob(-1), "") != strings.Join(drawKnob(0), "") {
		t.Fatal("a knob below zero did not stop at zero")
	}
}

func TestATwoPositionSwitchIsALamp(t *testing.T) {
	c := sel("test", "sw", "on", []string{"off", "on"}, "on")
	got := drawControl(c)
	if len(got) != 1 || !strings.HasPrefix(got[0], "◉") {
		t.Fatalf("a switch that is on drew %q, want a lit lamp", got)
	}
	c.Value = "off"
	if got := drawControl(c); !strings.HasPrefix(got[0], "○") {
		t.Fatalf("a switch that is off drew %q, want a dark lamp", got)
	}
}

func TestARotaryShowsWhichDetentItIsOn(t *testing.T) {
	c := sel("test", "r", "rate", []string{"100", "200", "500"}, "200")
	got := strings.Join(drawControl(c), "\n")
	if !strings.Contains(got, "▪200") {
		t.Fatalf("the chosen detent is not marked:\n%s", got)
	}
	if strings.Count(got, "▪") != 1 {
		t.Fatalf("more than one detent is marked:\n%s", got)
	}
}

func TestAPanelIsAFixedWidthBox(t *testing.T) {
	p := modulePanel{Name: "loudness", Ctls: []Control{
		ctl("loudness", "tgt", "tgt", -40, 0, 1, -23, "-23"),
		sel("loudness", "rate", "rate", []string{"100", "200"}, "200"),
	}}
	lines, rowOf := drawPanel(p)
	if len(lines) != len(rowOf) {
		t.Fatalf("%d lines but %d row owners", len(lines), len(rowOf))
	}
	for i, l := range lines {
		if n := len([]rune(l)); n != panelWidth {
			t.Fatalf("line %d is %d cells, want %d: %q", i, n, panelWidth, l)
		}
	}
	if !strings.Contains(lines[0], "LOUDNESS") {
		t.Fatalf("the panel is not silkscreened: %q", lines[0])
	}
}

func TestTheRackWrapsAndEveryControlHasAPlace(t *testing.T) {
	var ctls []Control
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		ctls = append(ctls,
			ctl(m, m+"1", "one", 0, 10, 1, 0, "5"),
			ctl(m, m+"2", "two", 0, 10, 1, 0, "7"))
	}
	panels, _ := groupByModule(ctls)
	if len(panels) != 5 {
		t.Fatalf("grouped into %d panels, want 5", len(panels))
	}
	lines, spots := layoutRack(panels, 80)
	if len(spots) != len(ctls) {
		t.Fatalf("%d controls have a place, want %d", len(spots), len(ctls))
	}
	for _, s := range spots {
		if s.Y0 < 0 || s.Y1 >= len(lines) || s.Y1 < s.Y0 {
			t.Fatalf("a control landed outside the drawing: %+v of %d lines", s, len(lines))
		}
	}
	// Eighty columns fits two 26-cell panels and a space, so five panels wrap
	// to three rows rather than running off the side.
	if len(lines) < 3 {
		t.Fatalf("five panels drew %d lines — they did not wrap", len(lines))
	}
}

func TestAControlWithNoModuleStillGetsAPanel(t *testing.T) {
	panels, _ := groupByModule([]Control{ctl("", "x", "x", 0, 1, 0.1, 0, "0")})
	if len(panels) != 1 || panels[0].Name != "console" {
		t.Fatalf("an unhomed control gave %+v", panels)
	}
}
