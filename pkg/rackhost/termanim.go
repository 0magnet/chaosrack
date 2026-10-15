package rackhost

import (
	"image"

	tcell2 "github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/rasterview"
	"github.com/0magnet/tuiwasm/demos"

	// Importing a demo registers it: the page's set (deskdemos_js.go).
	_ "github.com/0magnet/tuiwasm/demos/anim"
	_ "github.com/0magnet/tuiwasm/demos/charts"
	_ "github.com/0magnet/tuiwasm/demos/proxima"
	_ "github.com/0magnet/tuiwasm/demos/styles"
	_ "github.com/0magnet/tuiwasm/demos/tables"
	_ "github.com/0magnet/tuiwasm/demos/upstream/boxes"
	_ "github.com/0magnet/tuiwasm/demos/upstream/colors"
	_ "github.com/0magnet/tuiwasm/demos/upstream/unicode"
)

// The terminal animation model, natively: one of tuiwasm's demos running in
// a terminal in memory, as the page runs it in a terminal parked offscreen,
// and its screen shown on a quad of the terminal's shape.

// animCols and animRows are the terminal a demo runs in: about what the
// page's 900 by 560 box holds at a 9 by 18 cell.
const animCols, animRows = 100, 31

// animDefault is what the model shows before anything is picked.
const animDefault = "matrix"

// animView is the demo running: its name, where it draws, and how to stop it.
type animView struct {
	name string
	term *vterm                  // a demo on tcell v3, or one that writes text
	sim  tcell2.SimulationScreen // a demo on tcell v2
	stop func()
	// done is closed once the demo has ended and its screen is finished.
	done chan struct{}
}

// animRunning is the demo running, the picked one, started if it is not.
// Called with r.mu held.
func (r *Rack) animRunning() *animView {
	name := r.value("termanim-pick")
	if name == "" {
		name = animDefault
	}
	a := &r.anim
	if a.name != name {
		a.close()
		a.start(name)
	}
	return a
}

// animFigure is the picked demo's screen now, on a quad. Called with r.mu
// held.
func (r *Rack) animFigure() *rasterview.Quad {
	a := r.animRunning()
	var pic *image.RGBA
	switch {
	case a.sim != nil:
		pic = simPicture(a.sim)
	case a.term != nil:
		pic = a.term.picture()
	default:
		return nil
	}
	return termQuad(pic)
}

// termQuad is a terminal's picture on a quad of the picture's shape, which
// is the terminal's.
func termQuad(pic *image.RGBA) *rasterview.Quad {
	h := texHalf
	w := h * float64(pic.Rect.Dx()) / float64(pic.Rect.Dy())
	return &rasterview.Quad{Pic: pic, BL: [3]float64{-w, -h, 0}, BR: [3]float64{w, -h, 0}, TL: [3]float64{-w, h, 0}}
}

// isTerminalModel reports whether model key is a terminal's screen, drawn
// flat as an analyzer is.
func isTerminalModel(key string) bool { return key == "termanim" || key == "hostterm" }

// start runs demo name in a terminal of its own.
func (a *animView) start(name string) {
	a.name = name
	d, ok := demos.Lookup(name)
	if !ok {
		return
	}
	ran, done := make(chan struct{}), make(chan struct{})
	a.done = done
	// A demo is asked to end as a person would end it, with Escape at its
	// keyboard, and its screen finished only once it has: finishing a screen
	// a demo is still drawing on races inside tcell. The panel does not wait
	// for it; a demo slow to see the key ends in its own time.
	ended := func(quit, fini func()) func() {
		return func() {
			quit()
			go func() {
				<-ran
				fini()
				close(done)
			}()
		}
	}
	switch {
	case d.Screen != nil:
		t := newVterm(animCols, animRows, false)
		s, err := tcell.NewTerminfoScreenFromTty(t)
		if err == nil {
			err = s.Init()
		}
		if err != nil {
			t.end()
			close(done)
			return
		}
		a.term = t
		a.stop = ended(func() { t.send("\x1b") }, func() { s.Fini(); t.end() })
		go func() {
			defer close(ran)
			_ = d.Screen(s, animCols, animRows) //nolint:errcheck // it ends with its screen
		}()
	case d.ScreenV2 != nil:
		s := tcell2.NewSimulationScreen("UTF-8")
		if err := s.Init(); err != nil {
			close(done)
			return
		}
		s.SetSize(animCols, animRows)
		a.sim = s
		a.stop = ended(func() { s.InjectKey(tcell2.KeyEscape, 0, tcell2.ModNone) }, s.Fini)
		go func() {
			defer close(ran)
			_ = d.ScreenV2(s, animCols, animRows) //nolint:errcheck // it ends with its screen
		}()
	case d.Text != nil:
		t := newVterm(animCols, animRows, true)
		a.term = t
		a.stop = t.end
		go func() {
			defer close(done)
			_ = d.Text(t, animCols, animRows) //nolint:errcheck // what it wrote is what is shown
		}()
	}
}

// close stops the demo running, if one is.
func (a *animView) close() {
	if a.stop != nil {
		a.stop()
	}
	a.term, a.sim, a.stop, a.done = nil, nil, nil, nil
}

// simPicture is a tcell v2 simulated screen as a picture, as vterm draws one.
func simPicture(s tcell2.SimulationScreen) *image.RGBA {
	cells, w, h := simCells(s)
	m := cellPicture(w, h)
	for i, c := range cells {
		paintCell(m, i%w, i/w, c.Ch, c.Fg, c.Bg)
	}
	return m
}
