// This file replaces the upstream demo's main. Everything else in the package
// is Garrett D'Amore's, unchanged — see ../NOTICE.md.
package colors

import (
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name:   "colors",
		Desc:   "tcell's own colors demo — boxes cycling through the palette",
		Screen: run,
	})
}

// run is upstream's main, with the screen taken rather than made.
//
// The color walk is the point: it moves through the space one step at a time,
// so every box is a style the adapter has not sent before. That is the case
// the per-cell style dedup does not help with, which makes this the demo that
// says what the translation costs when nothing repeats.
func run(s tcell.Screen, _, _ int) error {
	s.SetStyle(tcell.StyleDefault.
		Foreground(tcell.ColorBlack).
		Background(tcell.ColorWhite))
	s.Clear()

	quit := make(chan struct{})
	go func() {
		for {
			ev, ok := <-s.EventQ()
			if !ok {
				// The queue closes when the screen is finalised, which happens
				// when the window closes. Without this the goroutine spins on a
				// dead screen for the life of the page.
				close(quit)
				return
			}
			switch ev := ev.(type) {
			case *tcell.EventKey:
				switch ev.Key() {
				case tcell.KeyEscape, tcell.KeyEnter, tcell.KeyCtrlC:
					close(quit)
					return
				case tcell.KeyCtrlL:
					s.Sync()
				}
			case *tcell.EventResize:
				s.Sync()
			case nil:
				// The screen was finalized — the window closed.
				close(quit)
				return
			}
		}
	}()

	cnt := 0
	for {
		select {
		case <-quit:
			return nil
		case <-time.After(time.Millisecond * 50):
		}
		makebox(s)
		cnt++
		if cnt%(256/int(inc)) == 0 {
			if flipcoin() {
				redi = -redi
			}
			if flipcoin() {
				grni = -grni
			}
			if flipcoin() {
				blui = -blui
			}
		}
	}
}
