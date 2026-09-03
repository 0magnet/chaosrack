// This file replaces the upstream demo's main. Everything else in the
// package is Garrett D'Amore's, unchanged — see ../NOTICE.md.
package unicode

import (
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/encoding"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name:   "unicode",
		Desc:   "tcell's own unicode demo — wide, combining and emoji glyphs",
		Screen: run,
	})
}

// run is upstream's main, with the screen taken rather than made and the
// blocking wait left to the caller's goroutine.
//
// This one is the useful test of the adapter: double-wide CJK, combining
// marks, zero-width joiners and regional indicators are where a cell grid and
// an escape-sequence translation disagree if anything is wrong.
func run(s tcell.Screen, _, _ int) error {
	encoding.Register()

	plain := tcell.StyleDefault
	bold := style.Bold(true)

	s.SetStyle(tcell.StyleDefault.
		Foreground(tcell.ColorBlack).
		Background(tcell.ColorWhite))
	s.Clear()
	s.SetTitle("Unicode Demonstration -- 🤯")

	style = bold
	putln(s, "Press ESC to Exit")
	putln(s, "Character set: "+s.CharacterSet())
	style = plain

	putln(s, "Icelandic: október")
	putln(s, "Arabic:    أكتوبر")
	putln(s, "Russian:   октября")
	putln(s, "Greek:     Οκτωβρίου")
	putln(s, "Chinese:   十月 (note, two double wide characters)")
	putln(s, "Combining: A\u030a (should look like Angstrom)")
	putln(s, "Emoticon:  \U0001f618 (blowing a kiss)")
	putln(s, "Airplane:  \u2708 (fly away)")
	putln(s, "Command:   \u2318 (mac clover key)")
	putln(s, "Enclose:   !\u20e3 (should be enclosed exclamation)")
	putln(s, "ZWJ:       \U0001f9db\u200d\u2640 (female vampire)")
	putln(s, "ZWJ:       \U0001f9db\u200d\u2642 (male vampire)")
	putln(s, "Family:    \U0001f469\u200d\U0001f467\u200d\U0001f467 (woman girl girl)\n")
	putln(s, "Region:    \U0001f1fa\U0001f1f8 (USA! USA!)\n")
	putln(s, "")
	putln(s, "Box:")
	putln(s, "┌─┬─┬──┐")
	putln(s, "│·│§│月│ (bullet, lantern, Swiss)")
	putln(s, "├─┼─┼──┤")
	putln(s, "│A│1│😘│ (A, 1, Kiss)")
	putln(s, "├─┼─┼──┤")
	putln(s, "│·│§│🇨🇭│ (bullet, lantern, Swiss)")
	putln(s, "├─┼─┼──┤")
	putln(s, "│◆│↑│  │ (diamond, up arrow, empty)")
	putln(s, "└─┴─┴──┘")

	s.Show()
	// The queue closes when the screen is finalised — the window closed — which
	// is what a nil event from PollEvent used to mean.
	for ev := range s.EventQ() {
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyEnter, tcell.KeyCtrlC:
				return nil
			case tcell.KeyCtrlL:
				s.Sync()
			}
		case *tcell.EventResize:
			s.Sync()
		}
	}
	return nil
}
