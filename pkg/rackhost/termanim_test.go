package rackhost

import (
	"testing"
	"time"

	tcell2 "github.com/gdamore/tcell/v2"

	"github.com/0magnet/tuiwasm/demos"
)

// animLit is how many pixels of the running demo's screen are not black.
func animLit(r *Rack) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.animFigure()
	if q == nil {
		return 0
	}
	n := 0
	for i := 0; i+3 < len(q.Pic.Pix); i += 4 {
		if q.Pic.Pix[i]|q.Pic.Pix[i+1]|q.Pic.Pix[i+2] != 0 {
			n++
		}
	}
	return n
}

// Every demo the page offers runs natively and puts something on its screen,
// and ends when another is picked or the model changes.
func TestEveryTerminalAnimationDraws(t *testing.T) {
	r, c := idleRack(t, "termanim")
	var ran []chan struct{}
	for _, d := range demos.All() {
		if d.Name == v2Demo {
			continue // not one the page offers
		}
		set(t, r, "termanim-pick", d.Name)
		deadline := time.Now().Add(5 * time.Second)
		for animLit(r) == 0 && time.Now().Before(deadline) {
			watch(t, r, c, 0.05)
			time.Sleep(20 * time.Millisecond) // the demo draws on its own goroutine
		}
		if animLit(r) == 0 {
			t.Errorf("%s drew nothing", d.Name)
		}
		r.mu.Lock()
		ran = append(ran, r.anim.done)
		r.mu.Unlock()
	}
	set(t, r, r.selector, "lorenz")
	watch(t, r, c, 0.05)
	r.mu.Lock()
	still := r.anim.done != nil
	r.mu.Unlock()
	if still {
		t.Error("the demo kept running after the model changed")
	}
	for i, done := range ran {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("demo %d did not end", i)
		}
	}
}

// v2Demo is a demo on tcell v2, which none of tuiwasm's are yet: a lit screen
// until Escape.
const v2Demo = "zz-v2-test"

func init() {
	demos.Register(demos.Demo{Name: v2Demo, ScreenV2: func(s tcell2.Screen, cols, rows int) error {
		st := tcell2.StyleDefault.Foreground(tcell2.ColorGreen)
		for y := range rows {
			for x := range cols {
				s.SetContent(x, y, '█', nil, st)
			}
		}
		s.Show()
		for {
			if ev, ok := s.PollEvent().(*tcell2.EventKey); ok && ev.Key() == tcell2.KeyEscape {
				return nil
			}
		}
	}})
}

// A demo on tcell v2 draws through the simulated screen, and ends when asked.
func TestATcellV2DemoDraws(t *testing.T) {
	var a animView
	a.start(v2Demo)
	deadline := time.Now().Add(5 * time.Second)
	for a.sim != nil && time.Now().Before(deadline) {
		if p := simPicture(a.sim); p.Pix[1] != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if a.sim == nil || simPicture(a.sim).Pix[1] == 0 {
		t.Fatal("the v2 demo drew nothing")
	}
	done := a.done
	a.close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the v2 demo did not end on Escape")
	}
}
