package rackspec

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"
)

const mmPerInch = 25.4

func close(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %g, want %g (±%g)", name, got, want, tol)
	}
}

// The constants are conversions of imperial standards, and a conversion that
// has drifted is the whole failure mode this package exists to prevent.
func TestStandardsAreTheStandards(t *testing.T) {
	close(t, "19 inch panel", PanelWidth19, 19*mmPerInch, 0.01)
	close(t, "HP", HP, 0.2*mmPerInch, 0.001)
	close(t, "U", U, 1.75*mmPerInch, 0.001)
	close(t, "tact switch", TactSwitch, 6, 0.001)
	close(t, "toggle bushing", ToggleBushing, 0.25*mmPerInch, 0.001)
	close(t, "seven-segment digit", DigitHeight, 0.2*mmPerInch, 0.001)
	close(t, "quarter-inch jack hole", JackHoleQuarterInch, 9.5, 0.001)
}

// The row has to fit the frame, with room left over for the frame.
func TestRowFitsTheFrame(t *testing.T) {
	if RowWidth() > RailOpening {
		t.Errorf("a %d HP row is %.2f mm, wider than the %.0f mm rail opening", RowHP, RowWidth(), RailOpening)
	}
	if RowWidth() > PanelWidth19 {
		t.Errorf("row %.2f mm exceeds the 19-inch panel itself", RowWidth())
	}
	// The leftover is the card guides, side members and mounting ears. If it
	// ever came out at zero or negative, the numbers would be describing a
	// frame with no frame in it.
	if slack := RailOpening - RowWidth(); slack < 5 || slack > 60 {
		t.Errorf("%.1f mm left between the row and the rails — not a plausible frame", slack)
	}
}

// A 3U panel has to clear the 3U opening, or it does not go in.
func TestPanelClearsTheOpening(t *testing.T) {
	if PanelHeight3U >= 3*U {
		t.Errorf("a 3U panel of %.1f mm does not clear a %.2f mm opening", PanelHeight3U, 3*U)
	}
}

// The point of milling a panel narrow: the PITCH is a whole number of HP, so
// module edges line up down the rack however the modules are combined. An
// N-slot module spans the N-1 seams inside it but not the one after it, so it
// measures one seam short of N whole pitches.
func TestSlotsTileToWholeHP(t *testing.T) {
	close(t, "slot pitch in HP", SlotPitch/HP, ModuleHP, 1e-9)
	for n := 1; n <= 12; n++ {
		width := float64(n)*SlotWidth + float64(n-1)*Seam
		close(t, "width of "+strconv.Itoa(n)+" slots",
			width, float64(n)*SlotPitch-Seam, 1e-9)
		// And the edge it leaves behind — where the next module starts — is
		// on the whole-HP grid.
		close(t, "right edge of "+strconv.Itoa(n)+" slots in HP",
			(width+Seam)/HP, float64(n*ModuleHP), 1e-9)
	}
}

// The content column plus the module's padding is what chose ModuleHP; if a
// narrower module would hold it, the choice is stale.
func TestModuleWidthHoldsItsContent(t *testing.T) {
	const contentColumn = 29.0 // --kcol, 116 px
	const padding = 2.0        // 8 px each side
	need := contentColumn + padding
	if SlotWidth < need {
		t.Errorf("a %d HP slot is %.2f mm, too narrow for %.1f mm of content", ModuleHP, SlotWidth, need)
	}
	if narrower := float64(ModuleHP-1)*HP - Seam; narrower >= need {
		t.Errorf("%d HP (%.2f mm) would also hold %.1f mm of content, so %d HP is wider than it needs to be",
			ModuleHP-1, narrower, need, ModuleHP)
	}
}

// Nothing may be drawn smaller than the parts bin allows.
func TestPinsAreNotJacks(t *testing.T) {
	if PinHead >= PinPitch {
		t.Errorf("pin head %.2f mm does not fit a %.2f mm pitch", PinHead, PinPitch)
	}
	if JackHole35 <= PinHead {
		t.Error("a 3.5 mm jack hole should be wider than a matrix pin's head; the sizes have been swapped")
	}
	if JackPitch35 <= PinPitch {
		t.Error("jacks need more room than pins; the pitches have been swapped")
	}
}

// The stylesheet and this package must agree about the scale, or the panel is
// drawn to one system and documented as another.
func TestStylesheetDeclaresTheSameScale(t *testing.T) {
	css, err := os.ReadFile("../attractor/panel.css")
	if err != nil {
		t.Skip("panel.css not readable from here:", err)
	}
	// --mm:calc(4px*var(--kscale,1))
	re := regexp.MustCompile(`--mm:\s*calc\(([0-9.]+)px`)
	m := re.FindSubmatch(css)
	if m == nil {
		t.Fatal("panel.css does not declare --mm; the scale is not written down where the panel is drawn")
	}
	got, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		t.Fatalf("--mm is not a number: %v", err)
	}
	if got != PxPerMM {
		t.Errorf("panel.css draws at %g px/mm, rackspec says %g", got, PxPerMM)
	}

	// The scale agreeing is not enough: the module box and the seam are the
	// numbers the rack's slot arithmetic uses, and a stylesheet that drew a
	// different module from the one the code snaps to would tile crookedly.
	for _, c := range []struct {
		name, pattern string
		want          float64
	}{
		{"--mod-w", `--mod-w:calc\(([0-9.]+)\*var\(--mm\)\)`, SlotWidth},
		{"--mod-h", `--mod-h:calc\(([0-9.]+)\*var\(--mm\)\)`, PanelHeight3U},
		{".modules gap", `\.modules\{[^}]*gap:calc\(([0-9.]+)\*var\(--mm\)\)`, Seam},
		{".mxpin", `\.mxpin\{width:calc\(([0-9.]+)\*var\(--mm\)\)`, PinHead},
		{".pslot", `\.pslot\{width:calc\(([0-9.]+)\*var\(--mm\)\)`, TactSwitch},
	} {
		m := regexp.MustCompile(c.pattern).FindSubmatch(css)
		if m == nil {
			t.Errorf("panel.css: no %s in millimeters", c.name)
			continue
		}
		got, err := strconv.ParseFloat(string(m[1]), 64)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		close(t, "panel.css "+c.name, got, c.want, 0.005)
	}

	// The pin grid is specified by its PITCH; the CSS can only state the gap,
	// so the gap has to come out at pitch minus head.
	m = regexp.MustCompile(`\.mxgrid\{[^}]*gap:calc\(([0-9.]+)\*var\(--mm\)\)`).FindSubmatch(css)
	if m == nil {
		t.Error("panel.css: no .mxgrid gap in millimeters")
		return
	}
	gap, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		t.Fatalf(".mxgrid gap: %v", err)
	}
	close(t, "pin pitch (head + gap)", PinHead+gap, PinPitch, 0.005)
}

// A division is the unit BOTH axes are read in, so it has to be square. A
// graticule whose divisions are oblong makes every volts-per-division
// reading a different size from every seconds-per-division one.
func TestScopeDivisionsAreSquare(t *testing.T) {
	if got := ScopeFaceW / ScopeDivX; got != ScopeDivMM {
		t.Errorf("a horizontal division is %v mm, want %v", got, ScopeDivMM)
	}
	if got := ScopeFaceH / ScopeDivY; got != ScopeDivMM {
		t.Errorf("a vertical division is %v mm, want %v", got, ScopeDivMM)
	}
}

// The tube has to fit the row it is let into, with enough panel left over
// for the clusters a scope is actually operated by. If it does not, the
// answer is a bigger panel, not a smaller screen.
func TestScopeFaceFitsItsRowWithRoomToOperateIt(t *testing.T) {
	if ScopeHP != RowHP {
		t.Errorf("the scope is %d HP, want a whole %d HP row", ScopeHP, RowHP)
	}
	if ScopeFaceOuterW >= RowWidth() {
		t.Errorf("the tube is %v mm wide in a %v mm row, leaving no panel",
			ScopeFaceOuterW, RowWidth())
	}
	// Three clusters of large knobs, two knobs wide each, is the floor for a
	// panel you could operate: below that the screen has taken the panel.
	want := 6 * KnobLarge
	if got := ScopeControlsW(); got < want {
		t.Errorf("%v mm left for the controls, want at least %v (six %v mm knobs)",
			got, want, KnobLarge)
	}
}

// The tube is shorter than the panel it is let into, or it does not fit the
// opening at all.
func TestScopeFaceFitsA3UPanel(t *testing.T) {
	if ScopeFaceOuterH >= PanelHeight3U {
		t.Errorf("the tube is %v mm tall on a %v mm panel", ScopeFaceOuterH, PanelHeight3U)
	}
}
