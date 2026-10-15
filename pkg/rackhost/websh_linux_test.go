package rackhost

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"
)

// The Terminal runs websh, which takes what is typed and answers on the
// screen, and a typed Ctrl+C interrupts the command it is running.
func TestTheTerminalRunsWebsh(t *testing.T) {
	r, c := idleRack(t, "terminal")
	watch(t, r, c, 0.05)
	typeText(t, r, "echo websh$((6*7))\n")
	waitScreen(t, r, c, "websh42")
	typeText(t, r, "sleep 30; echo slept\n")
	time.Sleep(300 * time.Millisecond)
	begun := time.Now()
	if !r.TypeKey(tcell.NewEventKey(tcell.KeyCtrlC, "", tcell.ModCtrl)) {
		t.Fatal("the Terminal did not take Ctrl+C")
	}
	// Quoted apart, so only the output, not the line typed, shows it whole.
	typeText(t, r, "echo af''ter\n")
	waitScreen(t, r, c, "after")
	if time.Since(begun) > 10*time.Second {
		t.Error("Ctrl+C did not interrupt the sleep")
	}
	r.mu.Lock()
	slept := strings.Contains(screenText(r.websh.term), "\nslept")
	r.mu.Unlock()
	if slept {
		t.Error("the line went on to its echo after Ctrl+C")
	}
	typeText(t, r, "exit\n")
	waitScreen(t, r, c, "the shell has exited")
}
