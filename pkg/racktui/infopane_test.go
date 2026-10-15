package racktui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/vt"

	"github.com/0magnet/chaosrack/manual"
	"github.com/0magnet/chaosrack/pkg/controlspec"
)

// manualRack is a fake rack with a manual: two controls on bay 2, a model
// on show.
type manualRack struct {
	fakeRack
	asked []int
}

func (m *manualRack) Manual(bay int) (ManualPage, error) {
	m.asked = append(m.asked, bay)
	return ManualPage{
		Model: "Torus", ModelText: "A ring.",
		Title: "Bay 2 — Visual",
		Mods: []manual.Module{{Name: "Torus", Range: "2.1.1–2.1.2", Cells: []manual.Cell{
			{Loc: "2.1.1", Tip: "R — the major radius. More."},
			{Loc: "2.1.2", Tip: "r — the minor radius."},
		}}},
	}, nil
}

func newManualPanel() (*panel, *manualRack) {
	at := func(id, loc string) Control {
		return Control{ControlInfo: controlspec.ControlInfo{ID: id, Label: id, Min: 0, Max: 5, Step: 0.1}, Value: "1", Loc: loc}
	}
	m := &manualRack{fakeRack: fakeRack{ctls: []Control{at("torus-R", "2.1.1"), at("torus-r", "2.1.2")}}}
	p := &panel{src: m}
	p.reload()
	return p, m
}

func screenOf(t *testing.T, w, h int) tcell.Screen {
	t.Helper()
	sc, err := tcell.NewTerminfoScreenFromTty(vt.NewMockTerm(vt.MockOptSize{X: vt.Col(w), Y: vt.Row(h)}))
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sc.Fini)
	return sc
}

// textOf is what screen row y says from column x on.
func textOf(sc tcell.Screen, x, y int) string {
	w, _ := sc.Size()
	var b strings.Builder
	for ; x < w; x++ {
		s, _, _ := sc.Get(x, y)
		if s == "" {
			s = " "
		}
		b.WriteString(s)
	}
	return strings.TrimRight(b.String(), " ")
}

func key(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, string(r), tcell.ModNone) }

// i opens the pane over the right of the screen: the model's description,
// then the page of the cursor's bay, its control marked.
func TestTheInfoPaneShowsTheManual(t *testing.T) {
	p, m := newManualPanel()
	sc := screenOf(t, 120, 30)
	p.key(key('i'))
	p.list = true
	p.draw(sc)
	var all []string
	for y := range 29 {
		all = append(all, textOf(sc, 120-infoWidth(120), y))
	}
	text := strings.Join(all, "\n")
	for _, want := range []string{"Torus", "A ring.", "Bay 2 — Visual", "2.1.1", "the major radius", "2.1.2"} {
		if !strings.Contains(text, want) {
			t.Errorf("the pane has no %q:\n%s", want, text)
		}
	}
	if len(m.asked) == 0 || m.asked[0] != 2 {
		t.Errorf("the pane asked for bays %v, want the cursor's, 2", m.asked)
	}
	p.key(key('i'))
	p.draw(sc)
	if strings.Contains(textOf(sc, 120-infoWidth(120), 2), "A ring.") {
		t.Error("i again left the pane open")
	}
}

// Moving the cursor puts the first sentence of its control's entry on the
// status line.
func TestTheStatusLineHints(t *testing.T) {
	p, _ := newManualPanel()
	sc := screenOf(t, 120, 30)
	p.list = true
	p.move(1)
	p.move(-1)
	p.draw(sc)
	if got := textOf(sc, 0, 29); !strings.Contains(got, "torus-R (2.1.1)") || !strings.Contains(got, "R — the major radius.") || strings.Contains(got, "More.") {
		t.Errorf("the status line says %q", got)
	}
}
