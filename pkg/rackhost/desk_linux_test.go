package rackhost

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
)

// deskText is the flat desk on w by h cells, as text.
func deskText(t *testing.T, r *Rack, w, h int) string {
	t.Helper()
	cells, cols, rows, ok := r.TextScene(w, h)
	if !ok {
		t.Fatal("the flat desk is not text")
	}
	var b strings.Builder
	for y := range rows {
		for x := range cols {
			b.WriteRune(cells[y*cols+x].Ch)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// waitDesk waits for the flat desk to show want.
func waitDesk(t *testing.T, r *Rack, want string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		text := deskText(t, r, 120, 40)
		if strings.Contains(text, want) {
			return text
		}
		if time.Now().After(deadline) {
			t.Fatalf("the desk never showed %q:\n%s", want, text)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func altKey(s string) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, s, tcell.ModAlt) }

// The desk starts with a terminal, in which open opens a window; Alt+N
// focuses one, Alt+W closes it, and a window whose program ends closes.
func TestTheDeskOpensWindows(t *testing.T) {
	r, _ := idleRack(t, "desk")
	waitDesk(t, r, "1 terminal")
	typeText(t, r, "open matrix\n")
	text := waitDesk(t, r, "2 matrix")
	if !strings.Contains(text, "2*matrix") {
		t.Errorf("the window opened is not the one focused:\n%s", text)
	}
	typeText(t, r, "open nosuch\n") // into matrix, which takes no typing
	if !r.TypeKey(altKey("1")) {
		t.Fatal("the desk did not take Alt+1")
	}
	waitDesk(t, r, "1*terminal")
	typeText(t, r, "open nosuch\n")
	waitDesk(t, r, "no app called nosuch")
	r.TypeKey(altKey("2"))
	r.TypeKey(altKey("w"))
	waitDesk(t, r, "1*terminal")
	if text := deskText(t, r, 120, 40); strings.Contains(text, "2 matrix") || strings.Contains(text, "2*matrix") {
		t.Errorf("Alt+W left matrix open:\n%s", text)
	}
	typeText(t, r, "exit\n")
	deadline := time.Now().Add(10 * time.Second)
	for strings.Contains(deskText(t, r, 120, 40), " 1 terminal ") {
		if time.Now().After(deadline) {
			t.Fatal("exit did not close the terminal's window")
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.TypeKey(altKey("t"))
	waitDesk(t, r, "1*terminal")
}

// Each of the desktops in space draws its windows, a quad each.
func TestTheDesktopsInSpaceDraw(t *testing.T) {
	r, c := idleRack(t, "desk")
	r.mu.Lock()
	r.desk.sync()
	r.desk.open("clock")
	r.mu.Unlock()
	for _, style := range []string{"flat", "glass", "cube", "metisse", "bump"} {
		set(t, r, "desk-style", style)
		watch(t, r, c, 0.1)
		r.mu.Lock()
		h := r.hear("desk", true, 80)
		r.mu.Unlock()
		if len(h.quads) != 2 {
			t.Errorf("%s: %d quads for 2 windows", style, len(h.quads))
		}
		if n, _ := lit(frame(t, r)); n == 0 {
			t.Errorf("%s drew nothing", style)
		}
	}
	if _, _, _, ok := r.TextScene(80, 24); ok {
		t.Error("a desktop in space is text")
	}
	set(t, r, "desk-style", "flat")
	if _, _, _, ok := r.TextScene(80, 24); !ok {
		t.Error("the flat desk is not text")
	}
}
