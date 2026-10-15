//go:build unix

package rackhost

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
)

// screenText is the terminal's screen as text, a line a row.
func screenText(v *vterm) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	buf := v.term.Buffer()
	var b strings.Builder
	for r := range v.rows {
		if line := buf.Lines.Get(buf.YDisp + r); line != nil {
			b.WriteString(line.TranslateToString(true, 0, v.cols))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// typeText types s at the rack, a key a character, Enter for a newline.
func typeText(t *testing.T, r *Rack, s string) {
	t.Helper()
	for _, c := range s {
		ev := tcell.NewEventKey(tcell.KeyRune, string(c), tcell.ModNone)
		if c == '\n' {
			ev = tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)
		}
		if !r.TypeKey(ev) {
			t.Fatalf("the rack did not take %q", c)
		}
	}
}

// waitScreen waits for the shell's screen to show want.
func waitScreen(t *testing.T, r *Rack, c *clock, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		watch(t, r, c, 0.05)
		r.mu.Lock()
		text := screenText(r.shellOf(r.model()).term)
		r.mu.Unlock()
		if strings.Contains(text, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("the screen never showed %q:\n%s", want, screenText(r.shellOf(r.model()).term))
}

// The Host Terminal runs the user's shell, which takes what is typed and
// answers on the screen; exited, Enter starts another. Keys are the shell's
// only while it is the model on show.
func TestTheHostTerminalRunsAShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")
	r, c := idleRack(t, "hostterm")
	watch(t, r, c, 0.05)
	typeText(t, r, "echo chaos$((6*7))\n")
	waitScreen(t, r, c, "chaos42")
	typeText(t, r, "exit\n")
	waitScreen(t, r, c, "the shell has exited")
	typeText(t, r, "\n")
	typeText(t, r, "echo again$((1+1))\n")
	waitScreen(t, r, c, "again2")
	typeText(t, r, "exit\n")
	waitScreen(t, r, c, "the shell has exited")

	set(t, r, r.selector, "lorenz")
	if r.TypeKey(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)) {
		t.Error("Lorenz took a key")
	}
}

// On a terminal the shell is its text, sized to the scene, which the shell
// is told; a demo is too, and another model is not text.
func TestATerminalModelIsTextTheSizeOfTheScene(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")
	r, c := idleRack(t, "hostterm")
	if _, cols, rows, ok := r.TextScene(60, 10); !ok || cols != 60 || rows != 10 {
		t.Fatalf("the shell is %dx%d (text %v), want 60x10", cols, rows, ok)
	}
	typeText(t, r, "stty size\n")
	waitScreen(t, r, c, "10 60")
	cells, cols, _, _ := r.TextScene(60, 10)
	var text strings.Builder
	for _, cell := range cells[:cols*3] {
		text.WriteRune(cell.Ch)
	}
	if !strings.Contains(text.String(), "stty size") {
		t.Errorf("the cells do not show what was typed: %q", text.String())
	}
	typeText(t, r, "exit\n")
	waitScreen(t, r, c, "the shell has exited")

	set(t, r, r.selector, "termanim", "termanim-pick", "matrix")
	deadline := time.Now().Add(5 * time.Second)
	for {
		cells, cols, rows, ok := r.TextScene(50, 12)
		lit := 0
		for _, cell := range cells {
			if cell.Ch > ' ' {
				lit++
			}
		}
		if ok && cols == 50 && rows == 12 && lit > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the demo is %dx%d with %d characters (text %v)", cols, rows, lit, ok)
		}
		time.Sleep(20 * time.Millisecond)
	}
	set(t, r, r.selector, "lorenz")
	if _, _, _, ok := r.TextScene(50, 12); ok {
		t.Error("Lorenz is text")
	}
}
