package racktui

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/rackpic"
	"github.com/gdamore/tcell/v3"
)

// pictured is a fake rack with a picture, which remembers what the pointer
// did to it.
type pictured struct {
	fakeRack
	acts []string
}

func (f *pictured) Picture() (*rackpic.Picture, error) { return &rackpic.Picture{W: 100, H: 100}, nil }

func (f *pictured) PictureChanges(int) (*rackpic.Patch, error) { return &rackpic.Patch{}, nil }

func (f *pictured) Act(_ int, _, _ float64, kind string, _ float64) error {
	f.acts = append(f.acts, kind)
	return nil
}

func (f *pictured) Scene(int, int, float64) (*rackpic.Image, error) { return nil, nil }

func (f *pictured) Canvases(int, []rackpic.CanvasWant) (map[int]*rackpic.Image, error) {
	return nil, nil
}

// A key is held from the press to the release: the page look tells the rack
// the button going down, each move while it is held, and its coming up,
// even when it comes up outside the window.
func TestThePageLookFollowsAHeldButton(t *testing.T) {
	f := &pictured{}
	p := &panel{src: f, paged: true}
	p.pic = &rackpic.Picture{W: 100, H: 100}
	p.mousePage(tcell.Button1, true, 2, 2)
	p.mousePage(tcell.Button1, false, 3, 2)
	p.mousePage(tcell.ButtonNone, false, 3, 2) // a move with nothing held
	p.mouseWindow(tcell.ButtonNone, false, 999, 999)
	p.mouseWindow(tcell.ButtonNone, false, 999, 999)
	want := []string{"click", "drag", "release"}
	if len(f.acts) != len(want) {
		t.Fatalf("the rack was told %v, want %v", f.acts, want)
	}
	for i := range want {
		if f.acts[i] != want[i] {
			t.Fatalf("the rack was told %v, want %v", f.acts, want)
		}
	}
}
