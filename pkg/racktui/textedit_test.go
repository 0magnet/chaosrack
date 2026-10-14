package racktui

import (
	"testing"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/controlspec"
)

// promptRack is a fakeRack that asks for text once.
type promptRack struct {
	fakeRack
	ask string
}

func (r *promptRack) Prompt() (id, ask string, ok bool) {
	if r.ask == "" {
		return "", "", false
	}
	id, r.ask = r.ask, ""
	return id, "a path", true
}

// Enter opens a text control, keys type into it (q and p among them), and
// Enter sets it; Escape leaves it as it was. A source that asks for text
// opens its control by itself.
func TestTypingIntoATextControl(t *testing.T) {
	text := Control{ControlInfo: controlspec.ControlInfo{ID: "banner", IsText: true}, Value: "ab"}
	f := &promptRack{fakeRack: fakeRack{ctls: []Control{text}}}
	p := newPanel(&f.fakeRack)
	p.src = f
	key := func(k tcell.Key, s string) {
		if p.key(tcell.NewEventKey(k, s, tcell.ModNone)) {
			t.Fatalf("%q quit the panel while typing", s)
		}
	}
	key(tcell.KeyEnter, "")
	if p.edit == nil || p.edit.buf != "ab" {
		t.Fatalf("Enter did not open the text control: %+v", p.edit)
	}
	key(tcell.KeyBackspace, "")
	for _, r := range "qp" {
		key(tcell.KeyRune, string(r))
	}
	key(tcell.KeyEnter, "")
	if got := f.ctls[0].Value; got != "aqp" {
		t.Errorf("the control was set to %q, want aqp", got)
	}
	key(tcell.KeyEnter, "")
	key(tcell.KeyRune, "z")
	key(tcell.KeyEscape, "")
	if f.ctls[0].Value != "aqp" || p.edit != nil {
		t.Error("Escape set what was typed")
	}
	f.ask = "banner"
	p.takePrompt()
	if p.edit == nil || p.edit.id != "banner" || p.edit.ask != "a path" {
		t.Errorf("the prompt did not open its control: %+v", p.edit)
	}
}
