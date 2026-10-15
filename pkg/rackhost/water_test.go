package rackhost

import (
	"bytes"
	"testing"
)

// On water, still water shows the scene as it is; a hand on it makes ripples
// that bend the picture, as the page's lens does.
func TestWaterBendsTheSceneWhereItIsDisturbed(t *testing.T) {
	r := newRack(t)
	set(t, r, "auto-rotate", "0", "pause-sw", "1") // a picture that holds still
	shot := func() []byte {
		m, err := r.Scene(160, 100, 1)
		if err != nil {
			t.Fatal(err)
		}
		return m.Px
	}
	shot()
	plain := shot()
	set(t, r, "bg-visual", "water")
	if still := shot(); !bytes.Equal(still, plain) {
		t.Error("still water changed the picture")
	}
	if err := r.SceneAct(160, 100, 1, 80, 50, "down", 0); err != nil {
		t.Fatal(err)
	}
	for x := 80.0; x < 120; x += 4 {
		if err := r.SceneAct(160, 100, 1, x, 50, "move", 0); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.SceneAct(160, 100, 1, 120, 50, "up", 0) //nolint:errcheck // a release
	differ := 0
	for range 5 {
		got := shot()
		for i := range got {
			if got[i] != plain[i] {
				differ++
			}
		}
	}
	if differ == 0 {
		t.Error("a drag across the water left the picture as it was")
	}
}
