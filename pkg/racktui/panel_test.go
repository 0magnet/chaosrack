package racktui

import (
	"errors"
	"testing"

	"github.com/0magnet/chaosrack/pkg/controlspec"
	"github.com/0magnet/chaosrack/pkg/racksurface"
)

// fakeRack is a rack that remembers what it was told.
type fakeRack struct {
	ctls []Control
	sets []string // "id=value", in order
	fail bool
}

// Modules reports one two-slot module per distinct module name among the
// controls, so a fake rack is described by its controls alone.
func (f *fakeRack) Modules() ([]racksurface.Item, int, error) {
	var out []racksurface.Item
	seen := map[string]bool{}
	for _, c := range f.ctls {
		k := c.Module
		if k == "" {
			k = "console"
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, racksurface.Item{Key: k, Title: k, Slots: 2, Section: "test"})
	}
	return out, 12, nil
}

func (f *fakeRack) Controls() ([]Control, error) {
	out := make([]Control, len(f.ctls))
	copy(out, f.ctls)
	return out, nil
}

func (f *fakeRack) Set(id, value string) error {
	if f.fail {
		return errors.New("the rack said no")
	}
	f.sets = append(f.sets, id+"="+value)
	// A real rack answers with what it TOOK, so the fake does too.
	for i := range f.ctls {
		if f.ctls[i].ID == id {
			f.ctls[i].Value = value
		}
	}
	return nil
}

func dial(id string, min, max, step, def float64, val string) Control {
	return Control{
		ControlInfo: controlspec.ControlInfo{ID: id, Label: id, Min: min, Max: max, Step: step, Def: def},
		Value:       val,
	}
}

func sw(id string, opts []string, def, val string) Control {
	return Control{
		ControlInfo: controlspec.ControlInfo{ID: id, Label: id, IsSelect: true, SelectDef: def},
		Options:     opts, Value: val,
	}
}

func newPanel(f *fakeRack) *panel {
	p := &panel{src: f}
	p.reload()
	return p
}

func TestTurningADialMovesItOneStep(t *testing.T) {
	f := &fakeRack{ctls: []Control{dial("zoom", -8, 8, 0.1, 0, "1.5")}}
	p := newPanel(f)
	p.nudge(1)
	if len(f.sets) != 1 || f.sets[0] != "zoom=1.6" {
		t.Fatalf("turning up sent %v, want [zoom=1.6]", f.sets)
	}
	p.nudge(-1)
	if f.sets[1] != "zoom=1.5" {
		t.Fatalf("turning back sent %q, want zoom=1.5", f.sets[1])
	}
}

func TestADialStopsAtItsEnds(t *testing.T) {
	// A knob has ends. Sending a value past one and letting the rack clamp it
	// would work, but the panel would then show a number the rack does not
	// have until the next reload.
	f := &fakeRack{ctls: []Control{dial("zoom", 0, 1, 0.5, 0, "1")}}
	p := newPanel(f)
	p.nudge(1)
	if f.sets[0] != "zoom=1" {
		t.Fatalf("turning past the end sent %q, want zoom=1", f.sets[0])
	}
}

func TestADialWithNoStepStillTurns(t *testing.T) {
	f := &fakeRack{ctls: []Control{dial("n", 0, 10, 0, 0, "3")}}
	p := newPanel(f)
	p.nudge(1)
	if f.sets[0] != "n=4" {
		t.Fatalf("a step of zero sent %q, want n=4 — a knob that cannot turn is not a knob", f.sets[0])
	}
}

func TestASwitchMovesOneDetentAndDoesNotWrap(t *testing.T) {
	// A rotary switch has stops. Wrapping from the last detent to the first
	// is how a timebase knob takes you from 0.5 s/div to the fastest sweep
	// in one keystroke.
	opts := []string{"100", "200", "500", "1000"}
	f := &fakeRack{ctls: []Control{sw("lufs-rate", opts, "200", "500")}}
	p := newPanel(f)
	p.nudge(1)
	if f.sets[0] != "lufs-rate=1000" {
		t.Fatalf("one detent up sent %q, want lufs-rate=1000", f.sets[0])
	}
	p.nudge(1)
	if f.sets[1] != "lufs-rate=1000" {
		t.Fatalf("past the last detent sent %q, want it to stay at 1000", f.sets[1])
	}
}

func TestASwitchOnAValueItDoesNotHaveStartsAtTheFirstDetent(t *testing.T) {
	f := &fakeRack{ctls: []Control{sw("s", []string{"a", "b"}, "a", "zzz")}}
	p := newPanel(f)
	p.nudge(1)
	if f.sets[0] != "s=b" {
		t.Fatalf("sent %q, want s=b", f.sets[0])
	}
}

func TestResetUsesTheDefaultTheRegistryKnows(t *testing.T) {
	// Not "what it was when the panel opened": the descriptor's default is
	// the same one the rack's own reset button uses.
	f := &fakeRack{ctls: []Control{
		dial("zoom", -8, 8, 0.1, 2.5, "7"),
		sw("mode", []string{"a", "b"}, "b", "a"),
	}}
	p := newPanel(f)
	p.reset()
	if f.sets[0] != "zoom=2.5" {
		t.Fatalf("resetting a dial sent %q, want zoom=2.5", f.sets[0])
	}
	p.move(1)
	p.reset()
	if f.sets[1] != "mode=b" {
		t.Fatalf("resetting a switch sent %q, want mode=b", f.sets[1])
	}
}

func TestTheFilterNarrowsAndTheCursorStaysInside(t *testing.T) {
	f := &fakeRack{ctls: []Control{
		dial("zoom", 0, 1, 0.1, 0, "0"),
		dial("wf-win", 0, 1, 0.1, 0, "0"),
		dial("wf-rate", 0, 1, 0.1, 0, "0"),
	}}
	p := newPanel(f)
	p.cur = 2
	p.filt = "wf-"
	p.refilter()
	if len(p.ctls) != 2 {
		t.Fatalf("filter left %d controls, want 2", len(p.ctls))
	}
	if p.cur != 1 {
		t.Fatalf("cursor at %d, want 1 — it must not point past the filtered list", p.cur)
	}
	p.filt = "nothing matches this"
	p.refilter()
	if len(p.ctls) != 0 || p.cur != 0 {
		t.Fatalf("an empty filter list left cur=%d len=%d, want 0/0", p.cur, len(p.ctls))
	}
	// And turning a knob that is not there must not panic.
	p.nudge(1)
	if len(f.sets) != 0 {
		t.Fatalf("turning with nothing selected sent %v", f.sets)
	}
}

func TestARefusedChangeIsReportedAndNotShown(t *testing.T) {
	f := &fakeRack{ctls: []Control{dial("zoom", 0, 8, 1, 0, "3")}, fail: true}
	p := newPanel(f)
	p.nudge(1)
	if p.err == "" {
		t.Fatal("a refused change said nothing")
	}
	if p.ctls[0].Value != "3" {
		t.Fatalf("the panel shows %q after a refusal, want the value the rack still has", p.ctls[0].Value)
	}
}

func TestThePanelShowsWhatTheRackTookAndNotWhatWasAsked(t *testing.T) {
	// A control can clamp or quantize. The panel reloads after every change
	// so it shows the rack's answer rather than its own request.
	f := &fakeRack{ctls: []Control{dial("n", 0, 10, 1, 0, "5")}}
	p := newPanel(f)
	p.nudge(1)
	if p.ctls[0].Value != "6" {
		t.Fatalf("panel shows %q, want the reloaded 6", p.ctls[0].Value)
	}
}

// sw2 is a two-state control. The id is fixed: what these tests care about
// is the KIND, and a switch behaves the same whichever one it is.
func sw2(val string) Control {
	return Control{
		ControlInfo: controlspec.ControlInfo{ID: "power", Label: "power", IsSwitch: true},
		Value:       val,
	}
}

// A switch has no range, so the numeric path read "1", added a step of 1 and
// wrote "2" — which the rack reads as OFF. Turning a switch up turned it off.
func TestThrowingASwitchWritesAPosition(t *testing.T) {
	f := &fakeRack{ctls: []Control{sw2("0")}}
	p := newPanel(f)
	p.nudge(1)
	if len(f.sets) != 1 || f.sets[0] != "power=1" {
		t.Fatalf("nudging up set %v, want [power=1]", f.sets)
	}
	p.nudge(-1)
	if f.sets[1] != "power=0" {
		t.Errorf("nudging down set %q, want power=0", f.sets[1])
	}
}

// Either arrow sets the position it points at, so the same key twice does not
// flip it back: a toggle that depends on where it was cannot be driven blind.
func TestASwitchIsNotAToggle(t *testing.T) {
	f := &fakeRack{ctls: []Control{sw2("0")}}
	p := newPanel(f)
	p.nudge(1)
	p.nudge(1)
	for _, s := range f.sets {
		if s != "power=1" {
			t.Fatalf("pressing right twice gave %v; both should be power=1", f.sets)
		}
	}
}

// And it reads as a position rather than a quantity.
func TestASwitchReadsAsAWord(t *testing.T) {
	if got := readingOf(sw2("1")); got != "on" {
		t.Errorf("a closed switch reads %q, want on", got)
	}
	if got := readingOf(sw2("0")); got != "off" {
		t.Errorf("an open switch reads %q, want off", got)
	}
}
