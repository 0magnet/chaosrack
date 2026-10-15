package rackhost

import (
	"github.com/gdamore/tcell/v3"
)

// keyBytes is what a terminal sends a program for key ev: the bytes an
// xterm would, with the cursor keys in application mode when appCursor says
// the program asked for it (vim and less do). Nil for a key with no bytes.
func keyBytes(ev *tcell.EventKey, appCursor bool) []byte {
	k, mod := ev.Key(), ev.Modifiers()
	alt := func(b []byte) []byte {
		if mod&(tcell.ModAlt|tcell.ModMeta) != 0 {
			return append([]byte{0x1b}, b...)
		}
		return b
	}
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		return alt([]byte{byte(k-tcell.KeyCtrlA) + 1})
	}
	if k == tcell.KeyRune {
		s := ev.Str()
		if mod&tcell.ModCtrl != 0 && len(s) == 1 {
			if b, ok := ctrlByte(s[0]); ok {
				return alt([]byte{b})
			}
		}
		return alt([]byte(s))
	}
	if b, ok := cursorKeys[k]; ok {
		if appCursor {
			return alt([]byte{0x1b, 'O', b})
		}
		return alt([]byte{0x1b, '[', b})
	}
	if k == tcell.KeyTab && mod&tcell.ModShift != 0 {
		return []byte("\x1b[Z")
	}
	if s, ok := specialKeys[k]; ok {
		return alt([]byte(s))
	}
	if k < 0x20 {
		// A control character as itself: tcell reports Ctrl+[ and the like so.
		return alt([]byte{byte(k)}) //nolint:gosec // under 0x20
	}
	return nil
}

// ctrlByte is the control character Ctrl with c makes, as a terminal sends
// it.
func ctrlByte(c byte) (byte, bool) {
	switch {
	case c >= 'a' && c <= 'z':
		return c - 'a' + 1, true
	case c >= '@' && c <= '_': // @, A-Z, [, \, ], ^, _
		return c - '@', true
	case c == ' ':
		return 0, true
	case c == '?':
		return 0x7f, true
	}
	return 0, false
}

// cursorKeys are the keys whose sequence is ESC [ or ESC O and a letter, by
// the cursor-key mode.
var cursorKeys = map[tcell.Key]byte{
	tcell.KeyUp: 'A', tcell.KeyDown: 'B', tcell.KeyRight: 'C', tcell.KeyLeft: 'D',
	tcell.KeyHome: 'H', tcell.KeyEnd: 'F',
}

// specialKeys are the other keys a terminal sends a sequence for.
var specialKeys = map[tcell.Key]string{
	tcell.KeyEnter:     "\r",
	tcell.KeyTab:       "\t",
	tcell.KeyBacktab:   "\x1b[Z",
	tcell.KeyBackspace: "\x7f",
	tcell.KeyEscape:    "\x1b",
	tcell.KeyInsert:    "\x1b[2~",
	tcell.KeyDelete:    "\x1b[3~",
	tcell.KeyPgUp:      "\x1b[5~",
	tcell.KeyPgDn:      "\x1b[6~",
	tcell.KeyF1:        "\x1bOP",
	tcell.KeyF2:        "\x1bOQ",
	tcell.KeyF3:        "\x1bOR",
	tcell.KeyF4:        "\x1bOS",
	tcell.KeyF5:        "\x1b[15~",
	tcell.KeyF6:        "\x1b[17~",
	tcell.KeyF7:        "\x1b[18~",
	tcell.KeyF8:        "\x1b[19~",
	tcell.KeyF9:        "\x1b[20~",
	tcell.KeyF10:       "\x1b[21~",
	tcell.KeyF11:       "\x1b[23~",
	tcell.KeyF12:       "\x1b[24~",
}
