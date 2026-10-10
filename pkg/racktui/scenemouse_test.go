package racktui

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/rackpic"
)

// sceneRack is a rack with a scene, which remembers what was done to it.
type sceneRack struct {
	fakeRack
	acts []string
}

func (s *sceneRack) SceneAct(w, h int, shape, x, y float64, kind string, delta float64) error {
	s.acts = append(s.acts, fmt.Sprintf("%s %g,%g %g", kind, x, y, delta))
	return nil
}

// scenePanel is a page-look panel on a 40 x 11 terminal, its window on the
// right half.
func scenePanel(s *sceneRack) *panel {
	p := &panel{src: s, pic: &rackpic.Picture{W: 100, H: 100}}
	p.scrW, p.scrH = 40, 11
	p.layoutWindow(p.scrW, p.scrH)
	return p
}

func TestADragOnTheSceneTurnsTheModel(t *testing.T) {
	s := &sceneRack{}
	p := scenePanel(s)
	m := func(x, y int, b tcell.ButtonMask) { p.mouse(tcell.NewEventMouse(x, y, b, 0), p.scrW, p.scrH) }
	m(3, 2, tcell.Button1)
	m(5, 2, tcell.Button1)
	m(30, 4, tcell.Button1) // over the window: still the drag's
	m(30, 4, 0)
	want := []string{"down 3.5,5 0", "move 5.5,5 0", "move 30.5,9 0", "up 30.5,9 0"}
	if fmt.Sprint(s.acts) != fmt.Sprint(want) {
		t.Fatalf("the drag did %q, want %q", s.acts, want)
	}
	if p.turning {
		t.Error("the drag is over and the scene still holds the mouse")
	}
}

func TestTheWheelOnTheSceneZooms(t *testing.T) {
	s := &sceneRack{}
	p := scenePanel(s)
	p.mouse(tcell.NewEventMouse(1, 1, tcell.WheelUp, 0), p.scrW, p.scrH)
	p.mouse(tcell.NewEventMouse(1, 1, tcell.WheelDown, 0), p.scrW, p.scrH)
	want := []string{"wheel 1.5,3 -100", "wheel 1.5,3 100"}
	if fmt.Sprint(s.acts) != fmt.Sprint(want) {
		t.Fatalf("the wheel did %q, want %q", s.acts, want)
	}
}

// The window, its title and the status line are not the scene; with the
// window over the whole terminal there is no scene to take the mouse.
func TestTheSceneIsOnlyWhereItShows(t *testing.T) {
	s := &sceneRack{}
	p := scenePanel(s)
	for _, c := range [][2]int{{p.winX, 0}, {p.winX + 2, 5}, {3, p.scrH - 1}} {
		p.mouse(tcell.NewEventMouse(c[0], c[1], tcell.Button1, 0), p.scrW, p.scrH)
		p.mouse(tcell.NewEventMouse(c[0], c[1], 0, 0), p.scrW, p.scrH)
	}
	p.winMode = winFull
	p.layoutWindow(p.scrW, p.scrH)
	p.mouse(tcell.NewEventMouse(3, 2, tcell.WheelUp, 0), p.scrW, p.scrH)
	if len(s.acts) != 0 {
		t.Fatalf("off the scene the scene was sent %q", s.acts)
	}
	p.winMode = winHidden
	p.layoutWindow(p.scrW, p.scrH)
	p.mouse(tcell.NewEventMouse(35, 2, tcell.WheelUp, 0), p.scrW, p.scrH)
	if len(s.acts) != 1 {
		t.Fatalf("with the window hidden the scene is everywhere, but got %q", s.acts)
	}
}

// Moves that pile up behind a slow page are merged into the last; nothing
// else is, and the order is kept.
func TestTheSceneQueueMergesMoves(t *testing.T) {
	var q sceneQueue
	var mu sync.Mutex
	var got []string
	gate := make(chan struct{})
	first := true
	do := func(a sceneAct) {
		if first {
			first = false
			<-gate // the page is slow with the first
		}
		mu.Lock()
		got = append(got, fmt.Sprintf("%s %g", a.kind, a.x))
		mu.Unlock()
	}
	q.add(sceneAct{kind: "down", x: 0}, do)
	for i := 1; i <= 5; i++ {
		q.add(sceneAct{kind: "move", x: float64(i)}, do)
	}
	q.add(sceneAct{kind: "up", x: 6}, do)
	close(gate)
	want := "[down 0 move 5 up 6]"
	for range 200 {
		mu.Lock()
		s := fmt.Sprint(got)
		mu.Unlock()
		if s == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the queue sent %v, want %s", got, want)
}
