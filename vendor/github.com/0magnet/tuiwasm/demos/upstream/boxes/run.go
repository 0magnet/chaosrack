// This file replaces the upstream demo's main. Everything else in the package
// is Garrett D'Amore's, unchanged — see ../NOTICE.md.
package boxes

import (
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/tuiwasm/demos"
)

func init() {
	demos.Register(demos.Demo{
		Name:   "boxes",
		Desc:   "tcell's own boxes demo — random boxes, timed",
		Screen: run,
	})
}

// run is upstream's main, with the screen taken rather than made.
//
// The differences from main are only that: it does not create or finalize the
// screen, since the caller owns one that may be a window among several, and it
// drops the timing summary that main printed to stdout after Fini — there is
// no stdout to print it to in a browser window.
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
			}
		}
	}()

	for {
		select {
		case <-quit:
			return nil
		case <-time.After(time.Millisecond * 50):
		}
		makebox(s)
	}
}
