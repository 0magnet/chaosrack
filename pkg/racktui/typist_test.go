package racktui

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

// typingRack is a rack whose model on show takes typing.
type typingRack struct {
	fakeRack
	typing bool
	typed  []string
}

func (y *typingRack) TypeKey(ev *tcell.EventKey) bool {
	if !y.typing {
		return false
	}
	y.typed = append(y.typed, ev.Str())
	return true
}

// With the window put away, every key is the model's, q and Escape too, but
// Ctrl+], which brings the window back. With the window up, or with a model
// that takes no typing, they are the panel's.
func TestTypingGoesToTheModelWithTheWindowAway(t *testing.T) {
	y := &typingRack{fakeRack: fakeRack{ctls: []Control{dial("zoom", 0, 1, 0.1, 0, "0")}}, typing: true}
	p := newPanel(&y.fakeRack)
	p.src = y
	q := tcell.NewEventKey(tcell.KeyRune, "q", tcell.ModNone)
	if p.typeKey(q) {
		t.Fatal("the window is up, and the model took q")
	}
	p.winMode = winHidden
	if !p.typeKey(q) || !p.typeKey(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone)) || len(y.typed) != 2 {
		t.Fatal("the model did not take q and Escape with the window away")
	}
	if p.msg != typingMsg {
		t.Errorf("the status line says %q while typing", p.msg)
	}
	for _, back := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyGS, "", tcell.ModNone),
		tcell.NewEventKeyEx(tcell.KeyRune, "]", tcell.ModCtrl, true, 0, 1),
	} {
		p.winMode = winHidden
		if !p.typeKey(back) || p.winMode != winHalf || len(y.typed) != 2 {
			t.Fatalf("%s did not bring the window back", back.Name())
		}
	}
	p.winMode = winHidden
	y.typing = false
	if p.typeKey(q) {
		t.Error("a model that takes no typing took q")
	}
}
