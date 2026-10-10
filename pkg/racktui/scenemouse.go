package racktui

import (
	"sync"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/panelart"
)

// The scene takes the mouse as the page's background does: a drag turns the
// model and the wheel zooms it, by the page's own handlers, at the point of
// the model the cell shows.

// SceneActor is a Pictured that can also act on the scene.
type SceneActor interface {
	// SceneAct does kind ("down", "move", "up" or "wheel") at x, y of the
	// scene as Scene(w, h, shape) showed it; delta is the wheel's.
	SceneAct(w, h int, shape, x, y float64, kind string, delta float64) error
}

// sceneWheel is one notch of the wheel, in the page's pixels: about what a
// mouse sends.
const sceneWheel = 100

// sceneSize is the scene as refreshPixels samples it: w x h pixels, each
// shape times as tall as wide, half a cell a pixel.
func (p *panel) sceneSize() (w, h int, shape float64) {
	return p.scrW, 2 * maxi(p.scrH-1, 1), panelart.CellAspect() / 2
}

// onScene reports whether the cell at x, y shows the scene: not the window,
// its title or its bars, and not the status line.
func (p *panel) onScene(x, y int) bool {
	if p.winMode == winFull || y >= maxi(p.scrH-1, 1) || x < 0 || y < 0 || x >= p.scrW {
		return false
	}
	return p.winMode == winHidden || x < p.winX
}

// sceneAct is one thing done to the scene.
type sceneAct struct {
	kind  string
	x, y  float64
	delta float64
}

// mouseScene takes the mouse over the scene and reports whether it did. A
// drag that began on the scene keeps the mouse, wherever it goes, until the
// button is let go, as the page's does.
func (p *panel) mouseScene(b tcell.ButtonMask, pressed bool, x, y int) bool {
	if _, ok := p.src.(SceneActor); !ok {
		return false
	}
	// The middle of the cell: its upper pixel's bottom edge.
	a := sceneAct{x: float64(x) + 0.5, y: 2*float64(y) + 1}
	switch {
	case p.turning:
		a.kind = "move"
		if b&tcell.Button1 == 0 {
			a.kind, p.turning = "up", false
		}
	case !p.onScene(x, y):
		return false
	case b&tcell.WheelUp != 0:
		a.kind, a.delta = "wheel", -sceneWheel
	case b&tcell.WheelDown != 0:
		a.kind, a.delta = "wheel", sceneWheel
	case pressed:
		a.kind, p.turning = "down", true
	default:
		return true
	}
	p.sendScene(a)
	return true
}

// sendScene does a to the page: at once when the panel runs on no screen (a
// test), and otherwise in the order it was done but off the event loop,
// with the moves that pile up behind a slow page merged into the last.
func (p *panel) sendScene(a sceneAct) {
	sa := p.src.(SceneActor) //nolint:forcetypeassert // mouseScene checked
	w, h, shape := p.sceneSize()
	do := func(a sceneAct) error { return sa.SceneAct(w, h, shape, a.x, a.y, a.kind, a.delta) }
	if p.q == nil {
		if err := do(a); err != nil {
			p.err = err.Error()
		}
		return
	}
	p.sceneQ.add(a, func(a sceneAct) {
		if err := do(a); err != nil {
			p.q.post(tcell.NewEventInterrupt(sceneErr(err.Error())))
		}
	})
}

// sceneErr is the Data of the interrupt saying a scene act failed.
type sceneErr string

// sceneQueue sends acts one at a time, in order, merging moves.
type sceneQueue struct {
	mu      sync.Mutex
	pending []sceneAct
	busy    bool
}

func (s *sceneQueue) add(a sceneAct, do func(sceneAct)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.pending); n > 0 && a.kind == "move" && s.pending[n-1].kind == "move" {
		s.pending[n-1] = a
	} else {
		s.pending = append(s.pending, a)
	}
	if s.busy {
		return
	}
	s.busy = true
	go func() {
		for {
			s.mu.Lock()
			if len(s.pending) == 0 {
				s.busy = false
				s.mu.Unlock()
				return
			}
			a := s.pending[0]
			s.pending = s.pending[1:]
			s.mu.Unlock()
			do(a)
		}
	}()
}
