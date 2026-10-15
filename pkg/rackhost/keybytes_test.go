package rackhost

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestKeyBytes(t *testing.T) {
	for _, tc := range []struct {
		ev        *tcell.EventKey
		appCursor bool
		want      string
	}{
		{tcell.NewEventKey(tcell.KeyRune, "a", tcell.ModNone), false, "a"},
		{tcell.NewEventKey(tcell.KeyRune, "é", tcell.ModNone), false, "é"},
		{tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModAlt), false, "\x1bx"},
		{tcell.NewEventKey(tcell.KeyCtrlC, "", tcell.ModCtrl), false, "\x03"},
		{tcell.NewEventKeyEx(tcell.KeyRune, "c", tcell.ModCtrl, true, 0, 1), false, "\x03"},
		{tcell.NewEventKeyEx(tcell.KeyRune, "[", tcell.ModCtrl, true, 0, 1), false, "\x1b"},
		{tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone), false, "\r"},
		{tcell.NewEventKey(tcell.KeyBackspace, "", tcell.ModNone), false, "\x7f"},
		{tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone), false, "\x1b"},
		{tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone), false, "\x1b[A"},
		{tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone), true, "\x1bOA"},
		{tcell.NewEventKey(tcell.KeyDelete, "", tcell.ModNone), false, "\x1b[3~"},
		{tcell.NewEventKey(tcell.KeyTab, "", tcell.ModShift), false, "\x1b[Z"},
		{tcell.NewEventKey(tcell.KeyF5, "", tcell.ModNone), false, "\x1b[15~"},
	} {
		if got := string(keyBytes(tc.ev, tc.appCursor)); got != tc.want {
			t.Errorf("%s (app cursor %v): %q, want %q", tc.ev.Name(), tc.appCursor, got, tc.want)
		}
	}
}
